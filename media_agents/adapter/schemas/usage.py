"""Pydantic schemas for usage pull APIs."""

from __future__ import annotations

from typing import Literal

from pydantic import BaseModel, ConfigDict, Field


class _AliasModel(BaseModel):
    model_config = ConfigDict(populate_by_name=True)


class FileFingerprintModel(_AliasModel):
    dev: int | float
    ino: int | float
    size: int | float
    mtime_ms: int | float = Field(alias="mtimeMs")

    def to_dict(self) -> dict[str, int | float]:
        return {
            "dev": self.dev,
            "ino": self.ino,
            "size": self.size,
            "mtimeMs": self.mtime_ms,
        }


class UsageRequest(_AliasModel):
    cursors: dict[str, FileFingerprintModel] = Field(default_factory=dict)
    max_files: int | None = Field(default=None, alias="maxFiles")

    def cursor_dict(self) -> dict[str, dict[str, int | float]]:
        return {path: fingerprint.to_dict() for path, fingerprint in self.cursors.items()}


class UsageRecordResponse(_AliasModel):
    source: Literal["claude", "codex", "goose"]
    session_id: str = Field(alias="sessionId")
    model: str
    ts: str
    input_tokens: int = Field(alias="inputTokens")
    output_tokens: int = Field(alias="outputTokens")
    cache_read_tokens: int = Field(alias="cacheReadTokens")
    cache_creation_tokens: int = Field(alias="cacheCreationTokens")
    cache_creation_5m_tokens: int = Field(alias="cacheCreation5mTokens")
    cache_creation_1h_tokens: int = Field(alias="cacheCreation1hTokens")
    reasoning_tokens: int = Field(alias="reasoningTokens")
    web_search_requests: int = Field(alias="webSearchRequests")
    speed: Literal["standard", "fast"] | None
    fields_incomplete: bool = Field(alias="fieldsIncomplete")
    dedup_key: str = Field(alias="dedupKey")


class UsageResponse(_AliasModel):
    records: list[UsageRecordResponse]
    cursors: dict[str, FileFingerprintModel]
    has_more: bool = Field(alias="hasMore")
