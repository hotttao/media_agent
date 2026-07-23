"""ACP domain 实体 / 值对象。

对标 internal/types/events.ts 的 UniversalEvent / UniversalItem / TurnStats /
ContextGauge 和 idl/llm_event/llm_event.proto。domain 层不依赖 FastAPI /
SQLAlchemy，只用到 Pydantic 解析 HTTP 请求体；值对象用 dataclass 以便
translator 轻量构造。

AcpSession 运行时状态（busy_turns / last_active / pending_destroy）定义在
session.py，此处只保留值对象与请求体。
"""

from __future__ import annotations

from dataclasses import dataclass, field
from enum import StrEnum
from typing import Any, Literal

from pydantic import BaseModel, Field


# ── PromptResponse.stop_reason ────────────────────────────
# 对齐 SDK schema.StopReason = Literal["end_turn", "max_tokens",
# "max_turn_requests", "refusal", "cancelled"]（acp/schema.py:14）。用
# StrEnum 固化取值，便于比较与日志输出；bridge.prompt 返回时统一映射。
class StopReason(StrEnum):
    """PromptResponse.stop_reason 的取值（对齐 SDK schema.StopReason）。"""

    END_TURN = "end_turn"
    MAX_TOKENS = "max_tokens"
    MAX_TURN_REQUESTS = "max_turn_requests"
    REFUSAL = "refusal"
    CANCELLED = "cancelled"


# ── UniversalEvent 协议数据类 ────────────────────────────
# 对标 internal/types/events.ts 的 UniversalEvent / UniversalItem /
# ContentPart / ContentDelta。字段命名对齐 idl/llm_event/llm_event.proto
# （全 snake_case），供 AcpEventTranslator 产出后由 SessionSink 直接
# json.dumps 发送给前端 SSE。

SessionEndReason = Literal["completed", "error", "interrupted"]
"""session.ended 事件的终止原因。"""


ItemKind = Literal["message", "tool_call", "tool_result", "status"]
ItemRole = Literal["user", "assistant", "tool"]
ItemStatus = Literal["in_progress", "completed", "failed"]


@dataclass(slots=True)
class ContextGauge:
    """当前 context window 占用快照。对标 TS ContextGauge。"""

    num_turns: int = 0
    context_tokens: int = 0
    context_window: int = 0


@dataclass(slots=True)
class TurnStats:
    """单次 turn 的 token 计量与耗时。对标 TS TurnStats。

    内联 ContextGauge 字段（num_turns / context_tokens / context_window）
    避免继承 slots dataclass 的默认值冲突；字段命名对齐 protobuf IDL。
    """

    cost_usd: float = 0.0
    duration_ms: int = 0
    input_tokens: int = 0
    output_tokens: int = 0
    cache_read_tokens: int = 0
    cache_creation_tokens: int = 0
    num_turns: int = 0
    context_tokens: int = 0
    context_window: int = 0


@dataclass(slots=True)
class ContentDelta:
    """item.delta 事件的增量内容。对标 TS ContentDelta。"""

    type: Literal["text", "reasoning"]
    text: str


@dataclass(slots=True)
class ContentPart:
    """UniversalItem.content 的一个元素。对标 TS ContentPart。"""

    type: Literal["text", "tool_call", "tool_result", "reasoning", "status", "image"]
    text: str | None = None
    call_id: str | None = None
    name: str | None = None
    arguments: str | None = None
    output: str | None = None
    is_error: bool | None = None
    label: str | None = None
    detail: str | None = None
    data: str | None = None
    media_type: str | None = None


@dataclass(slots=True)
class UniversalItem:
    """一个通用事件项。对标 TS UniversalItem。"""

    item_id: str
    kind: ItemKind
    role: ItemRole | None = None
    status: ItemStatus = "in_progress"
    content: list[ContentPart] = field(default_factory=list)
    parent_tool_use_id: str | None = None


@dataclass(slots=True)
class UniversalEvent:
    """前端可消费的通用事件。对标 TS UniversalEvent。

    一个 ACP SessionUpdate 经 translator 翻译后可能产出 0 或多个
    UniversalEvent；SessionSink.write 将其 json.dumps 后发送给 SSE。
    """

    type: str
    timestamp: int
    session_id: str | None = None
    reason: SessionEndReason | None = None
    stats: TurnStats | None = None
    item: UniversalItem | None = None
    item_id: str | None = None
    delta: ContentDelta | None = None
    request_id: str | None = None
    questions: list[Any] | None = None
    message: str | None = None
    code: str | None = None


# ── bridge.prompt 返回值 ──────────────────────────────────


@dataclass(slots=True)
class PromptResult:
    """bridge.prompt 的返回值。

    stop_reason 对齐 SDK schema.StopReason；usage 是 SDK schema.Usage 原样
    透传（agent-specific 字段在 _meta 内，由 agent 包自行解释）。
    """

    stop_reason: StopReason
    usage: Any | None = None


# ── HTTP 请求体 ────────────────────────────────────────────


class ChatImageAttachment(BaseModel):
    """对标 ChatImageAttachment：base64 编码的图片附件。"""

    data: str = Field(..., description="base64 encoded image bytes")
    media_type: str = Field(..., description="image/png | image/jpeg | image/gif | image/webp")


class Chat(BaseModel):
    """POST /chat 请求体。

    session_id 缺省表示新会话；session_token 是 CP 反向解析会话身份的不透明
    令牌，透传到 MCP 服务器头部（X-Session-Token），由 CP 代理翻译为
    X-Tos-Session-Id。
    """

    message: str
    session_id: str | None = None
    images: list[ChatImageAttachment] | None = None
    session_token: str | None = None
