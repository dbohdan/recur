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

import os
import re
import shlex
import subprocess
import sys
import unittest
from pathlib import Path

import pytest

PYTHON = sys.executable
TEST_PATH = Path(__file__).resolve().parent

COMMAND = shlex.split(os.environ.get("RECUR_COMMAND", ""))
if not COMMAND:
    COMMAND = [PYTHON, Path(TEST_PATH, "..", "recur.py")]


PYTHON_EXIT_99 = [PYTHON, "-c", "raise SystemExit(99)"]
PYTHON_HELLO = [PYTHON, "-c", "print('hello')"]


def run(
    *args: str,
    check: bool = True,
    return_stdout: bool = True,
    return_stderr: bool = False,
) -> str:
    completed = subprocess.run(
        COMMAND + list(args),
        check=check,
        stdin=None,
        capture_output=True,
    )

    output = ""
    if return_stdout:
        output += completed.stdout.decode("utf-8")
    if return_stderr:
        output += completed.stderr.decode("utf-8")

    return output


class TestRecur(unittest.TestCase):
    def test_usage(self) -> None:
        assert re.search("^usage", run(check=False, return_stderr=True))

    def test_version(self) -> None:
        assert re.search("\\d+\\.\\d+\\.\\d+", run("--version"))

    def test_echo(self) -> None:
        assert re.search("hello", run(*PYTHON_HELLO))

    def test_exit_code(self) -> None:
        with pytest.raises(subprocess.CalledProcessError) as e:
            run(*PYTHON_EXIT_99)
        assert e.value.returncode == 99

    def test_options(self) -> None:
        run(
            "-b",
            "1",
            "-d",
            "0",
            "--jitter",
            "0,0.1",
            "-m",
            "0",
            "-t",
            "0",
            *PYTHON_EXIT_99,
        )

    def test_verbose(self) -> None:
        output = run(
            "-v",
            "-t",
            "3",
            *PYTHON_EXIT_99,
            check=False,
            return_stdout=False,
            return_stderr=True,
        ).rstrip()

        assert len(re.findall("command exited with code", output)) == 3
        assert re.search("on attempt 3$", output)

    def test_verbose_no_command(self) -> None:
        output = run(
            "-v",
            "-t",
            "3",
            "no-such-command-should-exist",
            check=False,
            return_stdout=False,
            return_stderr=True,
        ).rstrip()

        assert len(re.findall("command was not found", output)) == 3

    def test_verbose_too_many(self) -> None:
        output = run(
            "-vvv",
            "",
            check=False,
            return_stdout=False,
            return_stderr=True,
        ).rstrip()

        assert re.search("error:.*?verbose flags", output)

    def test_stop_on_success(self) -> None:
        assert len(re.findall("hello", run(*PYTHON_HELLO))) == 1

    def test_condition_attempt(self) -> None:
        output = run("--condition", "attempt == 5", "--tries", "-1", *PYTHON_HELLO)
        assert len(re.findall("hello", output)) == 5

    def test_condition_code_and_exit(self) -> None:
        run("--condition", "exit(0) if code == 99 else 'fail'", *PYTHON_EXIT_99)

    def test_condition_time_and_total_time(self) -> None:
        output = run(
            "--condition",
            "total_time > time",
            PYTHON,
            "-c",
            "import time; time.sleep(0.1); print('T')",
        )
        assert len(re.findall("T", output)) == 2


if __name__ == "__main__":
    unittest.main()
