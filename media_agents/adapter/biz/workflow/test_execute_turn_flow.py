from __future__ import annotations

import asyncio
from dataclasses import replace
from datetime import UTC, datetime

import pytest

from adapter.biz.domain.runtime_session.entity import (
    AgentRequest,
    AgentRequestStatus,
    RuntimeSession,
    Turn,
    TurnStatus,
    UniversalEvent,
)
from adapter.biz.domain.runtime_session.errors import ExecutionCredentialsDeferred
from adapter.biz.service.runtime_session.command import SubmitTurn
from adapter.biz.service.runtime_session.service import (
    AgentRequestChange,
    TurnExecution,
    TurnFinalization,
    TurnSubmission,
)
from adapter.biz.workflow.execute_turn_flow import (
    AgentRequestHandler,
    ExecuteTurnFlow,
    RuntimeCredentials,
    RuntimePreparation,
    RuntimeResult,
)
from adapter.biz.workflow.turn_event_sink import RuntimeEventHandler, TurnEventSink
from adapter.session_event_bus import InMemorySessionEventBus


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
        self.terminal_turns: list[Turn] = []
        self.request_opened = asyncio.Event()
        self.agent_request: AgentRequest | None = None

    async def submit_turn_details(self, command: SubmitTurn) -> TurnSubmission:
        return TurnSubmission(self.turns[0], ())

    async def persist_runtime_event(
        self, session_id: str, turn_id: str, payload: dict[str, object]
    ) -> UniversalEvent:
        raise AssertionError("script runtime emitted an unexpected event")

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
        terminal_payload: dict[str, object] | None = None,
    ) -> TurnFinalization:
        self.finalized.append((turn_id, status))
        terminal = next(turn for turn in self.turns if turn.turn_id == turn_id)
        terminal = replace(terminal, status=status, error_class=error_class)
        self.terminal_turns.append(terminal)
        if turn_id == "turn-1" and status is TurnStatus.COMPLETED:
            self.turns[1] = replace(self.turns[1], status=TurnStatus.RUNNING)
            self.session = replace(self.session, active_turn_id="turn-2")
            return TurnFinalization(terminal, self.turns[1])
        self.session = replace(self.session, active_turn_id=None)
        return TurnFinalization(terminal, None)

    async def open_agent_request(
        self, session_id: str, turn_id: str, request_type: str, payload: dict[str, object]
    ) -> AgentRequestChange:
        self.agent_request = AgentRequest(
            "request-1",
            session_id,
            turn_id,
            request_type,
            payload,
            AgentRequestStatus.PENDING,
            None,
            None,
            datetime.now(UTC),
        )
        event = UniversalEvent(
            "event-request",
            session_id,
            turn_id,
            1,
            "agent.requested",
            {"type": "agent.requested", "request_id": "request-1"},
            datetime.now(UTC),
        )
        self.request_opened.set()
        return AgentRequestChange(self.agent_request, event)

    async def resolve_agent_request(
        self, session_id: str, turn_id: str, request_id: str, response: dict[str, object]
    ) -> AgentRequestChange:
        assert self.agent_request is not None
        self.agent_request = self.agent_request.resolve(response, now=datetime.now(UTC))
        event = UniversalEvent(
            "event-resolve",
            session_id,
            turn_id,
            2,
            "agent.request.resolved",
            {"type": "agent.request.resolved", "request_id": request_id},
            datetime.now(UTC),
        )
        return AgentRequestChange(self.agent_request, event)

    async def running_turns(self) -> list[Turn]:
        return [turn for turn in self.turns if turn.status is TurnStatus.RUNNING]

    async def get_turn(self, turn_id: str) -> Turn:
        return next(turn for turn in self.turns if turn.turn_id == turn_id)


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
        event_handler: RuntimeEventHandler,
        request_handler: AgentRequestHandler,
    ) -> RuntimeResult:
        self.calls.append(f"execute:{turn.turn_id}")
        return RuntimeResult(self.statuses.pop(0))

    async def interrupt(self, handle: str) -> bool:
        self.interrupted = True
        return True


class RequestRuntime(ScriptRuntime):
    def __init__(self) -> None:
        super().__init__([TurnStatus.COMPLETED])
        self.response: dict[str, object] | None = None

    async def execute(
        self,
        preparation: RuntimePreparation,
        session: RuntimeSession,
        turn: Turn,
        credentials: RuntimeCredentials,
        event_handler: RuntimeEventHandler,
        request_handler: AgentRequestHandler,
    ) -> RuntimeResult:
        self.response = await request_handler(
            "permission", {"options": [{"option_id": "allow-once"}]}
        )
        return RuntimeResult(TurnStatus.FAILED, "test_stop")


@pytest.mark.asyncio
async def test_flow_executes_outside_state_transactions_and_drains_fifo() -> None:
    service = ScriptService()
    runtime = ScriptRuntime([TurnStatus.COMPLETED, TurnStatus.COMPLETED])
    bus = InMemorySessionEventBus()
    flow = ExecuteTurnFlow(
        service,
        runtime,
        lambda session_id, turn_id: TurnEventSink(service, bus, session_id, turn_id),
    )

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
    bus = InMemorySessionEventBus()
    flow = ExecuteTurnFlow(
        service,
        runtime,
        lambda session_id, turn_id: TurnEventSink(service, bus, session_id, turn_id),
    )
    service.session = service.session.attach_runtime("runtime-1", now=datetime.now(UTC))

    assert await flow.interrupt("session-1", "turn-1")
    assert runtime.interrupted
    await flow.submit(SubmitTurn("session-1", "one", 1, {"message": "one"}))
    await flow.wait("session-1")
    assert service.finalized == [("turn-1", TurnStatus.FAILED)]


@pytest.mark.asyncio
async def test_flow_waits_for_durable_agent_request_resolution() -> None:
    service = ScriptService()
    runtime = RequestRuntime()
    bus = InMemorySessionEventBus()
    flow = ExecuteTurnFlow(
        service,
        runtime,
        lambda session_id, turn_id: TurnEventSink(service, bus, session_id, turn_id),
    )
    await flow.submit(SubmitTurn("session-1", "one", 1, {"message": "one"}))
    await asyncio.wait_for(service.request_opened.wait(), timeout=1)
    assert service.finalized == []
    request = service.agent_request
    assert request is not None and request.status is AgentRequestStatus.PENDING

    resolved = await flow.resolve_request(
        "session-1", "turn-1", request.request_id, {"option_id": "allow-once"}
    )
    await flow.wait("session-1")

    assert resolved.status is AgentRequestStatus.RESOLVED
    assert runtime.response == {"option_id": "allow-once"}
    assert service.finalized == [("turn-1", TurnStatus.FAILED)]


@pytest.mark.asyncio
async def test_recovery_fails_outcome_unknown_turn_without_reexecution() -> None:
    service = ScriptService()
    runtime = ScriptRuntime([])
    bus = InMemorySessionEventBus()
    flow = ExecuteTurnFlow(
        service,
        runtime,
        lambda session_id, turn_id: TurnEventSink(service, bus, session_id, turn_id),
    )

    recovered = await flow.recover()

    assert len(recovered) == 1
    assert recovered[0].status is TurnStatus.FAILED
    assert recovered[0].error_class == "recovery_outcome_unknown"
    assert runtime.calls == []
    assert service.finalized == [("turn-1", TurnStatus.FAILED)]


@pytest.mark.asyncio
async def test_each_claim_refetches_credentials_including_queued_turn() -> None:
    service = ScriptService()
    runtime = ScriptRuntime([TurnStatus.COMPLETED, TurnStatus.COMPLETED])
    bus = InMemorySessionEventBus()
    claims: list[tuple[str, int]] = []

    async def credentials(session_id: str, turn_id: str, epoch: int) -> RuntimeCredentials:
        claims.append((turn_id, epoch))
        return RuntimeCredentials(api_key="fresh-key", session_token="fresh-session-token")

    flow = ExecuteTurnFlow(
        service,
        runtime,
        lambda session_id, turn_id: TurnEventSink(service, bus, session_id, turn_id),
        credential_resolver=credentials,
    )
    await flow.submit(
        SubmitTurn("session-1", "first", 1, {"message": "first"}),
        RuntimeCredentials(api_key="stale-key"),
    )
    await flow.wait("session-1")
    assert claims == [("turn-1", 1), ("turn-2", 1)]
    assert runtime.calls == ["prepare:turn-1", "execute:turn-1", "prepare:turn-2", "execute:turn-2"]
    assert all(
        "key" not in turn.input and "session_token" not in turn.input for turn in service.turns
    )


@pytest.mark.asyncio
async def test_interrupt_during_credential_retry_never_starts_native_execution() -> None:
    service = ScriptService()
    runtime = ScriptRuntime([])
    bus = InMemorySessionEventBus()
    attempted = asyncio.Event()

    async def credentials(session_id: str, turn_id: str, epoch: int) -> RuntimeCredentials:
        attempted.set()
        raise ExecutionCredentialsDeferred()

    flow = ExecuteTurnFlow(
        service,
        runtime,
        lambda session_id, turn_id: TurnEventSink(service, bus, session_id, turn_id),
        credential_resolver=credentials,
    )
    await flow.submit(SubmitTurn("session-1", "first", 1, {"message": "first"}))
    await asyncio.wait_for(attempted.wait(), 1)
    assert await flow.interrupt("session-1", "turn-1")
    await asyncio.wait_for(flow.wait("session-1"), 1)
    assert runtime.calls == []
    assert service.finalized == [("turn-1", TurnStatus.INTERRUPTED)]


@pytest.mark.asyncio
async def test_model_authentication_failure_pauses_without_running_the_queue() -> None:
    from adapter.biz.domain.acp_adapter.errors import AuthenticationRequired

    class UnauthenticatedRuntime(ScriptRuntime):
        async def prepare(
            self, session: RuntimeSession, turn: Turn, credentials: RuntimeCredentials
        ) -> RuntimePreparation:
            raise AuthenticationRequired("Model authentication is required")

    service = ScriptService()
    bus = InMemorySessionEventBus()
    flow = ExecuteTurnFlow(
        service, UnauthenticatedRuntime([]), lambda sid, tid: TurnEventSink(service, bus, sid, tid)
    )
    await flow.submit(SubmitTurn("session-1", "one", 1, {"message": "one"}))
    await flow.wait("session-1")
    assert service.finalized == [("turn-1", TurnStatus.FAILED)]
    assert service.terminal_turns[0].error_class == "model_authentication_required"
