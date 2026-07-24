"""Codex agent configuration and hooks."""

from __future__ import annotations

import json
import logging
import os
from pathlib import Path
from typing import Any, cast

import tomli_w
from pydantic import BaseModel, ConfigDict, Field, field_validator, model_validator

from adapter.clients.cp import CpClient
from adapter.config.config import AdapterConfig, ConfigError, load_agent_config

logger = logging.getLogger(__name__)

HOME = Path(os.environ.get("HOME", "/root"))
DEFAULT_WORKSPACE_DIR = Path(os.environ.get("WORKSPACE_DIR", "/workspace"))


class CodexSectionConfig(BaseModel):
    model_config = ConfigDict(extra="forbid")

    program: str = "codex-acp"
    initial_agent_mode: str = "default"
    config_dir: str = "~/.codex"
    disable_response_storage: bool = True

    @field_validator("program", "initial_agent_mode")
    @classmethod
    def _required_text(cls, value: str) -> str:
        if not value.strip():
            raise ConfigError("codex program and initial_agent_mode must be non-empty")
        return value


class CodexConfig(AdapterConfig):
    model_config = ConfigDict(extra="forbid")

    codex: CodexSectionConfig = Field(default_factory=CodexSectionConfig)

    @model_validator(mode="after")
    def _validate_codex(self) -> CodexConfig:
        if self.agent.type != "codex":
            raise ConfigError("codex config requires agent.type=codex")
        return self


def load_codex_config(yaml_path: str | Path) -> CodexConfig:
    return load_agent_config(yaml_path, CodexConfig)


class CodexHooks:
    """AgentHooks implementation for codex."""

    def __init__(
        self,
        *,
        cp_client: CpClient | None = None,
        cp_url: str | None = None,
        workspace_id: str | None = None,
        workspace_dir: str | Path = DEFAULT_WORKSPACE_DIR,
        codex_config_dir: str | Path = HOME / ".codex",
    ) -> None:
        self._cp_client = cp_client or CpClient(base_url=cp_url, workspace_id=workspace_id)
        self._workspace_dir = Path(workspace_dir)
        self._codex_config_dir = Path(codex_config_dir).expanduser()
        self._api_key: str = ""
        self._user_mcp: dict[str, dict[str, Any]] = {}
        self._has_mcp = False

    def load_mcp_servers(self, session_token: str | None) -> list[Any]:
        if not self._cp_client.is_configured():
            return []
        url = self._cp_client.mcp_url()
        if not url:
            return []
        servers: list[Any] = [
            {
                "type": "http",
                "name": "tos-platform",
                "url": url,
                "headers": self._cp_client.mcp_headers(session_token),
            }
        ]
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
        config = await self._cp_client.fetch_workspace_config()
        if config is None:
            return False

        self._api_key = config.get("api_key") or ""
        mcp_servers: dict[str, Any] = {}
        try:
            parsed = json.loads(config.get("mcp_config") or "{}")
            mcp_servers = parsed.get("mcpServers") or parsed
        except json.JSONDecodeError:
            logger.warning("CP mcp_config is not valid JSON")
        self._user_mcp = mcp_servers
        self._has_mcp = bool(mcp_servers)

        self._workspace_dir.mkdir(parents=True, exist_ok=True)
        (self._workspace_dir / "runtime.json").write_text(
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

        _write_codex_toml(config, mcp_servers, self._codex_config_dir)
        _write_codex_auth(self._api_key, self._codex_config_dir)
        logger.info("Config written: model=%s", config.get("model"))
        return True

    async def load_skills(self) -> tuple[bool, list[str]]:
        logger.info("Skills load skipped (not implemented)")
        return True, []

    async def load_credentials(self) -> bool:
        credentials = await self._cp_client.fetch_credentials()
        if credentials is None:
            return False
        logger.info(
            'Credentials fetched but injection is not implemented count=%s',
            len(credentials),
        )
        return True

    async def restart_bridge(self) -> None:
        runtime = _load_runtime(self._workspace_dir)
        if runtime and self._api_key:
            os.environ["OPENAI_API_KEY"] = self._api_key
            if runtime.get("base_url"):
                os.environ["OPENAI_BASE_URL"] = runtime["base_url"]

    def record_usage(self, session_id: str, usage: Any) -> None:
        return


def _write_codex_toml(
    config: dict[str, Any], mcp_servers: dict[str, Any], codex_dir: Path
) -> None:
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
    mcp_section: dict[str, Any] = {}
    for name, cfg in mcp_servers.items():
        if cfg.get("url"):
            continue
        mcp_section[name] = {k: v for k, v in cfg.items() if k in {"command", "args", "env"}}
    if mcp_section:
        toml["mcp_servers"] = mcp_section
    toml["features"] = {"memories": False}

    (codex_dir / "config.toml").write_bytes(tomli_w.dumps(toml).encode("utf-8"))


def _write_codex_auth(api_key: str, codex_dir: Path) -> None:
    if not api_key:
        return
    codex_dir.mkdir(parents=True, exist_ok=True)
    (codex_dir / "auth.json").write_text(json.dumps({"OPENAI_API_KEY": api_key}), encoding="utf-8")


def _load_runtime(workspace_dir: Path) -> dict[str, Any] | None:
    try:
        data = json.loads((workspace_dir / "runtime.json").read_text(encoding="utf-8"))
        return cast(dict[str, Any], data)
    except (FileNotFoundError, json.JSONDecodeError):
        return None
