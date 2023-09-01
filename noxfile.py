from __future__ import annotations

from itertools import chain
from pathlib import Path

import nox

PYTHON_VERSIONS = ["3.8", "3.9", "3.10", "3.11"]


def source_files() -> list[str]:
    cwd = Path()
    return [str(path) for path in chain(cwd.glob("*.py"), cwd.glob("tests/*.py"))]


@nox.session(python=PYTHON_VERSIONS, tags=["test"])
def tests(session: nox.Session) -> None:
    session.install("pytest")
    session.run("pytest")


@nox.session(name="format", python=False, tags=["check"])
def format_(session: nox.Session) -> None:
    session.run("poetry", "run", "black", *source_files())


@nox.session(python=False, tags=["check"])
def lint(session: nox.Session) -> None:
    extra = ["--fix"] if "fix" in session.posargs else []
    session.run(
        "poetry",
        "run",
        "ruff",
        "check",
        "--target-version",
        "py38",
        *extra,
        *source_files(),
    )


@nox.session(name="type", python=False, tags=["check"])
def type_(session: nox.Session) -> None:
    files = source_files()
    session.run("poetry", "run", "pyright", "--pythonversion", "3.8", *files)
