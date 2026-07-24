"""Codex agent configuration and hooks."""

from __future__ import annotations

import json
import logging
import os
from pathlib import Path
from typing import Any

import tomli_w
from pydantic import BaseModel, ConfigDict, Field, field_validator, model_validator

from adapter.agent_environment import (
    apply_credentials,
    build_acp_mcp_servers,
    parse_mcp_servers,
    read_runtime_config,
    write_mcp_json,
    write_prompt_file,
    write_runtime_config,
)
from adapter.biz.shared.provider_environment import apply_openai_compatible_env
from adapter.clients.cp import CpClient
from adapter.config.config import AdapterConfig, ConfigError, load_agent_config

logger = logging.getLogger(__name__)

HOME = Path(os.environ.get("HOME", "/root"))
DEFAULT_WORKSPACE_DIR = Path(os.environ.get("WORKSPACE_DIR", "/workspace"))


class CodexSectionConfig(BaseModel):
    model_config = ConfigDict(extra="forbid")

    program: str = "codex-acp"
    initial_agent_mode: str = "agent-full-access"
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
        return build_acp_mcp_servers(
            platform_url=self._cp_client.mcp_url(),
            platform_headers=self._cp_client.mcp_headers(session_token),
            user_mcp_servers=self._user_mcp,
            session_token=session_token,
            include_user_http=True,
            include_user_stdio=False,
        )

    def has_mcp_servers(self) -> bool:
        return self._has_mcp

    async def load_config(self) -> bool:
        config = await self._cp_client.fetch_workspace_config()
        if config is None:
            return False

        self._api_key = str(config.get("api_key") or "")
        self._user_mcp = parse_mcp_servers(config.get("mcp_config"))
        self._has_mcp = bool(self._user_mcp)

        prompt = write_prompt_file(self._workspace_dir, config)
        write_runtime_config(self._workspace_dir, config)
        write_mcp_json(self._codex_config_dir, self._user_mcp)
        _write_codex_toml(config, self._user_mcp, self._codex_config_dir)
        _write_codex_auth(self._api_key, self._codex_config_dir)
        logger.info(
            "Config written: model=%s provider=%s prompt=%dchars",
            config.get("model"),
            config.get("provider_type"),
            len(prompt),
        )
        return True

    async def load_skills(self) -> tuple[bool, list[str]]:
        logger.info("Skills load skipped (not implemented)")
        return True, []

    async def load_credentials(self) -> bool:
        credentials = await self._cp_client.fetch_credentials()
        if credentials is None:
            return False
        result = apply_credentials(credentials, home=HOME)
        logger.info(
            "Credentials loaded: %d injected, %d cleaned",
            result.injected,
            result.cleaned,
        )
        return True

    async def restart_bridge(self) -> None:
        runtime = read_runtime_config(self._workspace_dir)
        apply_openai_compatible_env(runtime, api_key=self._api_key)

    def record_usage(self, session_id: str, usage: Any) -> None:
        return


def _write_codex_toml(
    config: dict[str, Any], mcp_servers: dict[str, Any], codex_dir: Path
) -> None:
    codex_dir.mkdir(parents=True, exist_ok=True)
    model = config.get("model", "")
    provider_name = "custom" if config.get("base_url") else "openai"
    agent_settings = str(config.get("agent_settings") or "").strip()
    is_toml_agent_settings = bool(
        agent_settings and agent_settings != "{}" and not agent_settings.startswith("{")
    )
    toml: dict[str, Any] = {
        "model_provider": provider_name,
        "model": model,
        "disable_response_storage": True,
    }
    if not _toml_has_key(agent_settings, "sandbox_mode"):
        toml["sandbox_mode"] = "danger-full-access"
    if not _toml_has_key(agent_settings, "approval_policy"):
        toml["approval_policy"] = "never"
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

    content = tomli_w.dumps(toml)
    if is_toml_agent_settings:
        content += "\n# agent_settings\n" + agent_settings.rstrip() + "\n"
    (codex_dir / "config.toml").write_bytes(content.encode("utf-8"))


def _toml_has_key(agent_settings: str, key: str) -> bool:
    return any(line.strip().startswith(f"{key} =") for line in agent_settings.splitlines())


def _write_codex_auth(api_key: str, codex_dir: Path) -> None:
    if not api_key:
        return
    codex_dir.mkdir(parents=True, exist_ok=True)
    (codex_dir / "auth.json").write_text(json.dumps({"OPENAI_API_KEY": api_key}), encoding="utf-8")
