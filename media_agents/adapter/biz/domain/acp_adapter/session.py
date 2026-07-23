"""ACP 会话：实现 SDK Client 协议，面向 SSE 连接层。

对标 TS AcpSessionHandler + SessionSink 的合并体。一个 session 绑定一个
AcpBridge 子进程；AcpSession 作为 connect_to_agent 的 client 参数，接收
SDK 的 session_update / request_permission 回调，翻译为 UniversalEvent 写入
SessionSink 供 SSE generator 消费。

设计要点（见 docs/wiki/agents/acp.md）：
1. 删除原 AcpSessionHandler，由 AcpSession 直接实现 Client 接口
2. AcpSession 面相 SSE 连接层（数据发送），AcpBridge 面向 agent core 层
3. busy_turns / last_active / pending_destroy 用于 LRU 驱逐与 busy bridge
   延后销毁
"""

from __future__ import annotations

import asyncio
import json
import logging
from typing import TYPE_CHECKING, Any

from acp import RequestError
from acp.schema import (
    AgentMessageChunk,
    AgentPlanContentUpdate,
    AgentPlanRemovedUpdate,
    AgentPlanUpdate,
    AgentThoughtChunk,
    AllowedOutcome,
    AvailableCommandsUpdate,
    ConfigOptionUpdate,
    CreateElicitationResponse,
    CreateTerminalResponse,
    CurrentModeUpdate,
    DeclineElicitationResponse,
    KillTerminalResponse,
    PermissionOption,
    ReadTextFileResponse,
    ReleaseTerminalResponse,
    RequestPermissionResponse,
    SessionInfoUpdate,
    TerminalOutputResponse,
    ToolCallProgress,
    ToolCallStart,
    ToolCallUpdate,
    UsageUpdate,
    UserMessageChunk,
    WaitForTerminalExitResponse,
    WriteTextFileResponse,
)

from adapter.biz.domain.acp_adapter.entity import PromptResult
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
    | UsageUpdate,
)


class SessionSink:
    """SSE 事件缓冲：asyncio.Queue 串起 translator 翻译后的 UniversalEvent。

    write 侧由 AcpSession.session_update 调用（async）；read 侧由 SSE
    generator 消费。close 后写入 sentinel 让 read generator 正常退出。
    """

    _SENTINEL: tuple[str, str] = ("\x00", "\x00")

    def __init__(self, session_id: str) -> None:
        self.session_id = session_id
        self._queue: asyncio.Queue[tuple[str, str]] = asyncio.Queue()
        self.closed: bool = False

    async def write(self, event: str, data: str) -> None:
        """写入一条 SSE 事件（event 名 + data 字符串）。"""
        if self.closed:
            return
        await self._queue.put((event, data))

    def close(self) -> None:
        """关闭 sink：写入 sentinel 让 read generator 退出。"""
        if self.closed:
            return
        self.closed = True
        self._queue.put_nowait(self._SENTINEL)

    async def read(self) -> AsyncIterator[str]:
        """SSE 事件 generator。遇到 sentinel 退出。"""
        while True:
            event, data = await self._queue.get()
            if (event, data) == self._SENTINEL:
                break
            yield f"event: {event}\ndata: {data}\n\n"


class AcpSession:
    """一个 ACP 会话。实现 SDK Client 协议，面向 SSE 连接层。

    - session_update: ACP session/update 通知 -> translator -> sink（SSE）
    - request_permission: 默认接受第一个选项（对标 TS onPermissionRequest）
    - 文件 / 终端 / 扩展方法: 默认 method_not_found（cp 端不支持），agent
      子类可 override 启用具体能力
    """

    def __init__(self, session_id: str, bridge: AcpBridge) -> None:
        self.session_id = session_id
        self.bridge = bridge
        self.sink = SessionSink(session_id)
        self.translator = AcpEventTranslator(session_id)
        # 运行时状态（原 entity.py 的 AcpSession dataclass 字段）
        self.busy_turns: int = 0
        self.last_active: float = 0.0
        self.pending_destroy: bool = False

    # ── Client 协议（SDK ClientSideConnection 回调）──────────

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
            await self.sink.write("message", json.dumps(event, ensure_ascii=False, default=str))

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

    # ── 文件系统（cp 端默认不开放，agent 子类可 override）────

    async def write_text_file(
        self, session_id: str, path: str, content: str, **kwargs: Any
    ) -> WriteTextFileResponse | None:
        raise RequestError.method_not_found("fs/write_text_file")

    async def read_text_file(
        self, session_id: str, path: str, line: int | None = None, limit: int | None = None, **kwargs: Any
    ) -> ReadTextFileResponse:
        raise RequestError.method_not_found("fs/read_text_file")

    # ── 终端（cp 端默认不开放，agent 子类可 override）──────

    async def create_terminal(
        self,
        session_id: str,
        command: str,
        args: list[str] | None = None,
        env: list[Any] | None = None,
        cwd: str | None = None,
        output_byte_limit: int | None = None,
        **kwargs: Any,
    ) -> CreateTerminalResponse:
        raise RequestError.method_not_found("terminal/create")

    async def terminal_output(
        self, session_id: str, terminal_id: str, **kwargs: Any
    ) -> TerminalOutputResponse:
        raise RequestError.method_not_found("terminal/output")

    async def release_terminal(
        self, session_id: str, terminal_id: str, **kwargs: Any
    ) -> ReleaseTerminalResponse | None:
        raise RequestError.method_not_found("terminal/release")

    async def wait_for_terminal_exit(
        self, session_id: str, terminal_id: str, **kwargs: Any
    ) -> WaitForTerminalExitResponse:
        raise RequestError.method_not_found("terminal/wait_for_exit")

    async def kill_terminal(
        self, session_id: str, terminal_id: str, **kwargs: Any
    ) -> KillTerminalResponse | None:
        raise RequestError.method_not_found("terminal/kill")

    # ── Elicitation（agent 请求用户输入，默认 decline）──────

    async def create_elicitation(
        self, message: str, mode: Any, **kwargs: Any
    ) -> CreateElicitationResponse:
        """agent 请求用户输入时默认拒绝（cp 端不展示 elicitation UI）。"""
        return DeclineElicitationResponse(action="decline")

    async def complete_elicitation(self, elicitation_id: str, **kwargs: Any) -> None:
        """elicitation 完成通知，默认无操作。"""

    # ── 扩展方法 / 通知 ─────────────────────────────────────

    async def ext_method(self, method: str, params: dict[str, Any]) -> dict[str, Any]:
        """扩展方法，默认 method_not_found。agent 子类可 override 处理私有方法。"""
        raise RequestError.method_not_found(method)

    async def ext_notification(self, method: str, params: dict[str, Any]) -> None:
        """扩展通知，默认忽略。agent 子类可 override 处理 goose/codex 私有事件。"""

    def on_connect(self, conn: Any) -> None:
        """ClientSideConnection 建立时的回调。骨架阶段无操作。"""

    # ── SSE 读取（router 层消费）────────────────────────────

    async def read(self) -> AsyncIterator[str]:
        """返回 SSE 事件 generator 供 StreamingResponse 消费。"""
        async for chunk in self.sink.read():
            yield chunk

    # ── turn 执行（AcpServer 调用）─────────────────────────

    async def run_turn(self, message: str, images: list[Any] | None = None) -> PromptResult:
        """运行一个 turn：busy_turns 计数 + bridge.prompt。

        busy_turns 用于 /reload-config 与 LRU 驱逐时避免销毁正在执行 turn 的
        bridge（见 entity.py 原 AcpSession dataclass 注释）。
        """
        self.busy_turns += 1
        try:
            return await self.bridge.prompt(self.session_id, message, images)
        finally:
            self.busy_turns -= 1

    def cancel(self) -> None:
        """中断当前 turn。"""
        self.bridge.cancel(self.session_id)

    def close(self) -> None:
        """关闭 sink，让 SSE read generator 退出。"""
        self.sink.close()
