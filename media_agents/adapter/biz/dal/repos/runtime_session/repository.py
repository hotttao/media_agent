"""SQLAlchemy implementation of the Runtime Session persistence port."""

from __future__ import annotations

from datetime import UTC

from adapter.biz.dal.models.runtime_session import (
    MessageRow,
    RuntimeSessionRow,
    TurnRow,
    UniversalEventRow,
)
from adapter.biz.dal.session import current_session
from adapter.biz.domain.runtime_session.entity import (
    ChatStatus,
    Message,
    RuntimeSession,
    SessionStatus,
    Turn,
    TurnStatus,
    UniversalEvent,
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

    async def get_turn(self, turn_id: str, *, for_update: bool = False) -> Turn | None:
        statement = select(TurnRow).where(TurnRow.turn_id == turn_id)
        if for_update:
            statement = statement.with_for_update()
        row = (await current_session().execute(statement)).scalar_one_or_none()
        return _turn_to_domain(row) if row is not None else None

    async def get_queue_head(self, session_id: str) -> Turn | None:
        statement = (
            select(TurnRow)
            .where(TurnRow.session_id == session_id, TurnRow.status == TurnStatus.QUEUED.value)
            .order_by(TurnRow.enqueue_sequence)
            .limit(1)
            .with_for_update()
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
        row.runtime_session_id = session.runtime_session_id
        row.chat_status = session.chat_status.value
        row.next_turn_sequence = session.next_turn_sequence
        row.next_event_sequence = session.next_event_sequence
        row.next_message_sequence = session.next_message_sequence
        row.updated_at = session.updated_at
        await db.flush()

    async def add_event(
        self, session: RuntimeSession, event: UniversalEvent, message: Message | None = None
    ) -> None:
        db = current_session()
        session_row = await db.get(RuntimeSessionRow, session.session_id)
        if session_row is None:
            raise RuntimeSessionAlreadyExists(session.session_id)
        session_row.next_event_sequence = session.next_event_sequence
        session_row.next_message_sequence = session.next_message_sequence
        session_row.updated_at = session.updated_at
        db.add(
            UniversalEventRow(
                event_id=event.event_id,
                session_id=event.session_id,
                turn_id=event.turn_id,
                sequence=event.sequence,
                event_type=event.event_type,
                payload=event.payload,
                created_at=event.created_at,
            )
        )
        if message is not None:
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

    async def find_completed_item(self, turn_id: str, item_id: str) -> UniversalEvent | None:
        statement = select(UniversalEventRow).where(
            UniversalEventRow.turn_id == turn_id,
            UniversalEventRow.event_type == "item.completed",
            UniversalEventRow.payload["item"]["item_id"].astext == item_id,
        )
        row = (await current_session().execute(statement)).scalar_one_or_none()
        return _event_to_domain(row) if row is not None else None

    async def list_events_after(
        self, session_id: str, after_sequence: int, limit: int
    ) -> list[UniversalEvent]:
        statement = (
            select(UniversalEventRow)
            .where(
                UniversalEventRow.session_id == session_id,
                UniversalEventRow.sequence > after_sequence,
            )
            .order_by(UniversalEventRow.sequence)
            .limit(limit)
        )
        rows = (await current_session().execute(statement)).scalars().all()
        return [_event_to_domain(row) for row in rows]

    async def list_messages(self, session_id: str, limit: int) -> list[Message]:
        statement = (
            select(MessageRow)
            .where(MessageRow.session_id == session_id)
            .order_by(MessageRow.sequence)
            .limit(limit)
        )
        rows = (await current_session().execute(statement)).scalars().all()
        return [_message_to_domain(row) for row in rows]

    async def save_execution(self, session: RuntimeSession, turn: Turn | None = None) -> None:
        db = current_session()
        row = await db.get(RuntimeSessionRow, session.session_id)
        if row is None:
            raise RuntimeSessionAlreadyExists(session.session_id)
        row.status = session.status.value
        row.chat_status = session.chat_status.value
        row.active_turn_id = session.active_turn_id
        row.runtime_session_id = session.runtime_session_id
        row.next_event_sequence = session.next_event_sequence
        row.next_message_sequence = session.next_message_sequence
        row.updated_at = session.updated_at
        if turn is not None:
            turn_row = await db.get(TurnRow, turn.turn_id)
            if turn_row is None:
                raise RuntimeSessionAlreadyExists(turn.turn_id)
            turn_row.status = turn.status.value
            turn_row.started_at = turn.started_at
            turn_row.finished_at = turn.finished_at
            turn_row.error_class = turn.error_class
        await db.flush()


def _to_row(session: RuntimeSession) -> RuntimeSessionRow:
    return RuntimeSessionRow(
        session_id=session.session_id,
        context_version=session.context_version,
        context_ref=session.context_ref,
        status=session.status.value,
        chat_status=session.chat_status.value,
        active_turn_id=session.active_turn_id,
        runtime_session_id=session.runtime_session_id,
        next_turn_sequence=session.next_turn_sequence,
        next_event_sequence=session.next_event_sequence,
        next_message_sequence=session.next_message_sequence,
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
        runtime_session_id=row.runtime_session_id,
        next_turn_sequence=row.next_turn_sequence,
        next_event_sequence=row.next_event_sequence,
        next_message_sequence=row.next_message_sequence,
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


def _message_to_domain(row: MessageRow) -> Message:
    return Message(
        message_id=row.message_id,
        session_id=row.session_id,
        turn_id=row.turn_id,
        role=row.role,
        content_ref=row.content_ref,
        sequence=row.sequence,
        created_at=row.created_at.replace(tzinfo=row.created_at.tzinfo or UTC),
    )


def _event_to_domain(row: UniversalEventRow) -> UniversalEvent:
    return UniversalEvent(
        event_id=row.event_id,
        session_id=row.session_id,
        turn_id=row.turn_id,
        sequence=row.sequence,
        event_type=row.event_type,
        payload=row.payload,
        created_at=row.created_at.replace(tzinfo=row.created_at.tzinfo or UTC),
    )
