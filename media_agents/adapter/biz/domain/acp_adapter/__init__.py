"""ACP 应用顶层：AcpServer 管理所有 session 和 bridge。

对标 TS AcpApp。全局唯一实例（FastAPI lifespan 注入 app.state）。承载原
/chat /reconnect /interrupt 的编排逻辑：session 解析 -> bridge create/load
-> run_turn。router 层只做请求解析 + 调 AcpServer + SSE 适配。

同时承载 session 生命周期管理（对标原 SessionManager）：
- bridge 缓存（session_id -> AcpBridge）：复用 / spawn / load / create
- busy_turns 引用计数：/reload-config 与 LRU 驱逐遇到 busy bridge 延后销毁
- LRU 驱逐：idle 超 TTL 或超 max_count 销毁（busy 进入 pending_destroy）
- pendingDestroy drain：/chat turn 结束时再尝试销毁

设计要点（见 docs/wiki/agents/acp.md）：
1. 顶层添加 AcpServer，管理所有 AcpSession 和 bridge
2. /chat /reconnect 的逻辑都交给 AcpServer
3. AcpSession 面相 SSE 连接层，AcpBridge 面向 agent core 层
"""

from __future__ import annotations

import asyncio
import logging
from typing import TYPE_CHECKING

from adapter.biz.domain.acp_adapter.acp_abs import AgentHooks, BridgeFactory
from adapter.biz.domain.acp_adapter.entity import Chat, ChatImageAttachment, PromptResult
from adapter.biz.domain.acp_adapter.errors import BridgeNotFound, LoadSessionFailed
from adapter.biz.domain.acp_adapter.session import AcpSession, SessionStream
from adapter.biz.shared.clock import Clock, SystemClock
from adapter.biz.shared.id import new_session_id

if TYPE_CHECKING:
    from adapter.biz.domain.acp_adapter.bridge import AcpBridge

logger = logging.getLogger(__name__)


class AcpServer:
    """对标 AcpApp + SessionManager 合并体：一个 ACP 应用，管理多个 session。

    全局唯一实例（FastAPI lifespan 注入 app.state）。承载 /chat /reconnect
    /interrupt 的编排逻辑 + LRU 驱逐。router 层只做请求解析 + 调 AcpServer +
    SSE 适配。
    """

    def __init__(
        self,
        agent_hooks: AgentHooks,
        bridge_factory: BridgeFactory,
        *,
        idle_ttl_ms: int = 10 * 60 * 1000,
        max_count: int = 10,
        clock: Clock | None = None,
    ) -> None:
        self.hooks: AgentHooks = agent_hooks
        self.factory: BridgeFactory = bridge_factory
        self.sessions: dict[str, AcpSession] = {}
        self.bridges: dict[str, AcpBridge] = {}
        # LRU 驱逐配置
        self.idle_ttl_ms = idle_ttl_ms
        self.max_count = max_count
        self.clock: Clock = clock or SystemClock()
        self.evict_task: asyncio.Task[None] | None = None

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
            session = self.sessions[session_id]
            # 复用内存中存活的 session（检查 bridge 是否还活着）
            session = self.sessions[session_id]
            if not session.bridge.is_alive():
                logger.warning("Bridge dead for session=%s, replacing", session_id)
                self._destroy(session_id)
                # 走 load 路径重新加载
                session = await self.load_session(session_id, chat.session_token, chat.api_key)
                session.open_stream()
                await session.emit_started()
                return session, False
            self._touch(session_id)
            session.open_stream()
            await session.emit_started()
            return session, False

        if session_id:
            session = await self.load_session(session_id, chat.session_token, chat.api_key)
            session.open_stream()
            # 加载已持久化的 session（rollout / SQLite）
            await session.emit_started()
            return session, False

        # 全新 session
        session = await self.create_session(chat.session_token, chat.api_key)
        session.open_stream()
        await session.emit_started()
        return session, True

    async def run_turn(
        self, session: AcpSession, message: str, images: list[ChatImageAttachment] | None = None
    ) -> PromptResult:
        """运行一个 turn：session.run_turn + 记录 usage + drain pending。

        单 turn 完成后调 hooks.record_usage；reused bridge poison 重建重试
        由 router 层 try/except 处理（对标原 /chat handler 的重试一次逻辑）。
        turn 结束后 drain_pending 清理 pending_destroy 的 bridge。
        """
        try:
            result = await session.run_turn(message, images)
            try:
                self.hooks.record_usage(session.session_id, result.usage)
            except Exception as err:
                logger.error("record_usage failed session=%s: %s", session.session_id, err)
            self._touch(session.session_id)
            return result
        finally:
            session.finish_stream()
            self.drain_pending(session.session_id)

    # ── /reconnect 编排 ─────────────────────────────────────

    async def reconnect(self, session_id: str) -> SessionStream:
        """POST /reconnect 编排：从已有 session 恢复 SSE 流。

        session 必须已在内存中（/chat 时创建）；否则返回 BridgeNotFound。
        router 层 flush buffer 后 async for session.read()。
        """
        session = self.sessions.get(session_id)
        if session is None or session.stream.closed:
            raise BridgeNotFound(session_id)
        self._touch(session_id)
        return session.attach_stream()

    def detach_stream(self, session_id: str, expected_writer_id: int | None) -> bool:
        session = self.sessions.get(session_id)
        if session is None:
            return False
        return session.detach_stream(expected_writer_id)

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

    # ── LRU 驱逐（对标原 SessionManager）─────────────────────

    def start_evictor(self, interval_ms: int) -> None:
        """Start the background idle bridge evictor once."""
        if self.evict_task is not None and not self.evict_task.done():
            return
        """启动后台驱逐循环：每 interval_ms 扫一次 idle TTL / cap。"""

        async def _evict_loop() -> None:
            while True:
                await asyncio.sleep(interval_ms / 1000)
                try:
                    self._evict_once()
                except Exception:
                    logger.exception("ACP bridge evictor failed")

        self.evict_task = asyncio.create_task(_evict_loop())

    def stop_evictor(self) -> None:
        if self.evict_task:
            self.evict_task.cancel()
            self.evict_task = None

    def destroy_idle(self) -> tuple[int, int]:
        """销毁所有 idle bridge，busy 延后。返回 (destroyed, deferred)。

        /reload-config 在 config/credentials 重载后调此方法刷新 bridge env。
        """
        destroyed = 0
        deferred = 0
        for sid in list(self.sessions):
            session = self.sessions.get(sid)
            if session is None:
                continue
            if session.busy_turns > 0:
                session.pending_destroy = True
                deferred += 1
            else:
                self._destroy(sid)
                destroyed += 1
        return destroyed, deferred

    def drain_pending(self, session_id: str) -> None:
        """turn 结束后再尝试销毁 pending_destroy 的 session。

        对标 /chat post-block：busy bridge 在 /reload-config 时进入
        pending_destroy，等 turn 结束后由 chat handler 的 finally 调此方法。
        """
        session = self.sessions.get(session_id)
        if session is None or not session.pending_destroy:
            return
        if session.busy_turns == 0:
            logger.info("Destroying bridge deferred by reload session=%s", session_id)
            self._destroy(session_id)

    def _evict_once(self) -> None:
        """单次驱逐扫描：drain pending -> TTL -> cap。"""
        now = self.clock.now()

        # Pass 0: drain pending_destroy（已 idle 的）
        for sid, session in list(self.sessions.items()):
            if session.pending_destroy and session.busy_turns == 0:
                logger.info("Destroying bridge deferred by reload session=%s", sid)
                self._destroy(sid)

        # Pass 1: TTL
        for sid, session in list(self.sessions.items()):
            if (now - session.last_active) * 1000 > self.idle_ttl_ms:
                if session.busy_turns > 0:
                    session.pending_destroy = True
                else:
                    self._destroy(sid)

        # Pass 2: cap（超 max_count 时按 LRU 驱逐最久未活动的）
        if len(self.sessions) > self.max_count:
            sorted_sessions = sorted(self.sessions.items(), key=lambda kv: kv[1].last_active)
            overflow = len(self.sessions) - self.max_count
            for sid, session in sorted_sessions:
                if overflow <= 0:
                    break
                if session.pending_destroy:
                    continue
                if session.busy_turns > 0:
                    session.pending_destroy = True
                    overflow -= 1
                else:
                    self._destroy(sid)
                    overflow -= 1

    # ── 内部：session 创建/加载 ──────────────────────────────

    async def create_session(
        self, session_token: str | None, api_key: str | None = None
    ) -> AcpSession:
        """创建新 session：bridge.start -> wait_for_mcp_ready -> create_session。"""
        draft_id = new_session_id()
        bridge = self._make_bridge(draft_id, api_key)
        try:
            await bridge.start()
            mcp_servers = self.hooks.load_mcp_servers(session_token)
            if self.hooks.has_mcp_servers():
                await bridge.wait_for_mcp_ready()
            session = await bridge.create_session(mcp_servers=mcp_servers)
        except Exception:
            bridge.destroy()
            raise
        session.last_active = self.clock.now()
        self.sessions[session.session_id] = session
        self.bridges[session.session_id] = bridge
        logger.info("acp session created: session=%s", session.session_id)
        return session

    async def load_session(
        self,
        session_id: str,
        session_token: str | None,
        api_key: str | None = None,
    ) -> AcpSession:
        """加载已持久化的 session：bridge.start -> load_session。"""
        bridge = self._make_bridge(session_id, api_key)
        try:
            await bridge.start()
            mcp_servers = self.hooks.load_mcp_servers(session_token)
            session = await bridge.load_session(session_id, mcp_servers=mcp_servers)
        except LoadSessionFailed:
            bridge.destroy()
            raise
        except Exception as err:
            bridge.destroy()
            raise LoadSessionFailed(session_id, str(err)) from err
        session.last_active = self.clock.now()
        self.sessions[session.session_id] = session
        self.bridges[session.session_id] = bridge
        logger.info("acp session loaded: session=%s", session.session_id)
        return session

    # ── 内部工具 ─────────────────────────────────────────────

    def _touch(self, session_id: str) -> None:
        session = self.sessions.get(session_id)
        if session:
            session.last_active = self.clock.now()

    def _make_bridge(self, session_id: str, api_key: str | None) -> AcpBridge:
        try:
            return self.factory(session_id, api_key)
        except TypeError:
            return self.factory(session_id)

    def _destroy(self, session_id: str) -> None:
        session = self.sessions.pop(session_id, None)
        bridge = self.bridges.pop(session_id, None)
        if session:
            session.close()
        if bridge:
            bridge.destroy()
