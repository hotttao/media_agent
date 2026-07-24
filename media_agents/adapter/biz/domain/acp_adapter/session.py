"""ACP 会话：面向 SSE 连接层的 session 状态与事件流。

AcpSession 绑定一个 AcpBridge 子进程，接收 AcpClientAdapter 转发来的
session_update / request_permission，翻译为 UniversalEvent 写入 SessionSink
供 SSE generator 消费。

设计要点（见 docs/wiki/agents/acp.md）：
1. 删除原 AcpSessionHandler，由 AcpSession 直接承载会话事件流状态
2. AcpSession 面相 SSE 连接层（数据发送），AcpBridge 面向 agent core 层
3. busy_turns / last_active / pending_destroy 用于 LRU 驱逐与 busy bridge
   延后销毁
"""

from __future__ import annotations

import asyncio
import json
import logging
from typing import TYPE_CHECKING, Any

from acp.schema import (
    AgentMessageChunk,
    AgentPlanContentUpdate,
    AgentPlanRemovedUpdate,
    AgentPlanUpdate,
    AgentThoughtChunk,
    AllowedOutcome,
    AvailableCommandsUpdate,
    ConfigOptionUpdate,
    CurrentModeUpdate,
    PermissionOption,
    RequestPermissionResponse,
    SessionInfoUpdate,
    ToolCallProgress,
    ToolCallStart,
    ToolCallUpdate,
    UsageUpdate,
    UserMessageChunk,
)

from adapter.biz.domain.acp_adapter.entity import PromptResult, StopReason
from adapter.biz.domain.acp_adapter.llm_event import to_event_dict
from adapter.biz.domain.acp_adapter.translator import AcpEventTranslator

if TYPE_CHECKING:
    from collections.abc import AsyncIterator

    from adapter.biz.domain.acp_adapter.bridge import AcpBridge

logger = logging.getLogger(__name__)


# SDK SessionUpdate union 的完整类型注解（对标 examples/client.py）
SessionUpdate = (
    UserMessageChunk
    | AgentMessageChunk
    | AgentThoughtChunk
    | ToolCallStart
    | ToolCallProgress
    | AgentPlanUpdate
    | AgentPlanContentUpdate
    | AgentPlanRemovedUpdate
    | AvailableCommandsUpdate
    | CurrentModeUpdate
    | ConfigOptionUpdate
    | SessionInfoUpdate
    | UsageUpdate
)


class SessionStream:
    """SSE 事件缓冲：asyncio.Queue 串起 translator 翻译后的 UniversalEvent。

    write 侧由 AcpSession.session_update 调用（async）；read 侧由 SSE
    generator 消费。close 后写入 sentinel 让 read generator 正常退出。
    """

    _SENTINEL: tuple[str, str] = ("\x00", "\x00")

    def __init__(self, session_id: str) -> None:
        self.session_id = session_id
        self._queue: asyncio.Queue[tuple[str, str]] = asyncio.Queue()
        self._buffer: list[tuple[str, str]] = []
        self._writer_seq: int = 0
        self._active_writer_id: int | None = None
        self.disconnected: bool = False
        self.closed: bool = False
        self.done: asyncio.Event = asyncio.Event()

    def set_session_id(self, session_id: str) -> None:
        self.session_id = session_id

    @property
    def writer_id(self) -> int | None:
        return self._active_writer_id

    @property
    def buffered_events(self) -> list[tuple[str, str]]:
        return list(self._buffer)

    def attach_writer(self) -> int:
        self._writer_seq += 1
        self._active_writer_id = self._writer_seq
        self.disconnected = False
        return self._writer_seq

    def detach_writer(self, expected_writer_id: int | None) -> bool:
        if expected_writer_id is not None and self._active_writer_id != expected_writer_id:
            return False
        if self.closed:
            return False
        self._active_writer_id = None
        self.disconnected = True
        return True

    async def write(self, event: str, data: str) -> None:
        """写入一条 SSE 事件（event 名 + data 字符串）。"""
        if self.closed:
            return
        if self.disconnected:
            self._buffer.append((event, data))
            return
        await self._queue.put((event, data))

    def close(self) -> None:
        """关闭 sink：写入 sentinel 让 read generator 退出。"""
        if self.closed:
            return
        self.closed = True
        self.done.set()
        self._queue.put_nowait(self._SENTINEL)

    async def read(self, keepalive_interval: float = 0.0) -> AsyncIterator[str]:
        """SSE 事件 generator。遇到 sentinel 退出。

        keepalive_interval > 0 时，空闲超时后 yield ``: \\n\\n`` comment frame
        作为心跳，避免 nginx / uvicorn 长连接超时断开（对标 TS KEEPALIVE_MS）。
        """
        while self._buffer:
            event, data = self._buffer.pop(0)
            yield self._format(event, data)
        while True:
            try:
                if keepalive_interval > 0:
                    event, data = await asyncio.wait_for(
                        self._queue.get(), timeout=keepalive_interval
                    )
                else:
                    event, data = await self._queue.get()
            except TimeoutError:
                yield ": \n\n"
                continue
            if (event, data) == self._SENTINEL:
                break
            yield self._format(event, data)

    @staticmethod
    def _format(event: str, data: str) -> str:
        if event == ":":
            return ": \n\n"
        return f"event: {event}\ndata: {data}\n\n"


SessionSink = SessionStream


class AcpSession:
    """一个 ACP 会话。面向 SSE 连接层。

    - session_update: ACP session/update 通知 -> translator -> sink（SSE）
    - request_permission: 默认接受第一个选项（对标 TS onPermissionRequest）
    """

    def __init__(self, session_id: str, bridge: AcpBridge) -> None:
        self.session_id = session_id
        self.bridge = bridge
        self.stream = SessionStream(session_id)
        self.sink = self.stream
        self.translator = AcpEventTranslator(session_id)
        # 运行时状态（原 entity.py 的 AcpSession dataclass 字段）
        self.busy_turns: int = 0
        self.last_active: float = 0.0
        self.pending_destroy: bool = False

    def set_session_id(self, session_id: str) -> None:
        """Update the resolved platform session id across session collaborators."""
        self.session_id = session_id
        self.stream.set_session_id(session_id)
        self.translator.set_session_id(session_id)

    def open_stream(self) -> SessionStream:
        """Create a fresh turn stream and attach the initial SSE reader."""
        self.stream = SessionStream(self.session_id)
        self.sink = self.stream
        self.stream.attach_writer()
        return self.stream

    def attach_stream(self) -> SessionStream:
        """Attach a reconnect reader to the current turn stream."""
        self.stream.attach_writer()
        if self.stream.done.is_set():
            self.stream.close()
        return self.stream

    def detach_stream(self, expected_writer_id: int | None) -> bool:
        """Detach a reader; switches to buffer mode if ownership still matches."""
        return self.stream.detach_writer(expected_writer_id)

    def finish_stream(self) -> None:
        """Mark the turn done; keep disconnected streams around for reconnect."""
        self.stream.done.set()
        if not self.stream.disconnected:
            self.stream.close()

    # ── Client adapter 转发来的核心回调 ─────────────────────

    async def session_update(
        self,
        session_id: str,
        update: SessionUpdate,
        **kwargs: Any,
    ) -> None:
        """ACP session/update -> translator 翻译为 UniversalEvent -> sink。

        translator 有状态，跨 update 维护 current_message_item / active_tool_calls。
        所有翻译出的 event 走 "message" 事件名，data 是 UniversalEvent 的
        json 序列化（对标 TS SSE event: message + data: JSON）。
        """
        for event in self.translator.translate(update):
            await self.write_event(event)

    async def write_event(self, event: Any) -> None:
        """Write one UniversalEvent to the SSE sink as structured JSON."""
        await self.sink.write("message", json.dumps(to_event_dict(event), ensure_ascii=False))

    async def emit_started(self) -> None:
        """Emit the session.started lifecycle event for this SSE stream."""
        await self.write_event(self.translator.session_started(self.session_id))

    async def request_permission(
        self,
        session_id: str,
        tool_call: ToolCallUpdate,
        options: list[PermissionOption],
        **kwargs: Any,
    ) -> RequestPermissionResponse:
        """默认接受第一个选项。

        对标 TS:
            const optionId = req.options?.[0]?.optionId ?? 'allow-once'
            return { outcome: { outcome: 'selected', optionId } }

        返回 SDK RequestPermissionResponse(AllowedOutcome(option_id=...))，
        而非裸 dict —— SDK 用 discriminated union 校验 outcome 字段。
        """
        option_id = options[0].option_id if options else "allow-once"
        return RequestPermissionResponse(
            outcome=AllowedOutcome(outcome="selected", option_id=option_id)
        )

    # ── SSE 读取（router 层消费）────────────────────────────

    async def read(self, keepalive_interval: float = 0.0) -> AsyncIterator[str]:
        """返回 SSE 事件 generator 供 StreamingResponse 消费。"""
        async for chunk in self.stream.read(keepalive_interval=keepalive_interval):
            yield chunk

    # ── turn 执行（AcpServer 调用）─────────────────────────

    async def run_turn(self, message: str, images: list[Any] | None = None) -> PromptResult:
        """运行一个 turn：busy_turns 计数 + bridge.prompt。

        busy_turns 用于 /reload-config 与 LRU 驱逐时避免销毁正在执行 turn 的
        bridge（见 entity.py 原 AcpSession dataclass 注释）。
        """
        self.busy_turns += 1
        try:
            result = await self.bridge.prompt(self.session_id, message, images)
            for event in self.translator.finalize():
                await self.write_event(event)
            reason = "interrupted" if result.stop_reason == StopReason.CANCELLED else "completed"
            await self.write_event(
                self.translator.session_ended(reason, self.translator.build_stats(result))
            )
            return result
        except Exception:
            await self.write_event(self.translator.session_ended("error"))
            raise
        finally:
            self.busy_turns -= 1

    def cancel(self) -> None:
        """中断当前 turn。"""
        self.bridge.cancel(self.session_id)

    def close(self) -> None:
        """关闭 sink，让 SSE read generator 退出。"""
        self.stream.close()
