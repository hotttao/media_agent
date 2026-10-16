"""Managed Claude Code CLI processes and Universal Event translation."""

from __future__ import annotations

import asyncio
import json
import os
import time
import uuid
from collections.abc import AsyncIterator
from dataclasses import dataclass

from adapter.biz.domain.acp_adapter.llm_event import (
    ContentDelta,
    ContentPart,
    SessionEndReason,
    UniversalEvent,
    UniversalItem,
    to_event_dict,
)
from claude_code.control import RequestHandler, tool_decision


class ClaudeCodeError(RuntimeError):
    """Claude CLI could not execute or emitted an invalid terminal result."""


@dataclass(slots=True)
class _RunningProcess:
    process: asyncio.subprocess.Process
    interrupted: bool = False


class ClaudeCodeAgent:
    def __init__(self, program: str = "claude", cwd: str = "/workspace") -> None:
        self._program = program
        self._cwd = cwd
        self._running: dict[str, _RunningProcess] = {}
        self._lock = asyncio.Lock()

    async def stream(
        self,
        *,
        message: str,
        session_id: str | None,
        api_key: str | None,
        resume: bool = True,
        metadata_args: list[str] | None = None,
        model_name: str = "",
        base_url: str = "",
        request_handler: RequestHandler | None = None,
    ) -> AsyncIterator[dict[str, object]]:
        runtime_session_id = session_id or str(uuid.uuid4())
        args = self._command(message, session_id if resume else None, runtime_session_id)
        if request_handler is not None:
            args.pop()  # The user message goes over stdin, never a second CLI prompt.
            args.extend(["--input-format", "stream-json", "--permission-prompt-tool", "stdio"])
        if metadata_args:
            args[1:1] = metadata_args
        env = os.environ.copy()
        for key in (
            "ANTHROPIC_API_KEY",
            "ANTHROPIC_AUTH_TOKEN",
            "CLAUDE_CODE_OAUTH_TOKEN",
            "ANTHROPIC_BASE_URL",
            "ANTHROPIC_MODEL",
        ):
            env.pop(key, None)
        if api_key:
            env["ANTHROPIC_API_KEY"] = api_key
        if model_name:
            env["ANTHROPIC_MODEL"] = model_name
        if base_url:
            env["ANTHROPIC_BASE_URL"] = base_url
        env["DISABLE_NONESSENTIAL_TRAFFIC"] = "1"
        process = await asyncio.create_subprocess_exec(
            *args,
            cwd=self._cwd,
            env=env,
            stdin=asyncio.subprocess.PIPE
            if request_handler is not None
            else asyncio.subprocess.DEVNULL,
            limit=1024 * 1024,
            stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.PIPE,
        )
        stderr_task = asyncio.create_task(_read_stderr(process))
        running = _RunningProcess(process)
        async with self._lock:
            if runtime_session_id in self._running:
                process.terminate()
                await process.wait()
                await stderr_task
                raise ClaudeCodeError("a Turn is already running for this session")
            self._running[runtime_session_id] = running

        yield _event(
            UniversalEvent(type="session.started", timestamp=_now(), session_id=runtime_session_id)
        )
        translator = ClaudeStreamTranslator(runtime_session_id)
        terminal: SessionEndReason = "completed"
        try:
            if request_handler is not None:
                await _write_control(
                    process,
                    {
                        "type": "control_request",
                        "request_id": "initialize",
                        "request": {"subtype": "initialize"},
                    },
                )
                await _write_control(
                    process,
                    {
                        "type": "user",
                        "session_id": runtime_session_id,
                        "message": {"role": "user", "content": message},
                        "parent_tool_use_id": None,
                    },
                )
            assert process.stdout is not None
            async for raw_line in process.stdout:
                line = raw_line.decode("utf-8", errors="replace").strip()
                if not line:
                    continue
                try:
                    payload = json.loads(line)
                except json.JSONDecodeError as error:
                    raise ClaudeCodeError("Claude CLI emitted invalid stream-json") from error
                if isinstance(payload, dict) and payload.get("type") == "control_request":
                    request = payload.get("request")
                    if (
                        request_handler is not None
                        and isinstance(request, dict)
                        and request.get("subtype") == "can_use_tool"
                    ):
                        decision = await tool_decision(request, request_handler)
                        await _write_control(
                            process,
                            {
                                "type": "control_response",
                                "response": {
                                    "subtype": "success",
                                    "request_id": payload.get("request_id"),
                                    "response": decision,
                                },
                            },
                        )
                    else:
                        await _write_control(
                            process,
                            {
                                "type": "control_response",
                                "response": {
                                    "subtype": "error",
                                    "request_id": payload.get("request_id"),
                                    "error": "Unsupported control request",
                                },
                            },
                        )
                    continue
                if isinstance(payload, dict) and payload.get("type") == "result":
                    subtype = str(payload.get("subtype") or "")
                    if payload.get("is_error") is True or subtype not in {"", "success"}:
                        terminal = "error"
                        yield _provider_error(str(payload.get("result") or subtype))
                    if process.stdin is not None:
                        process.stdin.close()
                    break
                for event in translator.translate(payload):
                    yield event
            return_code = await process.wait()
            stderr = await stderr_task
            if running.interrupted:
                terminal = "interrupted"
            elif return_code != 0:
                terminal = "error"
                yield _provider_error(stderr)
        except asyncio.CancelledError:
            await _terminate(process)
            await stderr_task
            raise
        except Exception as error:
            terminal = "interrupted" if running.interrupted else "error"
            await _terminate(process)
            await stderr_task
            if not running.interrupted:
                yield _provider_error(type(error).__name__)
        finally:
            async with self._lock:
                self._running.pop(runtime_session_id, None)
        yield _event(
            UniversalEvent(
                type="session.ended",
                timestamp=_now(),
                session_id=runtime_session_id,
                reason=terminal,
            )
        )

    async def interrupt(self, session_id: str) -> bool:
        async with self._lock:
            running = self._running.get(session_id)
            if running is None:
                return False
            running.interrupted = True
            process = running.process
        await _terminate(process)
        return True

    def _command(
        self, message: str, existing_session_id: str | None, new_session_id: str
    ) -> list[str]:
        args = [
            self._program,
            "--print",
            "--verbose",
            "--output-format",
            "stream-json",
            "--include-partial-messages",
        ]
        if existing_session_id:
            args.extend(["--resume", existing_session_id])
        else:
            args.extend(["--session-id", new_session_id])
        args.append(message)
        return args


async def _write_control(process: asyncio.subprocess.Process, payload: dict[str, object]) -> None:
    if process.stdin is None or process.stdin.is_closing():
        raise ClaudeCodeError("Claude control channel is closed")
    process.stdin.write((json.dumps(payload) + "\n").encode())
    await process.stdin.drain()


def _provider_error(message: str) -> dict[str, object]:
    authentication = any(
        v in message.lower()
        for v in ("authentication", "unauthorized", "invalid api key", "401", "please log in")
    )
    return {
        "type": "error",
        "timestamp": _now(),
        "error_class": "model_authentication_required" if authentication else "provider_error",
        "message": "Model authentication is required"
        if authentication
        else "The model request failed",
    }


class ClaudeStreamTranslator:
    def __init__(self, session_id: str) -> None:
        self.session_id = session_id
        self.message_id = ""
        self.started: set[str] = set()

    def translate(self, payload: object) -> list[dict[str, object]]:
        if isinstance(payload, dict) and payload.get("type") == "stream_event":
            event = payload.get("event")
            if not isinstance(event, dict):
                return []
            if event.get("type") == "message_start":
                message = event.get("message")
                self.message_id = (
                    str(message.get("id") or uuid.uuid4())
                    if isinstance(message, dict)
                    else str(uuid.uuid4())
                )
                self.started.add(self.message_id)
                return [
                    _event(
                        UniversalEvent(
                            type="item.started",
                            timestamp=_now(),
                            session_id=self.session_id,
                            item=UniversalItem(
                                item_id=self.message_id,
                                kind="message",
                                role="assistant",
                                content=[],
                            ),
                        )
                    )
                ]
            delta = event.get("delta")
            if self.message_id and isinstance(delta, dict) and delta.get("type") == "text_delta":
                return [
                    _event(
                        UniversalEvent(
                            type="item.delta",
                            timestamp=_now(),
                            session_id=self.session_id,
                            item_id=self.message_id,
                            delta=ContentDelta(type="text", text=str(delta.get("text") or "")),
                        )
                    )
                ]
            return []
        events = translate_claude_event(payload, self.session_id)
        filtered: list[dict[str, object]] = []
        for event in events:
            if event.get("type") == "item.delta" and event.get("item_id") in self.started:
                continue
            item = event.get("item")
            if (
                event.get("type") == "item.started"
                and isinstance(item, dict)
                and item.get("item_id") in self.started
            ):
                continue
            filtered.append(event)
        return filtered


def translate_claude_event(payload: object, session_id: str) -> list[dict[str, object]]:
    if not isinstance(payload, dict):
        return []
    event_type = payload.get("type")
    if event_type == "assistant":
        message = payload.get("message")
        if not isinstance(message, dict):
            return []
        item_id = str(message.get("id") or uuid.uuid4())
        content = message.get("content")
        if not isinstance(content, list):
            return []
        parts: list[ContentPart] = []
        text = ""
        events: list[dict[str, object]] = []
        for block in content:
            if not isinstance(block, dict):
                continue
            if block.get("type") == "text":
                block_text = str(block.get("text") or "")
                text += block_text
                parts.append(ContentPart(type="text", text=block_text))
            elif block.get("type") == "tool_use":
                call_id = str(block.get("id") or uuid.uuid4())
                arguments = json.dumps(block.get("input") or {}, separators=(",", ":"))
                tool = UniversalItem(
                    item_id=f"tool-{call_id}",
                    kind="tool_call",
                    role="assistant",
                    status="completed",
                    content=[
                        ContentPart(
                            type="tool_call",
                            call_id=call_id,
                            name=str(block.get("name") or "tool"),
                            arguments=arguments,
                        )
                    ],
                )
                events.append(
                    _event(
                        UniversalEvent(
                            type="item.completed",
                            timestamp=_now(),
                            session_id=session_id,
                            item=tool,
                        )
                    )
                )
        if parts:
            item = UniversalItem(item_id=item_id, kind="message", role="assistant", content=[])
            events.insert(
                0,
                _event(
                    UniversalEvent(
                        type="item.started", timestamp=_now(), session_id=session_id, item=item
                    )
                ),
            )
            if text:
                events.insert(
                    1,
                    _event(
                        UniversalEvent(
                            type="item.delta",
                            timestamp=_now(),
                            session_id=session_id,
                            item_id=item_id,
                            delta=ContentDelta(type="text", text=text),
                        )
                    ),
                )
            item.status = "completed"
            item.content = parts
            events.insert(
                2,
                _event(
                    UniversalEvent(
                        type="item.completed", timestamp=_now(), session_id=session_id, item=item
                    )
                ),
            )
        return events
    if event_type == "user":
        message = payload.get("message")
        content = message.get("content") if isinstance(message, dict) else None
        events = []
        if isinstance(content, list):
            for block in content:
                if not isinstance(block, dict) or block.get("type") != "tool_result":
                    continue
                call_id = str(block.get("tool_use_id") or uuid.uuid4())
                output = block.get("content")
                if not isinstance(output, str):
                    output = json.dumps(output, separators=(",", ":"))
                item = UniversalItem(
                    item_id=f"result-{call_id}",
                    kind="tool_result",
                    role="tool",
                    status="completed",
                    content=[
                        ContentPart(
                            type="tool_result",
                            call_id=call_id,
                            output=output,
                            is_error=bool(block.get("is_error")),
                        )
                    ],
                )
                events.append(
                    _event(
                        UniversalEvent(
                            type="item.completed",
                            timestamp=_now(),
                            session_id=session_id,
                            item=item,
                        )
                    )
                )
        return events
    return []


async def _terminate(process: asyncio.subprocess.Process) -> None:
    if process.returncode is not None:
        return
    process.terminate()
    try:
        await asyncio.wait_for(process.wait(), timeout=3)
    except TimeoutError:
        process.kill()
        await process.wait()


async def _read_stderr(process: asyncio.subprocess.Process) -> str:
    if process.stderr is None:
        return ""
    return (await process.stderr.read()).decode("utf-8", errors="replace").strip()


def _event(event: UniversalEvent) -> dict[str, object]:
    return to_event_dict(event)


def _now() -> int:
    return int(time.time() * 1000)
