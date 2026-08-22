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
    ) -> AsyncIterator[dict[str, object]]:
        runtime_session_id = session_id or str(uuid.uuid4())
        args = self._command(message, session_id if resume else None, runtime_session_id)
        env = os.environ.copy()
        if api_key:
            env["ANTHROPIC_API_KEY"] = api_key
        process = await asyncio.create_subprocess_exec(
            *args,
            cwd=self._cwd,
            env=env,
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
        terminal: SessionEndReason = "completed"
        try:
            assert process.stdout is not None
            async for raw_line in process.stdout:
                line = raw_line.decode("utf-8", errors="replace").strip()
                if not line:
                    continue
                try:
                    payload = json.loads(line)
                except json.JSONDecodeError as error:
                    raise ClaudeCodeError("Claude CLI emitted invalid stream-json") from error
                if isinstance(payload, dict) and payload.get("type") == "result":
                    subtype = str(payload.get("subtype") or "")
                    if payload.get("is_error") is True or subtype not in {"", "success"}:
                        terminal = "error"
                        yield {
                            "type": "error",
                            "timestamp": _now(),
                            "message": str(payload.get("result") or subtype or "Claude CLI failed"),
                        }
                for event in translate_claude_event(payload, runtime_session_id):
                    yield event
            return_code = await process.wait()
            stderr = await stderr_task
            if running.interrupted:
                terminal = "interrupted"
            elif return_code != 0:
                terminal = "error"
                yield {
                    "type": "error",
                    "timestamp": _now(),
                    "message": stderr or f"Claude CLI exited with {return_code}",
                }
        except asyncio.CancelledError:
            await _terminate(process)
            await stderr_task
            raise
        except Exception as error:
            terminal = "error"
            await _terminate(process)
            await stderr_task
            yield {"type": "error", "timestamp": _now(), "message": str(error)}
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
        args = [self._program, "--print", "--verbose", "--output-format", "stream-json"]
        if existing_session_id:
            args.extend(["--resume", existing_session_id])
        else:
            args.extend(["--session-id", new_session_id])
        args.append(message)
        return args


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
