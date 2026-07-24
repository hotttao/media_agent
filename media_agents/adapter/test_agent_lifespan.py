from __future__ import annotations

from types import SimpleNamespace
from typing import Any

import pytest

from adapter.agent_lifespan import agent_lifespan


class _Hooks:
    def __init__(self) -> None:
        self.calls: list[str] = []

    def load_mcp_servers(self, session_token: str | None) -> list[Any]:
        return []

    def has_mcp_servers(self) -> bool:
        return False

    async def load_config(self) -> bool:
        self.calls.append("config")
        return True

    async def load_skills(self) -> tuple[bool, list[str]]:
        self.calls.append("skills")
        return True, []

    async def load_credentials(self) -> bool:
        self.calls.append("credentials")
        return True

    async def restart_bridge(self) -> None:
        self.calls.append("restart")

    def record_usage(self, session_id: str, usage: Any) -> None:
        return None


@pytest.mark.asyncio
async def test_agent_lifespan_runs_boot_hooks_in_order() -> None:
    hooks = _Hooks()
    app = SimpleNamespace(state=SimpleNamespace())

    async with agent_lifespan(app, hooks=hooks):
        assert hooks.calls == ["config", "skills", "credentials", "restart"]
        assert app.state.agent_processes is not None
