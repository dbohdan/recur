# retry-cmd

This command-line tool runs a single command repeatedly until it succeeds or allowed attempts run out. It implements optional [exponential backoff](https://en.wikipedia.org/wiki/Exponential_backoff) with configurable [jitter](https://en.wikipedia.org/wiki/Thundering_herd_problem#Mitigation).

It was inspired by [retry-cli](https://github.com/tirsen/retry-cli). I wanted to have something like it, but as a single-file script without the Node.js dependency. The result depends only on Python with the standard library.

The CLI options are modeled after the parameters of the [`retry`](https://github.com/invl/retry) decorator, which Python programmers may know and like. However, I do not use the `retry` package or its code. The jitter behavior is different from `retry`. Jitter is applied starting with the first retry, not the second. I think this is what the user actually expects.

## Requirements

Python 3.8 or later.

## License

MIT.

## Alternatives

* [retry (joshdk)](https://github.com/joshdk/retry). Written in Go. `go install github.com/joshdk/retry@master`.
* [retry (kadwanev)](https://github.com/kadwanev/retry). Written in Bash.
* [retry (minfrin)](https://github.com/minfrin/retry). Written in C. Packaged in Debian and Ubuntu repositories. `sudo apt install retry`.
* [retry (timofurrer)](https://github.com/timofurrer/retry-cmd). Written in Rust. `cargo install retry-cmd`.
* [retry-cli](https://github.com/tirsen/retry-cli). Written in JavaScript for Node.js. `npx retry-cli`.
