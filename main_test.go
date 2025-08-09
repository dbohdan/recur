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
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

var (
	commandCat    = "test/cat"
	commandEnv    = "test/env"
	commandExit99 = "test/exit99"
	commandHello  = "test/hello"
	commandRecur  = "./recur"
	commandSleep  = "test/sleep"
	noSuchCommand = "no-such-command-should-exist"
)

func runCommand(args ...string) (string, string, error) {
	cmd := exec.Command(commandRecur, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()

	return stdout.String(), stderr.String(), err
}

func runCommandWithStdin(stdin string, args ...string) (string, string, error) {
	cmd := exec.Command(commandRecur, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Stdin = strings.NewReader(stdin)
	err := cmd.Run()

	return stdout.String(), stderr.String(), err
}

func TestUsage(t *testing.T) {
	_, stderr, _ := runCommand()

	if matched, _ := regexp.MatchString("Usage", stderr); !matched {
		t.Error("Expected 'Usage' in stderr")
	}
}

func TestVersion(t *testing.T) {
	stdout, _, _ := runCommand("--version")

	if matched, _ := regexp.MatchString(`\d+\.\d+\.\d+`, stdout); !matched {
		t.Error("Expected version format in stdout")
	}
}

func TestUnknownOptBeforeHelp(t *testing.T) {
	_, _, err := runCommand("--foo", "--help", commandExit99)

	if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() != 2 {
		t.Errorf("Expected exit status 2, got %v", err)
	}
}

func TestUnknownOptAfterHelp(t *testing.T) {
	_, _, err := runCommand("--help", "--foo", commandExit99)

	if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() != 2 {
		t.Errorf("Expected exit status 2, got %v", err)
	}
}

func TestEcho(t *testing.T) {
	stdout, _, _ := runCommand(commandHello)

	if matched, _ := regexp.MatchString("hello", stdout); !matched {
		t.Error("Expected 'hello' in stdout")
	}
}

func TestEnv(t *testing.T) {
	_, _, err := runCommand(commandEnv)

	if _, ok := err.(*exec.ExitError); ok {
		t.Errorf("Expected exit status 0, got %v", err)
	}
}

func TestExitCode(t *testing.T) {
	_, _, err := runCommand(commandExit99)

	if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() != 99 {
		t.Errorf("Expected exit status 99, got %v", err)
	}
}

func TestCommandNotFound(t *testing.T) {
	_, _, err := runCommand(noSuchCommand)

	if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() != 255 {
		t.Errorf("Expected exit status 255, got %v", err)
	}
}

func TestOptions(t *testing.T) {
	_, _, err := runCommand("-a", "0", "-b", "1s", "-d", "0", "-F", "--jitter", "0,0.1s", "-m", "0", commandHello)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
}

func TestEndOfOptions(t *testing.T) {
	_, _, err := runCommand("--", commandHello)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
}

func TestEndOfOptionsHelp(t *testing.T) {
	_, _, err := runCommand("--", commandExit99, "--help")

	if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() != 99 {
		t.Errorf("Expected exit status 99, got %v", err)
	}
}

func TestAttemptsTrailingGarbageOptions(t *testing.T) {
	_, _, err := runCommand("-a", "0abcdef", commandHello)

	if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() != 2 {
		t.Errorf("Expected exit status 2, got %v", err)
	}
}

func TestBackoffAndNegativeDelay(t *testing.T) {
	_, stderr, _ := runCommand("-a", "2", "-b", "1.05s", "-d", "-1s", "-v", commandExit99)

	if matched, _ := regexp.MatchString(`waiting \d{2}ms`, stderr); !matched {
		t.Error(`Expected 'waiting \d{2}ms' in stderr`)
	}
}

func TestFibonacciBackoff(t *testing.T) {
	_, stderr, _ := runCommand("-d", "-33.99s", "-F", "-v", commandExit99)

	if matched, _ := regexp.MatchString(`waiting 10ms after attempt 9\n`, stderr); !matched {
		t.Error(`Expected 'waiting 10ms after attempt 9\n' in stderr`)
	}
}

func TestVerbose(t *testing.T) {
	_, stderr, _ := runCommand("-v", "-a", "3", commandExit99)

	if count := len(regexp.MustCompile("command exited with code").FindAllString(stderr, -1)); count != 3 {
		t.Errorf("Expected 3 instances of 'command exited with code', got %d", count)
	}

	if !strings.Contains(stderr, "on attempt 3\n") {
		t.Error("Expected 'on attempt 3' in stderr")
	}
}

func TestVerboseCommandNotFound(t *testing.T) {
	_, stderr, _ := runCommand("-v", "-a", "3", noSuchCommand)

	if count := len(regexp.MustCompile("command was not found").FindAllString(stderr, -1)); count != 3 {
		t.Errorf("Expected 3 instances of 'command was not found', got %d", count)
	}
}

func TestVerboseConfig(t *testing.T) {
	_, stderr, _ := runCommand("-vv", "--verbose", commandHello)

	if matched, _ := regexp.MatchString(`main\.retryConfig{\n`, stderr); !matched {
		t.Error(`Expected 'main\.retryConfig{\n' in stderr`)
	}
}

func TestVerboseTooMany(t *testing.T) {
	_, stderr, _ := runCommand("-vvvvvv", "")

	if matched, _ := regexp.MatchString("Error:.*?verbose options", stderr); !matched {
		t.Error("Expected 'Error:.*?verbose options' in stderr")
	}
}

func TestStopOnSuccess(t *testing.T) {
	stdout, _, _ := runCommand(commandHello)

	if count := len(regexp.MustCompile("hello").FindAllString(stdout, -1)); count != 1 {
		t.Errorf("Expected 1 instance of 'hello', got %d", count)
	}
}

func TestConditionAttemptForever(t *testing.T) {
	stdout, _, _ := runCommand("--condition", "attempt == 5", "--forever", commandHello)

	if count := len(regexp.MustCompile("hello").FindAllString(stdout, -1)); count != 5 {
		t.Errorf("Expected 5 instances of 'hello', got %d", count)
	}
}

func TestConditionAttemptNegative(t *testing.T) {
	stdout, _, _ := runCommand("--attempts", "-1", "--condition", "attempt == 5", commandHello)

	if count := len(regexp.MustCompile("hello").FindAllString(stdout, -1)); count != 5 {
		t.Errorf("Expected 5 instances of 'hello', got %d", count)
	}
}

func TestConditionExitIfCode(t *testing.T) {
	_, _, err := runCommand("--condition", "exit(0) if code == 99 else 'fail'", commandExit99)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
}

func TestConditionExitArgNone(t *testing.T) {
	_, _, err := runCommand("-c", "exit(None)", commandHello)

	if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() != 255 {
		t.Errorf("Expected exit status 255, got %v", err)
	}
}

func TestConditionExitArgTooLarge(t *testing.T) {
	_, stderr, err := runCommand("--condition", "exit(10000000000000000000)", commandHello)

	if matched, _ := regexp.MatchString("code too large", stderr); !matched {
		t.Error("Expected 'code too large' in stderr")
	}

	if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() != 1 {
		t.Errorf("Expected exit status 1, got %v", err)
	}
}

func TestConditionExitArgWrongType(t *testing.T) {
	_, stderr, err := runCommand("--condition", "exit('foo')", commandHello)

	if matched, _ := regexp.MatchString("exit code wasn't", stderr); !matched {
		t.Error("Expected \"exit code wasn't\" in stderr")
	}

	if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() != 1 {
		t.Errorf("Expected exit status 1, got %v", err)
	}
}

func TestConditionInspect(t *testing.T) {
	_, stderr, err := runCommand("--condition", "inspect(code) == 99 and exit(0)", commandExit99)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if matched, _ := regexp.MatchString("inspect: 99", stderr); !matched {
		t.Error("Expected 'inspect: 99' in stderr")
	}
}

func TestConditionInspectWithPrefix(t *testing.T) {
	_, stderr, err := runCommand("--condition", "inspect(code, prefix='code = ') == 99 and exit(0)", commandExit99)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if matched, _ := regexp.MatchString("code = 99", stderr); !matched {
		t.Error("Expected 'code = 99' in stderr")
	}
}

func TestConditionTimeAndTotalTime(t *testing.T) {
	stdout, _, _ := runCommand("--condition", "total_time > time", commandSleep, "0.1")

	if count := len(regexp.MustCompile("T").FindAllString(stdout, -1)); count != 2 {
		t.Errorf("Expected 2 instances of 'T', got %d", count)
	}
}

func TestReset(t *testing.T) {
	_, stderr, err := runCommand("--backoff", "0.1s", "--condition", "attempt == 3", "--reset", "10ms", "--verbose", commandSleep, "0.01")
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if count := len(regexp.MustCompile("waiting 100ms").FindAllString(stderr, -1)); count != 2 {
		t.Errorf("Expected 2 instances of 'waiting 100ms', got %d", count)
	}
}

func TestConditionTotalTime(t *testing.T) {
	stdout, _, _ := runCommand("--condition", "total_time > 0.3", commandSleep, "0.1")

	if matched, _ := regexp.MatchString(`(?:T\s*){2,3}`, stdout); !matched {
		t.Error(`Expected '(?:T\s*){2,3}' in stdout`)
	}
}

func TestConditionCommandNotFound(t *testing.T) {
	_, _, err := runCommand("--condition", "command_found or exit(42)", noSuchCommand)

	if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() != 42 {
		t.Errorf("Expected exit status 42, got %v", err)
	}
}

func TestConditionCommandNotFoundCode(t *testing.T) {
	_, _, err := runCommand("--condition", "code == None and exit(42)", noSuchCommand)

	if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() != 42 {
		t.Errorf("Expected exit status 42, got %v", err)
	}
}

func TestConditionTimeout(t *testing.T) {
	_, stderr, _ := runCommand("--attempts", "3", "--timeout", "100ms", "--verbose", commandSleep, "1")

	if count := len(regexp.MustCompile("command timed out").FindAllString(stderr, -1)); count != 3 {
		t.Errorf("Expected 3 instances of 'command timed out', got %d", count)
	}
}

func TestReplayStdin(t *testing.T) {
	t.Run("without stdin replay", func(t *testing.T) {
		stdout, _, _ := runCommandWithStdin("hi\n", "-a", "3", "-c", "False", commandCat)
		if count := strings.Count(stdout, "hi"); count != 1 {
			t.Errorf("Expected 1 instance of 'hi', got %d", count)
		}
	})

	t.Run("with stdin replay", func(t *testing.T) {
		stdout, _, _ := runCommandWithStdin("hi\n", "-a", "3", "-c", "False", "-I", commandCat)
		if count := strings.Count(stdout, "hi"); count != 3 {
			t.Errorf("Expected 3 instances of 'hi', got %d", count)
		}
	})
}
