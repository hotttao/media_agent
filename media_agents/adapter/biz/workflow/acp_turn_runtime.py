"""ACP implementation of the Execute Turn Runtime port."""

from __future__ import annotations

from adapter.biz.domain.acp_adapter import AcpServer
from adapter.biz.domain.acp_adapter.entity import StopReason
from adapter.biz.domain.runtime_session.entity import RuntimeSession, Turn, TurnStatus
from adapter.biz.domain.runtime_session.errors import InvalidTurn
from adapter.biz.workflow.execute_turn_flow import (
    RuntimeCredentials,
    RuntimePreparation,
    RuntimeResult,
)
from adapter.biz.workflow.turn_event_sink import RuntimeEventHandler


class AcpTurnRuntime:
    def __init__(self, server: AcpServer) -> None:
        self._server = server

    async def prepare(
        self, session: RuntimeSession, turn: Turn, credentials: RuntimeCredentials
    ) -> RuntimePreparation:
        if session.runtime_session_id is not None:
            runtime_session = self._server.get(session.runtime_session_id)
            if runtime_session is None:
                runtime_session = await self._server.load_session(
                    session.runtime_session_id,
                    session_token=credentials.session_token,
                    api_key=credentials.api_key,
                )
            return RuntimePreparation(runtime_session.session_id, is_new=False)
        runtime_session = await self._server.create_session(
            session_token=credentials.session_token, api_key=credentials.api_key
        )
        return RuntimePreparation(runtime_session.session_id, is_new=True)

    async def execute(
        self,
        preparation: RuntimePreparation,
        session: RuntimeSession,
        turn: Turn,
        credentials: RuntimeCredentials,
        event_handler: RuntimeEventHandler,
    ) -> RuntimeResult:
        runtime_session = self._server.get(preparation.handle)
        if runtime_session is None:
            raise InvalidTurn("prepared ACP Runtime session is unavailable")
        message = turn.input.get("message")
        if not isinstance(message, str) or not message.strip():
            raise InvalidTurn("Turn input.message is required")
        runtime_session.set_event_observer(event_handler)
        try:
            result = await self._server.run_turn(runtime_session, message)
        finally:
            runtime_session.set_event_observer(None)
        status = (
            TurnStatus.INTERRUPTED
            if result.stop_reason is StopReason.CANCELLED
            else TurnStatus.COMPLETED
        )
        return RuntimeResult(status)

    async def interrupt(self, handle: str) -> bool:
        return self._server.cancel(handle)
