from __future__ import annotations

from typing import Any, Literal

from acp.schema import ToolCallProgress, ToolCallStart

from adapter.biz.domain.acp_adapter.translator import AcpEventTranslator

ToolKind = Literal[
    "read", "edit", "delete", "move", "search", "execute", "think", "fetch", "switch_mode", "other"
]
ToolStatus = Literal["pending", "in_progress", "completed", "failed"]


def _tool_start(
    tool_call_id: str,
    *,
    title: str = "Run tool",
    kind: ToolKind | None = "execute",
    status: ToolStatus | None = None,
    raw_input: object | None = None,
    field_meta: dict[str, object] | None = None,
) -> ToolCallStart:
    return ToolCallStart.model_construct(
        tool_call_id=tool_call_id,
        title=title,
        kind=kind,
        status=status,
        raw_input=raw_input,
        field_meta=field_meta,
        session_update="tool_call",
    )


def _tool_progress(
    tool_call_id: str,
    *,
    title: str | None = None,
    kind: ToolKind | None = None,
    status: ToolStatus | None = None,
    raw_input: object | None = None,
    raw_output: object | None = None,
    content: list[Any] | None = None,
    field_meta: dict[str, object] | None = None,
) -> ToolCallProgress:
    return ToolCallProgress.model_construct(
        tool_call_id=tool_call_id,
        title=title,
        kind=kind,
        status=status,
        raw_input=raw_input,
        raw_output=raw_output,
        content=content,
        field_meta=field_meta,
        session_update="tool_call_update",
    )


def test_mcp_startup_tool_call_is_suppressed() -> None:
    translator = AcpEventTranslator("s1")

    events = translator.translate(
        _tool_start(
            "mcp_startup.tos-platform",
            title="MCP startup",
            kind="other",
            status="completed",
        )
    )

    assert events == []


def test_goose_tool_name_prefers_structured_meta() -> None:
    translator = AcpEventTranslator("s1")
    meta: dict[str, object] = {
        "goose": {"toolCall": {"toolName": "tos-platform__sandbox_run_command"}}
    }

    events = translator.translate(
        _tool_start(
            "call_1",
            title="model generated display title",
            kind="other",
            field_meta=meta,
        )
    )

    assert len(events) == 1
    item = events[0].item
    assert item is not None
    assert item.content[0].name == "tos-platform__sandbox_run_command"


def test_image_generation_content_completes_and_summarizes_output() -> None:
    translator = AcpEventTranslator("s1")
    translator.translate(
        _tool_start(
            "ig_123",
            title="Image generation",
            kind="other",
            status="in_progress",
            raw_input={"prompt": "cat"},
        )
    )

    events = translator.translate(
        _tool_progress(
            "ig_123",
            content=[{"type": "image", "data": "base64-inline"}],
            raw_output={
                "result": "very-large-base64",
                "savedPath": "/workspace/out.png",
                "revisedPrompt": "a cat",
            },
        )
    )

    assert [event.type for event in events] == ["item.completed", "item.completed"]
    result_item = events[1].item
    assert result_item is not None
    output = result_item.content[0].output
    assert output == "Image generated.\nRevised prompt: a cat\nSaved to: /workspace/out.png"
    assert "very-large-base64" not in output


def test_question_requested_event_matches_universal_protocol() -> None:
    translator = AcpEventTranslator("s1")

    event = translator.question_requested("req_1", [{"id": "q1", "label": "Continue?"}])

    assert event.type == "question.requested"
    assert event.session_id == "s1"
    assert event.request_id == "req_1"
    assert event.questions == [{"id": "q1", "label": "Continue?"}]


def test_failed_tool_call_keeps_call_completed_and_marks_result_failed() -> None:
    translator = AcpEventTranslator("s1")
    translator.translate(_tool_start("call_fail", status="in_progress"))

    events = translator.translate(_tool_progress("call_fail", status="failed", raw_output="boom"))

    assert len(events) == 2
    tool_call = events[0].item
    tool_result = events[1].item
    assert tool_call is not None
    assert tool_result is not None
    assert tool_call.status == "completed"
    assert tool_result.status == "failed"
    assert tool_result.content[0].is_error is True
