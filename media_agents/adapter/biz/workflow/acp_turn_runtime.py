"""ACP implementation of the Execute Turn Runtime port."""

from __future__ import annotations

from collections.abc import Callable
from pathlib import Path

from acp.schema import HttpHeader, HttpMcpServer

from adapter.biz.domain.acp_adapter import AcpServer
from adapter.biz.domain.acp_adapter.bridge import AcpBridge, McpServerConfig
from adapter.biz.domain.acp_adapter.entity import StopReason
from adapter.biz.domain.runtime_metadata.entity import VersionLoader
from adapter.biz.domain.runtime_session.entity import RuntimeSession, Turn, TurnStatus
from adapter.biz.domain.runtime_session.errors import InvalidTurn
from adapter.biz.workflow.execute_turn_flow import (
    AgentRequestHandler,
    RuntimeCredentials,
    RuntimePreparation,
    RuntimeResult,
)
from adapter.biz.workflow.turn_event_sink import RuntimeEventHandler


class AcpTurnRuntime:
    def __init__(
        self,
        server: AcpServer,
        versions: VersionLoader | None = None,
        configure: Callable[[object, Path, str], None] | None = None,
    ) -> None:
        self._server = server
        self._versions = versions
        self._configure = configure

    async def prepare(
        self, session: RuntimeSession, turn: Turn, credentials: RuntimeCredentials
    ) -> RuntimePreparation:
        if session.context_ref.startswith("metadata:") and self._versions is None:
            raise InvalidTurn("versioned metadata unavailable")
        directory = (
            await self._versions.resolve_session(session.context_ref)
            if self._versions is not None
            else None
        )
        setup = None
        if directory is not None:
            if self._configure is None:
                raise InvalidTurn("versioned runtime configuration unavailable")
            configure = self._configure

            def setup(bridge: AcpBridge) -> None:
                configure(bridge, directory, session.session_id)

        servers: list[McpServerConfig] | None = None
        if credentials.mcp_url:
            servers = [
                HttpMcpServer(
                    type="http",
                    name="media-agent-gateway",
                    url=credentials.mcp_url,
                    headers=[
                        HttpHeader(name=k, value=v) for k, v in credentials.mcp_headers.items()
                    ],
                )
            ]
        if session.runtime_session_id is not None:
            if servers is not None:
                await self._server.detach_session(session.runtime_session_id)
            runtime_session = self._server.get(session.runtime_session_id)
            if runtime_session is None:
                runtime_session = await self._server.load_session(
                    session.runtime_session_id,
                    session_token=credentials.session_token,
                    api_key=credentials.api_key,
                    configure_bridge=setup,
                    mcp_servers_override=servers,
                )
            return RuntimePreparation(runtime_session.session_id, is_new=False)
        runtime_session = await self._server.create_session(
            session_token=credentials.session_token,
            api_key=credentials.api_key,
            configure_bridge=setup,
            mcp_servers_override=servers,
        )
        return RuntimePreparation(runtime_session.session_id, is_new=True)

    async def execute(
        self,
        preparation: RuntimePreparation,
        session: RuntimeSession,
        turn: Turn,
        credentials: RuntimeCredentials,
        event_handler: RuntimeEventHandler,
        request_handler: AgentRequestHandler,
    ) -> RuntimeResult:
        runtime_session = self._server.get(preparation.handle)
        if runtime_session is None:
            raise InvalidTurn("prepared ACP Runtime session is unavailable")
        message = turn.input.get("message")
        if not isinstance(message, str) or not message.strip():
            raise InvalidTurn("Turn input.message is required")
        runtime_session.set_event_observer(event_handler)
        runtime_session.set_permission_observer(
            lambda payload: request_handler("permission", payload)
        )
        try:
            result = await self._server.run_turn(runtime_session, message)
        finally:
            runtime_session.set_event_observer(None)
            runtime_session.set_permission_observer(None)
        status = (
            TurnStatus.INTERRUPTED
            if result.stop_reason is StopReason.CANCELLED
            else TurnStatus.COMPLETED
        )
        return RuntimeResult(status)

    async def interrupt(self, handle: str) -> bool:
        return self._server.cancel(handle)
