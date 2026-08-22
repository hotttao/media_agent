"""Claude Code agent configuration."""

from __future__ import annotations

from pathlib import Path

from pydantic import BaseModel, ConfigDict, Field, field_validator, model_validator

from adapter.config.config import AdapterConfig, ConfigError, load_agent_config


class ClaudeCodeSectionConfig(BaseModel):
    model_config = ConfigDict(extra="forbid")

    bin: str = "claude"
    session_store_dir: str = "/workspace/.home/.claude/projects"

    @field_validator("bin", "session_store_dir")
    @classmethod
    def _required_text(cls, value: str) -> str:
        if not value.strip():
            raise ConfigError("claude_code bin and session_store_dir must be non-empty")
        return value


class ClaudeCodeConfig(AdapterConfig):
    model_config = ConfigDict(extra="forbid")

    claude_code: ClaudeCodeSectionConfig = Field(default_factory=ClaudeCodeSectionConfig)

    @model_validator(mode="after")
    def _validate_claude_code(self) -> ClaudeCodeConfig:
        if self.agent.type != "claude-code":
            raise ConfigError("claude_code config requires agent.type=claude-code")
        return self


def load_claude_code_config(yaml_path: str | Path) -> ClaudeCodeConfig:
    return load_agent_config(yaml_path, ClaudeCodeConfig)
