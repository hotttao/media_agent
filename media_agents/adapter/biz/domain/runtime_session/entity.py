"""Runtime Session aggregate and stable invariants."""

from __future__ import annotations

from dataclasses import dataclass, replace
from datetime import UTC, datetime
from enum import StrEnum
from uuid import uuid4

from adapter.biz.domain.runtime_session.errors import InvalidRuntimeSession, InvalidTurn


class SessionStatus(StrEnum):
    ACTIVE = "active"
    PAUSED = "paused"
    FAILED = "failed"
    DELETED = "deleted"


class ChatStatus(StrEnum):
    HUMAN = "human"
    AGENT = "agent"


class TurnStatus(StrEnum):
    QUEUED = "queued"
    RUNNING = "running"
    COMPLETED = "completed"
    FAILED = "failed"
    INTERRUPTED = "interrupted"
    CANCELLED = "cancelled"


class AgentRequestStatus(StrEnum):
    PENDING = "pending"
    RESOLVED = "resolved"
    CANCELLED = "cancelled"
    EXPIRED = "expired"


@dataclass(frozen=True, slots=True)
class AgentRequest:
    request_id: str
    session_id: str
    turn_id: str
    request_type: str
    payload: dict[str, object]
    status: AgentRequestStatus
    response: dict[str, object] | None
    resolved_at: datetime | None
    created_at: datetime

    @classmethod
    def open(
        cls,
        *,
        session_id: str,
        turn_id: str,
        request_type: str,
        payload: dict[str, object],
        now: datetime,
    ) -> AgentRequest:
        normalized = request_type.strip()
        if normalized not in {"confirm", "input", "permission"}:
            raise InvalidTurn(f"invalid Agent Request type: {normalized}")
        if not payload:
            raise InvalidTurn("Agent Request payload is required")
        return cls(
            str(uuid4()),
            session_id,
            turn_id,
            normalized,
            payload,
            AgentRequestStatus.PENDING,
            None,
            None,
            now,
        )

    def resolve(self, response: dict[str, object], *, now: datetime) -> AgentRequest:
        if not response:
            raise InvalidTurn("Agent Request response is required")
        if self.status is AgentRequestStatus.RESOLVED:
            if self.response != response:
                raise InvalidTurn("Agent Request was resolved with a different response")
            return self
        if self.status is not AgentRequestStatus.PENDING:
            raise InvalidTurn(f"Agent Request is not pending: {self.status.value}")
        return replace(self, status=AgentRequestStatus.RESOLVED, response=response, resolved_at=now)

    def cancel(self, *, now: datetime) -> AgentRequest:
        if self.status is not AgentRequestStatus.PENDING:
            return self
        return replace(self, status=AgentRequestStatus.CANCELLED, resolved_at=now)


@dataclass(frozen=True, slots=True)
class Turn:
    turn_id: str
    session_id: str
    idempotency_key: str
    enqueue_sequence: int
    status: TurnStatus
    input: dict[str, object]
    context_version: int
    created_at: datetime
    started_at: datetime | None = None
    finished_at: datetime | None = None
    error_class: str | None = None
    execution_epoch: int = 1

    def finish(
        self,
        status: TurnStatus,
        *,
        now: datetime,
        error_class: str | None = None,
    ) -> Turn:
        if self.status is not TurnStatus.RUNNING:
            raise InvalidTurn(f"turn is not running: {self.status.value}")
        if status not in {TurnStatus.COMPLETED, TurnStatus.FAILED, TurnStatus.INTERRUPTED}:
            raise InvalidTurn(f"invalid terminal turn status: {status.value}")
        return replace(self, status=status, finished_at=now, error_class=error_class)


@dataclass(frozen=True, slots=True)
class Message:
    message_id: str
    session_id: str
    turn_id: str
    role: str
    content_ref: dict[str, object]
    sequence: int
    created_at: datetime


@dataclass(frozen=True, slots=True)
class UniversalEvent:
    event_id: str
    session_id: str
    turn_id: str | None
    sequence: int
    event_type: str
    payload: dict[str, object]
    created_at: datetime


@dataclass(frozen=True, slots=True)
class RuntimeSession:
    session_id: str
    context_version: int
    context_ref: str
    status: SessionStatus
    chat_status: ChatStatus
    active_turn_id: str | None
    runtime_session_id: str | None
    next_turn_sequence: int
    next_event_sequence: int
    next_message_sequence: int
    created_at: datetime
    updated_at: datetime
    workspace_id: str = ""
    agent_config_id: str = ""
    owner_instance_id: str = ""
    owner_incarnation_id: str = ""
    execution_epoch: int = 1
    last_transfer_id: str = ""
    handoff_id: str = ""

    @classmethod
    def register(
        cls,
        *,
        session_id: str,
        context_version: int,
        context_ref: str,
        now: datetime | None = None,
    ) -> RuntimeSession:
        normalized_id = session_id.strip()
        normalized_ref = context_ref.strip()
        if not normalized_id:
            raise InvalidRuntimeSession("session_id is required")
        if context_version < 1:
            raise InvalidRuntimeSession("context_version must be positive")
        if not normalized_ref:
            raise InvalidRuntimeSession("context_ref is required")
        timestamp = (now or datetime.now(UTC)).astimezone(UTC)
        return cls(
            session_id=normalized_id,
            context_version=context_version,
            context_ref=normalized_ref,
            status=SessionStatus.ACTIVE,
            chat_status=ChatStatus.HUMAN,
            active_turn_id=None,
            runtime_session_id=None,
            next_turn_sequence=1,
            next_event_sequence=1,
            next_message_sequence=1,
            created_at=timestamp,
            updated_at=timestamp,
        )

    def new_message(
        self,
        *,
        turn_id: str,
        role: str,
        content_ref: dict[str, object],
        now: datetime,
    ) -> tuple[RuntimeSession, Message]:
        if role not in {"user", "assistant", "tool", "system"}:
            raise InvalidTurn(f"invalid Message role: {role}")
        message = Message(
            message_id=str(uuid4()),
            session_id=self.session_id,
            turn_id=turn_id,
            role=role,
            content_ref=content_ref,
            sequence=self.next_message_sequence,
            created_at=now,
        )
        return (
            replace(
                self,
                next_message_sequence=self.next_message_sequence + 1,
                updated_at=now,
            ),
            message,
        )

    def new_event(
        self,
        *,
        event_type: str,
        payload: dict[str, object],
        turn_id: str | None,
        now: datetime,
    ) -> tuple[RuntimeSession, UniversalEvent]:
        normalized_type = event_type.strip()
        if not normalized_type:
            raise InvalidTurn("Universal Event type is required")
        event = UniversalEvent(
            event_id=str(uuid4()),
            session_id=self.session_id,
            turn_id=turn_id,
            sequence=self.next_event_sequence,
            event_type=normalized_type,
            payload=payload,
            created_at=now,
        )
        return (
            replace(
                self,
                next_event_sequence=self.next_event_sequence + 1,
                updated_at=now,
            ),
            event,
        )

    def attach_runtime(self, runtime_session_id: str, *, now: datetime) -> RuntimeSession:
        handle = runtime_session_id.strip()
        if not handle:
            raise InvalidRuntimeSession("runtime_session_id is required")
        if self.runtime_session_id is not None and self.runtime_session_id != handle:
            raise InvalidRuntimeSession("runtime_session_id cannot be replaced")
        return replace(self, runtime_session_id=handle, updated_at=now)

    def release_turn(self, turn: Turn, *, now: datetime) -> RuntimeSession:
        if self.active_turn_id != turn.turn_id or turn.status not in {
            TurnStatus.COMPLETED,
            TurnStatus.FAILED,
            TurnStatus.INTERRUPTED,
        }:
            raise InvalidTurn("terminal Turn does not match active_turn_id")
        status = (
            SessionStatus.ACTIVE if turn.status is TurnStatus.COMPLETED else SessionStatus.PAUSED
        )
        return replace(
            self,
            status=status,
            chat_status=ChatStatus.HUMAN,
            active_turn_id=None,
            updated_at=now,
        )

    def claim(self, turn: Turn, *, now: datetime) -> tuple[RuntimeSession, Turn]:
        if (
            self.status is not SessionStatus.ACTIVE
            or self.active_turn_id is not None
            or self.handoff_id
        ):
            raise InvalidTurn("runtime session cannot claim a Turn")
        if turn.session_id != self.session_id or turn.status is not TurnStatus.QUEUED:
            raise InvalidTurn("only this Session's queued head can be claimed")
        running = replace(
            turn, status=TurnStatus.RUNNING, started_at=now, execution_epoch=self.execution_epoch
        )
        return (
            replace(
                self,
                chat_status=ChatStatus.AGENT,
                active_turn_id=turn.turn_id,
                updated_at=now,
            ),
            running,
        )
