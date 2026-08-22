"""Adapter configuration definitions.

OmegaConf is responsible for YAML loading and environment interpolation;
Pydantic is responsible for schema validation and semantic checks. Agent
packages should load their own typed config subclass via ``load_agent_config``.
"""

from __future__ import annotations

from pathlib import Path
from typing import TypeVar, overload

from omegaconf import OmegaConf
from pydantic import BaseModel, ConfigDict, Field, field_validator, model_validator

from adapter.schemas.acp.response import AgentCapabilities


class ConfigError(ValueError):
    """Raised when adapter configuration is structurally valid but unsafe."""


class AgentConfig(BaseModel):
    model_config = ConfigDict(extra="forbid")

    type: str
    port: int = Field(default=3000, gt=0, le=65535)


class WorkspaceConfig(BaseModel):
    model_config = ConfigDict(extra="forbid")

    dir: str = "/workspace"
    keep_files: set[str] = Field(default_factory=lambda: {"AGENTS.md", "runtime.json"})

    @field_validator("keep_files", mode="before")
    @classmethod
    def _normalize_keep_files(cls, value: object) -> object:
        if value is None:
            return set()
        return value


class CpConfig(BaseModel):
    model_config = ConfigDict(extra="forbid")

    url: str | None = None
    workspace_id: str | None = None

    @field_validator("url", "workspace_id", mode="before")
    @classmethod
    def _empty_to_none(cls, value: object) -> object:
        if value is None:
            return None
        if isinstance(value, str) and value.strip().lower() in {"", "null", "none"}:
            return None
        return value


class BridgeTtlConfig(BaseModel):
    """Bridge LRU eviction configuration."""

    model_config = ConfigDict(extra="forbid")

    idle_ttl_ms: int = Field(default=10 * 60 * 1000, gt=0)
    max_count: int = Field(default=10, gt=0)
    evict_interval_ms: int = Field(default=60 * 1000, gt=0)


class KeepaliveConfig(BaseModel):
    """SSE heartbeat configuration."""

    model_config = ConfigDict(extra="forbid")

    interval_ms: int = Field(default=15_000, gt=0)


class DatabaseConfig(BaseModel):
    """Agent-owned durable Runtime Session database."""

    model_config = ConfigDict(extra="forbid")

    url: str = "postgresql+asyncpg://media_auth:media_auth@127.0.0.1:5432/media_auth"
    migrate_on_start: bool = True
    echo: bool = False

    @field_validator("url")
    @classmethod
    def _validate_postgresql_url(cls, value: str) -> str:
        normalized = value.strip()
        if not normalized.startswith("postgresql+asyncpg://"):
            raise ConfigError("database.url must use postgresql+asyncpg")
        return normalized


class RuntimeConfig(BaseModel):
    model_config = ConfigDict(extra="forbid")

    file: str = "runtime.json"
    prompt_file: str = "AGENTS.md"


class SkillsConfig(BaseModel):
    model_config = ConfigDict(extra="forbid")

    enabled: bool = False
    dir: str = ".agents/skills"


class EnvironmentConfig(BaseModel):
    model_config = ConfigDict(extra="forbid")

    shell_rc: str = ".bashrc"
    credentials_dir: str = ".credentials"


TConfig = TypeVar("TConfig", bound="AdapterConfig")


class AdapterConfig(BaseModel):
    """Shared adapter config loaded from common YAML plus agent overrides."""

    model_config = ConfigDict(extra="allow")

    agent: AgentConfig
    capabilities: AgentCapabilities
    workspace: WorkspaceConfig = Field(default_factory=WorkspaceConfig)
    cp: CpConfig = Field(default_factory=CpConfig)
    bridge_ttl: BridgeTtlConfig = Field(default_factory=BridgeTtlConfig)
    keepalive: KeepaliveConfig = Field(default_factory=KeepaliveConfig)
    database: DatabaseConfig = Field(default_factory=DatabaseConfig)
    runtime: RuntimeConfig = Field(default_factory=RuntimeConfig)
    skills: SkillsConfig = Field(default_factory=SkillsConfig)
    environment: EnvironmentConfig = Field(default_factory=EnvironmentConfig)

    @model_validator(mode="after")
    def _validate_adapter(self) -> AdapterConfig:
        self.validate_config()
        return self

    @property
    def agent_type(self) -> str:
        return self.agent.type

    @property
    def workspace_dir(self) -> str:
        return self.workspace.dir

    @property
    def keep_files(self) -> set[str]:
        return self.workspace.keep_files

    @property
    def cp_url(self) -> str | None:
        return self.cp.url

    @property
    def workspace_id(self) -> str | None:
        return self.cp.workspace_id

    def validate_config(self) -> None:
        if not self.agent.type.strip():
            raise ConfigError("agent.type is required")

        has_cp_url = bool(self.cp.url)
        has_workspace_id = bool(self.cp.workspace_id)
        if has_cp_url != has_workspace_id:
            raise ConfigError("cp.url and cp.workspace_id must be configured together")

        for keep_file in self.workspace.keep_files:
            keep_path = Path(keep_file)
            if keep_path.is_absolute() or ".." in keep_path.parts:
                raise ConfigError(
                    f"workspace.keep_files must be relative workspace files: {keep_file}"
                )

        self._assert_workspace_relative(self.runtime.file, "runtime.file")
        self._assert_workspace_relative(self.runtime.prompt_file, "runtime.prompt_file")

    def _assert_workspace_relative(self, value: str, field_name: str) -> None:
        path = Path(value)
        if path.is_absolute() or ".." in path.parts:
            raise ConfigError(f"{field_name} must be a relative path inside workspace")


@overload
def load_agent_config(
    yaml_path: str | Path,
    model_type: type[TConfig],
    *,
    base_path: str | Path | None = None,
) -> TConfig: ...


@overload
def load_agent_config(
    yaml_path: str | Path,
    model_type: None = None,
    *,
    base_path: str | Path | None = None,
) -> AdapterConfig: ...


def load_agent_config(
    yaml_path: str | Path,
    model_type: type[AdapterConfig] | None = None,
    *,
    base_path: str | Path | None = None,
) -> AdapterConfig:
    """Load common adapter config and one agent override into a typed model."""
    common_path = (
        Path(base_path) if base_path is not None else Path(__file__).with_name("base.yaml")
    )
    merged = OmegaConf.merge(OmegaConf.load(str(common_path)), OmegaConf.load(str(yaml_path)))
    data = OmegaConf.to_container(merged, resolve=True)
    return (model_type or AdapterConfig).model_validate(data)


def load_adapter_config(yaml_path: str | Path) -> AdapterConfig:
    """Backward-compatible shared config loader."""
    return load_agent_config(yaml_path, AdapterConfig)
