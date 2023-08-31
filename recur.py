#! /usr/bin/env python3
# recur
# Retry a command with exponential backoff and jitter.
# License: MIT.
#
# Copyright (c) 2023 D. Bohdan
#
# Permission is hereby granted, free of charge, to any person obtaining a copy
# of this software and associated documentation files (the "Software"), to deal
# in the Software without restriction, including without limitation the rights
# to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
# copies of the Software, and to permit persons to whom the Software is
# furnished to do so, subject to the following conditions:
#
# The above copyright notice and this permission notice shall be included in
# all copies or substantial portions of the Software.
#
# THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
# IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
# FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
# AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
# LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
# OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
# THE SOFTWARE.

from __future__ import annotations

import argparse
import itertools
import random
import subprocess as sp
import sys
import time

MAX_DELAY = 366 * 24 * 60 * 60
VERSION = "0.1.0"


def retry_command(
    args: list[str],
    *,
    backoff: float,
    min_fixed_delay: float,
    max_fixed_delay: float,
    min_random_delay: float,
    max_random_delay: float,
    tries: int,
) -> None:
    # Prevent `OverflowError` in `time.sleep`.
    for value in (
        min_fixed_delay,
        max_fixed_delay,
        min_random_delay,
        max_random_delay,
    ):
        if value < 0 or value > MAX_DELAY:
            msg = f"delay must be between zero and {MAX_DELAY}"
            raise ValueError(msg)

    iterator = range(tries) if tries >= 0 else itertools.count()
    for i in iterator:
        try:
            sp.run(args, check=True)
        except sp.CalledProcessError:
            if i == tries - 1:
                raise

            fixed_delay = min(max_fixed_delay, min_fixed_delay * backoff**i)
            random_delay = random.uniform(min_random_delay, max_random_delay)
            time.sleep(fixed_delay + random_delay)


def main() -> None:
    parser = argparse.ArgumentParser(
        description="Retry a command with exponential backoff and jitter.",
    )

    parser.add_argument(
        "command",
        help="command to run",
        type=str,
    )

    parser.add_argument(
        "args",
        help="arguments",
        metavar="arg",
        nargs=argparse.REMAINDER,
        type=str,
    )

    parser.add_argument(
        "-v",
        "--version",
        action="version",
        version=VERSION,
    )
    parser.add_argument(
        "-b",
        "--backoff",
        default=1,
        help=(
            "multiplier applied to delay on every attempt "
            "(default: %(default)s, no backoff)"
        ),
        type=float,
    )

    parser.add_argument(
        "-d",
        "--delay",
        default=0,
        help=("constant or initial exponential delay (seconds, default: %(default)s)"),
        type=float,
    )

    def jitter(arg: str) -> tuple[float, float]:
        commas = arg.count(",")
        if commas == 0:
            return (0, float(arg))

        if commas == 1:
            head, tail = arg.split(",", 1)
            return (float(head), float(tail))

        msg = "jitter range must contain no more than one comma"
        raise ValueError(msg)

    parser.add_argument(
        "-j",
        "--jitter",
        default="0,0",
        help=(
            "additional random delay "
            '(maximum seconds or "min,max" in seconds, default: "%(default)s")'
        ),
        type=jitter,
    )

    parser.add_argument(
        "-m",
        "--max-delay",
        default=24 * 60 * 60,
        help="maximum delay (seconds, default: %(default)s)",
        metavar="MAX",
        type=float,
    )

    parser.add_argument(
        "-t",
        "--tries",
        type=int,
        default=3,
        help="maximum number of attempts (negative for infinite, default: %(default)s)",
    )

    args = parser.parse_args()

    try:
        retry_command(
            [args.command, *args.args],
            backoff=args.backoff,
            min_fixed_delay=args.delay,
            max_fixed_delay=args.max_delay,
            min_random_delay=args.jitter[0],
            max_random_delay=args.jitter[1],
            tries=args.tries,
        )
    except sp.CalledProcessError as e:
        sys.exit(e.returncode)
    except KeyboardInterrupt as _:
        pass


if __name__ == "__main__":
    main()
