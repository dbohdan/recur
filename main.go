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
	"io"
	"log"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/alecthomas/repr"
	"go.starlark.net/starlark"
)

const (
	exitCodeCommandNotFound = 255
	exitCodeError           = -1
	maxAllowedDelay         = 366 * 24 * 60 * 60
	maxVerboseLevel         = 3
	version                 = "0.8.0"
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
	Condition   string
	FixedDelay  interval
	MaxAttempts int
	RandomDelay interval
	Timeout     time.Duration
	Verbose     int
}

const (
	backoffDefault     = time.Duration(0)
	conditionDefault   = "code == 0"
	delayDefault       = time.Duration(0)
	jitterDefault      = "0,0"
	maxDelayDefault    = time.Duration(time.Hour)
	maxAttemptsDefault = 5
	timeoutDefault     = time.Duration(-time.Second)
)

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
	d = d.Round(time.Millisecond)
	if d > time.Second {
		d = d.Round(100 * time.Millisecond)
	}

	zeroUnits := regexp.MustCompile("(^|[^0-9])(?:0h)?(?:0m)?(?:0s)?$")
	s := zeroUnits.ReplaceAllString(d.String(), "$1")

	if s == "" {
		return "0"
	}
	return s
}

func retry(config retryConfig) (int, error) {
	var cmdResult commandResult
	var startTime time.Time
	var totalTime time.Duration

	for attemptNum := 1; config.MaxAttempts < 0 || attemptNum <= config.MaxAttempts; attemptNum++ {
		delay := delayBeforeAttempt(attemptNum, config)
		if delay > 0 {
			if config.Verbose >= 1 {
				log.Printf("waiting %s before attempt %d", formatDuration(delay), attemptNum)
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
				log.Printf("command exited with code %d on attempt %d", cmdResult.ExitCode, attemptNum)
			case statusTimeout:
				log.Printf("command timed out after %s on attempt %d", formatDuration(attemptDuration), attemptNum)
			case statusNotFound:
				log.Printf("command was not found on attempt %d", attemptNum)
			case statusUnknownError:
				log.Printf("unknown error occurred on attempt %d", attemptNum)
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
			log.Printf("condition not met; continuing to next attempt")
		}
	}

	return cmdResult.ExitCode, fmt.Errorf("maximum %d attempts reached", config.MaxAttempts)
}

func usage(w io.Writer) {
	fmt.Fprintf(
		w,
		`Usage: %s [-b <backoff>] [-c <condition>] [-d <delay>] [-f] [-j <jitter>] [-m <max-delay>] [-n <attempt>] [-t <timeout>] [-v] <command> [<arg> ...]
`,
		filepath.Base(os.Args[0]),
	)
}

func help() {
	usage(os.Stdout)

	fmt.Printf(
		`
Retry a command with exponential backoff and jitter.

Arguments:
  <command>
  Command to run.

  [<arg> ...]
  Arguments to the command.

Flags:
  -h, --help
  Print this help message and exit.

  -V, --version
  Print version number and exit.

  -b, --backoff %v
  Base for exponential backoff (duration).

  -c, --condition "%v"
  Success condition (Starlark expression).

  -d, --delay %v
  Constant delay (duration).

  -f, --forever
  Infinite attempts.

  -j, --jitter "%v"
  Additional random delay (maximum duration or 'min,max' duration).

  -m, --max-delay %v
  Maximum allowed sum of constant delay and exponential backoff (duration).

  -n, --attempts %v
  Maximum number of attempts (negative for infinite).

  -t, --timeout %v
  Timeout for each attempt (duration; negative for no timeout).

  -v, --verbose
  Increase verbosity (up to %v times).
`,
		formatDuration(backoffDefault),
		conditionDefault,
		formatDuration(delayDefault),
		jitterDefault,
		formatDuration(maxDelayDefault),
		maxAttemptsDefault,
		formatDuration(timeoutDefault),
		maxVerboseLevel,
	)
}

func parseArgs() retryConfig {
	config := retryConfig{
		Args:        []string{},
		Backoff:     backoffDefault,
		Condition:   conditionDefault,
		FixedDelay:  interval{Start: delayDefault, End: maxDelayDefault},
		MaxAttempts: maxAttemptsDefault,
		Timeout:     timeoutDefault,
	}

	// Check early for special flags that override argument validation.
	for _, arg := range os.Args {
		switch arg {
		case "-h", "--help":
			help()
			os.Exit(0)

		case "-V", "--version":
			fmt.Printf("%s\n", version)
			os.Exit(0)
		}
	}

	usageError := func(message string, badValue interface{}) {
		fmt.Fprintf(os.Stderr, "Error: "+message+"\n", badValue)
		usage(os.Stderr)
		os.Exit(2)
	}

	vShortFlags := regexp.MustCompile("^-v+$")

	// Parse the command-line flags.
	var i int
	for i = 1; i < len(os.Args); i++ {
		arg := os.Args[i]
		nextArg := func(flag string) string {
			i++

			if i >= len(os.Args) {
				usageError("no value for flag '%s'", flag)
			}

			return os.Args[i]
		}

		if arg == "--" || !strings.HasPrefix(arg, "-") {
			break
		}

		switch arg {
		case "-b", "--backoff":
			value := nextArg(arg)

			backoff, err := time.ParseDuration(value)
			if err != nil {
				usageError("invalid backoff: %v", value)
			}

			config.Backoff = backoff

		case "-c", "--condition":
			config.Condition = nextArg(arg)

		case "-d", "--delay":
			value := nextArg(arg)

			delay, err := time.ParseDuration(value)
			if err != nil {
				fmt.Fprintf(os.Stderr, "invalid delay: %v", value)
				os.Exit(2)
			}

			config.FixedDelay.Start = delay

		case "-f", "--forever":
			config.MaxAttempts = -1

		case "-j", "--jitter":
			jitter, err := parseInterval(nextArg(arg))
			if err != nil {
				usageError("invalid jitter: %v", err)
			}

			config.RandomDelay = jitter

		case "-m", "--max-delay":
			value := nextArg(arg)

			maxDelay, err := time.ParseDuration(value)
			if err != nil {
				fmt.Fprintf(os.Stderr, "invalid maximum delay: %v", value)
				os.Exit(2)
			}

			config.FixedDelay.End = maxDelay

		case "-n", "--attempts", "--tries":
			value := nextArg(arg)

			var maxAttempts int
			_, err := fmt.Sscanf(value, "%d", &maxAttempts)
			if err != nil {
				fmt.Fprintf(os.Stderr, "invalid maximum number of attempts: %v", value)
				os.Exit(2)
			}

			config.MaxAttempts = maxAttempts

		case "-t", "--timeout":
			value := nextArg(arg)

			timeout, err := time.ParseDuration(value)
			if err != nil {
				fmt.Fprintf(os.Stderr, "invalid timeout: %v", value)
				os.Exit(2)
			}

			config.Timeout = timeout

		case "--verbose":
			config.Verbose++

		default:
			if vShortFlags.MatchString(arg) {
				config.Verbose += len(arg) - 1
				continue
			}

			usageError("unknown flag: %v", arg)
		}
	}

	if config.Verbose > maxVerboseLevel {
		usageError("up to %d verbose flags is allowed", maxVerboseLevel)
	}

	if i >= len(os.Args) {
		usageError("<command> is required%v", "")
	}

	config.Command = os.Args[i]
	config.Args = os.Args[i+1:]

	return config
}

func main() {
	config := parseArgs()

	// Configure logging.
	customWriter := &elapsedTimeWriter{
		startTime: time.Now(),
	}
	log.SetOutput(customWriter)
	log.SetFlags(0)

	if config.Verbose >= 3 {
		log.Printf("configuration: %s\n", repr.String(config, repr.OmitEmpty(false)))
	}

	exitCode, err := retry(config)
	if err != nil {
		log.Printf("%v", err)
	}

	os.Exit(exitCode)
}
