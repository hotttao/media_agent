from pathlib import Path

import pytest

from claude_code.agent import ClaudeCodeAgent, ClaudeStreamTranslator
from claude_code.control import tool_decision
from claude_code.test_agent import _fake_claude


def test_partial_messages_are_not_replayed_when_snapshot_arrives() -> None:
    translator = ClaudeStreamTranslator("session")
    frames = [
        {"type": "stream_event", "event": {"type": "message_start", "message": {"id": "m1"}}},
        {
            "type": "stream_event",
            "event": {
                "type": "content_block_delta",
                "delta": {"type": "text_delta", "text": "hel"},
            },
        },
        {
            "type": "stream_event",
            "event": {"type": "content_block_delta", "delta": {"type": "text_delta", "text": "lo"}},
        },
        {
            "type": "assistant",
            "message": {"id": "m1", "content": [{"type": "text", "text": "hello"}]},
        },
    ]
    events = [event for frame in frames for event in translator.translate(frame)]
    assert [e["type"] for e in events] == [
        "item.started",
        "item.delta",
        "item.delta",
        "item.completed",
    ]
    deltas = []
    for event in events:
        if event["type"] == "item.delta":
            delta = event.get("delta")
            assert isinstance(delta, dict)
            deltas.append(delta.get("text"))
    assert deltas == ["hel", "lo"]


@pytest.mark.asyncio
async def test_native_permission_round_trip_never_auto_allows(tmp_path: Path) -> None:
    native = _fake_claude(
        tmp_path,
        """import json,sys
assert "--input-format" in sys.argv and "--permission-prompt-tool" in sys.argv
assert "hello" not in sys.argv
init=json.loads(sys.stdin.readline());assert init["request"]["subtype"]=="initialize"
message=json.loads(sys.stdin.readline());assert message["message"]["content"]=="hello"
request={"subtype":"can_use_tool","tool_name":"Bash","input":{"command":"printf checked"}}
print(json.dumps({"type":"control_request","request_id":"approval-1","request":request}),flush=True)
response=json.loads(sys.stdin.readline())["response"]
assert response["request_id"]=="approval-1"
assert response["response"]["behavior"]=="deny"
print(json.dumps({"type":"result","subtype":"success"}),flush=True)
""",
    )
    requests: list[dict[str, object]] = []

    async def deny(kind: str, payload: dict[str, object]) -> dict[str, object]:
        assert kind == "permission"
        requests.append(payload)
        return {"option_id": "reject_once"}

    events = [
        e
        async for e in ClaudeCodeAgent(str(native), str(tmp_path)).stream(
            message="hello", session_id=None, api_key="private-test-key", request_handler=deny
        )
    ]
    assert len(requests) == 1
    assert events[-1]["reason"] == "completed"
    assert "private-test-key" not in repr(events) + repr(requests)


@pytest.mark.asyncio
async def test_questions_become_user_input_without_implicit_answers() -> None:
    async def answer(kind: str, payload: dict[str, object]) -> dict[str, object]:
        assert kind == "input" and "Which format?" in str(payload["question"])
        return {"value": "Markdown"}

    result = await tool_decision(
        {
            "tool_name": "AskUserQuestion",
            "input": {
                "questions": [
                    {
                        "question": "Which format?",
                        "options": [{"label": "Markdown"}, {"label": "Text"}],
                    }
                ]
            },
        },
        answer,
    )
    assert result == {
        "behavior": "allow",
        "updatedInput": {
            "questions": [
                {"question": "Which format?", "options": [{"label": "Markdown"}, {"label": "Text"}]}
            ],
            "answers": {"Which format?": "Markdown"},
        },
    }
