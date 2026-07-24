from __future__ import annotations

import json
import os
from pathlib import Path
from typing import Any

import pytest

from codex.config import CodexHooks


class _CpClient:
    def is_configured(self) -> bool:
        return True

    def mcp_url(self) -> str:
        return "http://cp.test/mcp"

    def mcp_headers(self, session_token: str | None = None) -> dict[str, str]:
        headers = {"X-Workspace-ID": "w1", "X-Agent-ID": "w1"}
        if session_token:
            headers["X-Session-Token"] = session_token
        return headers

    async def fetch_workspace_config(self) -> dict[str, Any]:
        return {
            "model": "gpt-test",
            "provider_type": "openai",
            "base_url": "http://llm.test/v1",
            "small_model": "small",
            "api_key": "sk-test",
            "prompt_content": "system prompt",
            "mcp_config": json.dumps(
                {
                    "mcpServers": {
                        "tos-platform": {"headers": {"X-Builder": "b"}},
                        "remote": {"url": "http://remote", "headers": {"A": "B"}},
                        "local": {"command": "tool", "args": ["--x"]},
                    }
                }
            ),
        }

    async def fetch_credentials(self) -> list[dict[str, Any]]:
        return []


@pytest.mark.asyncio
async def test_codex_hook_writes_shared_workspace_files_and_mcp(tmp_path: Path) -> None:
    workspace = tmp_path / "workspace"
    codex_dir = tmp_path / ".codex"
    hooks = CodexHooks(
        cp_client=_CpClient(),  # type: ignore[arg-type]
        workspace_dir=workspace,
        codex_config_dir=codex_dir,
    )

    assert await hooks.load_config() is True

    assert (workspace / "AGENTS.md").read_text(encoding="utf-8") == "system prompt"
    assert json.loads((workspace / "runtime.json").read_text(encoding="utf-8")) == {
        "model": "gpt-test",
        "provider_type": "openai",
        "base_url": "http://llm.test/v1",
        "small_model": "small",
    }
    assert json.loads((codex_dir / "mcp.json").read_text(encoding="utf-8"))["mcpServers"][
        "remote"
    ] == {"url": "http://remote", "headers": {"A": "B"}}
    assert json.loads((codex_dir / "auth.json").read_text(encoding="utf-8")) == {
        "OPENAI_API_KEY": "sk-test"
    }

    servers = hooks.load_mcp_servers("token")
    assert servers[0]["headers"] == {
        "X-Builder": "b",
        "X-Workspace-ID": "w1",
        "X-Agent-ID": "w1",
        "X-Session-Token": "token",
    }
    assert {server["name"] for server in servers} == {"tos-platform", "remote"}

    os.environ["ANTHROPIC_API_KEY"] = "old"
    os.environ["OPENAI_API_KEY"] = "old"
    await hooks.restart_bridge()
    assert "ANTHROPIC_API_KEY" not in os.environ
    assert os.environ["OPENAI_API_KEY"] == "sk-test"
    assert os.environ["OPENAI_BASE_URL"] == "http://llm.test/v1"
