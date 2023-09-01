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
import logging
import random
import subprocess as sp
import sys
import time
import traceback
from typing import Literal

MAX_DELAY = 366 * 24 * 60 * 60
VERSION = "0.1.0"


class RelativeTimeLevelSuffixFormatter(logging.Formatter):
    def __init__(
        self,
        fmt: str | None = None,
        style: Literal["%", "{", "$"] = "%",
        validate: bool = True,  # noqa: FBT001, FBT002
        *,
        reftime: float,
    ):
        super().__init__(
            fmt=fmt,
            style=style,
            validate=validate,
        )
        self._reftime = reftime

    def format(self, record: logging.LogRecord):  # noqa: A003
        record.levelsuffix = (
            f" {record.levelname.lower()}" if record.levelno >= logging.WARNING else ""
        )
        return super().format(record)

    def formatTime(self, record, datefmt=None):  # noqa: ARG002, N802
        delta_f = record.created - self._reftime
        d = int(delta_f)
        frac = delta_f - d

        d, seconds = divmod(d, 60)
        d, minutes = divmod(d, 60)
        hours = d

        return f"{hours:02d}:{minutes:02d}:{seconds:02d}.{10 * frac:01.0f}"


def configure_logging(*, verbose: bool):
    handler = logging.StreamHandler()
    formatter = RelativeTimeLevelSuffixFormatter(
        fmt="recur [{asctime}]{levelsuffix}: {message}",
        reftime=time.time(),
        style="{",
    )
    handler.setFormatter(formatter)

    root = logging.getLogger()
    root.setLevel(level=logging.INFO if verbose else logging.WARN)
    root.addHandler(handler)


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
    iterator = range(tries) if tries >= 0 else itertools.count()
    for i in iterator:
        try:
            sp.run(args, check=True)
        except sp.CalledProcessError as e:
            logging.info("command exited with code %u", e.returncode)

            if i == tries - 1:
                raise

            fixed_delay = min(max_fixed_delay, min_fixed_delay * backoff**i)
            random_delay = random.uniform(min_random_delay, max_random_delay)
            time.sleep(fixed_delay + random_delay)
        else:
            return


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
        nargs=argparse.REMAINDER,
        type=str,
    )

    parser.add_argument(
        "-V",
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

    def delay(arg: str) -> float:
        value = float(arg)

        if value < 0 or value > MAX_DELAY:
            msg = f"delay must be between zero and {MAX_DELAY}"
            raise ValueError(msg)

        return value

    parser.add_argument(
        "-d",
        "--delay",
        default=0,
        help=("constant or initial exponential delay (seconds, default: %(default)s)"),
        type=delay,
    )

    def jitter(arg: str) -> tuple[float, float]:
        commas = arg.count(",")
        if commas == 0:
            head, tail = "0", arg
        elif commas == 1:
            head, tail = arg.split(",", 1)
        else:
            msg = "jitter range must contain no more than one comma"
            raise ValueError(msg)

        return (delay(head), delay(tail))

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
        type=delay,
    )

    parser.add_argument(
        "-t",
        "--tries",
        type=int,
        default=3,
        help="maximum number of attempts (negative for infinite, default: %(default)s)",
    )

    parser.add_argument(
        "-v",
        "--verbose",
        action="store_true",
        help="announce failures",
    )

    args = parser.parse_args()
    configure_logging(verbose=args.verbose)

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
    except KeyboardInterrupt:
        pass
    except sp.CalledProcessError as e:
        sys.exit(e.returncode)
    except Exception as e:  # noqa: BLE001
        tb = sys.exc_info()[-1]
        frame = traceback.extract_tb(tb)[-1]
        line = frame.lineno

        logging.error(
            "%s (debug information: line %u, exception %s)",
            e,
            line,
            type(e).__name__,
        )
        sys.exit(1)


if __name__ == "__main__":
    main()
