from __future__ import annotations

from types import SimpleNamespace

import pytest

from adapter.biz.shared.agent_lifespan import agent_lifespan


class _Hooks:
    def __init__(self) -> None:
        self.calls: list[str] = []

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


@pytest.mark.asyncio
async def test_agent_lifespan_runs_boot_hooks_in_order() -> None:
    hooks = _Hooks()
    app = SimpleNamespace(state=SimpleNamespace())

    async with agent_lifespan(app, hooks=hooks):
        assert hooks.calls == ["config", "skills", "credentials", "restart"]
        assert app.state.agent_processes is not None
