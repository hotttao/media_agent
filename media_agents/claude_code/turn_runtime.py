"""Claude Code implementation of the Execute Turn Runtime port."""

from __future__ import annotations

import json

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
from adapter.runtime_metadata_files import claude_args
from claude_code.agent import ClaudeCodeAgent


class ClaudeCodeTurnRuntime:
    def __init__(self, agent: ClaudeCodeAgent, versions: VersionLoader | None = None) -> None:
        self._agent = agent
        self._versions = versions

    async def prepare(
        self, session: RuntimeSession, turn: Turn, credentials: RuntimeCredentials
    ) -> RuntimePreparation:
        handle = session.runtime_session_id or session.session_id
        return RuntimePreparation(handle, is_new=session.runtime_session_id is None)

    async def execute(
        self,
        preparation: RuntimePreparation,
        session: RuntimeSession,
        turn: Turn,
        credentials: RuntimeCredentials,
        event_handler: RuntimeEventHandler,
        request_handler: AgentRequestHandler,
    ) -> RuntimeResult:
        message = turn.input.get("message")
        if not isinstance(message, str) or not message.strip():
            raise InvalidTurn("Turn input.message is required")
        terminal = TurnStatus.FAILED
        error_class: str | None = None
        if session.context_ref.startswith("metadata:") and self._versions is None:
            raise InvalidTurn("versioned metadata unavailable")
        directory = (
            await self._versions.resolve_session(session.context_ref)
            if self._versions is not None
            else None
        )
        args = claude_args(directory) if directory else []
        # Inline configuration never persists entry or instance credentials to shared storage.
        if credentials.mcp_url:
            args.extend(
                [
                    "--strict-mcp-config",
                    "--mcp-config",
                    json.dumps(
                        {
                            "mcpServers": {
                                "media-agent-gateway": {
                                    "type": "http",
                                    "url": credentials.mcp_url,
                                    "headers": credentials.mcp_headers,
                                }
                            }
                        }
                    ),
                ]
            )
        async for event in self._agent.stream(
            message=message,
            session_id=preparation.handle,
            api_key=credentials.api_key,
            model_name=credentials.model_name,
            base_url=credentials.model_base_url,
            resume=not preparation.is_new,
            metadata_args=args or None,
            request_handler=request_handler,
        ):
            await event_handler(event)
            if event.get("type") == "error":
                error_class = str(event.get("error_class") or "provider_error")
            if event.get("type") == "session.ended":
                reason = event.get("reason")
                if reason == "completed":
                    terminal = TurnStatus.COMPLETED
                elif reason == "interrupted":
                    terminal = TurnStatus.INTERRUPTED
                else:
                    terminal = TurnStatus.FAILED
        return RuntimeResult(terminal, error_class)

    async def interrupt(self, handle: str) -> bool:
        return await self._agent.interrupt(handle)
