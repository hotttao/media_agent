from __future__ import annotations

import json
from pathlib import Path

from adapter.agent_environment import (
    apply_credentials,
    build_acp_mcp_servers,
    parse_mcp_servers,
    read_runtime_config,
    write_mcp_json,
    write_prompt_file,
    write_runtime_config,
)


def test_parse_mcp_servers_accepts_wrapped_and_raw_shapes() -> None:
    wrapped = parse_mcp_servers('{"mcpServers":{"a":{"url":"http://a"}}}')
    raw = parse_mcp_servers('{"b":{"command":"cmd"}}')

    assert wrapped == {"a": {"url": "http://a"}}
    assert raw == {"b": {"command": "cmd"}}
    assert parse_mcp_servers("not-json") == {}


def test_write_runtime_prompt_and_mcp_files(tmp_path: Path) -> None:
    config = {
        "model": "gpt-test",
        "provider_type": "openai",
        "base_url": "http://llm.test/v1",
        "small_model": "small",
        "prompt_content": "hello",
    }

    runtime = write_runtime_config(tmp_path, config)
    prompt = write_prompt_file(tmp_path, config)
    write_mcp_json(tmp_path / ".agent", {"a": {"url": "http://a"}})

    assert runtime == {
        "model": "gpt-test",
        "provider_type": "openai",
        "base_url": "http://llm.test/v1",
        "small_model": "small",
    }
    assert read_runtime_config(tmp_path) == runtime
    assert prompt == "hello"
    assert (tmp_path / "AGENTS.md").read_text(encoding="utf-8") == "hello"
    assert json.loads((tmp_path / ".agent" / "mcp.json").read_text(encoding="utf-8")) == {
        "mcpServers": {"a": {"url": "http://a"}}
    }


def test_build_acp_mcp_servers_merges_platform_headers_and_user_servers() -> None:
    servers = build_acp_mcp_servers(
        platform_url="http://cp.test/mcp",
        platform_headers={"X-Workspace-ID": "w1", "X-Agent-ID": "w1"},
        session_token="token",
        include_user_stdio=True,
        user_mcp_servers={
            "tos-platform": {"headers": {"X-Builder": "b", "X-Agent-ID": "user"}},
            "remote": {"url": "http://remote", "headers": {"A": "B"}},
            "local": {"command": "tool", "args": ["--x"], "env": {"E": "V"}},
        },
    )

    assert servers == [
        {
            "type": "http",
            "name": "tos-platform",
            "url": "http://cp.test/mcp",
            "headers": {"X-Builder": "b", "X-Agent-ID": "w1", "X-Workspace-ID": "w1"},
        },
        {
            "type": "http",
            "name": "remote",
            "url": "http://remote",
            "headers": {"A": "B", "X-Session-Token": "token"},
        },
        {"name": "local", "command": "tool", "args": ["--x"], "env": {"E": "V"}},
    ]


def test_apply_credentials_injects_env_files_and_shell_source(tmp_path: Path) -> None:
    env: dict[str, str] = {}
    target = tmp_path / ".ssh" / "id_rsa"

    result = apply_credentials(
        [
            {"name": "TOKEN", "value": "a'b", "inject": "env", "status": "active"},
            {
                "name": "KEY",
                "value": "secret",
                "inject": "file",
                "path": str(target),
                "mode": "0600",
                "status": "active",
            },
        ],
        home=tmp_path,
        environ=env,
    )

    assert result.injected == 2
    assert result.cleaned == 0
    assert env["TOKEN"] == "a'b"
    assert env["BASH_ENV"] == str(tmp_path / ".agent-credentials.env")
    assert (tmp_path / ".agent-credentials.env").read_text(encoding="utf-8") == (
        "export TOKEN='a'\\''b'\n"
    )
    assert target.read_text(encoding="utf-8") == "secret"
    assert "StrictHostKeyChecking accept-new" in (tmp_path / ".ssh" / "config").read_text(
        encoding="utf-8"
    )
    assert ".agent-credentials.env" in (tmp_path / ".bashrc").read_text(encoding="utf-8")


def test_apply_credentials_cleans_existing_values(tmp_path: Path) -> None:
    env = {"TOKEN": "old"}
    target = tmp_path / "token.txt"
    target.write_text("secret", encoding="utf-8")

    result = apply_credentials(
        [
            {"name": "TOKEN", "inject": "env", "status": "deleting"},
            {"name": "FILE", "inject": "file", "path": str(target), "status": "deleting"},
        ],
        home=tmp_path,
        environ=env,
    )

    assert result.injected == 0
    assert result.cleaned == 2
    assert "TOKEN" not in env
    assert not target.exists()