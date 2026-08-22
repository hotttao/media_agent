from __future__ import annotations

from dataclasses import replace
from datetime import UTC, datetime

import pytest

from adapter.biz.domain.runtime_session.entity import RuntimeSession, Turn, TurnStatus
from adapter.biz.service.runtime_session.command import SubmitTurn
from adapter.biz.service.runtime_session.service import TurnExecution, TurnFinalization
from adapter.biz.workflow.execute_turn_flow import (
    ExecuteTurnFlow,
    RuntimeCredentials,
    RuntimePreparation,
    RuntimeResult,
)


def _turn(turn_id: str, sequence: int, status: TurnStatus) -> Turn:
    return Turn(
        turn_id=turn_id,
        session_id="session-1",
        idempotency_key=turn_id,
        enqueue_sequence=sequence,
        status=status,
        input={"message": turn_id},
        context_version=1,
        created_at=datetime(2026, 8, 22, tzinfo=UTC),
        started_at=datetime(2026, 8, 22, tzinfo=UTC) if status is TurnStatus.RUNNING else None,
    )


class ScriptService:
    def __init__(self) -> None:
        self.session = RuntimeSession.register(
            session_id="session-1", context_version=1, context_ref="context:v1"
        )
        self.turns = [_turn("turn-1", 1, TurnStatus.RUNNING), _turn("turn-2", 2, TurnStatus.QUEUED)]
        self.session = replace(self.session, active_turn_id="turn-1")
        self.finalized: list[tuple[str, TurnStatus]] = []

    async def submit_turn(self, command: SubmitTurn) -> Turn:
        return self.turns[0]

    async def execution(self, session_id: str, turn_id: str) -> TurnExecution:
        turn = next(turn for turn in self.turns if turn.turn_id == turn_id)
        return TurnExecution(self.session, turn)

    async def attach_runtime(self, session_id: str, turn_id: str, handle: str) -> RuntimeSession:
        self.session = self.session.attach_runtime(handle, now=datetime.now(UTC))
        return self.session

    async def finalize_turn(
        self,
        session_id: str,
        turn_id: str,
        status: TurnStatus,
        *,
        error_class: str | None = None,
    ) -> TurnFinalization:
        self.finalized.append((turn_id, status))
        terminal = next(turn for turn in self.turns if turn.turn_id == turn_id)
        terminal = replace(terminal, status=status)
        if turn_id == "turn-1" and status is TurnStatus.COMPLETED:
            self.turns[1] = replace(self.turns[1], status=TurnStatus.RUNNING)
            self.session = replace(self.session, active_turn_id="turn-2")
            return TurnFinalization(terminal, self.turns[1])
        self.session = replace(self.session, active_turn_id=None)
        return TurnFinalization(terminal, None)


class ScriptRuntime:
    def __init__(self, statuses: list[TurnStatus]) -> None:
        self.statuses = statuses
        self.calls: list[str] = []
        self.interrupted = False

    async def prepare(
        self, session: RuntimeSession, turn: Turn, credentials: RuntimeCredentials
    ) -> RuntimePreparation:
        self.calls.append(f"prepare:{turn.turn_id}")
        return RuntimePreparation(
            session.runtime_session_id or "runtime-1", session.runtime_session_id is None
        )

    async def execute(
        self,
        preparation: RuntimePreparation,
        session: RuntimeSession,
        turn: Turn,
        credentials: RuntimeCredentials,
    ) -> RuntimeResult:
        self.calls.append(f"execute:{turn.turn_id}")
        return RuntimeResult(self.statuses.pop(0))

    async def interrupt(self, handle: str) -> bool:
        self.interrupted = True
        return True


@pytest.mark.asyncio
async def test_flow_executes_outside_state_transactions_and_drains_fifo() -> None:
    service = ScriptService()
    runtime = ScriptRuntime([TurnStatus.COMPLETED, TurnStatus.COMPLETED])
    flow = ExecuteTurnFlow(service, runtime)

    await flow.submit(SubmitTurn("session-1", "one", 1, {"message": "one"}))
    await flow.wait("session-1")

    assert runtime.calls == ["prepare:turn-1", "execute:turn-1", "prepare:turn-2", "execute:turn-2"]
    assert service.finalized == [
        ("turn-1", TurnStatus.COMPLETED),
        ("turn-2", TurnStatus.COMPLETED),
    ]
    assert service.session.runtime_session_id == "runtime-1"


@pytest.mark.asyncio
async def test_flow_maps_runtime_failure_and_interrupts_by_persisted_handle() -> None:
    service = ScriptService()
    runtime = ScriptRuntime([TurnStatus.FAILED])
    flow = ExecuteTurnFlow(service, runtime)
    service.session = service.session.attach_runtime("runtime-1", now=datetime.now(UTC))

    assert await flow.interrupt("session-1", "turn-1")
    assert runtime.interrupted
    await flow.submit(SubmitTurn("session-1", "one", 1, {"message": "one"}))
    await flow.wait("session-1")
    assert service.finalized == [("turn-1", TurnStatus.FAILED)]
