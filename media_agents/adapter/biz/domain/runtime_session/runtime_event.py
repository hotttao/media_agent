"""Runtime event classification and durable payload normalization."""

from __future__ import annotations

from copy import deepcopy
from dataclasses import dataclass

from adapter.biz.domain.runtime_session.errors import InvalidTurn

DEFAULT_MAX_TOOL_RESULT_BYTES = 32 * 1024


@dataclass(frozen=True, slots=True)
class RuntimeEventEnvelope:
    payload: dict[str, object]
    event_type: str
    item_id: str
    item_kind: str
    call_id: str

    @classmethod
    def parse(cls, payload: dict[str, object]) -> RuntimeEventEnvelope:
        event_type = _name(payload.get("type"))
        if not event_type:
            raise InvalidTurn("Runtime event type is required")
        item_value = payload.get("item")
        item = item_value if isinstance(item_value, dict) else {}
        item_id = str(item.get("item_id") or payload.get("item_id") or "")
        item_kind = _name(item.get("kind"))
        call_id = ""
        content = item.get("content")
        if isinstance(content, list):
            for part in content:
                if isinstance(part, dict) and part.get("call_id"):
                    call_id = str(part["call_id"])
                    break
        return cls(payload, event_type, item_id, item_kind, call_id)

    @property
    def is_live(self) -> bool:
        return self.event_type in {"item.started", "item.delta"}

    @property
    def is_terminal(self) -> bool:
        return self.event_type == "session.ended"

    @property
    def is_tool_result(self) -> bool:
        return self.event_type == "item.completed" and self.item_kind == "tool_result"

    @property
    def is_tool_call(self) -> bool:
        return self.event_type == "item.completed" and self.item_kind == "tool_call"

    @property
    def creates_message(self) -> bool:
        return self.event_type == "item.completed" and self.item_kind in {
            "message",
            "tool_result",
        }

    @property
    def message_role(self) -> str:
        item = self.payload.get("item")
        role = _name(item.get("role")) if isinstance(item, dict) else ""
        if role in {"user", "assistant", "tool", "system"}:
            return role
        return "tool" if self.item_kind == "tool_result" else "assistant"

    @property
    def message_content(self) -> dict[str, object]:
        item = self.payload.get("item")
        if not isinstance(item, dict):
            raise InvalidTurn("completed Message event has no item")
        return deepcopy(item)


def normalize_tool_result(
    payload: dict[str, object], max_bytes: int = DEFAULT_MAX_TOOL_RESULT_BYTES
) -> dict[str, object]:
    limit = max_bytes if max_bytes > 0 else DEFAULT_MAX_TOOL_RESULT_BYTES
    normalized = deepcopy(payload)
    envelope = RuntimeEventEnvelope.parse(normalized)
    if not envelope.is_tool_result:
        raise InvalidTurn("event is not a completed tool result")
    item = normalized.get("item")
    content = item.get("content") if isinstance(item, dict) else None
    if not isinstance(content, list):
        raise InvalidTurn("tool result item has no content")
    for part in content:
        if not isinstance(part, dict) or _name(part.get("type")) != "tool_result":
            continue
        output = str(part.get("output") or "").replace("\x00", "\ufffd")
        original_bytes = len(output.encode("utf-8"))
        stored, truncated = _truncate_utf8(output, limit)
        part["output"] = stored
        part["truncated"] = truncated
        part["original_bytes"] = original_bytes
        part["stored_bytes"] = len(stored.encode("utf-8"))
    return normalized


def _truncate_utf8(value: str, max_bytes: int) -> tuple[str, bool]:
    encoded = value.encode("utf-8")
    if len(encoded) <= max_bytes:
        return value, False
    marker = (
        f"\n...[truncated: {len(encoded) - max_bytes} bytes omitted, "
        f"original {len(encoded)} bytes]...\n"
    )
    marker_bytes = marker.encode("utf-8")
    if len(marker_bytes) >= max_bytes:
        return _valid_prefix(encoded, max_bytes), True
    remaining = max_bytes - len(marker_bytes)
    head_budget = remaining * 3 // 4
    tail_budget = remaining - head_budget
    head = _valid_prefix(encoded, head_budget)
    tail = _valid_suffix(encoded, tail_budget)
    result = head + marker + tail
    return _valid_prefix(result.encode("utf-8"), max_bytes), True


def _valid_prefix(value: bytes, limit: int) -> str:
    return value[:limit].decode("utf-8", errors="ignore")


def _valid_suffix(value: bytes, limit: int) -> str:
    return value[-limit:].decode("utf-8", errors="ignore") if limit > 0 else ""


def _name(value: object) -> str:
    return str(value or "").strip().lower().replace("-", "_")
