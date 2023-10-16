from __future__ import annotations

import nox

PYTHON_VERSIONS = ["3.8", "3.9", "3.10", "3.11", "3.12"]


@nox.session(python=PYTHON_VERSIONS, tags=["test"])
def tests(session: nox.Session) -> None:
    session.install("pytest == 7.*", "simpleeval == 0.9.*")
    session.run("pytest")
