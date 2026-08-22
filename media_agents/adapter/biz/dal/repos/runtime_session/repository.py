"""SQLAlchemy implementation of the Runtime Session persistence port."""

from __future__ import annotations

from datetime import UTC

from adapter.biz.dal.models.runtime_session import MessageRow, RuntimeSessionRow, TurnRow
from adapter.biz.dal.session import current_session
from adapter.biz.domain.runtime_session.entity import (
    ChatStatus,
    Message,
    RuntimeSession,
    SessionStatus,
    Turn,
    TurnStatus,
)
from adapter.biz.domain.runtime_session.errors import RuntimeSessionAlreadyExists
from sqlalchemy import select
from sqlalchemy.exc import IntegrityError


class SqlRuntimeSessionRepository:
    async def get(self, session_id: str, *, for_update: bool = False) -> RuntimeSession | None:
        statement = select(RuntimeSessionRow).where(RuntimeSessionRow.session_id == session_id)
        if for_update:
            statement = statement.with_for_update()
        row = (await current_session().execute(statement)).scalar_one_or_none()
        return _to_domain(row) if row is not None else None

    async def add(self, session: RuntimeSession) -> None:
        db = current_session()
        db.add(_to_row(session))
        try:
            await db.flush()
        except IntegrityError as error:
            raise RuntimeSessionAlreadyExists(session.session_id) from error

    async def get_turn_by_idempotency_key(
        self, session_id: str, idempotency_key: str
    ) -> Turn | None:
        statement = select(TurnRow).where(
            TurnRow.session_id == session_id, TurnRow.idempotency_key == idempotency_key
        )
        row = (await current_session().execute(statement)).scalar_one_or_none()
        return _turn_to_domain(row) if row is not None else None

    async def add_turn(self, turn: Turn, message: Message) -> None:
        db = current_session()
        db.add(
            TurnRow(
                turn_id=turn.turn_id,
                session_id=turn.session_id,
                idempotency_key=turn.idempotency_key,
                enqueue_sequence=turn.enqueue_sequence,
                status=turn.status.value,
                input=turn.input,
                context_version=turn.context_version,
                created_at=turn.created_at,
                started_at=turn.started_at,
                finished_at=turn.finished_at,
                error_class=turn.error_class,
            )
        )
        await db.flush()
        db.add(
            MessageRow(
                message_id=message.message_id,
                session_id=message.session_id,
                turn_id=message.turn_id,
                role=message.role,
                content_ref=message.content_ref,
                sequence=message.sequence,
                created_at=message.created_at,
            )
        )
        await db.flush()

    async def save_submission(self, session: RuntimeSession) -> None:
        db = current_session()
        row = await db.get(RuntimeSessionRow, session.session_id)
        if row is None:
            raise RuntimeSessionAlreadyExists(session.session_id)
        row.active_turn_id = session.active_turn_id
        row.chat_status = session.chat_status.value
        row.next_turn_sequence = session.next_turn_sequence
        row.updated_at = session.updated_at
        await db.flush()


def _to_row(session: RuntimeSession) -> RuntimeSessionRow:
    return RuntimeSessionRow(
        session_id=session.session_id,
        context_version=session.context_version,
        context_ref=session.context_ref,
        status=session.status.value,
        chat_status=session.chat_status.value,
        active_turn_id=session.active_turn_id,
        next_turn_sequence=session.next_turn_sequence,
        next_event_sequence=session.next_event_sequence,
        created_at=session.created_at,
        updated_at=session.updated_at,
    )


def _to_domain(row: RuntimeSessionRow) -> RuntimeSession:
    created_at = row.created_at.replace(tzinfo=row.created_at.tzinfo or UTC)
    updated_at = row.updated_at.replace(tzinfo=row.updated_at.tzinfo or UTC)
    return RuntimeSession(
        session_id=row.session_id,
        context_version=row.context_version,
        context_ref=row.context_ref,
        status=SessionStatus(row.status),
        chat_status=ChatStatus(row.chat_status),
        active_turn_id=row.active_turn_id,
        next_turn_sequence=row.next_turn_sequence,
        next_event_sequence=row.next_event_sequence,
        created_at=created_at,
        updated_at=updated_at,
    )


def _turn_to_domain(row: TurnRow) -> Turn:
    return Turn(
        turn_id=row.turn_id,
        session_id=row.session_id,
        idempotency_key=row.idempotency_key,
        enqueue_sequence=row.enqueue_sequence,
        status=TurnStatus(row.status),
        input=row.input,
        context_version=row.context_version,
        created_at=row.created_at.replace(tzinfo=row.created_at.tzinfo or UTC),
        started_at=row.started_at,
        finished_at=row.finished_at,
        error_class=row.error_class,
    )
