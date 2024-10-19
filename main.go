// Copyright (c) 2023-2024 D. Bohdan
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"time"

	"github.com/alecthomas/kong"
	"go.starlark.net/starlark"
)

const (
	CommandNotFoundExitCode = 255
	MaxAllowedDelay         = 366 * 24 * 60 * 60
	MaxVerboseLevel         = 2
	Version                 = "0.6.0"
)

type Attempt struct {
	CommandFound bool
	Duration     float64
	ExitCode     int
	MaxTries     int
	Number       int
	TotalTime    float64
}

type Interval struct {
	Start float64
	End   float64
}

type Result struct {
	CommandFound bool
	ExitCode     int
}

type RetryConfig struct {
	Command     string
	Args        []string
	Backoff     float64
	FixedDelay  Interval
	MaxTries    int
	RandomDelay Interval
	Condition   string
	Verbose     int
	Timeout     time.Duration
}

type CLI struct {
	Command   string           `arg:"" passthrough:"" help:"command to run"`
	Args      []string         `arg:"" optional:"" name:"args" help:"arguments"`
	Version   kong.VersionFlag `short:"V" help:"print version number and exit"`
	Backoff   float64          `default:"0" short:"b" help:"base for exponential backoff (0 for no exponential backoff)"`
	Condition string           `default:"code == 0" short:"c" help:"success condition (Starlark expression)"`
	Delay     float64          `default:"0" short:"d" help:"constant delay (seconds)"`
	Jitter    string           `default:"0,0" short:"j" help:"additional random delay (maximum seconds or 'min,max' seconds)"`
	MaxDelay  float64          `default:"3600" short:"m" help:"maximum total delay (seconds)"`
	Timeout   time.Duration    `short:"w" default:"0" help:"timeout for each attempt (seconds, 0 for no timeout)"`
	Tries     int              `default:"5" short:"t" help:"maximum number of attempts (negative for infinite)"`
	Verbose   int              `short:"v" type:"counter" help:"increase verbosity"`
}

type elapsedTimeWriter struct {
	startTime time.Time
}

func (w *elapsedTimeWriter) Write(bytes []byte) (int, error) {
	elapsed := time.Since(w.startTime)

	hours := int(elapsed.Hours())
	minutes := int(elapsed.Minutes()) % 60
	seconds := int(elapsed.Seconds()) % 60
	deciseconds := elapsed.Milliseconds() % 1000 / 100

	return fmt.Fprintf(os.Stderr, "recur [%02d:%02d:%02d.%01d]: %s", hours, minutes, seconds, deciseconds, string(bytes))
}

type exitRequestError struct {
	Code int
}

func (e *exitRequestError) Error() string {
	return fmt.Sprintf("exit requested with code %d", e.Code)
}

func StarlarkExit(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var code starlark.Value
	if err := starlark.UnpackPositionalArgs(b.Name(), args, kwargs, 1, &code); err != nil {
		return nil, err
	}

	if _, ok := code.(starlark.NoneType); ok {
		return starlark.None, &exitRequestError{Code: int(CommandNotFoundExitCode)}
	}

	if codeInt, ok := code.(starlark.Int); ok {
		exitCode, ok := codeInt.Int64()
		if !ok {
			return nil, fmt.Errorf("exit code too large")
		}

		return starlark.None, &exitRequestError{Code: int(exitCode)}
	}

	return nil, fmt.Errorf("exit code wasn't 'int' or 'None'")
}

func parseInterval(s string) (Interval, error) {
	var start, end float64

	if _, err := fmt.Sscanf(s, "%f,%f", &start, &end); err != nil {
		if _, err := fmt.Sscanf(s, "%f", &end); err != nil {
			return Interval{}, fmt.Errorf("invalid interval format: %s", s)
		}
		start = 0
	}

	if start < 0 || end < 0 || start > end {
		return Interval{}, fmt.Errorf("invalid interval values: start=%f, end=%f", start, end)
	}

	return Interval{Start: start, End: end}, nil
}

func evaluateCondition(attempt Attempt, expr string) (bool, error) {
	thread := &starlark.Thread{Name: "condition"}

	var code starlark.Value
	if attempt.CommandFound {
		code = starlark.MakeInt(attempt.ExitCode)
	} else {
		code = starlark.None
	}

	globals := starlark.StringDict{
		"exit": starlark.NewBuiltin("exit", StarlarkExit),

		"attempt":       starlark.MakeInt(attempt.Number),
		"code":          code,
		"command_found": starlark.Bool(attempt.CommandFound),
		"time":          starlark.Float(attempt.Duration),
		"total_time":    starlark.Float(attempt.TotalTime),
		"max_tries":     starlark.MakeInt(attempt.MaxTries),
	}

	val, err := starlark.Eval(thread, "", expr, globals)
	if err != nil {
		var exitErr *exitRequestError
		if errors.As(err, &exitErr) {
			return false, exitErr
		}

		return false, err
	}

	if val.Type() != "bool" {
		return false, fmt.Errorf("condition must return a boolean, got %s", val.Type())
	}

	return bool(val.Truth()), nil
}

func executeCommand(ctx context.Context, command string, args []string) Result {
	if _, err := exec.LookPath(command); err != nil {
		return Result{
			CommandFound: false,
			ExitCode:     CommandNotFoundExitCode,
		}
	}

	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	err := cmd.Run()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return Result{
				CommandFound: true,
				ExitCode:     exitErr.ExitCode(),
			}
		}

		return Result{
			CommandFound: false,
			ExitCode:     CommandNotFoundExitCode,
		}
	}

	return Result{
		CommandFound: true,
		ExitCode:     cmd.ProcessState.ExitCode(),
	}
}

func delayBeforeAttempt(attempt int, config RetryConfig) time.Duration {
	if attempt == 1 {
		return 0
	}

	currFixed := config.FixedDelay.Start + math.Pow(float64(config.Backoff), float64(attempt-1))
	if currFixed > config.FixedDelay.End {
		currFixed = config.FixedDelay.End
	}

	currRandom := config.RandomDelay.Start +
		rand.Float64()*(config.RandomDelay.End-config.RandomDelay.Start)

	return time.Duration((currFixed + currRandom) * float64(time.Second))
}

func retry(ctx context.Context, config RetryConfig) (int, error) {
	customWriter := &elapsedTimeWriter{
		startTime: time.Now(),
	}
	logger := log.New(customWriter, "", 0)

	log.SetOutput(customWriter)
	log.SetFlags(0)

	var result Result
	var startTime time.Time
	var totalTime time.Duration

	for attempt := 1; config.MaxTries < 0 || attempt <= config.MaxTries; attempt++ {
		delay := delayBeforeAttempt(attempt, config)
		if delay > 0 {
			if config.Verbose >= 1 {
				logger.Printf("waiting %v before attempt %d", delay, attempt)
			}
			time.Sleep(delay)
		}

		attemptStart := time.Now()
		if startTime.IsZero() {
			startTime = attemptStart
		}

		result = executeCommand(ctx, config.Command, config.Args)

		attemptEnd := time.Now()
		attemptDuration := attemptEnd.Sub(attemptStart)
		totalTime = attemptEnd.Sub(startTime)

		if config.Verbose >= 1 {
			if !result.CommandFound {
				logger.Printf("command was not found on attempt %d", attempt)
			} else {
				logger.Printf("command exited with code %d on attempt %d", result.ExitCode, attempt)
			}
		}

		attemptInfo := Attempt{
			CommandFound: result.CommandFound,
			Duration:     attemptDuration.Seconds(),
			ExitCode:     result.ExitCode,
			MaxTries:     config.MaxTries,
			Number:       attempt,
			TotalTime:    totalTime.Seconds(),
		}

		success, err := evaluateCondition(attemptInfo, config.Condition)
		if err != nil {
			var exitErr *exitRequestError
			if errors.As(err, &exitErr) {
				return exitErr.Code, nil
			}

			return 1, fmt.Errorf("condition evaluation failed: %w", err)
		}

		if success {
			return result.ExitCode, nil
		}

		if config.Verbose >= 2 {
			logger.Printf("condition not met; continuing to next attempt")
		}
	}

	return result.ExitCode, fmt.Errorf("maximum attempts reached (%d)", config.MaxTries)
}

func main() {
	var cli CLI
	kongCtx := kong.Parse(&cli,
		kong.Name("recur"),
		kong.Description("Retry a command with exponential backoff and jitter."),
		kong.UsageOnError(),
		kong.Vars{"version": Version},
	)

	if cli.Verbose > MaxVerboseLevel {
		kongCtx.Fatalf("up to %d verbose flags is allowed", MaxVerboseLevel)
	}

	jitter, err := parseInterval(cli.Jitter)
	if err != nil {
		kongCtx.Fatalf("invalid jitter: %v", err)
	}

	config := RetryConfig{
		Command:     cli.Command,
		Args:        cli.Args,
		Backoff:     cli.Backoff,
		FixedDelay:  Interval{Start: cli.Delay, End: cli.MaxDelay},
		MaxTries:    cli.Tries,
		RandomDelay: jitter,
		Condition:   cli.Condition,
		Verbose:     cli.Verbose,
		Timeout:     cli.Timeout,
	}

	ctx := context.Background()
	if config.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, config.Timeout)
		defer cancel()
	}

	exitCode, err := retry(ctx, config)
	if err != nil {
		log.Printf("%v", err)
	}

	os.Exit(exitCode)
}

// vim: set tabstop=4:
