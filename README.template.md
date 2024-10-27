# recur

**recur** is a command-line tool that runs a single command repeatedly until it succeeds or no more attempts are left.
It implements optional [exponential backoff](https://en.wikipedia.org/wiki/Exponential_backoff) with configurable [jitter](https://en.wikipedia.org/wiki/Thundering_herd_problem#Mitigation).
It lets you write the success condition in Starlark.

## Requirements

- Go 1.19
- POSIX Make for testing

## Installation

Install Go, then run the following command:

```shell
go install github.com/dbohdan/recur@latest
```

## Usage

```none
{{ .Help }}```

The "duration" arguments take [Go duration strings](https://pkg.go.dev/time#ParseDuration);
for example, `0`, `100ms`, `2.5s`, `0.5m`, or `1h`.
The `-j`/`--jitter` argument must be either one duration string or two joined with a comma, like `1s,2s`.

recur exits with the last command's exit code unless the user overrides this in the condition.
When the command is not found during the last attempt,
recur exits with the code 255.

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
- `code`: `int | None` — the exit code of the last command.
  `code` is `None` when the command was not found.
- `command_found`: `bool` — whether the last command was found.
- `max_attempts`: `int` — the value of the option `--attempts`.
  `--forever` sets it to -1.
- `time`: `float` — the time the most recent attempt took, in seconds.
- `total_time`: `float` — the time between the start of the first attempt and the end of the most recent, again in seconds.

recur defines one custom function:

- `exit(code: int | None) -> None` — exit with the exit code.
  If `code` is `None`, exit with the exit code for a missing command (255).

This function allows you to override the default behavior of returning the last command's exit code.
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
