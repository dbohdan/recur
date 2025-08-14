// Copyright (c) 2023-2025 D. Bohdan
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
	"bytes"
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
	"strconv"
	"strings"
	"time"

	"github.com/alecthomas/repr"
	tsize "github.com/kopoli/go-terminal-size"
	"github.com/mitchellh/go-wordwrap"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
)

const (
	envVarAttempt           = "RECUR_ATTEMPT"
	envVarMaxAttempts       = "RECUR_MAX_ATTEMPTS"
	envVarAttemptSinceReset = "RECUR_ATTEMPT_SINCE_RESET"
	exitCodeCommandNotFound = 255
	exitCodeError           = -1
	maxVerboseLevel         = 3
	starlarkVarFlushStdout  = "_flush_stdout"
	version                 = "2.5.0"
)

type attempt struct {
	CommandFound     bool
	Duration         time.Duration
	ExitCode         int
	MaxAttempts      int
	Number           int
	NumberSinceReset int
	TotalTime        time.Duration
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
	Fibonacci   bool
	FixedDelay  interval
	HoldStdout  bool
	MaxAttempts int
	RandomDelay interval
	ReplayStdin bool
	Reset       time.Duration
	Timeout     time.Duration
	Verbose     int
}

const (
	backoffDefault     = time.Duration(0)
	conditionDefault   = "code == 0"
	delayDefault       = time.Duration(0)
	jitterDefault      = "0,0"
	maxDelayDefault    = time.Duration(time.Hour)
	maxAttemptsDefault = 10
	resetDefault       = time.Duration(-time.Second)
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

func StarlarkFlushStdout(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := starlark.UnpackPositionalArgs(b.Name(), args, kwargs, 0); err != nil {
		return nil, err
	}

	thread.SetLocal(starlarkVarFlushStdout, starlark.True)
	return starlark.None, nil
}

func StarlarkInspect(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var prefix starlark.String
	var value starlark.Value

	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "value", &value, "prefix?", &prefix); err != nil {
		return nil, err
	}

	prefixStr := ""
	if prefix.Len() > 0 {
		prefixStr = prefix.GoString()
	}

	log.Printf("inspect: %s%v\n", prefixStr, value)

	return value, nil
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

func flushStdoutLocal(thread *starlark.Thread) bool {
	if v := thread.Local(starlarkVarFlushStdout); v != nil {
		if flushStdoutVal, ok := v.(starlark.Value); ok {
			return flushStdoutVal == starlark.True
		}
	}

	return false
}

func evaluateCondition(attemptInfo attempt, expr string) (bool, bool, error) {
	thread := &starlark.Thread{Name: "condition"}

	var code starlark.Value
	if attemptInfo.CommandFound {
		code = starlark.MakeInt(attemptInfo.ExitCode)
	} else {
		code = starlark.None
	}

	env := starlark.StringDict{
		"exit":         starlark.NewBuiltin("exit", StarlarkExit),
		"flush_stdout": starlark.NewBuiltin("flush_stdout", StarlarkFlushStdout),
		"inspect":      starlark.NewBuiltin("inspect", StarlarkInspect),

		"attempt":             starlark.MakeInt(attemptInfo.Number),
		"attempt_since_reset": starlark.MakeInt(attemptInfo.NumberSinceReset),
		"code":                code,
		"command_found":       starlark.Bool(attemptInfo.CommandFound),
		"max_attempts":        starlark.MakeInt(attemptInfo.MaxAttempts),
		"time":                starlark.Float(float64(attemptInfo.Duration) / float64(time.Second)),
		"total_time":          starlark.Float(float64(attemptInfo.TotalTime) / float64(time.Second)),
	}

	val, err := starlark.EvalOptions(syntax.LegacyFileOptions(), thread, "", expr, env)
	flushStdout := flushStdoutLocal(thread)
	if err != nil {
		var exitErr *exitRequestError
		if errors.As(err, &exitErr) {
			return false, flushStdout, exitErr
		}

		return false, false, err
	}

	if val.Type() != "bool" {
		return false, false, fmt.Errorf("condition must return a boolean, got %s", val.Type())
	}

	success := bool(val.Truth())
	flushStdout = flushStdout || success

	return success, flushStdout, nil
}

func executeCommand(command string, args []string, timeout time.Duration, envVars []string, stdinContent []byte, holdStdout bool) (commandResult, []byte) {
	if _, err := exec.LookPath(command); err != nil {
		return commandResult{
			Status:   statusNotFound,
			ExitCode: exitCodeCommandNotFound,
		}, nil
	}

	ctx := context.Background()
	if timeout >= 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, command, args...)
	var stdoutBuffer bytes.Buffer
	if holdStdout {
		cmd.Stdout = &stdoutBuffer
	} else {
		cmd.Stdout = os.Stdout
	}
	cmd.Stderr = os.Stderr
	if stdinContent == nil {
		cmd.Stdin = os.Stdin
	} else {
		cmd.Stdin = bytes.NewReader(stdinContent)
	}
	cmd.Env = append(os.Environ(), envVars...)

	err := cmd.Run()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return commandResult{
				Status:   statusTimeout,
				ExitCode: exitCodeError,
			}, stdoutBuffer.Bytes()
		}

		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return commandResult{
				Status:   statusFinished,
				ExitCode: exitErr.ExitCode(),
			}, stdoutBuffer.Bytes()
		}

		return commandResult{
			Status:   statusUnknownError,
			ExitCode: exitCodeError,
		}, stdoutBuffer.Bytes()
	}

	return commandResult{
		Status:   statusFinished,
		ExitCode: cmd.ProcessState.ExitCode(),
	}, stdoutBuffer.Bytes()
}

func fib(n int) float64 {
	nf := float64(n)
	return math.Round((math.Pow(math.Phi, nf) - math.Pow(-math.Phi, -nf)) * 0.4472135954999579)
}

func delayBeforeAttempt(attemptNum int, config retryConfig) time.Duration {
	if attemptNum == 1 {
		return 0
	}

	currFixed := config.FixedDelay.Start.Seconds()
	currFixed += math.Pow(config.Backoff.Seconds(), float64(attemptNum-1))
	if config.Fibonacci {
		currFixed += fib(attemptNum - 1)
	}
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

func retry(config retryConfig, stdinContent []byte) (int, error) {
	var cmdResult commandResult
	var stdoutContent []byte
	var startTime time.Time
	var totalTime time.Duration

	resetAttemptNum := 1
	for attemptNum := 1; config.MaxAttempts < 0 || attemptNum <= config.MaxAttempts; attemptNum++ {
		attemptSinceReset := attemptNum - resetAttemptNum + 1
		delay := delayBeforeAttempt(attemptSinceReset, config)
		if delay > 0 {
			if config.Verbose >= 1 {
				log.Printf("waiting %s after attempt %d", formatDuration(delay), attemptNum-1)
			}
			time.Sleep(delay)
		}

		attemptStart := time.Now()
		if startTime.IsZero() {
			startTime = attemptStart
		}

		envVars := []string{
			fmt.Sprintf("%s=%d", envVarAttempt, attemptNum),
			fmt.Sprintf("%s=%d", envVarAttemptSinceReset, attemptSinceReset),
			fmt.Sprintf("%s=%d", envVarMaxAttempts, config.MaxAttempts),
		}
		cmdResult, stdoutContent = executeCommand(config.Command, config.Args, config.Timeout, envVars, stdinContent, config.HoldStdout)

		attemptEnd := time.Now()
		attemptDuration := attemptEnd.Sub(attemptStart)
		totalTime = attemptEnd.Sub(startTime)

		if config.Reset >= 0 && attemptDuration >= config.Reset {
			resetAttemptNum = attemptNum
		}

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
			CommandFound:     cmdResult.Status != statusNotFound,
			Duration:         attemptDuration,
			ExitCode:         cmdResult.ExitCode,
			MaxAttempts:      config.MaxAttempts,
			Number:           attemptNum,
			NumberSinceReset: attemptSinceReset,
			TotalTime:        totalTime,
		}

		success, flushStdout, err := evaluateCondition(attemptInfo, config.Condition)
		if flushStdout {
			os.Stdout.Write(stdoutContent)
		}
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

func wrapForTerm(s string) string {
	size, err := tsize.GetSize()
	if err != nil {
		return s
	}

	return wordwrap.WrapString(s, uint(size.Width))
}

func usage(w io.Writer) {
	s := fmt.Sprintf(
		`Usage: %s [-h] [-V] [-a <attempts>] [-b <backoff>] [-c <condition>] [-d <delay>] [-F] [-f] [-I] [-j <jitter>] [-m <max-delay>] [-O] [-r <reset-time>] [-t <timeout>] [-v] [--] <command> [<arg> ...]`,
		filepath.Base(os.Args[0]),
	)

	fmt.Fprintln(w, wrapForTerm(s))
}

func help() {
	usage(os.Stdout)

	s := fmt.Sprintf(
		`
Retry a command with exponential backoff and jitter.

Arguments:
  <command>
          Command to run

  [<arg> ...]
          Arguments to the command

Options:
  -h, --help
          Print this help message and exit

  -V, --version
          Print version number and exit

  -a, --attempts %v
          Maximum number of attempts (negative for infinite)

  -b, --backoff %v
          Base for exponential backoff (duration)

  -c, --condition '%v'
          Success condition (Starlark expression)

  -d, --delay %v
          Constant delay (duration)

  -F, --fib
          Add Fibonacci backoff

  -f, --forever
          Infinite attempts

  -I, --replay-stdin
          Read standard input until EOF at the start and replay it on each attempt

  -j, --jitter '%v'
          Additional random delay (maximum duration or 'min,max' duration)

  -m, --max-delay %v
          Maximum allowed sum of constant delay, exponential backoff, and Fibonacci backoff (duration)

  -O, --hold-stdout
          Buffer standard output for each attempt and only print it on success

  -r, --reset %v
          Minimum attempt time that resets exponential and Fibonacci backoff (duration; negative for no reset)

  -t, --timeout %v
          Timeout for each attempt (duration; negative for no timeout)

  -v, --verbose
          Increase verbosity (up to %v times)
`,
		maxAttemptsDefault,
		formatDuration(backoffDefault),
		conditionDefault,
		formatDuration(delayDefault),
		jitterDefault,
		formatDuration(maxDelayDefault),
		formatDuration(resetDefault),
		formatDuration(timeoutDefault),
		maxVerboseLevel,
	)

	fmt.Print(wrapForTerm(s))
}

func parseArgs() retryConfig {
	config := retryConfig{
		Args:        []string{},
		Backoff:     backoffDefault,
		Condition:   conditionDefault,
		FixedDelay:  interval{Start: delayDefault, End: maxDelayDefault},
		MaxAttempts: maxAttemptsDefault,
		Reset:       resetDefault,
		Timeout:     timeoutDefault,
	}

	usageError := func(message string, badValue interface{}) {
		usage(os.Stderr)
		fmt.Fprintf(os.Stderr, "\nError: "+message+"\n", badValue)
		os.Exit(2)
	}

	vShortFlags := regexp.MustCompile("^-v+$")

	// Parse the command-line options.
	var i int
	printHelp := false
	printVersion := false

	nextArg := func(flag string) string {
		i++

		if i >= len(os.Args) {
			usageError("no value for option: %s", flag)
		}

		return os.Args[i]
	}

	for i = 1; i < len(os.Args); i++ {
		arg := os.Args[i]

		if arg == "--" {
			i++
			break
		}
		if !strings.HasPrefix(arg, "-") {
			break
		}

		switch arg {
		case "-a", "--attempts":
			value := nextArg(arg)

			var maxAttempts int
			maxAttempts, err := strconv.Atoi(value)
			if err != nil {
				usageError("invalid maximum number of attempts: %v", value)
			}

			config.MaxAttempts = maxAttempts

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
				usageError("invalid delay: %v", value)
			}

			config.FixedDelay.Start = delay
			if config.FixedDelay.End < config.FixedDelay.Start {
				config.FixedDelay.End = config.FixedDelay.Start
			}

		case "-F", "--fib":
			config.Fibonacci = true

		case "-f", "--forever":
			config.MaxAttempts = -1

		case "-h", "--help":
			printHelp = true

		case "-I", "--replay-stdin":
			config.ReplayStdin = true

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
				usageError("invalid maximum delay: %v", value)
			}

			config.FixedDelay.End = maxDelay

		case "-O", "--hold-stdout":
			config.HoldStdout = true

		case "-r", "--reset":
			value := nextArg(arg)

			reset, err := time.ParseDuration(value)
			if err != nil {
				usageError("invalid reset time: %v", value)
			}

			config.Reset = reset

		case "-t", "--timeout":
			value := nextArg(arg)

			timeout, err := time.ParseDuration(value)
			if err != nil {
				usageError("invalid timeout: %v", value)
			}

			config.Timeout = timeout

		// "-v" is handled in the default case.
		case "--verbose":
			config.Verbose++

		case "-V", "--version":
			printVersion = true

		default:
			if vShortFlags.MatchString(arg) {
				config.Verbose += len(arg) - 1
				continue
			}

			usageError("unknown option: %v", arg)
		}
	}

	if printHelp {
		help()
		os.Exit(0)
	}

	if printVersion {
		fmt.Printf("%s\n", version)
		os.Exit(0)
	}

	if config.Verbose > maxVerboseLevel {
		usageError("up to %d verbose options is allowed", maxVerboseLevel)
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

	var stdinContent []byte = nil
	if config.ReplayStdin {
		stdinContent = []byte{}

		stat, err := os.Stdin.Stat()
		if err != nil {
			log.Printf("failed to stat stdin: %v", err)
			os.Exit(1)
		}

		if stat.Mode()&os.ModeCharDevice == 0 {
			stdinContent, err = io.ReadAll(os.Stdin)
			if err != nil {
				log.Printf("failed to read stdin: %v", err)
				os.Exit(1)
			}
		}
	}

	if config.Verbose >= 3 {
		log.Printf("configuration:\n%s\n", repr.String(config, repr.Indent("\t"), repr.OmitEmpty(false)))
	}

	exitCode, err := retry(config, stdinContent)
	if err != nil {
		log.Printf("%v", err)
	}

	os.Exit(exitCode)
}
