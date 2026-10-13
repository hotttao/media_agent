"""Single-domain Runtime Session use cases."""

from __future__ import annotations

from collections.abc import Awaitable, Callable
from dataclasses import dataclass, replace
from datetime import UTC, datetime
from typing import Protocol, TypeVar
from uuid import uuid4

from adapter.biz.domain.runtime_session.entity import (
    AgentRequest,
    ChatStatus,
    Message,
    RuntimeSession,
    SessionStatus,
    Turn,
    TurnStatus,
    UniversalEvent,
)
from adapter.biz.domain.runtime_session.errors import (
    ContextVersionConflict,
    InstanceDraining,
    InvalidRuntimeSession,
    InvalidTurn,
    RuntimeSessionAlreadyExists,
    RuntimeSessionNotFound,
    StaleExecutionOwner,
    TransferConflict,
)
from adapter.biz.domain.runtime_session.ownership import (
    Admission,
    InstanceScope,
    OwnershipRepository,
)
from adapter.biz.domain.runtime_session.repository import RuntimeSessionRepository
from adapter.biz.domain.runtime_session.runtime_event import RuntimeEventEnvelope
from adapter.biz.service.runtime_session.command import RegisterRuntimeSession, SubmitTurn
from adapter.biz.service.runtime_session.fence import expected_execution_epoch

T = TypeVar("T")


class TransactionRunner(Protocol):
    async def run_in_tx(self, operation: Callable[[], Awaitable[T]]) -> T: ...


@dataclass(frozen=True, slots=True)
class TurnExecution:
    session: RuntimeSession
    turn: Turn


@dataclass(frozen=True, slots=True)
class TurnFinalization:
    terminal_turn: Turn
    next_turn: Turn | None
    events: tuple[UniversalEvent, ...] = ()


@dataclass(frozen=True, slots=True)
class TurnSubmission:
    turn: Turn
    events: tuple[UniversalEvent, ...]


@dataclass(frozen=True, slots=True)
class AgentRequestChange:
    request: AgentRequest
    event: UniversalEvent | None


class RuntimeSessionService:
    def __init__(
        self,
        tx: TransactionRunner,
        repository: RuntimeSessionRepository,
        scope: InstanceScope | None = None,
        ownership: OwnershipRepository | None = None,
    ) -> None:
        self._tx = tx
        self._repository = repository
        self.scope = scope
        self._ownership = ownership
        if scope is not None and ownership is None:
            raise InvalidTurn("ownership_repository_required")

    async def register(self, command: RegisterRuntimeSession) -> RuntimeSession:
        if command.initial_epoch < 1 or (command.initial_epoch > 1) != bool(command.transfer_id):
            raise InvalidRuntimeSession("invalid_initial_ownership")
        if self.scope is None and (command.initial_epoch != 1 or command.transfer_id):
            raise InvalidRuntimeSession("managed_instance_required")
        candidate = RuntimeSession.register(
            session_id=command.session_id,
            context_version=command.context_version,
            context_ref=command.context_ref,
        )

        if self.scope is not None:
            if (command.workspace_id, command.agent_config_id) != (
                self.scope.workspace_id,
                self.scope.agent_config_id,
            ):
                raise StaleExecutionOwner()
            candidate = replace(
                candidate,
                workspace_id=self.scope.workspace_id,
                agent_config_id=self.scope.agent_config_id,
                owner_instance_id=self.scope.replica_id,
                owner_incarnation_id=self.scope.incarnation_id,
                execution_epoch=command.initial_epoch,
                last_transfer_id=command.transfer_id,
            )

        async def operation() -> RuntimeSession:
            await self._admission()
            if self.scope is not None and self._ownership is not None:
                await self._ownership.fence(self.scope, command.initial_epoch)
            existing = await self._repository.get(candidate.session_id, for_update=True)
            if existing is not None:
                if existing.context_version != command.context_version:
                    raise ContextVersionConflict(
                        existing.session_id,
                        existing.context_version,
                        command.context_version,
                    )
                if existing.context_ref != command.context_ref:
                    raise TransferConflict()
                if self.scope is not None and not existing.workspace_id:
                    if command.initial_epoch != 1:
                        raise TransferConflict()
                    if existing.active_turn_id is not None:
                        raise TransferConflict()
                    existing = replace(
                        existing,
                        workspace_id=self.scope.workspace_id,
                        agent_config_id=self.scope.agent_config_id,
                        owner_instance_id=self.scope.replica_id,
                        owner_incarnation_id=self.scope.incarnation_id,
                    )
                    await self._repository.save_execution(existing)
                self._owner(existing)
                return existing
            await self._admission(claim=True)
            expected = expected_execution_epoch.get()
            if expected is not None and expected != command.initial_epoch:
                raise StaleExecutionOwner()
            await self._repository.add(candidate)
            return candidate

        try:
            return await self._tx.run_in_tx(operation)
        except RuntimeSessionAlreadyExists:
            existing = await self._tx.run_in_tx(lambda: self._repository.get(candidate.session_id))
            if existing is None:
                raise
            self._owner(existing)
            if existing.context_version != candidate.context_version:
                raise ContextVersionConflict(
                    existing.session_id,
                    existing.context_version,
                    candidate.context_version,
                ) from None
            if existing.context_ref != candidate.context_ref:
                raise TransferConflict()
            return existing

    async def get(self, session_id: str) -> RuntimeSession:
        session = await self._tx.run_in_tx(lambda: self._repository.get(session_id.strip()))
        if session is None:
            raise RuntimeSessionNotFound(session_id)
        self._query_scope(session)
        return session

    async def submit_turn(self, command: SubmitTurn) -> Turn:
        return (await self.submit_turn_details(command)).turn

    async def submit_turn_details(self, command: SubmitTurn) -> TurnSubmission:
        session_id = command.session_id.strip()
        key = command.idempotency_key.strip()
        if not key:
            raise InvalidTurn("idempotency_key is required")
        if not command.input:
            raise InvalidTurn("input is required")
        if _contains_secret(command.input):
            raise InvalidTurn("input must not contain runtime credentials")

        async def operation() -> TurnSubmission:
            session = await self._locked(session_id)
            if session is None:
                raise RuntimeSessionNotFound(session_id)
            existing = await self._repository.get_turn_by_idempotency_key(session_id, key)
            if existing is not None:
                return TurnSubmission(existing, ())
            if session.context_version != command.context_version:
                raise ContextVersionConflict(
                    session_id, session.context_version, command.context_version
                )
            if session.status is not SessionStatus.ACTIVE:
                raise InvalidTurn(f"runtime session is not active: {session.status.value}")

            now = datetime.now(UTC)
            turn_id = str(uuid4())
            turn = Turn(
                turn_id=turn_id,
                session_id=session_id,
                idempotency_key=key,
                enqueue_sequence=session.next_turn_sequence,
                status=TurnStatus.QUEUED,
                input=command.input,
                context_version=session.context_version,
                created_at=now,
                execution_epoch=session.execution_epoch,
            )
            session, message = session.new_message(
                turn_id=turn_id,
                role="user",
                content_ref=command.input,
                now=now,
            )
            admission = await self._admission()
            if (
                not session.handoff_id
                and session.active_turn_id is None
                and (admission is None or admission.state == "active")
            ):
                turn = replace(turn, status=TurnStatus.RUNNING, started_at=now)
                session = replace(
                    session,
                    active_turn_id=turn_id,
                    chat_status=ChatStatus.AGENT,
                    next_turn_sequence=session.next_turn_sequence + 1,
                    updated_at=now,
                )
            else:
                session = replace(
                    session,
                    next_turn_sequence=session.next_turn_sequence + 1,
                    updated_at=now,
                )
            await self._repository.add_turn(turn, message)
            events: list[UniversalEvent] = []
            session, queued_event = session.new_event(
                event_type="turn.queued",
                payload={
                    "type": "turn.queued",
                    "session_id": session_id,
                    "turn_id": turn_id,
                    "enqueue_sequence": turn.enqueue_sequence,
                },
                turn_id=turn_id,
                now=now,
            )
            await self._repository.add_event(session, queued_event)
            events.append(queued_event)
            if turn.status is TurnStatus.RUNNING:
                session, started_event = session.new_event(
                    event_type="turn.started",
                    payload={"type": "turn.started", "session_id": session_id, "turn_id": turn_id},
                    turn_id=turn_id,
                    now=now,
                )
                await self._repository.add_event(session, started_event)
                events.append(started_event)
            await self._repository.save_submission(session)
            return TurnSubmission(turn, tuple(events))

        return await self._tx.run_in_tx(operation)

    async def persist_runtime_event(
        self, session_id: str, turn_id: str, payload: dict[str, object]
    ) -> UniversalEvent:
        envelope = RuntimeEventEnvelope.parse(payload)
        if envelope.is_live or envelope.is_terminal:
            raise InvalidTurn("live and terminal events use dedicated workflow paths")

        async def operation() -> UniversalEvent:
            session = await self._locked(session_id)
            if session is None:
                raise RuntimeSessionNotFound(session_id)
            turn = await self._repository.get_turn(turn_id)
            if turn is None or turn.session_id != session_id:
                raise InvalidTurn(f"turn not found: {turn_id}")
            self._turn_epoch(session, turn)
            if turn.status is not TurnStatus.RUNNING:
                raise InvalidTurn("late Runtime event arrived after its Turn became terminal")
            if envelope.is_tool_call and envelope.item_id:
                existing = await self._repository.find_completed_item(turn_id, envelope.item_id)
                if existing is not None:
                    return existing
            now = datetime.now(UTC)
            message: Message | None = None
            if envelope.creates_message:
                session, message = session.new_message(
                    turn_id=turn_id,
                    role=envelope.message_role,
                    content_ref=envelope.message_content,
                    now=now,
                )
            session, event = session.new_event(
                event_type=envelope.event_type,
                payload=payload,
                turn_id=turn_id,
                now=now,
            )
            await self._repository.add_event(session, event, message)
            return event

        return await self._tx.run_in_tx(operation)

    async def replay_events(
        self, session_id: str, after_sequence: int, *, limit: int = 200
    ) -> list[UniversalEvent]:
        if after_sequence < 0:
            raise InvalidTurn("after_sequence must not be negative")
        bounded = min(max(limit, 1), 1000)
        session = await self.get(session_id)
        return await self._tx.run_in_tx(
            lambda: self._repository.list_events_after(session.session_id, after_sequence, bounded)
        )

    async def list_messages(self, session_id: str, *, limit: int = 200) -> list[Message]:
        bounded = min(max(limit, 1), 1000)
        session = await self.get(session_id)
        return await self._tx.run_in_tx(
            lambda: self._repository.list_messages(session.session_id, bounded)
        )

    async def get_turn(self, turn_id: str) -> Turn:
        turn = await self._tx.run_in_tx(lambda: self._repository.get_turn(turn_id.strip()))
        if turn is None:
            raise InvalidTurn(f"turn not found: {turn_id}")
        return turn

    async def open_agent_request(
        self, session_id: str, turn_id: str, request_type: str, payload: dict[str, object]
    ) -> AgentRequestChange:
        async def operation() -> AgentRequestChange:
            session = await self._locked(session_id)
            turn = await self._repository.get_turn(turn_id)
            if session is None:
                raise RuntimeSessionNotFound(session_id)
            if (
                turn is None
                or turn.session_id != session_id
                or turn.status is not TurnStatus.RUNNING
            ):
                raise InvalidTurn("Agent Request requires the active running Turn")
            self._turn_epoch(session, turn)
            if session.active_turn_id != turn_id:
                raise InvalidTurn("Agent Request Turn is no longer active")
            now = datetime.now(UTC)
            request = AgentRequest.open(
                session_id=session_id,
                turn_id=turn_id,
                request_type=request_type,
                payload=payload,
                now=now,
            )
            session, event = session.new_event(
                event_type="agent.requested",
                payload={
                    "type": "agent.requested",
                    "session_id": session_id,
                    "turn_id": turn_id,
                    "request_id": request.request_id,
                    "request_type": request.request_type,
                    "request": payload,
                },
                turn_id=turn_id,
                now=now,
            )
            await self._repository.add_agent_request(session, request, event)
            return AgentRequestChange(request, event)

        return await self._tx.run_in_tx(operation)

    async def resolve_agent_request(
        self, session_id: str, turn_id: str, request_id: str, response: dict[str, object]
    ) -> AgentRequestChange:
        async def operation() -> AgentRequestChange:
            session = await self._locked(session_id)
            request = await self._repository.get_agent_request(request_id, for_update=True)
            if request is None or request.session_id != session_id or request.turn_id != turn_id:
                raise InvalidTurn(f"Agent Request not found: {request_id}")
            now = datetime.now(UTC)
            resolved = request.resolve(response, now=now)
            if resolved is request:
                return AgentRequestChange(request, None)
            session = await self._locked(session_id)
            turn = await self._repository.get_turn(turn_id)
            if session is None:
                raise RuntimeSessionNotFound(session_id)
            if (
                turn is None
                or turn.status is not TurnStatus.RUNNING
                or session.active_turn_id != turn_id
            ):
                raise InvalidTurn("Agent Request response arrived after its Turn became terminal")
            self._turn_epoch(session, turn)
            session, event = session.new_event(
                event_type="agent.request.resolved",
                payload={
                    "type": "agent.request.resolved",
                    "session_id": session_id,
                    "turn_id": turn_id,
                    "request_id": request_id,
                },
                turn_id=turn_id,
                now=now,
            )
            await self._repository.save_agent_request(session, resolved, event)
            return AgentRequestChange(resolved, event)

        return await self._tx.run_in_tx(operation)

    async def list_agent_requests(
        self, session_id: str, *, turn_id: str | None = None
    ) -> list[AgentRequest]:
        await self.get(session_id)
        return await self._tx.run_in_tx(
            lambda: self._repository.list_agent_requests(session_id, turn_id)
        )

    async def running_turns(self) -> list[Turn]:
        if self.scope is None:
            values = await self._tx.run_in_tx(self._repository.list_running_turns)
            result = []
            for turn in values:
                if not (await self.get(turn.session_id)).workspace_id:
                    result.append(turn)
            return result
        sessions = await self.owned_sessions()
        return [
            await self.get_turn(s.active_turn_id) for s in sessions if s.active_turn_id is not None
        ]

    async def execution(self, session_id: str, turn_id: str) -> TurnExecution:
        async def operation() -> TurnExecution:
            session = await self._locked(session_id)
            turn = await self._repository.get_turn(turn_id)
            if session is None:
                raise RuntimeSessionNotFound(session_id)
            if turn is None or turn.session_id != session_id:
                raise InvalidTurn(f"turn not found: {turn_id}")
            self._turn_epoch(session, turn)
            if session.active_turn_id != turn_id or turn.status is not TurnStatus.RUNNING:
                raise InvalidTurn("turn is not the active running Turn")
            return TurnExecution(session, turn)

        return await self._tx.run_in_tx(operation)

    async def attach_runtime(self, session_id: str, turn_id: str, handle: str) -> RuntimeSession:
        async def operation() -> RuntimeSession:
            session = await self._locked(session_id)
            if session is None:
                raise RuntimeSessionNotFound(session_id)
            turn = await self._repository.get_turn(turn_id)
            self._turn_epoch(session, turn)
            if session.active_turn_id != turn_id:
                raise InvalidTurn("turn is no longer active")
            attached = session.attach_runtime(handle, now=datetime.now(UTC))
            await self._repository.save_execution(attached)
            return attached

        return await self._tx.run_in_tx(operation)

    async def finalize_turn(
        self,
        session_id: str,
        turn_id: str,
        status: TurnStatus,
        *,
        error_class: str | None = None,
        terminal_payload: dict[str, object] | None = None,
    ) -> TurnFinalization:
        async def operation() -> TurnFinalization:
            session = await self._locked(session_id)
            turn = await self._repository.get_turn(turn_id, for_update=True)
            if session is None:
                raise RuntimeSessionNotFound(session_id)
            if turn is None or turn.session_id != session_id:
                raise InvalidTurn(f"turn not found: {turn_id}")
            self._turn_epoch(session, turn)
            admission = await self._admission()
            return await self._finish(
                session,
                turn,
                status,
                error_class,
                terminal_payload,
                admission is None or admission.state == "active",
            )

        return await self._tx.run_in_tx(operation)

    async def _finish(
        self,
        session: RuntimeSession,
        turn: Turn,
        status: TurnStatus,
        error_class: str | None,
        terminal_payload: dict[str, object] | None,
        claim_next: bool,
    ) -> TurnFinalization:
        session_id = session.session_id
        turn_id = turn.turn_id
        now = datetime.now(UTC)
        terminal = turn.finish(status, now=now, error_class=error_class)
        released = session.release_turn(terminal, now=now)
        for request in await self._repository.list_agent_requests(session_id, turn_id):
            cancelled = request.cancel(now=now)
            if cancelled is not request:
                await self._repository.save_agent_request_state(cancelled)
        next_turn: Turn | None = None
        events: list[UniversalEvent] = []
        reason = {
            TurnStatus.COMPLETED: "completed",
            TurnStatus.FAILED: "error",
            TurnStatus.INTERRUPTED: "interrupted",
        }[status]
        if terminal_payload is not None and str(terminal_payload.get("reason") or "") == reason:
            payload: dict[str, object] = terminal_payload
        else:
            payload = {
                "type": "session.ended",
                "session_id": session.runtime_session_id or session_id,
                "reason": reason,
            }
        released, terminal_event = released.new_event(
            event_type="session.ended", payload=payload, turn_id=turn_id, now=now
        )
        await self._repository.add_event(released, terminal_event)
        events.append(terminal_event)
        lifecycle_type = f"turn.{status.value}"
        released, lifecycle_event = released.new_event(
            event_type=lifecycle_type,
            payload={
                "type": lifecycle_type,
                "session_id": session_id,
                "turn_id": turn_id,
                "error_class": error_class,
            },
            turn_id=turn_id,
            now=now,
        )
        await self._repository.add_event(released, lifecycle_event)
        events.append(lifecycle_event)
        if status is TurnStatus.COMPLETED and claim_next and not session.handoff_id:
            head = await self._repository.get_queue_head(session_id)
            if head is not None:
                released, next_turn = released.claim(head, now=now)
                released, started_event = released.new_event(
                    event_type="turn.started",
                    payload={
                        "type": "turn.started",
                        "session_id": session_id,
                        "turn_id": next_turn.turn_id,
                    },
                    turn_id=next_turn.turn_id,
                    now=now,
                )
                await self._repository.add_event(released, started_event)
                events.append(started_event)
        await self._repository.save_execution(released, terminal)
        if next_turn is not None:
            await self._repository.save_execution(released, next_turn)
        return TurnFinalization(terminal, next_turn, tuple(events))

    def _query_scope(self, session: RuntimeSession) -> None:
        if self.scope is not None and (session.workspace_id, session.agent_config_id) != (
            self.scope.workspace_id,
            self.scope.agent_config_id,
        ):
            raise StaleExecutionOwner()

    def _owner(self, session: RuntimeSession) -> None:
        self._query_scope(session)
        if self.scope is None:
            if session.workspace_id:
                raise StaleExecutionOwner()
            return
        if (session.owner_instance_id, session.owner_incarnation_id) != (
            self.scope.replica_id,
            self.scope.incarnation_id,
        ):
            raise StaleExecutionOwner()
        expected = expected_execution_epoch.get()
        if expected is not None and expected != session.execution_epoch:
            raise StaleExecutionOwner()

    @staticmethod
    def _turn_epoch(session: RuntimeSession, turn: Turn | None) -> None:
        if turn is None or turn.execution_epoch != session.execution_epoch:
            raise StaleExecutionOwner()

    async def _admission(self, *, claim: bool = False) -> Admission | None:
        if self.scope is None or self._ownership is None:
            return None
        value = await self._ownership.admission(self.scope, for_update=True)
        if value is None or value.scope.incarnation_id != self.scope.incarnation_id:
            raise StaleExecutionOwner()
        if claim and value.state != "active":
            raise InstanceDraining()
        return value

    async def _locked(self, session_id: str) -> RuntimeSession:
        await self._admission()
        session = await self._repository.get(session_id, for_update=True)
        if session is None:
            raise RuntimeSessionNotFound(session_id)
        self._owner(session)
        if self.scope is not None and self._ownership is not None:
            await self._ownership.fence(self.scope, session.execution_epoch)
        return session

    async def initialize_instance(
        self, registered_admission: str | None = None, drain_operation_id: str = ""
    ) -> None:
        if self.scope is None or self._ownership is None:
            return
        if registered_admission not in {None, "active", "draining"} or (
            registered_admission == "draining" and not drain_operation_id
        ):
            raise InvalidTurn("invalid_registered_admission")
        ownership, scope = self._ownership, self.scope

        async def initialize() -> None:
            previous = await ownership.admission(scope, for_update=True)
            value = await ownership.initialize_admission(scope)
            new_incarnation = (
                previous is None or previous.scope.incarnation_id != scope.incarnation_id
            )
            if registered_admission is not None and new_incarnation:
                state = registered_admission
                operation_id = drain_operation_id if state == "draining" else ""
                if value.state != state or value.operation_id != operation_id:
                    await ownership.save_admission(
                        replace(
                            value, state=state, operation_id=operation_id, version=value.version + 1
                        )
                    )

        await self._tx.run_in_tx(initialize)

    async def owned_sessions(self) -> list[RuntimeSession]:
        if self.scope is None or self._ownership is None:
            return []
        ownership, scope = self._ownership, self.scope
        return await self._tx.run_in_tx(lambda: ownership.sessions(scope))

    async def begin_drain(self, operation_id: str) -> Admission:
        if not operation_id or len(operation_id) > 128:
            raise InvalidTurn("invalid_drain_operation")

        async def operation() -> Admission:
            value = await self._admission()
            if value is None or self._ownership is None:
                raise InvalidTurn("managed_instance_required")
            if value.state == "draining":
                if value.operation_id != operation_id:
                    raise TransferConflict()
                return value
            value = replace(
                value, state="draining", operation_id=operation_id, version=value.version + 1
            )
            await self._ownership.save_admission(value)
            return value

        return await self._tx.run_in_tx(operation)

    async def drain_state(self) -> tuple[Admission, list[RuntimeSession]]:
        async def operation() -> tuple[Admission, list[RuntimeSession]]:
            value = await self._admission()
            if value is None or self.scope is None or self._ownership is None:
                raise InvalidTurn("managed_instance_required")
            return value, await self._ownership.sessions(self.scope)

        return await self._tx.run_in_tx(operation)

    async def quiesce(self, session_id: str, transfer_id: str) -> RuntimeSession:
        """Stop claiming this Session's queue while its current Turn can finish."""
        if not transfer_id or len(transfer_id) > 128:
            raise InvalidTurn("invalid_transfer_operation")

        async def operation() -> RuntimeSession:
            session = await self._locked(session_id)
            if self.scope is None:
                raise InvalidTurn("managed_instance_required")
            if session.handoff_id:
                if session.handoff_id != transfer_id:
                    raise TransferConflict()
                return session
            session = replace(session, handoff_id=transfer_id, updated_at=datetime.now(UTC))
            await self._repository.save_execution(session)
            return session

        return await self._tx.run_in_tx(operation)

    async def transfer(
        self,
        session_id: str,
        transfer_id: str,
        source_replica: str,
        source_incarnation: str,
        expected_epoch: int,
        *,
        termination_confirmed: bool = False,
    ) -> tuple[RuntimeSession, tuple[UniversalEvent, ...]]:
        async def operation() -> tuple[RuntimeSession, tuple[UniversalEvent, ...]]:
            await self._admission(claim=True)
            if (
                self.scope is None
                or self._ownership is None
                or not transfer_id
                or expected_epoch < 1
            ):
                raise TransferConflict()
            session = await self._repository.get(session_id, for_update=True)
            if session is None:
                raise RuntimeSessionNotFound(session_id)
            self._query_scope(session)
            target = (self.scope.replica_id, self.scope.incarnation_id)
            if (
                session.last_transfer_id == transfer_id
                and (session.owner_instance_id, session.owner_incarnation_id) == target
                and session.execution_epoch == expected_epoch + 1
            ):
                return session, ()
            if (
                session.owner_instance_id,
                session.owner_incarnation_id,
                session.execution_epoch,
            ) != (source_replica, source_incarnation, expected_epoch) or target == (
                source_replica,
                source_incarnation,
            ):
                raise TransferConflict()
            await self._ownership.fence(self.scope, expected_epoch, transfer=True)
            events: tuple[UniversalEvent, ...] = ()
            if session.active_turn_id is not None:
                if not termination_confirmed:
                    raise TransferConflict()
                turn = await self._repository.get_turn(session.active_turn_id, for_update=True)
                if turn is None:
                    raise TransferConflict()
                self._turn_epoch(session, turn)
                result = await self._finish(
                    session,
                    turn,
                    TurnStatus.FAILED,
                    "owner_terminated_outcome_unknown",
                    None,
                    False,
                )
                events = result.events
                session = await self._repository.get(session_id, for_update=True)
                if session is None:
                    raise RuntimeSessionNotFound(session_id)
            moved = replace(
                session,
                owner_instance_id=target[0],
                owner_incarnation_id=target[1],
                execution_epoch=expected_epoch + 1,
                last_transfer_id=transfer_id,
                handoff_id="",
                updated_at=datetime.now(UTC),
            )
            await self._repository.save_execution(moved)
            return moved, events

        return await self._tx.run_in_tx(operation)

    async def adopt(
        self, session_id: str, *, resume: bool = False, operation_id: str = ""
    ) -> TurnSubmission | None:
        async def operation() -> TurnSubmission | None:
            await self._admission(claim=True)
            session = await self._locked(session_id)
            if resume:
                if not operation_id or len(operation_id) > 128 or self._ownership is None:
                    raise InvalidTurn("resume_operation_required")
                if not await self._ownership.resume_once(
                    session_id, operation_id, session.execution_epoch
                ):
                    return None
            if session.active_turn_id is not None or session.handoff_id:
                return None
            if session.status is SessionStatus.PAUSED:
                if not resume:
                    return None
                session = replace(session, status=SessionStatus.ACTIVE)
            if session.status is not SessionStatus.ACTIVE:
                raise InvalidTurn("session_not_active")
            head = await self._repository.get_queue_head(session_id)
            if head is None:
                await self._repository.save_execution(session)
                return None
            claimed, turn = session.claim(head, now=datetime.now(UTC))
            claimed, event = claimed.new_event(
                event_type="turn.started",
                payload={"type": "turn.started", "session_id": session_id, "turn_id": turn.turn_id},
                turn_id=turn.turn_id,
                now=datetime.now(UTC),
            )
            await self._repository.save_execution(claimed, turn)
            await self._repository.add_event(claimed, event)
            return TurnSubmission(turn, (event,))

        return await self._tx.run_in_tx(operation)


_SECRET_KEYS = {"api_key", "apikey", "authorization", "session_token", "access_token"}


def _contains_secret(value: object) -> bool:
    if isinstance(value, dict):
        return any(
            str(key).lower() in _SECRET_KEYS or _contains_secret(item)
            for key, item in value.items()
        )
    if isinstance(value, list):
        return any(_contains_secret(item) for item in value)
    return False
