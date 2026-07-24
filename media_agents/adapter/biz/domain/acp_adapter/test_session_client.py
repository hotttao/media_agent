import json
from dataclasses import dataclass
from typing import Any

import pytest
from acp import RequestError

from adapter.biz.domain.acp_adapter.client import AcpClientAdapter
from adapter.biz.domain.acp_adapter.llm_event import ContentDelta, UniversalEvent
from adapter.biz.domain.acp_adapter.session import AcpSession, SessionStream


class _Bridge:
    async def prompt(self, session_id: str, message: str, images: list[Any] | None = None) -> Any:
        return None

    def cancel(self, session_id: str) -> None:
        pass


@dataclass
class _FakeTranslator:
    event: UniversalEvent

    def translate(self, update: Any) -> list[UniversalEvent]:
        return [self.event]

    def set_session_id(self, session_id: str) -> None:
        self.event.session_id = session_id


@pytest.mark.asyncio
async def test_session_update_writes_structured_json_event() -> None:
    event = UniversalEvent(
        type="item.delta",
        timestamp=123,
        session_id="s1",
        item_id="i1",
        delta=ContentDelta(type="text", text="hello"),
    )
    session = AcpSession("s1", _Bridge())
    session.translator = _FakeTranslator(event)  # type: ignore[assignment]

    await session.session_update("s1", object())

    chunk = await anext(session.read())
    assert chunk.startswith("event: message\n")
    data = chunk.split("data: ", 1)[1].strip()
    assert json.loads(data) == {
        "type": "item.delta",
        "timestamp": 123,
        "session_id": "s1",
        "item_id": "i1",
        "delta": {"type": "text", "text": "hello"},
    }


def test_set_session_id_keeps_session_collaborators_in_sync() -> None:
    session = AcpSession("draft", _Bridge())

    session.set_session_id("resolved")

    assert session.session_id == "resolved"
    assert session.sink.session_id == "resolved"
    assert session.translator.session_id == "resolved"


@pytest.mark.asyncio
async def test_client_adapter_rejects_terminal_capability_by_default() -> None:
    session = AcpSession("s1", _Bridge())
    client = AcpClientAdapter(session)

    with pytest.raises(RequestError):
        await client.create_terminal("s1", "echo", args=["ok"])


@pytest.mark.asyncio
async def test_session_stream_buffers_events_after_owned_writer_detaches() -> None:
    stream = SessionStream("s1")
    writer_id = stream.attach_writer()

    stream.detach_writer(writer_id)
    await stream.write("message", "{}")

    assert stream.disconnected is True
    assert stream.buffered_events == [("message", "{}")]


@pytest.mark.asyncio
async def test_session_stream_old_writer_cannot_detach_reconnected_writer() -> None:
    stream = SessionStream("s1")
    old_writer_id = stream.attach_writer()
    new_writer_id = stream.attach_writer()

    stream.detach_writer(old_writer_id)
    await stream.write("message", "{}")

    assert stream.disconnected is False
    assert stream.buffered_events == []
    reader = stream.read()
    assert await anext(reader) == "event: message\ndata: {}\n\n"
    await reader.aclose()
    stream.detach_writer(new_writer_id)


@pytest.mark.asyncio
async def test_session_stream_reconnect_flushes_buffer_then_live_events() -> None:
    stream = SessionStream("s1")
    writer_id = stream.attach_writer()
    stream.detach_writer(writer_id)
    await stream.write("message", '{"n":1}')

    stream.attach_writer()
    reader = stream.read()
    assert await anext(reader) == 'event: message\ndata: {"n":1}\n\n'

    await stream.write("message", '{"n":2}')
    assert await anext(reader) == 'event: message\ndata: {"n":2}\n\n'
    await reader.aclose()


@pytest.mark.asyncio
async def test_session_stream_reconnect_preserves_queued_event_order() -> None:
    stream = SessionStream("s1")
    writer_id = stream.attach_writer()
    await stream.write("message", '{"n":1}')

    stream.detach_writer(writer_id)
    await stream.write("message", '{"n":2}')

    stream.attach_writer()
    reader = stream.read()
    assert await anext(reader) == 'event: message\ndata: {"n":1}\n\n'
    assert await anext(reader) == 'event: message\ndata: {"n":2}\n\n'
    await reader.aclose()
