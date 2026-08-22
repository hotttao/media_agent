from __future__ import annotations

from collections.abc import Awaitable, Callable
from typing import Any

import pytest

from adapter.biz.domain.runtime_session.entity import (
    AgentRequest,
    Message,
    RuntimeSession,
    Turn,
    TurnStatus,
    UniversalEvent,
)
from adapter.biz.domain.runtime_session.errors import (
    ContextVersionConflict,
    InvalidTurn,
    RuntimeSessionNotFound,
)
from adapter.biz.service.runtime_session.command import RegisterRuntimeSession, SubmitTurn
from adapter.biz.service.runtime_session.service import RuntimeSessionService


class FakeTransactionRunner:
    calls = 0

    async def run_in_tx(self, operation: Callable[[], Awaitable[Any]]) -> Any:
        self.calls += 1
        return await operation()


class FakeRepository:
    def __init__(self) -> None:
        self.sessions: dict[str, RuntimeSession] = {}
        self.locked = False
        self.turns: dict[tuple[str, str], Turn] = {}
        self.messages: list[Message] = []
        self.events: list[UniversalEvent] = []
        self.requests: dict[str, AgentRequest] = {}

    async def get(self, session_id: str, *, for_update: bool = False) -> RuntimeSession | None:
        self.locked = self.locked or for_update
        return self.sessions.get(session_id)

    async def add(self, session: RuntimeSession) -> None:
        self.sessions[session.session_id] = session

    async def get_turn_by_idempotency_key(self, session_id: str, key: str) -> Turn | None:
        return self.turns.get((session_id, key))

    async def add_turn(self, turn: Turn, message: Message) -> None:
        self.turns[(turn.session_id, turn.idempotency_key)] = turn
        self.messages.append(message)

    async def save_submission(self, session: RuntimeSession) -> None:
        self.sessions[session.session_id] = session

    async def add_event(
        self, session: RuntimeSession, event: UniversalEvent, message: Message | None = None
    ) -> None:
        self.sessions[session.session_id] = session
        self.events.append(event)
        if message is not None:
            self.messages.append(message)

    async def find_completed_item(self, turn_id: str, item_id: str) -> UniversalEvent | None:
        return next(
            (event for event in self.events if _matches_completed_item(event, turn_id, item_id)),
            None,
        )

    async def list_events_after(
        self, session_id: str, after_sequence: int, limit: int
    ) -> list[UniversalEvent]:
        return [
            event
            for event in self.events
            if event.session_id == session_id and event.sequence > after_sequence
        ][:limit]

    async def list_messages(self, session_id: str, limit: int) -> list[Message]:
        return [message for message in self.messages if message.session_id == session_id][:limit]

    async def get_turn(self, turn_id: str, *, for_update: bool = False) -> Turn | None:
        self.locked = self.locked or for_update
        return next((turn for turn in self.turns.values() if turn.turn_id == turn_id), None)

    async def get_queue_head(self, session_id: str) -> Turn | None:
        queued = [
            turn
            for turn in self.turns.values()
            if turn.session_id == session_id and turn.status is TurnStatus.QUEUED
        ]
        return min(queued, key=lambda turn: turn.enqueue_sequence, default=None)

    async def save_execution(self, session: RuntimeSession, turn: Turn | None = None) -> None:
        self.sessions[session.session_id] = session
        if turn is not None:
            self.turns[(turn.session_id, turn.idempotency_key)] = turn

    async def add_agent_request(
        self, session: RuntimeSession, request: AgentRequest, event: UniversalEvent
    ) -> None:
        self.sessions[session.session_id] = session
        self.requests[request.request_id] = request
        self.events.append(event)

    async def get_agent_request(
        self, request_id: str, *, for_update: bool = False
    ) -> AgentRequest | None:
        self.locked = self.locked or for_update
        return self.requests.get(request_id)

    async def save_agent_request(
        self, session: RuntimeSession, request: AgentRequest, event: UniversalEvent
    ) -> None:
        self.sessions[session.session_id] = session
        self.requests[request.request_id] = request
        self.events.append(event)

    async def list_agent_requests(
        self, session_id: str, turn_id: str | None = None
    ) -> list[AgentRequest]:
        return [
            request
            for request in self.requests.values()
            if request.session_id == session_id and (turn_id is None or request.turn_id == turn_id)
        ]

    async def save_agent_request_state(self, request: AgentRequest) -> None:
        self.requests[request.request_id] = request

    async def list_running_turns(self) -> list[Turn]:
        return [turn for turn in self.turns.values() if turn.status is TurnStatus.RUNNING]


def _matches_completed_item(event: UniversalEvent, turn_id: str, item_id: str) -> bool:
    item = event.payload.get("item")
    return (
        event.turn_id == turn_id
        and event.event_type == "item.completed"
        and isinstance(item, dict)
        and item.get("item_id") == item_id
    )


@pytest.mark.asyncio
async def test_register_is_idempotent_for_same_context_version() -> None:
    tx = FakeTransactionRunner()
    repository = FakeRepository()
    service = RuntimeSessionService(tx, repository)
    command = RegisterRuntimeSession("session-1", 2, "context:v2")

    first = await service.register(command)
    second = await service.register(command)

    assert second is first
    assert len(repository.sessions) == 1
    assert repository.locked
    assert tx.calls == 2


@pytest.mark.asyncio
async def test_register_rejects_context_version_conflict() -> None:
    service = RuntimeSessionService(FakeTransactionRunner(), FakeRepository())
    await service.register(RegisterRuntimeSession("session-1", 2, "context:v2"))

    with pytest.raises(ContextVersionConflict, match="already uses context version 2"):
        await service.register(RegisterRuntimeSession("session-1", 3, "context:v3"))


@pytest.mark.asyncio
async def test_get_reports_missing_session() -> None:
    tx = FakeTransactionRunner()
    service = RuntimeSessionService(tx, FakeRepository())

    with pytest.raises(RuntimeSessionNotFound, match="session-404"):
        await service.get("session-404")
    assert tx.calls == 1


@pytest.mark.asyncio
async def test_submit_turn_is_idempotent_and_queues_fifo() -> None:
    repository = FakeRepository()
    service = RuntimeSessionService(FakeTransactionRunner(), repository)
    await service.register(RegisterRuntimeSession("session-1", 2, "context:v2"))

    first_command = SubmitTurn("session-1", "request-1", 2, {"text": "one"})
    first = await service.submit_turn(first_command)
    repeated = await service.submit_turn(first_command)
    second = await service.submit_turn(SubmitTurn("session-1", "request-2", 2, {"text": "two"}))

    assert first == repeated
    assert first.status is TurnStatus.RUNNING
    assert second.status is TurnStatus.QUEUED
    assert (first.enqueue_sequence, second.enqueue_sequence) == (1, 2)
    assert len(repository.messages) == 2
    assert repository.sessions["session-1"].active_turn_id == first.turn_id


@pytest.mark.asyncio
async def test_finalize_completed_claims_next_but_failure_pauses_queue() -> None:
    repository = FakeRepository()
    service = RuntimeSessionService(FakeTransactionRunner(), repository)
    await service.register(RegisterRuntimeSession("session-1", 1, "context:v1"))
    first = await service.submit_turn(SubmitTurn("session-1", "one", 1, {"message": "one"}))
    second = await service.submit_turn(SubmitTurn("session-1", "two", 1, {"message": "two"}))
    await service.attach_runtime("session-1", first.turn_id, "runtime-1")

    completed = await service.finalize_turn("session-1", first.turn_id, TurnStatus.COMPLETED)
    assert completed.next_turn is not None
    assert completed.next_turn.turn_id == second.turn_id
    assert completed.next_turn.status is TurnStatus.RUNNING

    failed = await service.finalize_turn(
        "session-1",
        second.turn_id,
        TurnStatus.FAILED,
        error_class="RuntimeError",
        terminal_payload={"type": "session.ended", "reason": "completed"},
    )
    assert failed.next_turn is None
    assert repository.sessions["session-1"].status.value == "paused"
    assert repository.sessions["session-1"].runtime_session_id == "runtime-1"
    assert failed.events[0].payload["reason"] == "error"


@pytest.mark.asyncio
async def test_runtime_events_allocate_independent_event_and_message_sequences() -> None:
    repository = FakeRepository()
    service = RuntimeSessionService(FakeTransactionRunner(), repository)
    await service.register(RegisterRuntimeSession("session-1", 1, "context:v1"))
    turn = await service.submit_turn(SubmitTurn("session-1", "one", 1, {"message": "hello"}))
    assistant: dict[str, object] = {
        "type": "item.completed",
        "item": {
            "item_id": "message-1",
            "kind": "message",
            "role": "assistant",
            "content": [{"type": "text", "text": "world"}],
        },
    }
    tool_call: dict[str, object] = {
        "type": "item.completed",
        "item": {
            "item_id": "tool-call-1",
            "kind": "tool_call",
            "content": [{"type": "tool_call", "call_id": "call-1"}],
        },
    }

    assistant_event = await service.persist_runtime_event("session-1", turn.turn_id, assistant)
    first_tool_event = await service.persist_runtime_event("session-1", turn.turn_id, tool_call)
    repeated_tool_event = await service.persist_runtime_event("session-1", turn.turn_id, tool_call)

    assert repeated_tool_event == first_tool_event
    assert assistant_event.sequence + 1 == first_tool_event.sequence
    assert [message.sequence for message in await service.list_messages("session-1")] == [1, 2]
    assert [
        event.sequence
        for event in await service.replay_events("session-1", assistant_event.sequence)
    ] == [first_tool_event.sequence]


@pytest.mark.asyncio
async def test_submit_turn_rejects_nested_runtime_credentials() -> None:
    service = RuntimeSessionService(FakeTransactionRunner(), FakeRepository())
    await service.register(RegisterRuntimeSession("session-1", 1, "context:v1"))

    with pytest.raises(InvalidTurn, match="must not contain runtime credentials"):
        await service.submit_turn(
            SubmitTurn("session-1", "secret", 1, {"message": "hi", "meta": {"api_key": "x"}})
        )


@pytest.mark.asyncio
async def test_agent_request_open_and_resolve_are_durable_and_idempotent() -> None:
    repository = FakeRepository()
    service = RuntimeSessionService(FakeTransactionRunner(), repository)
    await service.register(RegisterRuntimeSession("session-1", 1, "context:v1"))
    turn = await service.submit_turn(SubmitTurn("session-1", "one", 1, {"message": "hi"}))

    opened = await service.open_agent_request(
        "session-1",
        turn.turn_id,
        "permission",
        {"tool_call": {"name": "shell"}, "options": [{"option_id": "allow-once"}]},
    )
    resolved = await service.resolve_agent_request(
        "session-1",
        turn.turn_id,
        opened.request.request_id,
        {"option_id": "allow-once"},
    )
    repeated = await service.resolve_agent_request(
        "session-1",
        turn.turn_id,
        opened.request.request_id,
        {"option_id": "allow-once"},
    )

    assert opened.request.status.value == "pending"
    assert resolved.request.status.value == "resolved"
    assert repeated.request == resolved.request
    assert repeated.event is None
    assert [event.event_type for event in repository.events[-2:]] == [
        "agent.requested",
        "agent.request.resolved",
    ]


@pytest.mark.asyncio
async def test_agent_request_conflicting_or_late_response_is_rejected() -> None:
    repository = FakeRepository()
    service = RuntimeSessionService(FakeTransactionRunner(), repository)
    await service.register(RegisterRuntimeSession("session-1", 1, "context:v1"))
    turn = await service.submit_turn(SubmitTurn("session-1", "one", 1, {"message": "hi"}))
    opened = await service.open_agent_request(
        "session-1", turn.turn_id, "confirm", {"question": "continue?"}
    )
    await service.resolve_agent_request(
        "session-1", turn.turn_id, opened.request.request_id, {"confirmed": True}
    )
    with pytest.raises(InvalidTurn, match="different response"):
        await service.resolve_agent_request(
            "session-1", turn.turn_id, opened.request.request_id, {"confirmed": False}
        )

    late = await service.open_agent_request("session-1", turn.turn_id, "input", {"prompt": "value"})
    await service.finalize_turn("session-1", turn.turn_id, TurnStatus.FAILED)
    with pytest.raises(InvalidTurn, match="not pending: cancelled"):
        await service.resolve_agent_request(
            "session-1", turn.turn_id, late.request.request_id, {"value": "late"}
        )


@pytest.mark.asyncio
async def test_late_runtime_event_does_not_mutate_terminal_turn_history() -> None:
    repository = FakeRepository()
    service = RuntimeSessionService(FakeTransactionRunner(), repository)
    await service.register(RegisterRuntimeSession("session-1", 1, "context:v1"))
    turn = await service.submit_turn(SubmitTurn("session-1", "one", 1, {"message": "hi"}))
    await service.finalize_turn("session-1", turn.turn_id, TurnStatus.FAILED)
    event_count = len(repository.events)

    with pytest.raises(InvalidTurn, match="late Runtime event"):
        await service.persist_runtime_event(
            "session-1",
            turn.turn_id,
            {"type": "item.completed", "item": {"type": "agent_message", "text": "late"}},
        )

    assert len(repository.events) == event_count
