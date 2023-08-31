# recur

This command-line tool runs a single command repeatedly until it succeeds or allowed attempts run out. It implements optional [exponential backoff](https://en.wikipedia.org/wiki/Exponential_backoff) with configurable [jitter](https://en.wikipedia.org/wiki/Thundering_herd_problem#Mitigation).

It was inspired by [retry-cli](https://github.com/tirsen/retry-cli). I wanted to have something like it, but as a single-file script without the Node.js dependency. The result depends only on Python and its standard library.

The CLI options are modeled after the parameters of the [`retry`](https://github.com/invl/retry) decorator, which Python programmers may know and like. However, I do not use the `retry` package or its code. The jitter behavior is different from `retry`. Jitter is applied starting with the first retry, not the second. I think this is what the user expects.

## Requirements

Python 3.8 or later.


## Installation

The recommended way to install recur is from [PyPI](https://pypi.org/project/recur-command/) with [pipx](https://github.com/pypa/pipx).

```shell
pipx install recur-command
# or
pip install --user recur-command
```

## Usage

```none
usage: recur [-h] [-V] [-b BACKOFF] [-d DELAY] [-j JITTER] [-m MAX] [-t TRIES]
             [-v]
             command ...

Retry a command with exponential backoff and jitter.

positional arguments:
  command               command to run
  arg                   arguments

options:
  -h, --help            show this help message and exit
  -V, --version         show program's version number and exit
  -b BACKOFF, --backoff BACKOFF
                        multiplier applied to delay on every attempt (default:
                        1, no backoff)
  -d DELAY, --delay DELAY
                        constant or initial exponential delay (seconds,
                        default: 0)
  -j JITTER, --jitter JITTER
                        additional random delay (maximum seconds or "min,max"
                        in seconds, default: "0,0")
  -m MAX, --max-delay MAX
                        maximum delay (seconds, default: 86400)
  -t TRIES, --tries TRIES
                        maximum number of attempts (negative for infinite,
                        default: 3)
  -v, --verbose         announce failures
```

## License

MIT.

## Alternatives

* [retry (joshdk)](https://github.com/joshdk/retry). Written in Go. `go install github.com/joshdk/retry@master`.
* [retry (kadwanev)](https://github.com/kadwanev/retry). Written in Bash.
* [retry (minfrin)](https://github.com/minfrin/retry). Written in C. Packaged in Debian and Ubuntu repositories. `sudo apt install retry`.
* [retry (timofurrer)](https://github.com/timofurrer/retry-cmd). Written in Rust. `cargo install retry-cmd`.
* [retry-cli](https://github.com/tirsen/retry-cli). Written in JavaScript for Node.js. `npx retry-cli`.
