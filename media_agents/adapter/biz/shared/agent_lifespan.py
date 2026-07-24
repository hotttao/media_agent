"""Reusable agent startup lifecycle orchestration."""

from __future__ import annotations

import logging
from collections.abc import AsyncIterator, Sequence
from contextlib import asynccontextmanager
from typing import Protocol

from adapter.biz.shared.managed_process import ManagedProcessGroup, ManagedProcessSpec

logger = logging.getLogger(__name__)


class AgentLifecycleHooks(Protocol):
    async def load_config(self) -> bool: ...
    async def load_skills(self) -> tuple[bool, list[str]]: ...
    async def load_credentials(self) -> bool: ...
    async def restart_bridge(self) -> None: ...


@asynccontextmanager
async def agent_lifespan(
    app: object,
    *,
    hooks: AgentLifecycleHooks,
    processes: Sequence[ManagedProcessSpec] = (),
) -> AsyncIterator[None]:
    """Run common boot-time hooks and own agent auxiliary processes."""
    config_ok = await hooks.load_config()
    skills_ok, failed = await hooks.load_skills()
    if not skills_ok and failed:
        raise RuntimeError(f"agent skills failed to load: {', '.join(failed)}")
    credentials_ok = await hooks.load_credentials()
    if config_ok or credentials_ok:
        await hooks.restart_bridge()

    group = ManagedProcessGroup(processes)
    group.start()
    app.state.agent_processes = group
    logger.info(
        "agent lifecycle started: config=%s skills=%s credentials=%s processes=%d",
        config_ok,
        skills_ok,
        credentials_ok,
        len(processes),
    )
    try:
        yield
    finally:
        await group.stop()
        logger.info("agent lifecycle shutdown")

