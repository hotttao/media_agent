"""Async managed subprocess helpers shared by agent packages."""

from __future__ import annotations

import asyncio
import logging
from collections.abc import Mapping, Sequence
from dataclasses import dataclass, field
from pathlib import Path

logger = logging.getLogger(__name__)


@dataclass(frozen=True)
class ManagedProcessSpec:
    label: str
    program: str
    args: Sequence[str] = field(default_factory=tuple)
    cwd: str | Path | None = None
    env: Mapping[str, str] | None = None
    restart_delay_s: float = 2.0


class ManagedProcess:
    """Run one child process and restart it until stopped."""

    def __init__(self, spec: ManagedProcessSpec) -> None:
        self.spec = spec
        self._task: asyncio.Task[None] | None = None
        self._proc: asyncio.subprocess.Process | None = None
        self._stopping = False

    def start(self) -> None:
        if self._task is not None and not self._task.done():
            return
        self._stopping = False
        self._task = asyncio.create_task(self._run_loop())

    async def stop(self) -> None:
        self._stopping = True
        if self._proc is not None and self._proc.returncode is None:
            self._proc.terminate()
            try:
                await asyncio.wait_for(self._proc.wait(), timeout=5)
            except TimeoutError:
                self._proc.kill()
                await self._proc.wait()
        if self._task is not None:
            self._task.cancel()
            try:
                await self._task
            except asyncio.CancelledError:
                pass
            self._task = None

    async def _run_loop(self) -> None:
        while not self._stopping:
            try:
                self._proc = await asyncio.create_subprocess_exec(
                    self.spec.program,
                    *self.spec.args,
                    cwd=str(self.spec.cwd) if self.spec.cwd is not None else None,
                    env=dict(self.spec.env) if self.spec.env is not None else None,
                )
                logger.info(
                    "managed process started: label=%s pid=%s",
                    self.spec.label,
                    self._proc.pid,
                )
                code = await self._proc.wait()
                if self._stopping:
                    return
                logger.warning(
                    "managed process exited: label=%s code=%s; restarting in %.1fs",
                    self.spec.label,
                    code,
                    self.spec.restart_delay_s,
                )
            except asyncio.CancelledError:
                raise
            except Exception as err:
                logger.error(
                    "managed process failed: label=%s error=%s; restarting in %.1fs",
                    self.spec.label,
                    err,
                    self.spec.restart_delay_s,
                )
            await asyncio.sleep(self.spec.restart_delay_s)


class ManagedProcessGroup:
    """Own a group of managed processes for an agent lifespan."""

    def __init__(self, specs: Sequence[ManagedProcessSpec]) -> None:
        self._processes = [ManagedProcess(spec) for spec in specs]

    def start(self) -> None:
        for process in self._processes:
            process.start()

    async def stop(self) -> None:
        await asyncio.gather(
            *(process.stop() for process in self._processes),
            return_exceptions=True,
        )
