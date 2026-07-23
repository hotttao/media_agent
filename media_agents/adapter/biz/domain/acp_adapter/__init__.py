"""ACP 应用顶层：AcpServer 管理所有 session 和 bridge。

对标 TS AcpApp。全局唯一实例（FastAPI lifespan 注入 app.state）。承载原
/chat /reconnect /interrupt 的编排逻辑：session 解析 -> bridge create/load
-> run_turn。router 层只做请求解析 + 调 AcpServer + SSE 适配。

设计要点（见 docs/wiki/agents/acp.md）：
1. 顶层添加 AcpServer，管理所有 AcpSession 和 bridge
2. /chat /reconnect 的逻辑都交给 AcpServer
3. AcpSession 面相 SSE 连接层，AcpBridge 面向 agent core 层
"""

from __future__ import annotations

import logging
import uuid
from typing import TYPE_CHECKING, Any

from adapter.biz.domain.acp_adapter.acp_abs import AgentHooks, BridgeFactory
from adapter.biz.domain.acp_adapter.entity import Chat, PromptResult
from adapter.biz.domain.acp_adapter.errors import BridgeNotFound, LoadSessionFailed
from adapter.biz.domain.acp_adapter.session import AcpSession

if TYPE_CHECKING:
    from adapter.biz.domain.acp_adapter.bridge import AcpBridge

logger = logging.getLogger(__name__)


class AcpServer:
    """对标 AcpApp: 一个 ACP 应用，管理多个 session 和 bridge。

    全局唯一实例（FastAPI lifespan 注入 app.state）。承载 /chat /reconnect
    /interrupt 的编排逻辑。router 层只做请求解析 + 调 AcpServer + SSE 适配。
    """

    def __init__(self, agent_hooks: AgentHooks, bridge_factory: BridgeFactory) -> None:
        self.hooks: AgentHooks = agent_hooks
        self.factory: BridgeFactory = bridge_factory
        self.sessions: dict[str, AcpSession] = {}
        self.bridges: dict[str, AcpBridge] = {}

    # ── /chat 编排 ──────────────────────────────────────────

    async def chat(self, chat: Chat) -> tuple[AcpSession, bool]:
        """POST /chat 编排：resolve/load/create session。

        返回 (session, is_new)。router 层拿到 session 后：
            1. 发送 session.started
            2. 调 AcpServer.run_turn(session, message, images)
            3. 发送 session.ended
            4. async for session.read() 出 SSE 流
        """
        session_id = chat.session_id

        if session_id and session_id in self.sessions:
            # 复用内存中存活的 session
            session = self.sessions[session_id]
            return session, False

        if session_id:
            # 加载已持久化的 session（rollout / SQLite）
            session = await self.load_session(session_id, chat.session_token)
            return session, False

        # 全新 session
        session = await self.create_session(chat.session_token)
        return session, True

    async def run_turn(
        self, session: AcpSession, message: str, images: list[Any] | None = None
    ) -> PromptResult:
        """运行一个 turn：session.run_turn + 记录 usage。

        单 turn 完成后调 hooks.record_usage；reused bridge poison 重建重试
        由 router 层 try/except 处理（对标原 /chat handler 的重试一次逻辑）。
        """
        result = await session.run_turn(message, images)
        try:
            self.hooks.record_usage(session.session_id, result.usage)
        except Exception as err:
            logger.error("record_usage failed session=%s: %s", session.session_id, err)
        return result

    # ── /reconnect 编排 ─────────────────────────────────────

    async def reconnect(self, session_id: str) -> AcpSession:
        """POST /reconnect 编排：从已有 session 恢复 SSE 流。

        session 必须已在内存中（/chat 时创建）；否则返回 BridgeNotFound。
        router 层 flush buffer 后 async for session.read()。
        """
        session = self.sessions.get(session_id)
        if session is None:
            raise BridgeNotFound(session_id)
        return session

    # ── /interrupt 编排 ─────────────────────────────────────

    def cancel(self, session_id: str) -> bool:
        """POST /interrupt 编排。"""
        session = self.sessions.get(session_id)
        if session is None:
            return False
        session.cancel()
        return True

    # ── 查询 ─────────────────────────────────────────────────

    def get(self, session_id: str) -> AcpSession | None:
        """查询 session（供 router health check 等）。"""
        return self.sessions.get(session_id)

    def __len__(self) -> int:
        return len(self.sessions)

    # ── 内部：session 创建/加载 ──────────────────────────────

    async def create_session(self, session_token: str | None) -> AcpSession:
        """创建新 session：bridge.start -> wait_for_mcp_ready -> create_session。"""
        draft_id = str(uuid.uuid4())
        bridge = self.factory(draft_id)
        await bridge.start()
        mcp_servers = self.hooks.load_mcp_servers(session_token)
        if self.hooks.has_mcp_servers():
            await bridge.wait_for_mcp_ready()
        session = await bridge.create_session(mcp_servers=mcp_servers)
        self.sessions[session.session_id] = session
        self.bridges[session.session_id] = bridge
        logger.info("acp session created: session=%s", session.session_id)
        return session

    async def load_session(self, session_id: str, session_token: str | None) -> AcpSession:
        """加载已持久化的 session：bridge.start -> load_session。"""
        bridge = self.factory(session_id)
        await bridge.start()
        mcp_servers = self.hooks.load_mcp_servers(session_token)
        try:
            session = await bridge.load_session(session_id, mcp_servers=mcp_servers)
        except LoadSessionFailed:
            bridge.destroy()
            raise
        except Exception as err:
            bridge.destroy()
            raise LoadSessionFailed(session_id, str(err)) from err
        self.sessions[session.session_id] = session
        self.bridges[session.session_id] = bridge
        logger.info("acp session loaded: session=%s", session.session_id)
        return session
