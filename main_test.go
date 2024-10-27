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
	"bytes"
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

var (
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

func TestEcho(t *testing.T) {
	stdout, _, _ := runCommand(commandHello)

	if matched, _ := regexp.MatchString("hello", stdout); !matched {
		t.Error("Expected 'hello' in stdout")
	}
}

func TestExitCode(t *testing.T) {
	_, _, err := runCommand(commandExit99)

	if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() != 99 {
		t.Errorf("Expected exit code 99, got %v", err)
	}
}

func TestCommandNotFound(t *testing.T) {
	_, _, err := runCommand(noSuchCommand)

	if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() != 255 {
		t.Errorf("Expected exit code 255, got %v", err)
	}
}

func TestOptions(t *testing.T) {
	_, _, err := runCommand("-b", "1s", "-d", "0", "--jitter", "0,0.1s", "-m", "0", "-n", "0", commandHello)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
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

	if matched, _ := regexp.MatchString("Error:.*?verbose flags", stderr); !matched {
		t.Error("Expected 'Error:.*?verbose flags' in stderr")
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
	stdout, _, _ := runCommand("--tries", "-1", "--condition", "attempt == 5", commandHello)

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
		t.Errorf("Expected exit code 255, got %v", err)
	}
}

func TestConditionExitArgTooLarge(t *testing.T) {
	_, stderr, err := runCommand("--condition", "exit(10000000000000000000)", commandHello)

	if matched, _ := regexp.MatchString("code too large", stderr); !matched {
		t.Error("Expected 'code too large' in stderr")
	}

	if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() != 1 {
		t.Errorf("Expected exit code 1, got %v", err)
	}
}

func TestConditionExitArgWrongType(t *testing.T) {
	_, stderr, err := runCommand("--condition", "exit('foo')", commandHello)

	if matched, _ := regexp.MatchString("exit code wasn't", stderr); !matched {
		t.Error("Expected \"exit code wasn't\" in stderr")
	}

	if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() != 1 {
		t.Errorf("Expected exit code 1, got %v", err)
	}
}

func TestConditionTimeAndTotalTime(t *testing.T) {
	stdout, _, _ := runCommand("--condition", "total_time > time", commandSleep, "0.1")

	if count := len(regexp.MustCompile("T").FindAllString(stdout, -1)); count != 2 {
		t.Errorf("Expected 2 instances of 'T', got %d", count)
	}
}

func TestConditionCommandNotFound(t *testing.T) {
	_, _, err := runCommand("--condition", "command_found or exit(42)", noSuchCommand)

	if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() != 42 {
		t.Errorf("Expected exit code 42, got %v", err)
	}
}

func TestConditionCommandNotFoundCode(t *testing.T) {
	_, _, err := runCommand("--condition", "code == None and exit(42)", noSuchCommand)

	if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() != 42 {
		t.Errorf("Expected exit code 42, got %v", err)
	}
}

func TestConditionTimeout(t *testing.T) {
	_, stderr, _ := runCommand("--attempts", "3", "--timeout", "100ms", "--verbose", commandSleep, "1")

	if count := len(regexp.MustCompile("command timed out").FindAllString(stderr, -1)); count != 3 {
		t.Errorf("Expected 3 instances of 'command timed out', got %d", count)
	}
}
