from __future__ import annotations

from datetime import UTC, datetime
from typing import Any, cast

import pytest

from adapter.biz.domain.acp_adapter import AcpServer
from adapter.biz.domain.acp_adapter.entity import PromptResult, StopReason
from adapter.biz.domain.acp_adapter.session import AcpSession
from adapter.biz.domain.runtime_session.entity import RuntimeSession, Turn, TurnStatus
from adapter.biz.workflow.acp_turn_runtime import AcpTurnRuntime
from adapter.biz.workflow.execute_turn_flow import RuntimeCredentials


class Hooks:
    token: str | None = None

    def load_mcp_servers(self, session_token: str | None) -> list[Any]:
        self.token = session_token
        return []

    def has_mcp_servers(self) -> bool:
        return False

    def record_usage(self, session_id: str, usage: object) -> None:
        pass


class Bridge:
    cancelled = False

    async def start(self) -> None:
        pass

    async def create_session(self, mcp_servers: list[Any]) -> AcpSession:
        return AcpSession("acp-runtime-1", cast(Any, self))

    async def load_session(self, session_id: str, mcp_servers: list[Any]) -> AcpSession:
        return AcpSession(session_id, cast(Any, self))

    async def prompt(
        self, session_id: str, message: str, images: list[Any] | None = None
    ) -> PromptResult:
        return PromptResult(StopReason.CANCELLED if self.cancelled else StopReason.END_TURN)

    def cancel(self, session_id: str) -> None:
        self.cancelled = True

    def destroy(self) -> None:
        pass

    def is_alive(self) -> bool:
        return True


@pytest.mark.asyncio
async def test_acp_runtime_creates_executes_and_interrupts_with_ephemeral_credentials() -> None:
    hooks = Hooks()
    bridge = Bridge()
    observed_key: list[str | None] = []

    def factory(draft_id: str, api_key: str | None = None) -> Bridge:
        observed_key.append(api_key)
        return bridge

    server = AcpServer(cast(Any, hooks), cast(Any, factory))
    runtime = AcpTurnRuntime(server)
    session = RuntimeSession.register(session_id="business-1", context_version=1, context_ref="ctx")
    turn = Turn(
        "turn-1",
        "business-1",
        "key",
        1,
        TurnStatus.RUNNING,
        {"message": "hello"},
        1,
        datetime.now(UTC),
        started_at=datetime.now(UTC),
    )
    credentials = RuntimeCredentials(api_key="sk-acp", session_token="session-token")

    prepared = await runtime.prepare(session, turn, credentials)
    assert prepared.handle == "acp-runtime-1"
    assert observed_key == ["sk-acp"]
    assert hooks.token == "session-token"
    assert (
        await runtime.execute(
            prepared,
            session,
            turn,
            credentials,
            _ignore_event,
            lambda _request_type, _payload: _allow_once(),
        )
    ).status is TurnStatus.COMPLETED
    assert await runtime.interrupt(prepared.handle)
    assert (
        await runtime.execute(
            prepared,
            session,
            turn,
            credentials,
            _ignore_event,
            lambda _request_type, _payload: _allow_once(),
        )
    ).status is TurnStatus.INTERRUPTED


async def _ignore_event(payload: dict[str, object]) -> None:
    pass


async def _allow_once() -> dict[str, object]:
    return {"option_id": "allow-once"}
