# recur

**recur** is a command-line tool that runs a single command repeatedly until it succeeds or no more attempts are left.
It implements optional [exponential backoff](https://en.wikipedia.org/wiki/Exponential_backoff) with configurable [jitter](https://en.wikipedia.org/wiki/Thundering_herd_problem#Mitigation).
It lets you write the success condition in Starlark.

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

### Go

Install Go, then run the following command:

```shell
go install dbohdan.com/recur/v2@latest
```

## Build requirements

- Go 1.19
- [Task](https://taskfile.dev/) (go-task) 3.28

## Usage

```none
{{ .Help | wrap 80 -}}
```

The "duration" arguments take [Go duration strings](https://pkg.go.dev/time#ParseDuration);
for example, `0`, `100ms`, `2.5s`, `0.5m`, or `1h`.
The value of `-j`/`--jitter` must be either one duration string or two joined with a comma, like `1s,2s`.

Setting the delay (`-d`/`--delay`) increases the maximum delay (`-m`/`--max-delay`) to that value when the maximum delay is shorter.
Use `-m`/`--max-delay` after `-d`/`--delay` if you want a shorter maximum delay.

The following recur options run the command `foo --config bar.cfg` indefinitely.
Every time `foo` exits, there is a delay that grows exponentially from two seconds to a minute.
The delay resets back to two seconds if the command runs for at least five minutes.

```shell
recur --backoff 2s --condition False --forever --max-delay 1m --reset 5m foo --config bar.cfg
```

recur exits with the last command's exit code unless the user overrides this in the condition.
When the command is not found during the last attempt,
recur exits with the code 255.

recur sets the environment variable `RECUR_ATTEMPT` for the command it runs to the current attempt number.
This way the command can access the attempt counter.
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
It means recur will stop retrying when the exit code of the command is zero.

If you know Python, you can quickly start writing recur conditions in Starlark.
The most significant differences between Starlark and Python for this purpose are:

- Starlark has no `is`.
  You must write `code == None`, not `code is None`.
- Starlark has no sets.
  Write `code in (1, 2, 3)` or `code in [1, 2, 3]` rather than `code in {1, 2, 3}`.

You can use the following variables in the condition expression:

- `attempt`: `int` — the number of the current attempt, starting at one.
  Combine with `--forever` to use the condition instead of the built-in attempt counter.
- `attempt_since_reset`: `int` — the attempt number since exponential and Fibonacci backoff were reset, starting at one.
- `code`: `int | None` — the exit code of the last command.
  `code` is `None` when the command was not found.
- `command_found`: `bool` — whether the last command was found.
- `max_attempts`: `int` — the value of the option `--attempts`.
  `--forever` sets it to -1.
- `time`: `float` — the time the most recent attempt took, in seconds.
- `total_time`: `float` — the time between the start of the first attempt and the end of the most recent, again in seconds.

recur defines two custom functions:

- `exit(code: int | None) -> None` — exit with the exit code.
  If `code` is `None`, exit with the exit code for a missing command (255).
- `inspect(value: Any, *, prefix: str = "") -> Any` — log `value` prefixed by `prefix` and return `value`.
  This is useful for debugging.

The `exit` function allows you to override the default behavior of returning the last command's exit code.
For example, you can make recur exit with success when the command fails.

```shell
recur --condition 'code != 0 and exit(0)' sh -c 'exit 1'
# or
recur --condition 'False if code == 0 else exit(0)' sh -c 'exit 1'
```

In the following example we stop early and do not retry when the command's exit code indicates incorrect usage or a problem with the installation.

```shell
recur --condition 'code == 0 or (code in (1, 2, 3, 4) and exit(code))' curl "$url"
```

## License

MIT.

## Alternatives

recur was inspired by [retry-cli](https://github.com/tirsen/retry-cli).
I wanted something like retry-cli but without the Node.js dependency.

There are other similar tools:

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
