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
	exitCodeCommandNotFound = 255
	exitCodeError           = -1
	maxAllowedDelay         = 366 * 24 * 60 * 60
	maxVerboseLevel         = 2
	version                 = "0.7.0"
)

type attempt struct {
	CommandFound bool
	Duration     float64
	ExitCode     int
	MaxAttempts  int
	Number       int
	TotalTime    float64
}

type interval struct {
	Start float64
	End   float64
}

type commandStatus int

const (
	statusFinished commandStatus = iota
	statusTimeout
	statusNotFound
	statusUnknownError
)

type commandResult struct {
	Status   commandStatus
	ExitCode int
}

type retryConfig struct {
	Command     string
	Args        []string
	Backoff     float64
	FixedDelay  interval
	MaxAttempts int
	RandomDelay interval
	Condition   string
	Verbose     int
	Timeout     time.Duration
}

type cli struct {
	Command     string           `arg:"" passthrough:"" help:"command to run"`
	Args        []string         `arg:"" optional:"" name:"args" help:"arguments"`
	Version     kong.VersionFlag `short:"V" help:"print version number and exit"`
	Backoff     float64          `default:"0" short:"b" help:"base for exponential backoff (0 for no exponential backoff)"`
	Condition   string           `default:"code == 0" short:"c" help:"success condition (Starlark expression)"`
	Delay       float64          `default:"0" short:"d" help:"constant delay (seconds)"`
	Forever     bool             `short:"f" help:"infinite attempts"`
	Jitter      string           `default:"0,0" short:"j" help:"additional random delay (maximum seconds or 'min,max' seconds)"`
	MaxDelay    float64          `default:"3600" short:"m" help:"maximum total delay (seconds)"`
	MaxAttempts int              `default:"5" short:"n" name:"attempts" aliases:"tries" help:"maximum number of attempts (negative for infinite)"`
	Timeout     float64          `short:"t" default:"0" help:"timeout for each attempt (seconds, 0 for no timeout)"`
	Verbose     int              `short:"v" type:"counter" help:"increase verbosity"`
}

type elapsedTimeWriter struct {
	startTime time.Time
}

type exitRequestError struct {
	Code int
}

func (w *elapsedTimeWriter) Write(bytes []byte) (int, error) {
	elapsed := time.Since(w.startTime)

	hours := int(elapsed.Hours())
	minutes := int(elapsed.Minutes()) % 60
	seconds := int(elapsed.Seconds()) % 60
	deciseconds := elapsed.Milliseconds() % 1000 / 100

	return fmt.Fprintf(os.Stderr, "recur [%02d:%02d:%02d.%01d]: %s", hours, minutes, seconds, deciseconds, string(bytes))
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
		return starlark.None, &exitRequestError{Code: int(exitCodeCommandNotFound)}
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

func parseInterval(s string) (interval, error) {
	var start, end float64

	if _, err := fmt.Sscanf(s, "%f,%f", &start, &end); err != nil {
		if _, err := fmt.Sscanf(s, "%f", &end); err != nil {
			return interval{}, fmt.Errorf("invalid interval format: %s", s)
		}
		start = 0
	}

	if start < 0 || end < 0 || start > end {
		return interval{}, fmt.Errorf("invalid interval values: start=%f, end=%f", start, end)
	}

	return interval{Start: start, End: end}, nil
}

func evaluateCondition(attemptInfo attempt, expr string) (bool, error) {
	thread := &starlark.Thread{Name: "condition"}

	var code starlark.Value
	if attemptInfo.CommandFound {
		code = starlark.MakeInt(attemptInfo.ExitCode)
	} else {
		code = starlark.None
	}

	globals := starlark.StringDict{
		"exit": starlark.NewBuiltin("exit", StarlarkExit),

		"attempt":       starlark.MakeInt(attemptInfo.Number),
		"code":          code,
		"command_found": starlark.Bool(attemptInfo.CommandFound),
		"max_attempts":  starlark.MakeInt(attemptInfo.MaxAttempts),
		"time":          starlark.Float(attemptInfo.Duration),
		"total_time":    starlark.Float(attemptInfo.TotalTime),
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

func executeCommand(command string, args []string, timeout time.Duration) commandResult {
	if _, err := exec.LookPath(command); err != nil {
		return commandResult{
			Status:   statusNotFound,
			ExitCode: exitCodeCommandNotFound,
		}
	}

	ctx := context.Background()
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	err := cmd.Run()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return commandResult{
				Status:   statusTimeout,
				ExitCode: exitCodeError,
			}
		}

		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return commandResult{
				Status:   statusFinished,
				ExitCode: exitErr.ExitCode(),
			}
		}

		return commandResult{
			Status:   statusUnknownError,
			ExitCode: exitCodeError,
		}
	}

	return commandResult{
		Status:   statusFinished,
		ExitCode: cmd.ProcessState.ExitCode(),
	}
}

func delayBeforeAttempt(attemptNum int, config retryConfig) time.Duration {
	if attemptNum == 1 {
		return 0
	}

	currFixed := config.FixedDelay.Start + math.Pow(float64(config.Backoff), float64(attemptNum-1))
	if currFixed > config.FixedDelay.End {
		currFixed = config.FixedDelay.End
	}

	currRandom := config.RandomDelay.Start +
		rand.Float64()*(config.RandomDelay.End-config.RandomDelay.Start)

	return time.Duration((currFixed + currRandom) * float64(time.Second))
}

func formatDuration(d time.Duration) string {
	return d.Round(time.Millisecond).String()
}

func retry(config retryConfig) (int, error) {
	customWriter := &elapsedTimeWriter{
		startTime: time.Now(),
	}
	logger := log.New(customWriter, "", 0)

	log.SetOutput(customWriter)
	log.SetFlags(0)

	var cmdResult commandResult
	var startTime time.Time
	var totalTime time.Duration

	for attemptNum := 1; config.MaxAttempts < 0 || attemptNum <= config.MaxAttempts; attemptNum++ {
		delay := delayBeforeAttempt(attemptNum, config)
		if delay > 0 {
			if config.Verbose >= 1 {
				logger.Printf("waiting %s before attempt %d", formatDuration(delay), attemptNum)
			}
			time.Sleep(delay)
		}

		attemptStart := time.Now()
		if startTime.IsZero() {
			startTime = attemptStart
		}

		cmdResult = executeCommand(config.Command, config.Args, config.Timeout)

		attemptEnd := time.Now()
		attemptDuration := attemptEnd.Sub(attemptStart)
		totalTime = attemptEnd.Sub(startTime)

		if config.Verbose >= 1 {
			switch cmdResult.Status {
			case statusFinished:
				logger.Printf("command exited with code %d on attempt %d", cmdResult.ExitCode, attemptNum)
			case statusTimeout:
				logger.Printf("command timed out on attempt %d", attemptNum)
			case statusNotFound:
				logger.Printf("command was not found on attempt %d", attemptNum)
			case statusUnknownError:
				logger.Printf("unknown error occurred on attempt %d", attemptNum)
			}
		}

		attemptInfo := attempt{
			CommandFound: cmdResult.Status != statusNotFound,
			Duration:     attemptDuration.Seconds(),
			ExitCode:     cmdResult.ExitCode,
			MaxAttempts:  config.MaxAttempts,
			Number:       attemptNum,
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
			return cmdResult.ExitCode, nil
		}

		if config.Verbose >= 2 {
			logger.Printf("condition not met; continuing to next attempt")
		}
	}

	return cmdResult.ExitCode, fmt.Errorf("maximum attempts reached (%d)", config.MaxAttempts)
}

func main() {
	var cliConfig cli
	kongCtx := kong.Parse(&cliConfig,
		kong.Name("recur"),
		kong.Description("Retry a command with exponential backoff and jitter."),
		kong.UsageOnError(),
		kong.Vars{"version": version},
	)

	if cliConfig.Forever {
		cliConfig.MaxAttempts = -1
	}

	if cliConfig.Verbose > maxVerboseLevel {
		kongCtx.Fatalf("up to %d verbose flags is allowed", maxVerboseLevel)
	}

	jitterInterval, err := parseInterval(cliConfig.Jitter)
	if err != nil {
		kongCtx.Fatalf("invalid jitter: %v", err)
	}

	config := retryConfig{
		Command:     cliConfig.Command,
		Args:        cliConfig.Args,
		Backoff:     cliConfig.Backoff,
		FixedDelay:  interval{Start: cliConfig.Delay, End: cliConfig.MaxDelay},
		MaxAttempts: cliConfig.MaxAttempts,
		RandomDelay: jitterInterval,
		Condition:   cliConfig.Condition,
		Verbose:     cliConfig.Verbose,
		Timeout:     time.Duration(cliConfig.Timeout * float64(time.Second)),
	}

	exitCode, err := retry(config)
	if err != nil {
		log.Printf("%v", err)
	}

	os.Exit(exitCode)
}
