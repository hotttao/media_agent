from __future__ import annotations

from collections.abc import Awaitable, Callable
from typing import Any

import pytest

from adapter.biz.domain.runtime_session.entity import Message, RuntimeSession, Turn, TurnStatus
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
        "session-1", second.turn_id, TurnStatus.FAILED, error_class="RuntimeError"
    )
    assert failed.next_turn is None
    assert repository.sessions["session-1"].status.value == "paused"
    assert repository.sessions["session-1"].runtime_session_id == "runtime-1"


@pytest.mark.asyncio
async def test_submit_turn_rejects_nested_runtime_credentials() -> None:
    service = RuntimeSessionService(FakeTransactionRunner(), FakeRepository())
    await service.register(RegisterRuntimeSession("session-1", 1, "context:v1"))

    with pytest.raises(InvalidTurn, match="must not contain runtime credentials"):
        await service.submit_turn(
            SubmitTurn("session-1", "secret", 1, {"message": "hi", "meta": {"api_key": "x"}})
        )
