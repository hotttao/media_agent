"""Universal LLM event protocol values.

Python counterpart of ``internal/types/events.ts`` for the event stream between
agent adapters and the control plane/web UI.
"""

from __future__ import annotations

from dataclasses import asdict, dataclass, field, is_dataclass
from typing import Any, Literal, cast

SessionEndReason = Literal["completed", "error", "interrupted"]
"""session.ended 事件的终止原因。"""

ItemKind = Literal["message", "tool_call", "tool_result", "status"]
ItemRole = Literal["user", "assistant", "tool"]
ItemStatus = Literal["in_progress", "completed", "failed"]


@dataclass(frozen=True, slots=True)
class ChatImageAttachment:
    """对标 ChatImageAttachment：base64 编码的图片附件。"""

    data: str
    media_type: str


@dataclass(slots=True)
class ContextGauge:
    """当前 context window 占用快照。对标 TS ContextGauge。"""

    num_turns: int = 0
    context_tokens: int = 0
    context_window: int = 0


@dataclass(slots=True)
class TurnStats:
    """单次 turn 的 token 计量与耗时。对标 TS TurnStats。"""

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
    """前端可消费的通用事件。对标 TS UniversalEvent。"""

    type: str
    timestamp: int
    session_id: str | None = None
    reason: SessionEndReason | None = None
    stats: TurnStats | None = None
    item: UniversalItem | None = None
    item_id: str | None = None
    delta: ContentDelta | None = None
    request_id: str | None = None
    questions: list[object] | None = None
    message: str | None = None
    code: str | None = None


def to_event_dict(event: UniversalEvent) -> dict[str, object]:
    """Convert a UniversalEvent dataclass into compact JSON-ready data."""

    def compact(value: object) -> object:
        if is_dataclass(value):
            value = asdict(cast(Any, value))
        if isinstance(value, dict):
            return {key: compact(item) for key, item in value.items() if item is not None}
        if isinstance(value, list):
            return [compact(item) for item in value]
        return value

    data = compact(event)
    return cast(dict[str, object], data) if isinstance(data, dict) else {}
