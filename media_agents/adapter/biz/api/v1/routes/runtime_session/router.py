"""Internal Gateway-facing Runtime Session HTTP API."""

from fastapi import APIRouter, HTTPException, status

from adapter.biz.api.v1.deps import RuntimeSessionServiceDep
from adapter.biz.domain.runtime_session.entity import RuntimeSession, Turn
from adapter.biz.domain.runtime_session.errors import (
    ContextVersionConflict,
    InvalidRuntimeSession,
    InvalidTurn,
    RuntimeSessionNotFound,
)
from adapter.biz.service.runtime_session.command import RegisterRuntimeSession, SubmitTurn
from adapter.schemas.runtime_session.request import RegisterRuntimeSessionRequest, SubmitTurnRequest
from adapter.schemas.runtime_session.response import RuntimeSessionResponse, TurnResponse


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

    @router.post(
        "/{session_id}/turns", response_model=TurnResponse, status_code=status.HTTP_202_ACCEPTED
    )
    async def submit_turn(
        session_id: str, body: SubmitTurnRequest, service: RuntimeSessionServiceDep
    ) -> TurnResponse:
        try:
            command = SubmitTurn(session_id=session_id, **body.model_dump())
            return _turn_response(await service.submit_turn(command))
        except RuntimeSessionNotFound as error:
            raise HTTPException(status_code=404, detail=str(error)) from error
        except InvalidTurn as error:
            raise HTTPException(status_code=400, detail=str(error)) from error
        except ContextVersionConflict as error:
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
        next_turn_sequence=value.next_turn_sequence,
        next_event_sequence=value.next_event_sequence,
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
    )
