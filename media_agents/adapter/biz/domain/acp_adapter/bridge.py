"""ACP bridge：封装子进程生命周期 + SDK ClientSideConnection。面向 agent core 层。

对标 TS AcpBridge：一个 ACP session 对应一个子进程 bridge。Agent 实现自己的
AcpBridge（codex-acp / goose-acp 子进程），注入到 AcpServer 的 bridge_factory。

数据流：
    AcpBridge.prompt -> ClientSideConnection.prompt -> 子进程 stdin（JSON-RPC）
    子进程 stdout -> ClientSideConnection -> AcpSession.session_update -> sink

关键：用 SDK 的 connect_to_agent + ClientSideConnection，不手写 JSON-RPC 帧编解码。
启动顺序严格遵守 SDK 协议：connect_to_agent -> initialize（握手）-> new_session。
"""

from __future__ import annotations

import asyncio
import logging
import os
from typing import TYPE_CHECKING, Any

from acp import PROTOCOL_VERSION, connect_to_agent, image_block, text_block
from acp.schema import (
    ClientCapabilities,
    HttpMcpServer,
    Implementation,
    LoadSessionResponse,
    McpServerStdio,
    NewSessionResponse,
    SseMcpServer,
)

from adapter.biz.domain.acp_adapter.entity import PromptResult, StopReason
from adapter.biz.domain.acp_adapter.errors import LoadSessionFailed
from adapter.biz.domain.acp_adapter.session import AcpSession

if TYPE_CHECKING:
    # ClientSideConnection 从 acp.core 导入会触发 deprecation warning
    # （推荐用 connect_to_agent 工厂），故仅在类型检查期导入。
    from acp.client.connection import ClientSideConnection

logger = logging.getLogger(__name__)


# MCP 服务器配置的联合类型（对标 SDK schema 的 mcp_servers 参数）
McpServerConfig = HttpMcpServer | SseMcpServer | McpServerStdio


class AcpBridge:
    """一个 ACP session 对应一个子进程 bridge。面向 agent core 层。

    Agent 实现自己的 AcpBridge（codex-acp / goose-acp 子进程），注入到
    AcpServer 的 bridge_factory。adapter 不关心具体子进程，只通过此类
    调用 SDK ClientSideConnection。
    """

    def __init__(
        self,
        *,
        program: str,
        args: list[str] | None = None,
        cwd: str = "/workspace",
        env: dict[str, str] | None = None,
        session_id: str = "",
    ) -> None:
        self.program = program
        self.args = args or []
        self.cwd = cwd
        self.env = env or {}
        # draft UUID（platform 生成），create/load 后由 AcpSession.session_id
        # 更新为 agent resolved 的真实 session_id
        self.session_id = session_id
        self.proc: asyncio.subprocess.Process | None = None
        self.conn: ClientSideConnection | None = None
        self.session: AcpSession | None = None

    async def start(self) -> None:
        """启动子进程 + 创建 ClientSideConnection + JSON-RPC 握手。

        严格遵循 SDK 协议顺序：
            1. asyncio.create_subprocess_exec 启动 agent 子进程
            2. connect_to_agent(session, proc.stdin, proc.stdout) 创建连接
            3. conn.initialize() 完成 JSON-RPC 握手（必须在 new_session 之前）

        AcpSession 作为 client 传给 connect_to_agent；SDK 的
        ClientSideConnection 内部启动 reader loop 读取子进程 stdout，
        将 notification 分发到 AcpSession.session_update 等回调。
        """
        env = {**os.environ, **self.env}
        self.proc = await asyncio.create_subprocess_exec(
            self.program,
            *self.args,
            stdin=asyncio.subprocess.PIPE,
            stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.PIPE,
            cwd=self.cwd,
            env=env,
        )
        self.session = AcpSession(self.session_id, self)
        self.conn = connect_to_agent(
            client=self.session,
            input_stream=self.proc.stdin,
            output_stream=self.proc.stdout,
        )
        # JSON-RPC 握手：必须在 new_session / load_session 之前完成。
        # agent_info 标识 adapter 自身，agent_capabilities 声明支持的扩展。
        await self.conn.initialize(
            protocol_version=PROTOCOL_VERSION,
            client_capabilities=ClientCapabilities(),
            client_info=Implementation(
                name="acp-bridge",
                title="ACP Bridge",
                version="0.1.0",
            ),
        )
        logger.info(
            "acp bridge started: program=%s pid=%s",
            self.program,
            self.proc.pid,
        )

    def is_alive(self) -> bool:
        """子进程是否存活（进程级检查，不能发现 session 级 poison）。"""
        return self.proc is not None and self.proc.returncode is None

    async def create_session(
        self, *, mcp_servers: list[McpServerConfig] | None = None
    ) -> AcpSession:
        """创建新 ACP session。返回 AcpSession（session_id 更新为 resolved_id）。

        对标 SDK: ``conn.new_session(cwd, mcp_servers)``。resp.session_id 是
        agent resolved 的真实 session_id（可能不同于 draft UUID）。
        """
        assert self.conn is not None and self.session is not None, "bridge not started"
        resp: NewSessionResponse = await self.conn.new_session(
            cwd=self.cwd,
            mcp_servers=mcp_servers or [],
        )
        self.session.session_id = resp.session_id
        return self.session

    async def load_session(
        self, session_id: str, *, mcp_servers: list[McpServerConfig] | None = None
    ) -> AcpSession:
        """加载已持久化的 session（rollout / SQLite）。失败抛 LoadSessionFailed。

        对标 SDK: ``conn.load_session(cwd, session_id, mcp_servers)``。
        LoadSessionResponse 无 session_id 字段（agent 沿用传入的 id），故
        不更新 self.session.session_id。
        """
        assert self.conn is not None and self.session is not None, "bridge not started"
        try:
            resp: LoadSessionResponse | None = await self.conn.load_session(
                cwd=self.cwd,
                session_id=session_id,
                mcp_servers=mcp_servers or [],
            )
        except Exception as err:
            raise LoadSessionFailed(session_id, str(err)) from err
        # LoadSessionResponse 只有 modes / config_options / _meta，无 session_id。
        # agent 确认加载后沿用传入的 session_id，无需更新。
        if resp is not None and getattr(resp, "modes", None):
            # modes 不为空时记录日志，供调试（骨架阶段不消费 modes）
            logger.debug("acp session loaded: session=%s modes=%s", session_id, resp.modes)
        return self.session

    async def wait_for_mcp_ready(self) -> None:
        """等待 MCP 服务器启动完成。

        ACP 协议无标准 wait_for_mcp_ready 方法；codex/goose 通过 ext_method
        或轮询 session_update 实现。骨架阶段留空，agent 子类可 override。
        """

    async def prompt(
        self, session_id: str, message: str, images: list[Any] | None = None
    ) -> PromptResult:
        """发送一个 prompt turn，等待 PromptResponse。

        构造 SDK text_block / image_block 作为 prompt content，对齐 ACP 协议。
        stop_reason 直接从 SDK StopReason Literal 映射到 entity.StopReason
        StrEnum（值一一对应）。
        """
        assert self.conn is not None, "bridge not started"
        prompt: list[Any] = [text_block(message)]
        if images:
            for img in images:
                data = getattr(img, "data", None) or (
                    img.get("data") if isinstance(img, dict) else ""
                )
                media_type = getattr(img, "media_type", None) or (
                    img.get("media_type") if isinstance(img, dict) else ""
                )
                if data and media_type:
                    prompt.append(image_block(data, media_type))

        resp = await self.conn.prompt(session_id=session_id, prompt=prompt)
        # SDK StopReason Literal 与 entity.StopReason StrEnum 值一一对应，
        # 可直接构造；None 兜底为 END_TURN（不应发生但防御性处理）
        raw_stop = getattr(resp, "stop_reason", None)
        stop = StopReason(raw_stop) if raw_stop else StopReason.END_TURN
        return PromptResult(stop_reason=stop, usage=getattr(resp, "usage", None))

    def cancel(self, session_id: str) -> None:
        """中断正在执行的 turn。"""
        if self.conn:
            asyncio.create_task(self.conn.cancel(session_id))

    def destroy(self) -> None:
        """销毁子进程 + 关闭 connection + 关闭 sink。"""
        if self.conn:
            asyncio.create_task(self.conn.close())
        if self.proc and self.proc.returncode is None:
            self.proc.terminate()
        if self.session:
            self.session.close()
