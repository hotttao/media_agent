"""Codex agent configuration and hooks."""

from __future__ import annotations

import json
import logging
import os
from pathlib import Path

import tomli_w
from pydantic import BaseModel, ConfigDict, Field, field_validator, model_validator

from adapter.agent_environment import (
    AcpMcpServerDeclaration,
    McpServerMap,
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
from adapter.platform_prompt import normalize_memory_attachments, write_platform_prompt

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
        mock_control: bool = False,
    ) -> None:
        self._cp_client = cp_client or CpClient(base_url=cp_url, workspace_id=workspace_id)
        self._workspace_dir = Path(workspace_dir)
        self._codex_config_dir = Path(codex_config_dir).expanduser()
        self._api_key: str = ""
        self._user_mcp: McpServerMap = {}
        self._has_mcp = False
        self._user_display_name: str | None = None
        self._mock_control = mock_control

    def load_mcp_servers(self, session_token: str | None) -> list[AcpMcpServerDeclaration]:
        if self._mock_control:
            return []
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
        if self._mock_control:
            self._workspace_dir.mkdir(parents=True, exist_ok=True)
            self._codex_config_dir.mkdir(parents=True, exist_ok=True)
            # Keep the image's auth.json intact; mock control only supplies
            # non-secret runtime defaults.
            write_runtime_config(
                self._workspace_dir,
                {"model": "default", "provider_type": "openai", "mcp_config": {}},
            )
            return True
        config = await self._cp_client.fetch_workspace_config()
        if config is None:
            return False

        if isinstance(config.get("user_display_name"), str):
            self._user_display_name = config["user_display_name"]
        write_platform_prompt(
            agent_kind="codex",
            target_dir=self._codex_config_dir,
            filename="AGENTS.md",
            workspace_id=self._cp_client.workspace_id,
            user_name=self._user_display_name,
            memory_attachments=normalize_memory_attachments(config.get("memory_attachments")),
        )

        self._api_key = str(config.get("api_key") or "")
        self._user_mcp = parse_mcp_servers(config.get("mcp_config"))
        self._has_mcp = bool(self._user_mcp)

        prompt = write_prompt_file(self._workspace_dir, config)
        write_runtime_config(self._workspace_dir, config)
        write_mcp_json(self._codex_config_dir, self._user_mcp)
        _write_codex_toml(dict(config), self._user_mcp, self._codex_config_dir)
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
        if self._mock_control:
            return True
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

    def record_usage(self, session_id: str, usage: object) -> None:
        return


def _write_codex_toml(
    config: dict[str, object], mcp_servers: McpServerMap, codex_dir: Path
) -> None:
    codex_dir.mkdir(parents=True, exist_ok=True)
    model = config.get("model", "")
    base_url = config.get("base_url")
    provider_name = "custom" if base_url else "openai"
    agent_settings = str(config.get("agent_settings") or "").strip()
    is_toml_agent_settings = bool(
        agent_settings and agent_settings != "{}" and not agent_settings.startswith("{")
    )
    toml: dict[str, object] = {
        "model_provider": provider_name,
        "model": model,
        "disable_response_storage": True,
    }
    if not _toml_has_key(agent_settings, "sandbox_mode"):
        toml["sandbox_mode"] = "danger-full-access"
    if not _toml_has_key(agent_settings, "approval_policy"):
        toml["approval_policy"] = "never"
    if isinstance(base_url, str) and base_url:
        toml["model_providers"] = {
            "custom": {
                "name": "custom",
                "base_url": base_url,
                "wire_api": "responses",
            }
        }
    if any(cfg.get("url") for cfg in mcp_servers.values()):
        toml["experimental_use_rmcp_client"] = True
    mcp_section: dict[str, object] = {}
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
