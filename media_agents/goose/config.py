"""Goose agent configuration and hooks."""

from __future__ import annotations

import json
import logging
import os
from collections.abc import Mapping
from datetime import UTC, datetime
from pathlib import Path
from typing import cast
from urllib.parse import urlparse

from pydantic import BaseModel, ConfigDict, Field, field_validator, model_validator

from adapter.agent_environment import (
    AcpMcpServerDeclaration,
    McpServerMap,
    RuntimeConfig,
    apply_credentials,
    build_acp_mcp_servers,
    parse_mcp_servers,
    read_runtime_config,
    write_mcp_json,
    write_prompt_file,
    write_runtime_config,
)
from adapter.clients.cp import CpClient
from adapter.config.config import AdapterConfig, ConfigError, load_agent_config

logger = logging.getLogger(__name__)

HOME = Path(os.environ.get("HOME", "/root"))
DEFAULT_WORKSPACE_DIR = Path(os.environ.get("WORKSPACE_DIR", "/workspace"))


class GooseSectionConfig(BaseModel):
    model_config = ConfigDict(extra="forbid")

    program: str = "goose"
    args: list[str] = Field(default_factory=list)
    path_root: str | None = None
    config_dir: str = "~/.config/goose"

    @field_validator("program")
    @classmethod
    def _program_required(cls, value: str) -> str:
        if not value.strip():
            raise ConfigError("goose.program must be non-empty")
        return value

    @field_validator("path_root", mode="before")
    @classmethod
    def _empty_to_none(cls, value: object) -> object:
        if value is None:
            return None
        if isinstance(value, str) and value.strip().lower() in {"", "null", "none"}:
            return None
        return value


class GooseConfig(AdapterConfig):
    model_config = ConfigDict(extra="forbid")

    goose: GooseSectionConfig = Field(default_factory=GooseSectionConfig)

    @model_validator(mode="after")
    def _validate_goose(self) -> GooseConfig:
        if self.agent.type != "goose":
            raise ConfigError("goose config requires agent.type=goose")
        return self


def load_goose_config(yaml_path: str | Path) -> GooseConfig:
    return load_agent_config(yaml_path, GooseConfig)


def _num(value: object) -> int:
    return int(value) if isinstance(value, (int, float)) and value == value else 0


class GooseHooks:
    """AgentHooks implementation for goose."""

    def __init__(
        self,
        *,
        cp_client: CpClient | None = None,
        cp_url: str | None = None,
        workspace_id: str | None = None,
        goose_path_root: str | None = None,
        workspace_dir: str | Path = DEFAULT_WORKSPACE_DIR,
        goose_config_dir: str | Path = HOME / ".config" / "goose",
    ) -> None:
        self._cp_client = cp_client or CpClient(base_url=cp_url, workspace_id=workspace_id)
        self._goose_path_root = goose_path_root
        self._workspace_dir = Path(workspace_dir)
        self._goose_config_dir = Path(goose_config_dir).expanduser()
        self._api_key = ""
        self._user_mcp: McpServerMap = {}
        self._has_mcp = False
        self._accumulated: dict[str, tuple[int, int]] = {}

    def track_ext_notification(
        self, platform_session_id: str, method: str, params: Mapping[str, object]
    ) -> None:
        if method != "_goose/unstable/session/update":
            return
        update = params.get("update")
        if not isinstance(update, Mapping):
            return
        if update.get("sessionUpdate") != "usage_update":
            return
        inp = _num(update.get("accumulatedInputTokens"))
        out = _num(update.get("accumulatedOutputTokens"))
        if inp == 0 and out == 0:
            return
        self._accumulated[platform_session_id] = (inp, out)

    def load_mcp_servers(self, session_token: str | None) -> list[AcpMcpServerDeclaration]:
        return build_acp_mcp_servers(
            platform_url=self._cp_client.mcp_url(),
            platform_headers=self._cp_client.mcp_headers(session_token),
            user_mcp_servers=self._user_mcp,
            session_token=session_token,
            include_user_http=True,
            include_user_stdio=True,
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
        runtime = write_runtime_config(self._workspace_dir, config)
        write_mcp_json(self._goose_config_dir, self._user_mcp)
        _write_goose_config_yaml(dict(config), self._goose_config_dir, runtime)
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
        if runtime is not None:
            _apply_goose_provider_env(runtime, self._api_key)

    def record_usage(self, session_id: str, usage: object) -> None:
        acc = self._accumulated.get(session_id)
        model = _load_runtime_model(self._workspace_dir)
        payload: dict[str, object] | None = None
        if acc:
            payload = {
                "ts": datetime.now(UTC).isoformat(),
                "model": model,
                "accumulated_input_tokens": acc[0],
                "accumulated_output_tokens": acc[1],
            }
        elif isinstance(usage, Mapping):
            inp = _num(usage.get("inputTokens"))
            out = _num(usage.get("outputTokens"))
            if inp == 0 and out == 0:
                return
            payload = {
                "ts": datetime.now(UTC).isoformat(),
                "model": model,
                "input_tokens": inp,
                "output_tokens": out,
                "total_tokens": _num(usage.get("totalTokens")) or inp + out,
            }
        if not payload:
            return
        try:
            acp_dir = HOME / ".acp-usage"
            acp_dir.mkdir(parents=True, exist_ok=True)
            with (acp_dir / f"{session_id}.jsonl").open("a", encoding="utf-8") as f:
                f.write(json.dumps(payload) + "\n")
        except OSError:
            logger.exception("record_usage write failed session=%s", session_id)


def _write_goose_config_yaml(
    config: dict[str, object], goose_dir: Path, runtime: RuntimeConfig
) -> None:
    goose_dir.mkdir(parents=True, exist_ok=True)
    yaml_lines = [
        f"GOOSE_MODEL: {json.dumps(runtime['model'])}",
        "GOOSE_MODE: auto",
        "extensions:",
        "  developer:",
        "    type: builtin",
        "    name: developer",
        "    enabled: true",
        "  memory:",
        "    type: builtin",
        "    name: memory",
        "    enabled: false",
        "  chatrecall:",
        "    type: builtin",
        "    name: chatrecall",
        "    enabled: false",
    ]
    agent_settings = str(config.get("agent_settings") or "").strip()
    if _is_yaml_agent_settings(agent_settings):
        yaml_lines.extend(
            ["", "# agent_settings", _strip_platform_managed_yaml_keys(agent_settings)]
        )
    (goose_dir / "config.yaml").write_text("\n".join(yaml_lines) + "\n", encoding="utf-8")


def _is_yaml_agent_settings(agent_settings: str) -> bool:
    if not agent_settings or agent_settings == "{}" or agent_settings.startswith("{"):
        return False
    return not any("=" in line for line in agent_settings.splitlines() if line.strip())


def _strip_platform_managed_yaml_keys(yaml: str) -> str:
    managed = {"GOOSE_MODEL", "GOOSE_MODE", "extensions"}
    kept: list[str] = []
    skipping = False
    for line in yaml.splitlines():
        key = _top_level_yaml_key(line)
        if key is not None:
            skipping = key in managed
        if not skipping:
            kept.append(line)
    return "\n".join(kept)


def _top_level_yaml_key(line: str) -> str | None:
    if not line or line[0].isspace() or ":" not in line:
        return None
    key = line.split(":", 1)[0]
    if not key.replace("_", "").replace("-", "").replace(".", "").isalnum():
        return None
    return key


def _apply_goose_provider_env(runtime: RuntimeConfig, api_key: str) -> None:
    for key in [
        "ANTHROPIC_API_KEY",
        "ANTHROPIC_BASE_URL",
        "OPENAI_API_KEY",
        "OPENAI_HOST",
        "OPENAI_BASE_PATH",
        "GOOSE_FAST_MODEL",
    ]:
        os.environ.pop(key, None)
    os.environ["GOOSE_PROVIDER"] = "openai"
    os.environ["GOOSE_MODEL"] = runtime["model"]
    os.environ["GOOSE_MODE"] = "auto"
    os.environ["GOOSE_DISABLE_KEYRING"] = "1"
    os.environ["OPENAI_CUSTOM_HEADERS"] = "User-Agent=agent-platform-goose/1.0"
    os.environ["GOOSE_DISABLE_TOOL_CALL_SUMMARY"] = "1"
    os.environ["GOOSE_DISABLE_SESSION_NAMING"] = "1"
    if api_key:
        os.environ["OPENAI_API_KEY"] = api_key
    if runtime.get("small_model"):
        os.environ["GOOSE_FAST_MODEL"] = runtime["small_model"]
    if runtime.get("base_url"):
        parsed = urlparse(runtime["base_url"])
        if parsed.scheme and parsed.netloc:
            os.environ["OPENAI_HOST"] = f"{parsed.scheme}://{parsed.netloc}"
            base_path = parsed.path.rstrip("/").lstrip("/")
            os.environ["OPENAI_BASE_PATH"] = f"{base_path}/chat/completions"
        else:
            logger.error("Invalid base_url, ignoring: %s", runtime["base_url"])


def _load_runtime_model(workspace_dir: Path) -> str | None:
    runtime = read_runtime_config(workspace_dir)
    return cast(str | None, runtime.get("model") if runtime else None)
