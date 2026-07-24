"""Goose agent configuration and hooks."""

from __future__ import annotations

import json
import logging
import os
from datetime import UTC, datetime
from pathlib import Path
from typing import Any, cast

from pydantic import BaseModel, ConfigDict, Field, field_validator, model_validator

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
    def _empty_to_none(cls, value: Any) -> Any:
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


def _num(v: Any) -> int:
    return int(v) if isinstance(v, (int, float)) and v == v else 0


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
    ) -> None:
        self._cp_client = cp_client or CpClient(base_url=cp_url, workspace_id=workspace_id)
        self._goose_path_root = goose_path_root
        self._workspace_dir = Path(workspace_dir)
        self._accumulated: dict[str, tuple[int, int]] = {}

    def track_ext_notification(
        self, platform_session_id: str, method: str, params: dict[str, Any]
    ) -> None:
        if method != "_goose/unstable/session/update":
            return
        update = params.get("update") or {}
        if update.get("sessionUpdate") != "usage_update":
            return
        inp = _num(update.get("accumulatedInputTokens"))
        out = _num(update.get("accumulatedOutputTokens"))
        if inp == 0 and out == 0:
            return
        self._accumulated[platform_session_id] = (inp, out)

    def load_mcp_servers(self, session_token: str | None) -> list[Any]:
        if not self._cp_client.is_configured():
            return []
        url = self._cp_client.mcp_url()
        if not url:
            return []
        return [
            {
                "type": "http",
                "name": "tos-platform",
                "url": url,
                "headers": self._cp_client.mcp_headers(session_token),
            }
        ]

    def has_mcp_servers(self) -> bool:
        return self._cp_client.is_configured()

    async def load_config(self) -> bool:
        config = await self._cp_client.fetch_workspace_config()
        if config is None:
            return False
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
        prompt = config.get("prompt_content") or config.get("system_prompt") or ""
        (self._workspace_dir / "AGENTS.md").write_text(prompt, encoding="utf-8")
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
        return

    def record_usage(self, session_id: str, usage: Any) -> None:
        acc = self._accumulated.get(session_id)
        model = _load_runtime_model(self._workspace_dir)
        payload: dict[str, Any] | None = None
        if acc:
            payload = {
                "ts": datetime.now(UTC).isoformat(),
                "model": model,
                "accumulated_input_tokens": acc[0],
                "accumulated_output_tokens": acc[1],
            }
        elif isinstance(usage, dict):
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


def _load_runtime_model(workspace_dir: Path) -> str | None:
    try:
        text = (workspace_dir / "runtime.json").read_text(encoding="utf-8")
        return cast(str | None, json.loads(text).get("model"))
    except (FileNotFoundError, json.JSONDecodeError):
        return None
