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
	"errors"
	"fmt"
	"log"
	"time"

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
)

const (
	starlarkVarFlushStdout = "_flush_stdout"
)

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

	success := bool(val.Truth())
	flushStdout = flushStdout || success

	return success, flushStdout, nil
}
