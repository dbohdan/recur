# recur
# Retry a command with exponential backoff and jitter.
# License: MIT.

from __future__ import annotations

import argparse
import itertools
import logging
import random
import subprocess as sp
import sys
import time
import traceback
from dataclasses import dataclass
from pathlib import Path
from typing import Callable, Literal, Union

from simpleeval import EvalWithCompoundTypes

__version__ = "0.4.1"

COMMAND_NOT_FOUND_EXIT_CODE = 255
MAX_ALLOWED_DELAY = 366 * 24 * 60 * 60
MAX_VERBOSE_LEVEL = 2

VERBOSE_LEVEL_INFO = 1
VERBOSE_LEVEL_DEBUG = 2


@dataclass(frozen=True)
class Attempt:
    attempt: int
    code: int | None
    command_found: bool
    time: float
    total_time: float
    max_tries: int


ConditionFunc = Callable[[Attempt], bool]


@dataclass(frozen=True)
class Code:
    code: int


@dataclass(frozen=True)
class CommandNotFound:
    pass


CommandResult = Union[Code, CommandNotFound]


@dataclass(frozen=True)
class Interval:
    start: float
    end: float


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
            f" {record.levelname.lower()}"
            if record.levelno <= logging.DEBUG or record.levelno >= logging.WARNING
            else ""
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


def configure_logging(*, start_time: float, verbose: int):
    handler = logging.StreamHandler()
    formatter = RelativeTimeLevelSuffixFormatter(
        fmt="recur [{asctime}]{levelsuffix}: {message}",
        reftime=start_time,
        style="{",
    )
    handler.setFormatter(formatter)

    root = logging.getLogger()
    root.addHandler(handler)

    if verbose >= VERBOSE_LEVEL_DEBUG:
        level = logging.DEBUG
    elif verbose == VERBOSE_LEVEL_INFO:
        level = logging.INFO
    else:
        level = logging.WARNING
    root.setLevel(level)


def retry_command(
    args: list[str],
    *,
    backoff: float,
    fixed_delay: Interval,
    max_tries: int,
    random_delay: Interval,
    success: ConditionFunc,
    start_time: float | None = None,
) -> CommandResult:
    result: CommandResult = Code(0)

    # Count attempts from one.
    iterator = range(1, max_tries + 1) if max_tries >= 0 else itertools.count(1)
    for attempt_number in iterator:
        if attempt_number > 1:
            curr_fixed = min(
                fixed_delay.end,
                fixed_delay.start * backoff**attempt_number,
            )
            curr_random = random.uniform(random_delay.start, random_delay.end)
            time.sleep(curr_fixed + curr_random)

        attempt_start = time.time()
        if start_time is None:
            start_time = attempt_start

        try:
            completed = sp.run(args, check=False)
            result = Code(completed.returncode)
            logging.info(
                "command exited with code %d on attempt %d",
                result.code,
                attempt_number,
            )
        except FileNotFoundError:
            result = CommandNotFound()
            logging.info("command was not found on attempt %d", attempt_number)

        attempt_end = time.time()

        attempt = Attempt(
            attempt=attempt_number,
            code=None if isinstance(result, CommandNotFound) else result.code,
            command_found=not isinstance(result, CommandNotFound),
            time=attempt_end - attempt_start,
            total_time=attempt_end - start_time,
            max_tries=max_tries,
        )

        if success(attempt):
            return result

    return result


def main() -> None:
    argv0 = Path(sys.argv[0])

    parser = argparse.ArgumentParser(
        description="Retry a command with exponential backoff and jitter.",
        prog=(
            f"{Path(sys.executable).name} -m {argv0.parent.name}"
            if argv0.name == "__main__.py"
            else argv0.name
        ),
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
        version=__version__,
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
        "-c",
        "--condition",
        default="code == 0",
        help=('success condition (simpleeval expression, default: "%(default)s")'),
        metavar="COND",
        type=str,
    )

    def delay(arg: str) -> float:
        value = float(arg)

        if value < 0 or value > MAX_ALLOWED_DELAY:
            msg = f"delay must be between zero and {MAX_ALLOWED_DELAY}"
            raise ValueError(msg)

        return value

    parser.add_argument(
        "-d",
        "--delay",
        default=0,
        help=("constant or initial exponential delay (seconds, default: %(default)s)"),
        type=delay,
    )

    def jitter(arg: str) -> Interval:
        commas = arg.count(",")
        if commas == 0:
            head, tail = "0", arg
        elif commas == 1:
            head, tail = arg.split(",", 1)
        else:
            msg = "jitter range must contain no more than one comma"
            raise ValueError(msg)

        return Interval(delay(head), delay(tail))

    parser.add_argument(
        "-j",
        "--jitter",
        default="0,0",
        help=(
            "additional random delay "
            '(maximum seconds or "min,max" seconds, default: "%(default)s")'
        ),
        type=jitter,
    )

    parser.add_argument(
        "-m",
        "--max-delay",
        default=60 * 60,
        help="maximum delay (seconds, default: %(default)s)",
        metavar="MAX",
        type=delay,
    )

    parser.add_argument(
        "-t",
        "--tries",
        type=int,
        default=5,
        help="maximum number of attempts (negative for infinite, default: %(default)s)",
    )

    parser.add_argument(
        "-v",
        "--verbose",
        action="count",
        default=0,
        help=(
            "announce exit code and attempt number; "
            "adds debug information for errors if used twice"
        ),
    )

    args = parser.parse_args()
    if args.verbose > MAX_VERBOSE_LEVEL:
        parser.error(f"up to {MAX_VERBOSE_LEVEL} verbose flags is allowed")

    configure_logging(start_time=time.time(), verbose=args.verbose)

    def exit_from_cond(code: int | None) -> None:
        if code is None:
            code = COMMAND_NOT_FOUND_EXIT_CODE

        sys.exit(code)

    def success(attempt: Attempt) -> bool:
        evaluator = EvalWithCompoundTypes(
            functions={"exit": exit_from_cond},
            names=vars(attempt),
        )
        result = evaluator.eval(args.condition)

        if not isinstance(result, bool):
            msg = (
                "success condition must return a boolean; "
                f"got type {type(result).__name__!r}"
            )
            raise TypeError(msg)

        return result

    try:
        result = retry_command(
            [args.command, *args.args],
            backoff=args.backoff,
            fixed_delay=Interval(args.delay, args.max_delay),
            max_tries=args.tries,
            random_delay=args.jitter,
            success=success,
        )

        sys.exit(
            COMMAND_NOT_FOUND_EXIT_CODE
            if isinstance(result, CommandNotFound)
            else result.code,
        )
    except KeyboardInterrupt:
        pass
    except Exception as e:  # noqa: BLE001
        tb = sys.exc_info()[-1]
        frame = traceback.extract_tb(tb)[-1]
        file_info = (
            f"file {Path(frame.filename).name!r}, "
            if "__file__" in globals() and frame.filename != __file__
            else ""
        )
        line = frame.lineno

        logging.error("%s", e)
        logging.debug(
            "%sline %d, exception %r",
            file_info,
            line,
            type(e).__name__,
        )
        sys.exit(1)


if __name__ == "__main__":
    main()
