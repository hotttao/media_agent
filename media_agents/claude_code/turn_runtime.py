"""Claude Code implementation of the Execute Turn Runtime port."""

from __future__ import annotations

from adapter.biz.domain.runtime_session.entity import RuntimeSession, Turn, TurnStatus
from adapter.biz.domain.runtime_session.errors import InvalidTurn
from adapter.biz.workflow.execute_turn_flow import (
    AgentRequestHandler,
    RuntimeCredentials,
    RuntimePreparation,
    RuntimeResult,
)
from adapter.biz.workflow.turn_event_sink import RuntimeEventHandler
from claude_code.agent import ClaudeCodeAgent


class ClaudeCodeTurnRuntime:
    def __init__(self, agent: ClaudeCodeAgent) -> None:
        self._agent = agent

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
        async for event in self._agent.stream(
            message=message,
            session_id=preparation.handle,
            api_key=credentials.api_key,
            resume=not preparation.is_new,
        ):
            await event_handler(event)
            if event.get("type") == "error":
                error_class = "provider_error"
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
