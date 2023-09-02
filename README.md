# recur

**recur** is a command-line tool that runs a single command repeatedly until it succeeds or allowed attempts run out. It implements optional [exponential backoff](https://en.wikipedia.org/wiki/Exponential_backoff) with configurable [jitter](https://en.wikipedia.org/wiki/Thundering_herd_problem#Mitigation). It lets you define the success condition.


## Requirements

Python 3.8 or later, PyPI package `simpleeval` (installed automatically with `recur-command`).


## Installation

The recommended way to install recur is from [PyPI](https://pypi.org/project/recur-command/) with [pipx](https://github.com/pypa/pipx).

```shell
pipx install recur-command
# or
pip install --user recur-command
```


## Usage

```none
usage: recur [-h] [-V] [-b BACKOFF] [-c COND] [-d DELAY] [-j JITTER] [-m MAX]
             [-t TRIES] [-v]
             command ...

Retry a command with exponential backoff and jitter.

positional arguments:
  command               command to run
  args                  arguments

options:
  -h, --help            show this help message and exit
  -V, --version         show program's version number and exit
  -b BACKOFF, --backoff BACKOFF
                        multiplier applied to delay on every attempt (default:
                        1, no backoff)
  -c COND, --condition COND
                        success condition (simpleeval expression, default:
                        "code == 0")
  -d DELAY, --delay DELAY
                        constant or initial exponential delay (seconds,
                        default: 0)
  -j JITTER, --jitter JITTER
                        additional random delay (maximum seconds or "min,max"
                        seconds, default: "0,0")
  -m MAX, --max-delay MAX
                        maximum delay (seconds, default: 86400)
  -t TRIES, --tries TRIES
                        maximum number of attempts (negative for infinite,
                        default: 3)
  -v, --verbose         announce failures
```

recur exits with the last command's exit code, unless the user overrides this in the condition.

The CLI options are modeled after the parameters of the [`retry`](https://github.com/invl/retry) decorator, which Python programmers may recognize. However, recur does not use `retry`. The jitter (random delay) behavior is different. Jitter is applied starting with the first retry, not the second. I think this is what the user expects. A single-number jitter argument results in random jitter.     


## Conditions

recur is lightly scriptable. It allows you to specify the success condition using the simpleeval [expression language](https://github.com/danthedeckie/simpleeval#operators), which is a subset of Python. The default condition is `code == 0`. It means recur should stop retrying when the exit code of the command is zero.

You can use the following variables in the condition expression:

* `attempt`: `int` — the number of the current attempt, starting at one. If you want to use this variable to control when recur stops, pass it the option `--tries -1` to recur for otherwise infinite attempts. 
* `code`: `int` — the exit code of the last command.
* `time`: `float` — the time the last command took, in seconds.
* `total_time`: `float` — the total run time, again in seconds.
*  `max_tries`: `int` — the value of the option `--tries`.

recur defines one custom function:

* `exit(code: int) -> None` — exit with the exit code.

This function allows you to override the default behavior of returning the last command's exit code. For example, you can make recur exit with success when the command fails.

```shell
recur --condition 'code != 0 and exit(0)' sh -c 'exit 1'
# or
recur --condition 'exit(0) if code != 0 else False' sh -c 'exit 1'
```

In the following example we stop early and do not retry when the command's exit code indicates incorrect usage.

```shell
recur --condition 'code == 0 or (code in {1, 2, 3, 4} and exit(code))' curl "$url"
```

## License

MIT.


## Alternatives

recur was inspired by [retry-cli](https://github.com/tirsen/retry-cli). I wanted something like retry-cli, but without the Node.js dependency. There are other tools like this.

* [retry (joshdk)](https://github.com/joshdk/retry). Written in Go. `go install github.com/joshdk/retry@master`.
* [retry (kadwanev)](https://github.com/kadwanev/retry). Written in Bash.
* [retry (minfrin)](https://github.com/minfrin/retry). Written in C. Packaged in Debian and Ubuntu repositories. `sudo apt install retry`.
* [retry (timofurrer)](https://github.com/timofurrer/retry-cmd). Written in Rust. `cargo install retry-cmd`.
* [retry-cli](https://github.com/tirsen/retry-cli). Written in JavaScript for Node.js. `npx retry-cli`.
