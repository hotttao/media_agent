"""ACP 会话管理 service。

对标 internal/acp-adapter/acp-server.ts 的会话生命周期逻辑：
- bridge cache（session_id -> AcpBridge）：复用 / spawn / load / create
- busy_turns 引用计数：/reload-config 与 LRU 驱逐遇到 busy bridge 延后销毁
- LRU 驱逐：idle 超 TTL 或超 max_count 销毁（busy 进入 pending_destroy）
- pendingDestroy drain：/chat turn 结束时再尝试销毁

service 不开事务（无 DB），仅做 in-memory 状态编排。SSE sink 由 router 层管理。
"""

from __future__ import annotations

import asyncio
import logging
from typing import Any

from adapter.biz.domain.acp_adapter.entity import AcpSession, PromptResult
from adapter.biz.domain.acp_adapter.errors import LoadSessionFailed
from adapter.biz.domain.acp_adapter.repository import (
    AcpSessionHandler,
    AgentHooks,
    BridgeFactory,
)
from adapter.biz.shared.clock import Clock, SystemClock

logger = logging.getLogger(__name__)


class SessionManager:
    """对标 acp-server.ts 里的 sessionBridges / busyTurns / pendingDestroy 三件套。

    一个 session 对应一个 bridge（子进程），turn 期间 busy_turns 自增，turn 结束
    自减；/reload-config 或 LRU 驱逐遇到 busy bridge 进入 pending_destroy，等
    turn 结束后由 chat post-block 或下一轮 evict drain。
    """

    def __init__(
        self,
        *,
        bridge_factory: BridgeFactory,
        hooks: AgentHooks,
        idle_ttl_ms: int = 10 * 60 * 1000,
        max_count: int = 10,
        clock: Clock | None = None,
    ) -> None:
        self._factory = bridge_factory
        self._hooks = hooks
        self._idle_ttl_ms = idle_ttl_ms
        self._max_count = max_count
        self._clock = clock or SystemClock()
        self._sessions: dict[str, AcpSession] = {}
        self._evict_task: asyncio.Task[None] | None = None

    # ── bridge 生命周期 ──────────────────────────────────────────────

    def get(self, session_id: str) -> AcpSession | None:
        s = self._sessions.get(session_id)
        if s and not s.bridge.is_alive():
            # bridge 进程死掉，移除 stale，调用方走 load 路径
            logger.warning("Bridge dead for session=%s, replacing", session_id)
            self._destroy(session_id)
            return None
        return s

    async def load_or_create(
        self, session_id: str | None, session_token: str | None
    ) -> tuple[AcpSession, bool]:
        """对标 /chat handler 的 session 解析路径：复用 / load / create。

        返回 (session, is_new)。session_id 为 None 时新建并预制 draft UUID。
        """
        if session_id:
            existing = self.get(session_id)
            if existing:
                self._touch(session_id)
                return existing, False
            # 未命中或已 stale -> 走 load
            bridge = self._factory(session_id)
            try:
                mcp_servers = self._hooks.load_mcp_servers(session_token)
                resolved_id = await bridge.load_session(session_id, mcp_servers=mcp_servers)
            except Exception as err:
                bridge.destroy()
                raise LoadSessionFailed(session_id, str(err)) from err
            session = AcpSession(
                session_id=resolved_id,
                bridge=bridge,
                last_active=self._clock.now(),
            )
            self._sessions[resolved_id] = session
            return session, False

        # 全新 session
        from adapter.biz.shared.id import new_session_id

        draft_id = new_session_id()
        bridge = self._factory(draft_id)
        mcp_servers = self._hooks.load_mcp_servers(session_token)
        resolved_id = await bridge.create_session(mcp_servers=mcp_servers)
        session = AcpSession(session_id=resolved_id, bridge=bridge, last_active=self._clock.now())
        self._sessions[resolved_id] = session
        return session, True

    async def run_turn(
        self,
        session: AcpSession,
        handler: AcpSessionHandler,
        message: str,
        images: list[Any] | None = None,
    ) -> PromptResult:
        """对标 /chat handler 的 runTurn：busy_turns 计数 + prompt。"""
        session.bridge.register_handler(session.session_id, handler)
        session.busy_turns += 1
        try:
            return await session.bridge.prompt(session.session_id, message, images)
        finally:
            session.busy_turns -= 1
            self._touch(session.session_id)

    def cancel(self, session_id: str) -> bool:
        session = self._sessions.get(session_id)
        if not session:
            return False
        session.bridge.cancel(session_id)
        return True

    def destroy_idle(self) -> tuple[int, int]:
        """对标 destroyIdleBridges：销毁所有 idle bridge，busy 延后。返回 (destroyed, deferred)。"""
        destroyed = 0
        deferred = 0
        for sid in list(self._sessions):
            s = self._sessions.get(sid)
            if not s:
                continue
            if s.busy_turns > 0:
                s.pending_destroy = True
                deferred += 1
            else:
                self._destroy(sid)
                destroyed += 1
        return destroyed, deferred

    # ── LRU 驱逐后台任务 ────────────────────────────────────────────

    def start_evictor(self, interval_ms: int) -> None:
        async def _evict_loop() -> None:
            while True:
                await asyncio.sleep(interval_ms / 1000)
                self._evict_once()

        self._evict_task = asyncio.create_task(_evict_loop())

    def stop_evictor(self) -> None:
        if self._evict_task:
            self._evict_task.cancel()
            self._evict_task = None

    def _evict_once(self) -> None:
        """对标 evictIdleBridges：drain pendingDestroy + TTL + cap。"""
        now = self._clock.now()

        # Pass 0: drain pending_destroy（已 idle 的）
        for sid, s in list(self._sessions.items()):
            if s.pending_destroy and s.busy_turns == 0:
                logger.info("Destroying bridge deferred by reload session=%s", sid)
                self._destroy(sid)

        # Pass 1: TTL
        for sid, s in list(self._sessions.items()):
            if (now - s.last_active) * 1000 > self._idle_ttl_ms:
                if s.busy_turns > 0:
                    s.pending_destroy = True
                else:
                    self._destroy(sid)

        # Pass 2: cap
        if len(self._sessions) > self._max_count:
            sorted_sessions = sorted(self._sessions.items(), key=lambda kv: kv[1].last_active)
            overflow = len(self._sessions) - self._max_count
            for sid, s in sorted_sessions:
                if overflow <= 0:
                    break
                if s.pending_destroy:
                    continue
                if s.busy_turns > 0:
                    s.pending_destroy = True
                else:
                    self._destroy(sid)
                    overflow -= 1

    def drain_pending(self, session_id: str) -> None:
        """turn 结束后再尝试销毁 pending_destroy 的 session。对标 /chat post-block。"""
        s = self._sessions.get(session_id)
        if not s or not s.pending_destroy:
            return
        if s.busy_turns == 0:
            logger.info("Destroying bridge deferred by reload session=%s", session_id)
            self._destroy(session_id)

    # ── 内部工具 ─────────────────────────────────────────────────────

    def _touch(self, session_id: str) -> None:
        s = self._sessions.get(session_id)
        if s:
            s.last_active = self._clock.now()

    def _destroy(self, session_id: str) -> None:
        s = self._sessions.pop(session_id, None)
        if s:
            s.bridge.destroy()

    def __len__(self) -> int:
        return len(self._sessions)
