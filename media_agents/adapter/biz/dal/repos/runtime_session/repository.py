"""SQLAlchemy implementation of the Runtime Session persistence port."""

from __future__ import annotations

from datetime import UTC

from adapter.biz.dal.models.runtime_session import (
    AgentRequestRow,
    InstanceAdmissionRow,
    MessageRow,
    RuntimeSessionRow,
    TurnRow,
    UniversalEventRow,
)
from adapter.biz.dal.session import current_session
from adapter.biz.domain.runtime_session.entity import (
    AgentRequest,
    AgentRequestStatus,
    ChatStatus,
    Message,
    RuntimeSession,
    SessionStatus,
    Turn,
    TurnStatus,
    UniversalEvent,
)
from adapter.biz.domain.runtime_session.errors import RuntimeSessionAlreadyExists, TransferConflict
from adapter.biz.domain.runtime_session.ownership import Admission, InstanceScope
from sqlalchemy import select, text
from sqlalchemy.dialects.postgresql import insert
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
                execution_epoch=turn.execution_epoch,
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
        row.workspace_id = session.workspace_id
        row.agent_config_id = session.agent_config_id
        row.owner_instance_id = session.owner_instance_id
        row.owner_incarnation_id = session.owner_incarnation_id
        row.execution_epoch = session.execution_epoch
        row.last_transfer_id = session.last_transfer_id
        row.handoff_id = session.handoff_id
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
            turn_row.execution_epoch = turn.execution_epoch
            turn_row.status = turn.status.value
            turn_row.started_at = turn.started_at
            turn_row.finished_at = turn.finished_at
            turn_row.error_class = turn.error_class
        await db.flush()

    async def add_agent_request(
        self, session: RuntimeSession, request: AgentRequest, event: UniversalEvent
    ) -> None:
        db = current_session()
        db.add(_request_to_row(request))
        await self.add_event(session, event)

    async def get_agent_request(
        self, request_id: str, *, for_update: bool = False
    ) -> AgentRequest | None:
        statement = select(AgentRequestRow).where(AgentRequestRow.request_id == request_id)
        if for_update:
            statement = statement.with_for_update()
        row = (await current_session().execute(statement)).scalar_one_or_none()
        return _request_to_domain(row) if row is not None else None

    async def save_agent_request(
        self, session: RuntimeSession, request: AgentRequest, event: UniversalEvent
    ) -> None:
        db = current_session()
        row = await db.get(AgentRequestRow, request.request_id)
        if row is None:
            raise RuntimeSessionAlreadyExists(request.request_id)
        row.status = request.status.value
        row.response = request.response
        row.resolved_at = request.resolved_at
        await self.add_event(session, event)

    async def list_agent_requests(
        self, session_id: str, turn_id: str | None = None
    ) -> list[AgentRequest]:
        statement = select(AgentRequestRow).where(AgentRequestRow.session_id == session_id)
        if turn_id is not None:
            statement = statement.where(AgentRequestRow.turn_id == turn_id)
        rows = (
            (await current_session().execute(statement.order_by(AgentRequestRow.created_at)))
            .scalars()
            .all()
        )
        return [_request_to_domain(row) for row in rows]

    async def save_agent_request_state(self, request: AgentRequest) -> None:
        row = await current_session().get(AgentRequestRow, request.request_id)
        if row is None:
            raise RuntimeSessionAlreadyExists(request.request_id)
        row.status = request.status.value
        row.response = request.response
        row.resolved_at = request.resolved_at
        await current_session().flush()

    async def list_running_turns(self) -> list[Turn]:
        rows = (
            (
                await current_session().execute(
                    select(TurnRow)
                    .where(TurnRow.status == TurnStatus.RUNNING.value)
                    .order_by(TurnRow.started_at, TurnRow.turn_id)
                )
            )
            .scalars()
            .all()
        )
        return [_turn_to_domain(row) for row in rows]

    async def admission(
        self, scope: InstanceScope, *, for_update: bool = False
    ) -> Admission | None:
        statement = select(InstanceAdmissionRow).where(
            InstanceAdmissionRow.workspace_id == scope.workspace_id,
            InstanceAdmissionRow.agent_config_id == scope.agent_config_id,
            InstanceAdmissionRow.replica_id == scope.replica_id,
        )
        if for_update:
            statement = statement.with_for_update()
        row = (await current_session().execute(statement)).scalar_one_or_none()
        if row is None:
            return None
        return Admission(
            InstanceScope(
                row.workspace_id, row.agent_config_id, row.replica_id, row.incarnation_id
            ),
            row.state,
            row.operation_id,
            row.version,
        )

    async def initialize_admission(self, scope: InstanceScope) -> Admission:
        await current_session().execute(
            insert(InstanceAdmissionRow)
            .values(
                workspace_id=scope.workspace_id,
                agent_config_id=scope.agent_config_id,
                replica_id=scope.replica_id,
                incarnation_id=scope.incarnation_id,
                state="active",
                operation_id="",
                version=1,
            )
            .on_conflict_do_nothing()
        )
        value = await self.admission(scope, for_update=True)
        if value is None:
            raise RuntimeError("admission_missing")
        if value.scope.incarnation_id != scope.incarnation_id:
            value = Admission(scope, value.state, value.operation_id, value.version + 1)
            await self.save_admission(value)
        return value

    async def save_admission(self, value: Admission) -> None:
        row = await current_session().get(
            InstanceAdmissionRow,
            (value.scope.workspace_id, value.scope.agent_config_id, value.scope.replica_id),
        )
        if row is None:
            raise RuntimeError("admission_missing")
        row.incarnation_id = value.scope.incarnation_id
        row.state = value.state
        row.operation_id = value.operation_id
        row.version = value.version
        await current_session().flush()

    async def sessions(self, scope: InstanceScope) -> list[RuntimeSession]:
        rows = (
            (
                await current_session().execute(
                    select(RuntimeSessionRow)
                    .where(
                        RuntimeSessionRow.workspace_id == scope.workspace_id,
                        RuntimeSessionRow.agent_config_id == scope.agent_config_id,
                        RuntimeSessionRow.owner_instance_id == scope.replica_id,
                        RuntimeSessionRow.owner_incarnation_id == scope.incarnation_id,
                    )
                    .order_by(RuntimeSessionRow.session_id)
                )
            )
            .scalars()
            .all()
        )
        return [_to_domain(row) for row in rows]

    async def fence(self, scope: InstanceScope, epoch: int, *, transfer: bool = False) -> None:
        for name, value in {
            "workspace": scope.workspace_id,
            "config": scope.agent_config_id,
            "replica": scope.replica_id,
            "incarnation": scope.incarnation_id,
            "epoch": str(epoch),
            "transfer": "true" if transfer else "false",
        }.items():
            await current_session().execute(
                text("SELECT set_config(:name,:value,true)"),
                {"name": "media_agent." + name, "value": value},
            )

    async def resume_once(self, session_id: str, operation_id: str, epoch: int) -> bool:
        result = await current_session().execute(
            text(
                "INSERT INTO media_agents.runtime_session_resumes"
                "(session_id,operation_id,execution_epoch) "
                "VALUES(:session,:operation,:epoch) ON CONFLICT DO NOTHING "
                "RETURNING execution_epoch"
            ),
            {"session": session_id, "operation": operation_id, "epoch": epoch},
        )
        if result.scalar_one_or_none() is not None:
            return True
        result = await current_session().execute(
            text(
                "SELECT execution_epoch FROM media_agents.runtime_session_resumes "
                "WHERE session_id=:session AND operation_id=:operation"
            ),
            {"session": session_id, "operation": operation_id},
        )
        if result.scalar_one() != epoch:
            raise TransferConflict()
        return False


def _to_row(session: RuntimeSession) -> RuntimeSessionRow:
    return RuntimeSessionRow(
        session_id=session.session_id,
        workspace_id=session.workspace_id,
        agent_config_id=session.agent_config_id,
        owner_instance_id=session.owner_instance_id,
        owner_incarnation_id=session.owner_incarnation_id,
        execution_epoch=session.execution_epoch,
        last_transfer_id=session.last_transfer_id,
        handoff_id=session.handoff_id,
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
        workspace_id=row.workspace_id,
        agent_config_id=row.agent_config_id,
        owner_instance_id=row.owner_instance_id,
        owner_incarnation_id=row.owner_incarnation_id,
        execution_epoch=row.execution_epoch,
        last_transfer_id=row.last_transfer_id,
        handoff_id=row.handoff_id,
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
        execution_epoch=row.execution_epoch,
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


def _request_to_row(request: AgentRequest) -> AgentRequestRow:
    return AgentRequestRow(
        request_id=request.request_id,
        session_id=request.session_id,
        turn_id=request.turn_id,
        request_type=request.request_type,
        payload=request.payload,
        status=request.status.value,
        response=request.response,
        resolved_at=request.resolved_at,
        created_at=request.created_at,
    )


def _request_to_domain(row: AgentRequestRow) -> AgentRequest:
    return AgentRequest(
        request_id=row.request_id,
        session_id=row.session_id,
        turn_id=row.turn_id,
        request_type=row.request_type,
        payload=row.payload,
        status=AgentRequestStatus(row.status),
        response=row.response,
        resolved_at=row.resolved_at,
        created_at=row.created_at.replace(tzinfo=row.created_at.tzinfo or UTC),
    )
