"""Internal Gateway-facing Runtime Session HTTP API."""

from fastapi import APIRouter, HTTPException, status

from adapter.biz.api.v1.deps import RuntimeSessionServiceDep
from adapter.biz.domain.runtime_session.entity import RuntimeSession
from adapter.biz.domain.runtime_session.errors import (
    ContextVersionConflict,
    InvalidRuntimeSession,
    RuntimeSessionNotFound,
)
from adapter.biz.service.runtime_session.command import RegisterRuntimeSession
from adapter.schemas.runtime_session.request import RegisterRuntimeSessionRequest
from adapter.schemas.runtime_session.response import RuntimeSessionResponse


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
