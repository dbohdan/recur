from __future__ import annotations

from nox import Session, session

PYTHON_VERSIONS = ["3.9", "3.10", "3.11", "3.12", "3.13", "pypy3.10"]


@session(python=PYTHON_VERSIONS)
def tests(session: Session) -> None:
    session.install(".", "pytest>=8,<9")
    session.run("pytest")
