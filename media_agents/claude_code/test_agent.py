from __future__ import annotations

from pathlib import Path

import pytest

from claude_code.agent import ClaudeCodeAgent, translate_claude_event


def _fake_claude(tmp_path: Path, body: str) -> Path:
    executable = tmp_path / "fake-claude"
    executable.write_text("#!/usr/bin/env python3\n" + body, encoding="utf-8")
    executable.chmod(0o755)
    return executable


@pytest.mark.asyncio
async def test_agent_streams_universal_events_and_passes_key_only_in_environment(
    tmp_path: Path,
) -> None:
    executable = _fake_claude(
        tmp_path,
        """import json, os, sys
assert os.environ.get("ANTHROPIC_API_KEY") == "sk-ant-test"
assert "--session-id" in sys.argv and "hello" == sys.argv[-1]
content = [{"type":"text","text":"Hello from Claude"}]
message = {"type":"assistant","message":{"id":"message-1","content":content}}
print(json.dumps(message), flush=True)
print(json.dumps({"type":"result","subtype":"success"}), flush=True)
""",
    )
    agent = ClaudeCodeAgent(str(executable), str(tmp_path))

    events = [
        event
        async for event in agent.stream(message="hello", session_id=None, api_key="sk-ant-test")
    ]

    assert [event["type"] for event in events] == [
        "session.started",
        "item.started",
        "item.delta",
        "item.completed",
        "session.ended",
    ]
    assert events[0]["session_id"] == events[-1]["session_id"]
    assert events[3]["item"]["content"][0]["text"] == "Hello from Claude"  # type: ignore[index]
    assert "sk-ant-test" not in repr(events)


@pytest.mark.asyncio
async def test_agent_interrupts_active_process(tmp_path: Path) -> None:
    executable = _fake_claude(
        tmp_path,
        """import time
time.sleep(30)
""",
    )
    agent = ClaudeCodeAgent(str(executable), str(tmp_path))
    stream = agent.stream(message="wait", session_id="session-1", api_key=None)

    started = await anext(stream)
    assert started["type"] == "session.started"
    assert await agent.interrupt("session-1") is True
    remaining = [event async for event in stream]
    assert remaining[-1]["type"] == "session.ended"
    assert remaining[-1]["reason"] == "interrupted"
    assert await agent.interrupt("session-1") is False


@pytest.mark.asyncio
async def test_agent_maps_failed_result_to_error_terminal(tmp_path: Path) -> None:
    executable = _fake_claude(
        tmp_path,
        """import json
result = {"type":"result","subtype":"error_during_execution"}
result.update({"is_error":True,"result":"provider rejected request"})
print(json.dumps(result), flush=True)
""",
    )
    agent = ClaudeCodeAgent(str(executable), str(tmp_path))

    events = [event async for event in agent.stream(message="fail", session_id=None, api_key=None)]

    assert [event["type"] for event in events] == [
        "session.started",
        "error",
        "session.ended",
    ]
    assert events[-1]["reason"] == "error"


def test_translate_claude_tool_use_and_result() -> None:
    tool_events = translate_claude_event(
        {
            "type": "assistant",
            "message": {
                "id": "m1",
                "content": [
                    {"type": "tool_use", "id": "call-1", "name": "Read", "input": {"path": "a"}}
                ],
            },
        },
        "session-1",
    )
    result_events = translate_claude_event(
        {
            "type": "user",
            "message": {
                "content": [{"type": "tool_result", "tool_use_id": "call-1", "content": "ok"}]
            },
        },
        "session-1",
    )

    assert tool_events[0]["item"]["kind"] == "tool_call"  # type: ignore[index]
    assert result_events[0]["item"]["kind"] == "tool_result"  # type: ignore[index]
