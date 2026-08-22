"""Internal Gateway-facing Runtime Session HTTP API."""

from fastapi import APIRouter, HTTPException, Query, status

from adapter.biz.api.v1.deps import ExecuteTurnFlowDep, RuntimeSessionServiceDep
from adapter.biz.domain.runtime_session.entity import Message, RuntimeSession, Turn, UniversalEvent
from adapter.biz.domain.runtime_session.errors import (
    ContextVersionConflict,
    InvalidRuntimeSession,
    InvalidTurn,
    RuntimeSessionNotFound,
)
from adapter.biz.service.runtime_session.command import RegisterRuntimeSession, SubmitTurn
from adapter.biz.workflow.execute_turn_flow import RuntimeCredentials
from adapter.schemas.runtime_session.request import RegisterRuntimeSessionRequest, SubmitTurnRequest
from adapter.schemas.runtime_session.response import (
    MessageResponse,
    RuntimeSessionResponse,
    TurnResponse,
    UniversalEventResponse,
)


def build_runtime_session_router() -> APIRouter:
    router = APIRouter(prefix="/api/v1/runtime-sessions", tags=["runtime-session"])

    @router.post("", response_model=RuntimeSessionResponse, status_code=status.HTTP_201_CREATED)
    async def register(
        body: RegisterRuntimeSessionRequest,
        service: RuntimeSessionServiceDep,
    ) -> RuntimeSessionResponse:
        try:
            value = await service.register(RegisterRuntimeSession(**body.model_dump()))
        except InvalidRuntimeSession as error:
            raise HTTPException(status_code=400, detail=str(error)) from error
        except ContextVersionConflict as error:
            raise HTTPException(status_code=409, detail=str(error)) from error
        return _response(value)

    @router.get("/{session_id}", response_model=RuntimeSessionResponse)
    async def get(
        session_id: str,
        service: RuntimeSessionServiceDep,
    ) -> RuntimeSessionResponse:
        try:
            return _response(await service.get(session_id))
        except RuntimeSessionNotFound as error:
            raise HTTPException(status_code=404, detail=str(error)) from error

    @router.get("/{session_id}/events", response_model=list[UniversalEventResponse])
    async def replay_events(
        session_id: str,
        service: RuntimeSessionServiceDep,
        after_sequence: int = Query(default=0, ge=0),
        limit: int = Query(default=200, ge=1, le=1000),
    ) -> list[UniversalEventResponse]:
        try:
            values = await service.replay_events(session_id, after_sequence, limit=limit)
            return [_event_response(value) for value in values]
        except RuntimeSessionNotFound as error:
            raise HTTPException(status_code=404, detail=str(error)) from error

    @router.get("/{session_id}/messages", response_model=list[MessageResponse])
    async def list_messages(
        session_id: str,
        service: RuntimeSessionServiceDep,
        limit: int = Query(default=200, ge=1, le=1000),
    ) -> list[MessageResponse]:
        try:
            values = await service.list_messages(session_id, limit=limit)
            return [_message_response(value) for value in values]
        except RuntimeSessionNotFound as error:
            raise HTTPException(status_code=404, detail=str(error)) from error

    @router.post(
        "/{session_id}/turns", response_model=TurnResponse, status_code=status.HTTP_202_ACCEPTED
    )
    async def submit_turn(
        session_id: str, body: SubmitTurnRequest, flow: ExecuteTurnFlowDep
    ) -> TurnResponse:
        try:
            command = SubmitTurn(
                session_id=session_id,
                idempotency_key=body.idempotency_key,
                context_version=body.context_version,
                input=body.input,
            )
            credentials = RuntimeCredentials(api_key=body.api_key, session_token=body.session_token)
            return _turn_response(await flow.submit(command, credentials))
        except RuntimeSessionNotFound as error:
            raise HTTPException(status_code=404, detail=str(error)) from error
        except InvalidTurn as error:
            raise HTTPException(status_code=400, detail=str(error)) from error
        except ContextVersionConflict as error:
            raise HTTPException(status_code=409, detail=str(error)) from error

    @router.get("/{session_id}/turns/{turn_id}", response_model=TurnResponse)
    async def get_turn(
        session_id: str, turn_id: str, service: RuntimeSessionServiceDep
    ) -> TurnResponse:
        try:
            turn = await service.get_turn(turn_id)
            if turn.session_id != session_id:
                raise InvalidTurn(f"turn not found: {turn_id}")
            return _turn_response(turn)
        except InvalidTurn as error:
            raise HTTPException(status_code=404, detail=str(error)) from error

    @router.post("/{session_id}/turns/{turn_id}/interrupt", status_code=status.HTTP_202_ACCEPTED)
    async def interrupt_turn(
        session_id: str, turn_id: str, flow: ExecuteTurnFlowDep
    ) -> dict[str, bool]:
        try:
            return {"interrupted": await flow.interrupt(session_id, turn_id)}
        except InvalidTurn as error:
            raise HTTPException(status_code=409, detail=str(error)) from error

    return router


def _response(value: RuntimeSession) -> RuntimeSessionResponse:
    return RuntimeSessionResponse(
        session_id=value.session_id,
        context_version=value.context_version,
        context_ref=value.context_ref,
        status=value.status.value,
        chat_status=value.chat_status.value,
        active_turn_id=value.active_turn_id,
        runtime_session_id=value.runtime_session_id,
        next_turn_sequence=value.next_turn_sequence,
        next_event_sequence=value.next_event_sequence,
        next_message_sequence=value.next_message_sequence,
        created_at=value.created_at.isoformat(),
        updated_at=value.updated_at.isoformat(),
    )


def _turn_response(value: Turn) -> TurnResponse:
    return TurnResponse(
        turn_id=value.turn_id,
        session_id=value.session_id,
        idempotency_key=value.idempotency_key,
        enqueue_sequence=value.enqueue_sequence,
        status=value.status.value,
        context_version=value.context_version,
        input=value.input,
        created_at=value.created_at.isoformat(),
        started_at=value.started_at.isoformat() if value.started_at else None,
        finished_at=value.finished_at.isoformat() if value.finished_at else None,
        error_class=value.error_class,
    )


def _event_response(value: UniversalEvent) -> UniversalEventResponse:
    return UniversalEventResponse(
        event_id=value.event_id,
        session_id=value.session_id,
        turn_id=value.turn_id,
        sequence=value.sequence,
        event_type=value.event_type,
        payload=value.payload,
        created_at=value.created_at.isoformat(),
    )


def _message_response(value: Message) -> MessageResponse:
    return MessageResponse(
        message_id=value.message_id,
        session_id=value.session_id,
        turn_id=value.turn_id,
        role=value.role,
        content_ref=value.content_ref,
        sequence=value.sequence,
        created_at=value.created_at.isoformat(),
    )
