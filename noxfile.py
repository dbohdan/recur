from __future__ import annotations

from nox import Session, session

PYTHON_VERSIONS = ["3.8", "3.9", "3.10", "3.11", "3.12"]


@session(python=PYTHON_VERSIONS)
def tests(session: Session) -> None:
    session.install(".", "pytest>=7,<8")
    session.run("pytest")
