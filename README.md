# recur

**recur** is a command-line tool that runs a single command repeatedly until it succeeds or no more attempts are left.
It implements optional [exponential backoff](https://en.wikipedia.org/wiki/Exponential_backoff) with configurable [jitter](https://en.wikipedia.org/wiki/Thundering_herd_problem#Mitigation).
It lets you write the success condition in [Starlark](https://laurent.le-brun.eu/blog/an-overview-of-starlark).

## Installation

### Prebuilt binaries

Prebuilt binaries for
FreeBSD (amd64),
Linux (aarch64, riscv64, x86_64),
macOS (arm64, x86_64),
NetBSD (amd64),
OpenBSD (amd64),
and Windows (amd64, arm64, x86)
are attached to [releases](https://github.com/dbohdan/recur/releases).

### Homebrew

You can install recur [from Homebrew](https://formulae.brew.sh/formula/recur) on macOS and Linux:

```shell
brew install recur
```

### Go

Install Go, then run:

```shell
go install dbohdan.com/recur/v3@latest
```

## Build requirements

- Go 1.22
- [Task](https://taskfile.dev/) (go-task) 3.28

## Usage

### Command-line interface

<!-- BEGIN USAGE -->
```none
Usage: recur [-h] [-V] [-a <attempts>] [-b <backoff>] [-c <condition>] [-d
<delay>] [-E] [-F] [-f] [-I] [-j <jitter>] [-m <max-delay>] [-O] [-o <path>] [-R
<format>] [-r <reset-time>] [-s <seed>] [-t <timeout>] [-v] [--] <command>
[<arg> ...]

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

  -a, --attempts 10
          Maximum number of attempts (negative for infinite)

  -b, --backoff 0
          Base for exponential backoff (duration)

  -c, --condition "code == 0"
          Success condition (Starlark expression)

  -d, --delay 0
          Constant delay (duration)

  -E, --hold-stderr
          Buffer standard error for each attempt and only print it on success

  -F, --fib
          Add Fibonacci backoff

  -f, --forever
          Infinite attempts

  -I, --replay-stdin
          Read standard input until EOF at the start and replay it on each
attempt

  -j, --jitter "0,0"
          Additional random delay (maximum duration or "min,max" duration)

  -m, --max-delay 1h
          Maximum allowed sum of constant delay, exponential backoff, and
Fibonacci backoff (duration)

  -O, --hold-stdout
          Buffer standard output for each attempt and only print it on success

  -o, --report-file "-"
          Output file for the report ("-" for stderr)

  -R, --report "none"
          Report format ("none", "json", or "text")

  -r, --reset -1s
          Minimum attempt time that resets exponential and Fibonacci backoff
(duration; negative for no reset)

  -s, --seed 0
          Random seed for jitter (0 for automatic)

  -t, --timeout -1s
          Timeout for each attempt (duration; negative for no timeout)

  -v, --verbose
          Increase verbosity (up to 3 times)
```
<!-- END USAGE -->

Duration arguments take [Go duration strings](https://pkg.go.dev/time#ParseDuration);
for example, `0`, `100ms`, `2.5s`, `0.5m`, or `1h`.
The value of `-j`/`--jitter` must be either a single duration or two durations joined with a comma, like `1s,2s` or `500ms, 0.5m`.

If the maximum delay (`-m`/`--max-delay`) is shorter than the constant delay (`-d`/`--delay`), the constant delay will automatically increase the maximum delay to match it.
Use `-m`/`--max-delay` after `-d`/`--delay` if you want a shorter maximum delay.

The following recur options run the command `foo --config bar.cfg` indefinitely.
Every time `foo` exits, there is a delay that grows exponentially from two seconds to a minute.
The delay resets back to two seconds if the command runs for at least five minutes.

```shell
recur --backoff 2s --condition False --forever --max-delay 1m --reset 5m foo --config bar.cfg
```

recur exits with the last command's exit code unless the user overrides this in the condition.
When the command is not found during the last attempt, recur exits with code 127.
recur exits with code 124 on timeout and 255 on internal error.

### Standard input

By default, the command run by recur inherits its [standard input](https://en.wikipedia.org/wiki/Standard_streams#Standard_input_(stdin)).
This means that if standard input is a terminal, every attempt can read interactively from the terminal.
If standard input is a pipe or redirected file, the data is consumed on the first attempt;
later attempts see an immediate [EOF](https://en.wikipedia.org/wiki/End-of-file).

To feed the command the same data on each attempt, use the `-I`/`--replay-stdin` option.
With this option, recur reads its entire stdin into memory and replays it on each attempt.

```none
$ echo hi | recur -a 3 -c False cat
hi
recur [00:00:00.0]: maximum 3 attempts reached

$ echo hi | recur -a 3 -c False -I cat
hi
hi
hi
recur [00:00:00.0]: maximum 3 attempts reached
```

Because the data is buffered in memory, `--replay-stdin` is not recommended for very large inputs.

### Standard output and standard error

The command's [standard output](https://en.wikipedia.org/wiki/Standard_streams#Standard_output_(stdout)) and [standard error](https://en.wikipedia.org/wiki/Standard_streams#Standard_error_(stderr)) are passed through to recur's standard output and standard error by default.
To buffer standard output and only print it on success, use `-O`/`--hold-stdout`.
With this option, recur buffers the command's standard output and only prints it if the success condition is met or the condition expression calls `stdout.flush()`.

```none
$ recur -c 'attempt == 3' sh -c 'echo "$RECUR_ATTEMPT"'
1
2
3

$ recur -c 'attempt == 3' -O sh -c 'echo "$RECUR_ATTEMPT"'
3

$ recur -c 'stdout.flush() or attempt == 3' -O sh -c 'echo "$RECUR_ATTEMPT"'
1
2
3
```

The `-E`/`--hold-stderr` option and `stderr.flush()` method work similarly for standard error.

Because the data is buffered in memory, `--hold-stdout` and `--hold-stderr` are not recommended for commands that produce very large output.

### Regular-expression matching

You can match regular expressions against recur's input and the command's output in your success condition using methods on the built-in objects `stdin`, `stdout`, and `stderr`:

- `stdin.search()` — matches against standard input (requires `-I`/`--replay-stdin`)
- `stdout.search()` — matches against standard output (requires `-O`/`--hold-stdout`)
- `stderr.search()` — matches against standard error (requires `-E`/`--hold-stderr`)

These methods use [Go regular expressions](https://pkg.go.dev/regexp) with the [RE2 syntax](https://github.com/google/re2/wiki/Syntax).

The `stdin`, `stdout`, and `stderr` objects are `None` without their respective command-line option (`-I`/`--replay-stdin`, `-O`/`--hold-stdout`, or `-E`/`--hold-stderr`).
Calling methods on `None` will result in an error.

Standard input, standard output, and standard error are not available directly as Starlark strings to reduce memory usage.
The methods provide the only way to access them in conditions.

#### Matching standard input

The following example waits for the input to contain `done` on a line after `status:`:

```none
$ printf 'Status:\nDONE\n' | recur \
    --condition 'stdin.search(r"(?im)status:\s*done$")' \
    --replay-stdin \
    cat \
    ;
Status:
DONE
```

`r"..."` disables the processing of backslash escapes in the string.
It is necessary because `\s` is not a valid backslash escape.
The regular expression `(?im)status:\s*done$` uses [RE2 inline flags](https://github.com/google/re2/wiki/Syntax#:~:text=case-insensitive):
- `i` for case-insensitive matching
- `m` for multiline mode (`$` matches the end of each line)

The condition evaluates to true when `stdin.search()` finds a match (returns a non-empty list) and false when no match is found (returns `None`).

Standard input is perhaps of limited use for retrying a command because it is read once and never changes.
However, it can be used to exit early.

#### Matching standard output and standard error

This example extracts a status value from the command's output and validates it:

```none
$ recur \
    --condition 'stdout.search(r"(?i)status:([^\n]+)", group=1, default="fail").strip().lower() != "fail"' \
    --hold-stdout \
    echo 'Status: OK' \
    ;
Status: OK
```

In this condition:

- `stdout.search(r"(?i)status:([^\n]+)", group=1, default="fail")` searches for `"status:"` followed by text on the same line
  - `r"..."` disables the processing of backslash escapes like `\n` in the string
  - `group=1` extracts just the captured text (for example, `" OK"` with a leading space)
  - `default="fail"` returns `"fail"` if no match is found
- `.strip().lower()` normalizes the extracted value

Matching against standard error with `stderr.search` works similarly.

### Environment variables

recur sets the environment variable `RECUR_ATTEMPT` to the current attempt number so the command can access it.
recur also sets `RECUR_MAX_ATTEMPTS` to the value of `-a`/`--attempts`
and `RECUR_ATTEMPT_SINCE_RESET` to the attempt number since exponential and Fibonacci backoff were reset.

The following command succeeds on the last attempt:

```none
$ recur sh -c 'echo "Attempt $RECUR_ATTEMPT of $RECUR_MAX_ATTEMPTS"; exit $((RECUR_MAX_ATTEMPTS - RECUR_ATTEMPT))'
Attempt 1 of 10
Attempt 2 of 10
Attempt 3 of 10
Attempt 4 of 10
Attempt 5 of 10
Attempt 6 of 10
Attempt 7 of 10
Attempt 8 of 10
Attempt 9 of 10
Attempt 10 of 10
```

## Conditions

recur supports a limited form of scripting.
You can define the success condition using an expression in [Starlark](https://laurent.le-brun.eu/blog/an-overview-of-starlark), a small scripting language derived from Python.
The default condition is `code == 0`.
This means recur stops retrying when the command exits with code zero.

The condition expression can evaluate to any value.
`False`, `None`, numeric zero (`0`, `0.0`), and empty collections (`""`, `()`, `[]`, `{}`) are considered false.
All other values are considered true.

If you know Python, you can quickly start writing recur conditions in Starlark.
The most significant differences between Starlark and Python for this purpose are:

- Starlark has no `is`.
  You must write `code == None`, not `code is None`.
- Starlark has no sets.
  Write `code in (1, 2, 3)` or `code in [1, 2, 3]` instead of `code in {1, 2, 3}`.

You can use the following variables in the condition expression:

- `attempt`: `int` — the number of the current attempt, starting at one.
  Combine with `--forever` to use the condition instead of the built-in attempt counter.
- `attempt_since_reset`: `int` — the attempt number since exponential and Fibonacci backoff were reset, starting at one.
- `code`: `int | None` — the exit code of the last command.
  `code` is `None` when the command was not found and 124 when a timeout occurred.
- `command_found`: `bool` — whether the last command was found.
- `max_attempts`: `int` — the value of the `--attempts` option.
  `--forever` sets it to -1.
- `stderr`: `io_buffer | None` — an object representing standard error.
  `None` without `-E`/`--hold-stderr`.
- `stdin`: `io_buffer | None` — an object representing standard input.
  `None` without `-I`/`--replay-stdin`.
- `stdout`: `io_buffer | None` — an object representing standard output.
  `None` without `-O`/`--hold-stdout`.
- `time`: `float` — the time the most recent attempt took, in seconds.
- `total_time`: `float` — the elapsed time from the start of the first attempt to the end of the most recent attempt, in seconds.

recur defines the following custom functions:

- `exit(code: int | None) -> None` — exit with the given exit code.
  If `code` is `None`, exit with code 127 (command not found).
- `inspect(value: Any, *, prefix: str = "") -> Any` — log `value` prefixed by `prefix` and return `value`.
  This is useful for debugging.

The `stdin`, `stdout`, and `stderr` objects have the following methods:

- `stdout.flush() -> None`, `stderr.flush() -> None` — if recur is running with `-O`/`--hold-stdout` or `-E`/`--hold-stderr` respectively, recur will output the command's buffered standard output or standard error after evaluating the condition.
  The output is printed whether the condition is true or false, and also if `exit` is called.
  These methods cannot be called without their respective options because the objects are `None`.
- `stdin.search`, `stdout.search`, and `stderr.search` with signature `search(pattern: str, *, group: int | None = None, default: Any = None) -> Any` — match a [Go regular expression](https://pkg.go.dev/regexp) against standard input, standard output, or standard error.
  `pattern` uses the [RE2 syntax](https://github.com/google/re2/wiki/Syntax).
  If `group` is not specified, the function returns a list of submatches (with the full match as the first element) or `default` if no match is found.
  If `group` is specified, it returns the given capture group or `default` if the group is not found.
  These methods require `-I`/`--replay-stdin`, `-O`/`--hold-stdout`, and `-E`/`--hold-stderr` respectively.
  Without the option, the corresponding object is `None`, and calling methods on it will result in an error.

The `exit` function allows you to override the default behavior of returning the last command's exit code.
For example, you can make recur exit with success when the command fails:

```shell
recur --condition 'code != 0 and exit(0)' sh -c 'exit 1'
# or
recur --condition 'False if code == 0 else exit(0)' sh -c 'exit 1'
```

In the following example, recur stops early and does not retry when the command's exit code indicates incorrect usage or a problem with the installation:

```shell
recur --condition 'code == 0 or (code in (1, 2, 3, 4) and exit(code))' curl "$url"
```

## Reports

The `-R`/`--report` option controls the output of statistics when recur exits.
The available formats are:

- `none` (default): no statistics are printed
- `text`: human-readable text
- `json`: machine-readable JSON

By default, reports are written to standard error.
Use `-o`/`--report-file` to write to a file instead.
Use `-o -`/`--report-file -` to explicitly write to standard error.

### Text report

The text report displays statistics in a tabular format:

```none
$ recur -a 3 -c False -R text sh -c 'exit 99'
recur [00:00:00.0]: maximum 3 attempts reached

     Total attempts: 3
          Successes: 0
           Failures: 3

         Total time: 0.003
         Wait times: 0.000, 0.000, 0.000

      Condition met: false, false, false
      Command found: true, true, true
         Exit codes: 99, 99, 99
```

See the JSON section for an explanation of each value.

### JSON Report

The JSON report provides the same information in a machine-readable format:

```none
> ./recur -a 3 -c False -R json sh -c 'exit 99'
recur [00:00:00.0]: maximum 3 attempts reached
{"attempts":3,"command_found":[true,true,true],"condition_met":[false,false,false],"exit_codes":[99,99,99],"failures":3,"successes":0,"total_time":0.010311538,"wait_times":[0,0,0]}
```

When writing to a file (using `-o`/`--report-file` with a path other than `-`), the JSON data is formatted with indentation;
on standard error, it is a single line for easy filtering.

The JSON report contains:

- `attempts`: the number of times the command was run
- `command_found`: an array of booleans indicating whether the command was found for each attempt
- `condition_met`: an array of booleans indicating whether the success condition was met for each attempt
- `exit_codes`: an array of exit codes from each attempt
- `failures`: the number of attempts where the condition was not met
- `successes`: the number of attempts where the condition was met
- `total_time`: the elapsed time from the start of the first attempt to the end of the last attempt, in seconds
- `wait_times`: an array of delays _before_ each attempt, in seconds

## License

MIT.

## Alternatives

recur was inspired by [retry-cli](https://github.com/tirsen/retry-cli).
I wanted something like retry-cli but without the Node.js dependency.

Other similar tools include:

- [attempt](https://github.com/MaxBondABE/attempt).
  Written in Rust.
  `cargo install attempt-cli`.
- [eb](https://github.com/rye/eb).
  Written in Rust.
  `cargo install eb`.
- [retry (joshdk)](https://github.com/joshdk/retry).
  Written in Go.
  `go install github.com/joshdk/retry@master`.
- [retry (kadwanev)](https://github.com/kadwanev/retry).
  Written in Bash.
- [retry (minfrin)](https://github.com/minfrin/retry).
  Written in C.
  Packaged for Debian and Ubuntu.
  `sudo apt install retry`.
- [retry (timofurrer)](https://github.com/timofurrer/retry-cmd).
  Written in Rust.
  `cargo install retry-cmd`.
- [retry-cli](https://github.com/tirsen/retry-cli).
  Written in JavaScript for Node.js.
  `npx retry-cli`.
- [SysBox](https://github.com/skx/sysbox) includes the command `splay`.
  Written in Go.
  `go install github.com/skx/sysbox@latest`.
