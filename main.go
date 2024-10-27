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
	"strings"
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
	Duration     time.Duration
	ExitCode     int
	MaxAttempts  int
	Number       int
	TotalTime    time.Duration
}

type interval struct {
	Start time.Duration
	End   time.Duration
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
	Backoff     time.Duration
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
	Backoff     time.Duration    `default:"0" short:"b" help:"base for exponential backoff (duration)"`
	Condition   string           `default:"code == 0" short:"c" help:"success condition (Starlark expression)"`
	Delay       time.Duration    `default:"0" short:"d" help:"constant delay (duration)"`
	Forever     bool             `short:"f" help:"infinite attempts"`
	Jitter      string           `default:"0,0" short:"j" help:"additional random delay (maximum duration or 'min,max' duration)"`
	MaxDelay    time.Duration    `default:"1h" short:"m" help:"maximum allowed sum of constant delay and exponential backoff (duration)"`
	MaxAttempts int              `default:"5" short:"n" name:"attempts" aliases:"tries" help:"maximum number of attempts (negative for infinite)"`
	Timeout     time.Duration    `short:"t" default:"-1s" help:"timeout for each attempt (duration; negative for no timeout)"`
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
	var start, end time.Duration
	var err error

	parts := strings.Split(s, ",")
	if len(parts) == 2 {
		start, err = time.ParseDuration(parts[0])
		if err != nil {
			return interval{}, fmt.Errorf("invalid start duration: %s", parts[0])
		}
		end, err = time.ParseDuration(parts[1])
		if err != nil {
			return interval{}, fmt.Errorf("invalid end duration: %s", parts[1])
		}
	} else if len(parts) == 1 {
		end, err = time.ParseDuration(parts[0])
		if err != nil {
			return interval{}, fmt.Errorf("invalid end duration: %s", parts[0])
		}
		start = 0
	} else {
		return interval{}, fmt.Errorf("invalid interval format: %s", s)
	}

	if start < 0 || end < 0 || start > end {
		return interval{}, fmt.Errorf("invalid interval values: start=%s, end=%s", start.String(), end.String())
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
	if timeout >= 0 {
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

	currFixed := config.FixedDelay.Start.Seconds() + math.Pow(config.Backoff.Seconds(), float64(attemptNum-1))
	if currFixed > config.FixedDelay.End.Seconds() {
		currFixed = config.FixedDelay.End.Seconds()
	}

	currRandom := config.RandomDelay.Start.Seconds() +
		rand.Float64()*(config.RandomDelay.End-config.RandomDelay.Start).Seconds()

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

	if config.Verbose >= 1 && strings.HasPrefix(config.Command, "-") {
		logger.Printf("warning: command starts with '-': %s", config.Command)
	}

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
				logger.Printf("command timed out after %s on attempt %d", formatDuration(attemptDuration), attemptNum)
			case statusNotFound:
				logger.Printf("command was not found on attempt %d", attemptNum)
			case statusUnknownError:
				logger.Printf("unknown error occurred on attempt %d", attemptNum)
			}
		}

		attemptInfo := attempt{
			CommandFound: cmdResult.Status != statusNotFound,
			Duration:     attemptDuration,
			ExitCode:     cmdResult.ExitCode,
			MaxAttempts:  config.MaxAttempts,
			Number:       attemptNum,
			TotalTime:    totalTime,
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
		Timeout:     cliConfig.Timeout,
	}

	exitCode, err := retry(config)
	if err != nil {
		log.Printf("%v", err)
	}

	os.Exit(exitCode)
}
