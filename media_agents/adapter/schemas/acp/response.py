"""ACP agent API response DTOs (Pydantic v2)。

对标 internal/types/events.ts 的 AgentCapabilities 与 /info 端点响应。仅 biz/api/**
可 import。
"""

from __future__ import annotations

from pydantic import BaseModel, Field


class AgentCapabilities(BaseModel):
    """对标 internal/types/events.ts AgentCapabilities。声明 agent 能力。"""

    system_prompt: bool
    mcp: bool
    skills: bool
    questions: bool
    reconnect: bool
    permissions: bool
    streaming_deltas: bool


class HealthResponse(BaseModel):
    status: str = "ok"
    reason: str | None = None


class AgentInfo(BaseModel):
    """GET /info 响应。对标 acp-server.ts /info。"""

    agent_type: str
    model: str = "default"
    capabilities: AgentCapabilities


class InterruptResponse(BaseModel):
    success: bool
    interrupted: bool = False


class ReloadConfigResponse(BaseModel):
    success: bool
    skills_failed: list[str] = Field(default_factory=list)
    error: str | None = None


class ResetWorkspaceResponse(BaseModel):
    success: bool
    removed: list[str] = Field(default_factory=list)
    error: str | None = None


class PendingQuestionResponse(BaseModel):
    """GET /sessions/:id/pending-question 的 stub 响应（ACP 用 permissions，不用 ASQ）。"""

    root: None = None
