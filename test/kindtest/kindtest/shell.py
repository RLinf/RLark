"""Command execution with logging and dry-run support.

Every external effect in a test goes through :class:`Shell`, so a whole run can
be inspected with ``--dry-run`` and every command is captured in a per-run log.
"""

from __future__ import annotations

import os
import shlex
import subprocess
import sys
from dataclasses import dataclass
from typing import Callable, Mapping, Sequence


@dataclass
class Ran:
    argv: list[str]
    rc: int
    stdout: str = ""
    stderr: str = ""
    seconds: float = 0.0

    @property
    def ok(self) -> bool:
        return self.rc == 0

    @property
    def out(self) -> str:
        return self.stdout.strip()

    @property
    def err(self) -> str:
        return self.stderr.strip()

    def combined(self) -> str:
        return (self.stdout + self.stderr).strip()


class Shell:
    def __init__(
        self,
        *,
        dry_run: bool = False,
        verbose: bool = True,
        log: Callable[[str], None] | None = None,
        cwd: str | None = None,
        env: Mapping[str, str] | None = None,
    ) -> None:
        self.dry_run = dry_run
        self.verbose = verbose
        self.cwd = cwd
        self.env = dict(env or {})
        self._log = log
        self.history: list[Ran] = []

    def _emit(self, message: str) -> None:
        if self._log is not None:
            self._log(message)
        elif self.verbose:
            print(message, file=sys.stderr, flush=True)

    def run(
        self,
        argv: Sequence[object],
        *,
        cwd: str | None = None,
        env: Mapping[str, str] | None = None,
        timeout: float | None = None,
        input: str | None = None,  # noqa: A002 - mirrors subprocess
        check: bool = False,
        quiet: bool = False,
    ) -> Ran:
        import time

        argv = [str(a) for a in argv]
        cmd = shlex.join(argv)
        if self.dry_run:
            self._emit(f"[dry-run] {cmd}")
            return Ran(argv, 0)
        if not quiet:
            self._emit(f"$ {cmd}")
        full_env = dict(os.environ)
        full_env.update(self.env)
        if env:
            full_env.update(env)
        workdir = cwd or self.cwd
        started = time.time()
        try:
            proc = subprocess.run(
                argv,
                cwd=workdir,
                env=full_env,
                input=input,
                capture_output=True,
                text=True,
                timeout=timeout,
            )
        except FileNotFoundError as exc:
            ran = Ran(argv, 127, "", str(exc), time.time() - started)
            self.history.append(ran)
            if check:
                raise RuntimeError(f"command not found: {cmd}") from exc
            return ran
        except subprocess.TimeoutExpired as exc:
            stdout = _text(exc.stdout)
            stderr = _text(exc.stderr) or f"timeout after {timeout}s"
            ran = Ran(argv, 124, stdout, stderr, time.time() - started)
            self.history.append(ran)
            if check:
                raise RuntimeError(f"timeout after {timeout}s: {cmd}")
            return ran
        ran = Ran(argv, proc.returncode, proc.stdout or "", proc.stderr or "", time.time() - started)
        self.history.append(ran)
        if check and ran.rc != 0:
            raise RuntimeError(f"command failed rc={ran.rc}: {cmd}\n{ran.err}")
        return ran


def _text(value: object) -> str:
    if value is None:
        return ""
    if isinstance(value, bytes):
        return value.decode(errors="replace")
    return str(value)
