"""Codex AgentHooks 实现。

对标 agents/codex/src/config.ts 的 loadConfig / loadSkills / loadCredentials /
loadAcpMcpServers / applyProviderEnv，以及 agents/codex/src/server.ts 的 restartBridge。
"""

from __future__ import annotations

import json
import logging
import os
from pathlib import Path
from typing import Any, cast

import httpx
import tomli_w

logger = logging.getLogger(__name__)

HOME = Path(os.environ.get("HOME", "/root"))
WORKSPACE_DIR = Path(os.environ.get("WORKSPACE_DIR", "/workspace"))


class CodexHooks:
    """AgentHooks Protocol 的 codex 实现。

    静态配置（agent_type / capabilities / workspace_dir 等）走 AdapterConfig
    （omegaconf 从 conf/config.yaml 加载），hooks 只负责回调。
    """

    def __init__(self, *, cp_url: str | None, workspace_id: str | None) -> None:
        self._cp_url = cp_url
        self._workspace_id = workspace_id
        self._api_key: str = ""
        self._user_mcp: dict[str, dict[str, Any]] = {}
        self._has_mcp = False

    # ── AgentHooks 协议实现 ────────────────────────────────────────

    def load_mcp_servers(self, session_token: str | None) -> list[Any]:
        if not self._cp_url or not self._workspace_id:
            return []
        servers: list[Any] = []
        headers = {
            "X-Workspace-ID": self._workspace_id,
            "X-Agent-ID": self._workspace_id,
        }
        if session_token:
            headers["X-Session-Token"] = session_token
        servers.append(
            {
                "type": "http",
                "name": "tos-platform",
                "url": f"{self._cp_url}/mcp",
                "headers": headers,
            }
        )
        for name, cfg in self._user_mcp.items():
            if name == "tos-platform" or not cfg.get("url"):
                continue
            entry_headers = dict(cfg.get("headers") or {})
            if session_token:
                entry_headers["X-Session-Token"] = session_token
            servers.append(
                {"type": "http", "name": name, "url": cfg["url"], "headers": entry_headers}
            )
        return servers

    def has_mcp_servers(self) -> bool:
        return self._has_mcp

    async def load_config(self) -> bool:
        if not self._cp_url or not self._workspace_id:
            logger.info("Config skipped: CP_URL/WORKSPACE_ID unset")
            return False
        url = f"{self._cp_url}/_cp/workspaces/{self._workspace_id}/config"
        async with httpx.AsyncClient() as client:
            resp = await client.get(url)
            if not resp.is_success:
                logger.error(
                    "Config fetch failed %s %s url=%s",
                    resp.status_code,
                    resp.reason_phrase,
                    url,
                )
                return False
            config = resp.json()

        self._api_key = config.get("api_key") or ""
        mcp_servers: dict[str, Any] = {}
        try:
            parsed = json.loads(config.get("mcp_config") or "{}")
            mcp_servers = parsed.get("mcpServers") or parsed
        except json.JSONDecodeError:
            pass
        self._user_mcp = mcp_servers
        self._has_mcp = bool(mcp_servers)

        # 写 runtime.json
        (WORKSPACE_DIR / "runtime.json").write_text(
            json.dumps(
                {
                    "model": config.get("model", ""),
                    "provider_type": config.get("provider_type", ""),
                    "base_url": config.get("base_url", ""),
                    "small_model": config.get("small_model", ""),
                }
            ),
            encoding="utf-8",
        )

        _write_codex_toml(config, mcp_servers)
        _write_codex_auth(self._api_key)
        logger.info("Config written: model=%s", config.get("model"))
        return True

    async def load_skills(self) -> tuple[bool, list[str]]:
        # TODO: 接 agent-skills 同步逻辑（对标 agents/codex/src/config.ts loadSkills）
        logger.info("Skills load skipped (not implemented)")
        return True, []

    async def load_credentials(self) -> bool:
        # TODO: 接 credentials 注入（对标 agents/codex/src/config.ts loadCredentials）
        logger.info("Credentials load skipped (not implemented)")
        return False

    async def restart_bridge(self) -> None:
        # codex-acp 读 OPENAI_API_KEY / OPENAI_BASE_URL 环境变量
        rc = _load_runtime()
        if rc and rc.get("api_key"):
            os.environ["OPENAI_API_KEY"] = rc["api_key"]
            if rc.get("base_url"):
                os.environ["OPENAI_BASE_URL"] = rc["base_url"]

    def record_usage(self, session_id: str, usage: Any) -> None:
        # codex 的 PromptResponse.usage 可能为 None；agent-usage sweeper 从 .codex
        # transcripts 直接读，这里不需要写 JSONL。对标 TS codex 不传 recordUsage。
        return


# ── 内部工具 ──────────────────────────────────────────────────────


def _write_codex_toml(config: dict[str, Any], mcp_servers: dict[str, Any]) -> None:
    codex_dir = HOME / ".codex"
    codex_dir.mkdir(parents=True, exist_ok=True)
    model = config.get("model", "")
    provider_name = "custom" if config.get("base_url") else "openai"
    toml: dict[str, Any] = {
        "model_provider": provider_name,
        "model": model,
        "disable_response_storage": True,
        "sandbox_mode": "danger-full-access",
        "approval_policy": "never",
    }
    if config.get("base_url"):
        toml["model_providers"] = {
            "custom": {
                "name": "custom",
                "base_url": config["base_url"],
                "wire_api": "responses",
            }
        }
    if any(cfg.get("url") for cfg in mcp_servers.values()):
        toml["experimental_use_rmcp_client"] = True
    # stdio MCP entries
    mcp_section: dict[str, Any] = {}
    for name, cfg in mcp_servers.items():
        if cfg.get("url"):
            continue
        mcp_section[name] = {k: v for k, v in cfg.items() if k in {"command", "args", "env"}}
    if mcp_section:
        toml["mcp_servers"] = mcp_section
    toml["features"] = {"memories": False}

    (codex_dir / "config.toml").write_bytes(tomli_w.dumps(toml).encode("utf-8"))


def _write_codex_auth(api_key: str) -> None:
    if not api_key:
        return
    codex_dir = HOME / ".codex"
    codex_dir.mkdir(parents=True, exist_ok=True)
    (codex_dir / "auth.json").write_text(json.dumps({"OPENAI_API_KEY": api_key}), encoding="utf-8")


def _load_runtime() -> dict[str, Any] | None:
    try:
        data = json.loads((WORKSPACE_DIR / "runtime.json").read_text(encoding="utf-8"))
        data["api_key"] = os.environ.get("OPENAI_API_KEY", "")
        return cast(dict[str, Any], data)
    except (FileNotFoundError, json.JSONDecodeError):
        return None
