from __future__ import annotations

from datetime import UTC, datetime
from pathlib import Path

import pytest

from adapter.biz.domain.runtime_session.entity import RuntimeSession, Turn, TurnStatus
from adapter.biz.workflow.execute_turn_flow import RuntimeCredentials
from claude_code.agent import ClaudeCodeAgent, translate_claude_event
from claude_code.turn_runtime import ClaudeCodeTurnRuntime


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


@pytest.mark.asyncio
async def test_turn_runtime_uses_new_then_resumed_runtime_handle(tmp_path: Path) -> None:
    executable = _fake_claude(
        tmp_path,
        """import json, os, sys
assert ("--session-id" in sys.argv) != ("--resume" in sys.argv)
assert os.environ.get("ANTHROPIC_API_KEY") == "sk-ant-runtime"
print(json.dumps({"type":"result","subtype":"success"}), flush=True)
""",
    )
    runtime = ClaudeCodeTurnRuntime(ClaudeCodeAgent(str(executable), str(tmp_path)))
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

    credentials = RuntimeCredentials(api_key="sk-ant-runtime")
    first = await runtime.prepare(session, turn, credentials)
    assert first.handle == "business-1" and first.is_new
    assert (
        await runtime.execute(first, session, turn, credentials, _ignore_event, _ignore_request)
    ).status is TurnStatus.COMPLETED

    resumed_session = session.attach_runtime("business-1", now=datetime.now(UTC))
    resumed = await runtime.prepare(resumed_session, turn, credentials)
    assert not resumed.is_new
    assert (
        await runtime.execute(
            resumed, resumed_session, turn, credentials, _ignore_event, _ignore_request
        )
    ).status is TurnStatus.COMPLETED


async def _ignore_event(payload: dict[str, object]) -> None:
    pass


async def _ignore_request(request_type: str, payload: dict[str, object]) -> dict[str, object]:
    return {"option_id": "allow-once"}
