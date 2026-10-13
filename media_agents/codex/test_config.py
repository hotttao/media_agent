from __future__ import annotations

import json
import os
from pathlib import Path
from typing import Any

import pytest

from codex.config import CodexConfig, CodexHooks, load_codex_config

ROOT = Path(__file__).resolve().parents[1]


def test_load_codex_config_merges_common_defaults() -> None:
    cfg = load_codex_config(ROOT / "codex" / "conf" / "config.yaml")

    assert isinstance(cfg, CodexConfig)
    assert cfg.agent_type == "codex"
    assert cfg.agent.port == 3001
    assert cfg.workspace_dir == "/workspace"
    assert cfg.bridge_ttl.max_count == 10
    assert cfg.codex.program == "codex-acp"


class _CpClient:
    @property
    def workspace_id(self) -> str:
        return "w1"

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
            "platform_prompt": "Gateway platform body {{literal}}",
            "user_display_name": "Alice",
            "memory_attachments": [
                {
                    "store_id": "mem1",
                    "store_name": "Project Memory",
                    "store_description": "notes",
                    "access": "read_only",
                    "instructions": "Read first.",
                    "index_content": "# Memory",
                }
            ],
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
    platform_prompt = (codex_dir / "AGENTS.md").read_text(encoding="utf-8")
    assert platform_prompt == "Gateway platform body {{literal}}"
    assert json.loads((workspace / "runtime.json").read_text(encoding="utf-8")) == {
        "model": "gpt-test",
        "provider_type": "openai",
        "base_url": "http://llm.test/v1",
        "small_model": "small",
    }
    assert json.loads((codex_dir / "mcp.json").read_text())["mcpServers"] == {}
    assert not (codex_dir / "auth.json").exists()
    servers = hooks.load_mcp_servers("token")
    assert len(servers) == 1
    assert servers[0]["name"] == "media-agent-gateway"
    assert servers[0]["url"] == "http://cp.test/mcp"
    assert servers[0]["headers"]["X-Session-Token"] == "token"

    os.environ["ANTHROPIC_API_KEY"] = "old"
    os.environ["OPENAI_API_KEY"] = "old"
    await hooks.restart_bridge()
    assert "ANTHROPIC_API_KEY" not in os.environ
    assert os.environ["OPENAI_API_KEY"] == "sk-test"
    assert os.environ["OPENAI_BASE_URL"] == "http://llm.test/v1"


@pytest.mark.asyncio
async def test_mock_control_preserves_image_auth_and_disables_mcp(tmp_path: Path) -> None:
    workspace = tmp_path / "workspace"
    codex_dir = tmp_path / ".codex"
    codex_dir.mkdir()
    auth = codex_dir / "auth.json"
    auth.write_text('{"existing":"login"}', encoding="utf-8")
    hooks = CodexHooks(
        workspace_dir=workspace,
        codex_config_dir=codex_dir,
        mock_control=True,
    )

    assert await hooks.load_config() is True
    assert await hooks.load_credentials() is True
    assert hooks.load_mcp_servers(None) == []
    assert auth.read_text(encoding="utf-8") == '{"existing":"login"}'
    runtime = json.loads((workspace / "runtime.json").read_text(encoding="utf-8"))
    assert runtime["model"] == "default"
