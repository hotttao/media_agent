from __future__ import annotations

import asyncio
from collections.abc import AsyncIterator
from datetime import UTC, datetime

import pytest

from adapter.biz.domain.runtime_session.entity import UniversalEvent
from adapter.biz.domain.runtime_session.event_bus import SessionEventNotification
from adapter.biz.workflow.turn_event_sink import TurnEventSink
from adapter.session_event_bus import InMemorySessionEventBus


class EventStore:
    def __init__(self) -> None:
        self.payloads: list[dict[str, object]] = []

    async def persist_runtime_event(
        self, session_id: str, turn_id: str, payload: dict[str, object]
    ) -> UniversalEvent:
        self.payloads.append(payload)
        return UniversalEvent(
            event_id=f"event-{len(self.payloads)}",
            session_id=session_id,
            turn_id=turn_id,
            sequence=len(self.payloads),
            event_type=str(payload["type"]),
            payload=payload,
            created_at=datetime.now(UTC),
        )


class FailingBus:
    async def publish(self, notification: SessionEventNotification) -> None:
        raise ConnectionError("RabbitMQ unavailable")

    def subscribe(self, session_id: str) -> AsyncIterator[SessionEventNotification]:
        raise NotImplementedError("publish-failure fixture has no subscriber")

    async def close(self) -> None:
        pass


def _tool_result(output: str) -> dict[str, object]:
    return {
        "type": "item.completed",
        "item": {
            "item_id": "result-1",
            "kind": "tool_result",
            "content": [{"type": "tool_result", "call_id": "call-1", "output": output}],
        },
    }


def _result_part(payload: dict[str, object]) -> dict[str, object]:
    item = payload.get("item")
    assert isinstance(item, dict)
    content = item.get("content")
    assert isinstance(content, list) and content
    part = content[0]
    assert isinstance(part, dict)
    return part


@pytest.mark.asyncio
async def test_sink_keeps_live_frames_transient_and_flushes_latest_tool_snapshot() -> None:
    store = EventStore()
    bus = InMemorySessionEventBus()
    sink = TurnEventSink(
        store,
        bus,
        "session-1",
        "turn-1",
        tool_result_max_bytes=64,
        tool_result_window_ms=10_000,
    )
    received: list[SessionEventNotification] = []

    async def consume() -> None:
        async for notification in bus.subscribe("session-1"):
            received.append(notification)
            if len(received) == 3:
                return

    subscriber = asyncio.create_task(consume())
    await asyncio.sleep(0)
    await sink.accept({"type": "item.delta", "delta": "中"})
    await sink.accept(_tool_result("first"))
    await sink.accept(_tool_result("middle"))
    await sink.accept(_tool_result("末" * 100))
    await sink.accept({"type": "session.ended", "reason": "completed"})
    await sink.flush()
    await asyncio.wait_for(subscriber, timeout=1)

    assert len(store.payloads) == 2
    assert [notification.sequence for notification in received] == [0, 1, 2]
    first_part = _result_part(store.payloads[0])
    last_part = _result_part(store.payloads[1])
    assert first_part["output"] == "first"
    assert last_part["truncated"] is True
    last_output = last_part["output"]
    assert isinstance(last_output, str)
    assert len(last_output.encode("utf-8")) <= 64
    assert sink.terminal_payload == {"type": "session.ended", "reason": "completed"}


@pytest.mark.asyncio
async def test_two_subscribers_receive_the_same_notification() -> None:
    bus = InMemorySessionEventBus()

    async def receive_one() -> SessionEventNotification:
        async for notification in bus.subscribe("session-1"):
            return notification
        raise AssertionError("subscription stopped")

    first = asyncio.create_task(receive_one())
    second = asyncio.create_task(receive_one())
    await asyncio.sleep(0)
    expected = SessionEventNotification("session-1", 9, {"type": "item.completed"})
    await bus.publish(expected)

    assert await asyncio.wait_for(first, timeout=1) == expected
    assert await asyncio.wait_for(second, timeout=1) == expected


@pytest.mark.asyncio
async def test_publish_failure_does_not_rollback_persisted_event() -> None:
    store = EventStore()
    sink = TurnEventSink(store, FailingBus(), "session-1", "turn-1")

    await sink.accept(
        {
            "type": "item.completed",
            "item": {
                "item_id": "message-1",
                "kind": "message",
                "content": [{"type": "text", "text": "durable"}],
            },
        }
    )

    assert len(store.payloads) == 1
