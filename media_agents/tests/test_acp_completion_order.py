"""A slow persistent observer must not lose chunks at the native RPC boundary."""

import asyncio
import sys
from pathlib import Path

import pytest
from acp import RequestError

from adapter.biz.domain.acp_adapter.bridge import AcpBridge


@pytest.mark.asyncio
@pytest.mark.parametrize("failing", [False, True])
async def test_prompt_response_waits_for_all_native_chunks_before_finalizing(
    tmp_path: Path,
    failing: bool,
) -> None:
    bridge = AcpBridge(
        program=sys.executable,
        args=[str(Path(__file__).with_name("acp_chunk_fixture.py"))],
        cwd=str(tmp_path),
    )
    blocked = asyncio.Event()
    release = asyncio.Event()
    events: list[dict[str, object]] = []

    async def persist(event: dict[str, object]) -> None:
        if event["type"] == "item.delta" and not blocked.is_set():
            blocked.set()
            await release.wait()
        events.append(event)

    try:
        await bridge.start()
        session = await bridge.create_session()
        session.set_event_observer(persist)
        task = asyncio.create_task(session.run_turn("fail" if failing else "reply"))
        await asyncio.wait_for(blocked.wait(), 3)
        await asyncio.sleep(0.05)
        assert not task.done()
        release.set()
        if failing:
            with pytest.raises(RequestError):
                await asyncio.wait_for(task, 3)
        else:
            await asyncio.wait_for(task, 3)
        completed = [e for e in events if e["type"] == "item.completed"]
        if not failing:
            assert len(completed) == 1
            assert "8c7189b0-7cd3-4c8d-927f-4ace400a2ebc" in str(completed[0])
        assert sum(e["type"] == "item.delta" for e in events) == 5
        assert events[-1]["type"] == "session.ended"
        terminal_count = len(events)
        await asyncio.sleep(0.05)
        assert len(events) == terminal_count
    finally:
        release.set()
        bridge.destroy()
        if bridge.proc is not None:
            await bridge.proc.wait()
