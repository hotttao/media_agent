from __future__ import annotations

import json
from pathlib import Path
from typing import Any

import pytest

from goose.config import GooseHooks


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
            "system_prompt": "goose prompt",
            "mcp_config": json.dumps(
                {
                    "mcpServers": {
                        "remote": {"url": "http://remote"},
                        "local": {"command": "tool", "args": ["--x"], "env": {"E": "V"}},
                    }
                }
            ),
            "agent_settings": "GOOSE_MODE: approve\ncustom: true",
        }

    async def fetch_credentials(self) -> list[dict[str, Any]]:
        return []


@pytest.mark.asyncio
async def test_goose_hook_writes_shared_files_and_session_mcp(tmp_path: Path) -> None:
    workspace = tmp_path / "workspace"
    goose_dir = tmp_path / ".config" / "goose"
    hooks = GooseHooks(
        cp_client=_CpClient(),  # type: ignore[arg-type]
        workspace_dir=workspace,
        goose_config_dir=goose_dir,
    )

    assert await hooks.load_config() is True

    assert (workspace / "AGENTS.md").read_text(encoding="utf-8") == "goose prompt"
    assert json.loads((workspace / "runtime.json").read_text(encoding="utf-8"))["model"] == (
        "gpt-test"
    )
    assert json.loads((goose_dir / "mcp.json").read_text(encoding="utf-8"))["mcpServers"][
        "local"
    ] == {"command": "tool", "args": ["--x"], "env": {"E": "V"}}

    goose_yaml = (goose_dir / "config.yaml").read_text(encoding="utf-8")
    assert "GOOSE_MODEL" in goose_yaml
    assert "GOOSE_MODE: approve" not in goose_yaml
    assert "custom: true" in goose_yaml

    servers = hooks.load_mcp_servers("token")
    assert {server["name"] for server in servers} == {"tos-platform", "remote", "local"}
    assert {server["name"]: server for server in servers}["local"] == {
        "name": "local",
        "command": "tool",
        "args": ["--x"],
        "env": {"E": "V"},
    }