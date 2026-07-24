from __future__ import annotations

import asyncio
from typing import Any

import pytest

from adapter.biz.domain.acp_adapter import AcpServer
from adapter.biz.domain.acp_adapter.entity import Chat, PromptResult, StopReason
from adapter.biz.domain.acp_adapter.errors import BridgeNotFound


class _Hooks:
    def load_mcp_servers(self, session_token: str | None) -> list[Any]:
        return []

    def has_mcp_servers(self) -> bool:
        return False

    async def load_config(self) -> bool:
        return True

    async def load_skills(self) -> tuple[bool, list[str]]:
        return True, []

    async def load_credentials(self) -> bool:
        return True

    async def restart_bridge(self) -> None:
        pass

    def record_usage(self, session_id: str, usage: Any) -> None:
        pass


class _Bridge:
    def __init__(self, draft_id: str) -> None:
        self.draft_id = draft_id
        self.destroyed = False

    async def start(self) -> None:
        pass

    async def wait_for_mcp_ready(self) -> None:
        pass

    async def create_session(self, mcp_servers: list[Any]) -> Any:
        from adapter.biz.domain.acp_adapter.session import AcpSession

        return AcpSession("s1", self)

    async def load_session(self, session_id: str, mcp_servers: list[Any]) -> Any:
        from adapter.biz.domain.acp_adapter.session import AcpSession

        return AcpSession(session_id, self)

    async def prompt(
        self, session_id: str, message: str, images: list[Any] | None = None
    ) -> PromptResult:
        return PromptResult(stop_reason=StopReason.END_TURN)

    def cancel(self, session_id: str) -> None:
        pass

    def destroy(self) -> None:
        self.destroyed = True

    def is_alive(self) -> bool:
        return not self.destroyed


@pytest.mark.asyncio
async def test_chat_opens_stream_and_reconnect_attaches_same_stream() -> None:
    server = AcpServer(_Hooks(), lambda draft_id: _Bridge(draft_id))
    session, _ = await server.chat(Chat(message="hello"))
    stream = session.stream
    writer_id = stream.writer_id

    assert writer_id is not None
    server.detach_stream(session.session_id, writer_id)
    await session.write_event(session.translator.session_started(session.session_id))

    reconnected = await server.reconnect(session.session_id)

    assert reconnected is stream
    assert reconnected.writer_id != writer_id
    assert reconnected.buffered_events


@pytest.mark.asyncio
async def test_reconnect_requires_active_stream_not_just_session() -> None:
    server = AcpServer(_Hooks(), lambda draft_id: _Bridge(draft_id))
    session, _ = await server.chat(Chat(message="hello"))
    session.finish_stream()

    with pytest.raises(BridgeNotFound):
        await server.reconnect(session.session_id)


@pytest.mark.asyncio
async def test_destroy_closes_active_stream() -> None:
    server = AcpServer(_Hooks(), lambda draft_id: _Bridge(draft_id))
    session, _ = await server.chat(Chat(message="hello"))

    server._destroy(session.session_id)

    assert session.stream.closed is True


@pytest.mark.asyncio
async def test_reconnect_after_done_flushes_buffer_and_exits() -> None:
    server = AcpServer(_Hooks(), lambda draft_id: _Bridge(draft_id))
    session, _ = await server.chat(Chat(message="hello"))
    writer_id = session.stream.writer_id
    server.detach_stream(session.session_id, writer_id)
    await session.write_event(session.translator.session_started(session.session_id))
    session.finish_stream()

    stream = await server.reconnect(session.session_id)
    reader = stream.read()

    assert "session.started" in await anext(reader)
    assert "session.started" in await anext(reader)
    with pytest.raises(StopAsyncIteration):
        await asyncio.wait_for(anext(reader), timeout=0.1)
