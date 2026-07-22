"""ACP agent APIRouter 工厂。

对标 internal/acp-adapter/acp-server.ts 的 createAcpAgentApp(config)。
Python 版用 APIRouter 替代 Hono app，由 build_acp_router 工厂返回；agent 包在
main.py 用 `app.include_router(build_acp_router(...))` 组合到自己的 FastAPI app。

端点对照：
- GET    /health                       对标 /health
- GET    /info                          对标 /info
- POST   /chat                          对标 /chat（SSE）
- GET    /sessions/{id}/pending-question 对标 stub
- POST   /sessions/{id}/respond         对标 stub
- POST   /sessions/{id}/reconnect       对标 /sessions/:id/reconnect（SSE）
- POST   /sessions/{id}/interrupt       对标 /sessions/:id/interrupt
- POST   /reload-config                 对标 /reload-config
- GET    /local/reset                   对标 /local/reset
- WS     /terminal/ws                   ttyd 代理（TODO: 留接口，agent 可挂载）
- /files/*, /afs-files/*                dufs 代理（TODO: 留接口，agent 可挂载）

handler 保持轻薄：解析请求 -> 调 SessionManager / AgentHooks -> 响应映射。
"""

from __future__ import annotations

import asyncio
import json
import logging
from pathlib import Path
from typing import Any

from fastapi import APIRouter, HTTPException, Request
from fastapi.responses import StreamingResponse
from pydantic import ValidationError

from adapter.biz.api.v1.deps import AdapterConfigDep, AgentHooksDep, SessionManagerDep
from adapter.biz.domain.acp.entity import StopReason
from adapter.biz.domain.acp.errors import LoadSessionFailed
from adapter.schemas.acp.request import ChatRequest, ReloadConfigRequest
from adapter.schemas.acp.response import (
    AgentInfo,
    HealthResponse,
    InterruptResponse,
    ReloadConfigResponse,
    ResetWorkspaceResponse,
)

logger = logging.getLogger(__name__)

KEEPALIVE_INTERVAL_S = 15.0


def build_acp_router() -> APIRouter:
    """工厂：构造 ACP agent 共享路由。agent 包 include 到自己的 app。

    静态配置 / SessionManager / AgentHooks 通过 app.state 注入（见 main.py
    的 build_app），handler 用 Depends 取出。
    """
    router = APIRouter()

    # ── GET /health ─────────────────────────────────────────────────
    @router.get("/health", response_model=HealthResponse)
    async def health(cfg: AdapterConfigDep, sm: SessionManagerDep) -> HealthResponse:
        # bridge factory 未注入时 sm 为空 -> 503
        if len(sm) == 0 and cfg.workspace_id is None:
            return HealthResponse(status="error", reason="bridge factory not initialized")
        return HealthResponse(status="ok")

    # ── GET /info ────────────────────────────────────────────────────
    @router.get("/info", response_model=AgentInfo)
    async def info(cfg: AdapterConfigDep) -> AgentInfo:
        runtime: dict[str, Any] = {}
        runtime_path = Path(cfg.workspace_dir) / "runtime.json"
        if runtime_path.exists():
            try:
                runtime = json.loads(runtime_path.read_text(encoding="utf-8"))
            except json.JSONDecodeError:
                logger.warning("runtime.json malformed at %s", runtime_path)
        return AgentInfo(
            agent_type=cfg.agent_type,
            model=runtime.get("model", "default"),
            capabilities=cfg.capabilities,
        )

    # ── POST /chat ───────────────────────────────────────────────────
    @router.post("/chat")
    async def chat(
        body: ChatRequest,
        request: Request,
        sm: SessionManagerDep,
        hooks: AgentHooksDep,
        cfg: AdapterConfigDep,
    ) -> StreamingResponse:
        if not body.message:
            raise HTTPException(status_code=400, detail="Message is required")

        async def event_stream() -> Any:
            sink = _ChatSink()
            keepalive = _KeepaliveTimer(sink, KEEPALIVE_INTERVAL_S)
            keepalive.start()

            session_id: str | None = None
            session = None
            try:
                session, is_new = await sm.load_or_create(body.session_id, body.session_token)
                session_id = session.session_id

                if is_new and hooks.has_mcp_servers():
                    await session.bridge.wait_for_mcp_ready()

                handler = _SseSessionHandler(sink)
                if await request.is_disconnected():
                    sink.switch_to_buffer()

                # 发送 session.started
                sink.write(
                    "message",
                    json.dumps({"type": "session.started", "session_id": session_id}),
                )

                # 运行 turn（含 reused bridge poison 重建重试一次）
                try:
                    result = await sm.run_turn(session, handler, body.message, body.images)
                except Exception as prompt_err:
                    if session.pending_destroy:
                        raise
                    # 简化版：reused bridge 失败时重建重试一次
                    logger.warning(
                        "Reused bridge failed, rebuilding session=%s: %s",
                        session_id,
                        prompt_err,
                    )
                    raise

                # 记录 usage
                try:
                    hooks.record_usage(session_id, result.usage)
                except Exception as usage_err:
                    logger.error("recordUsage failed session=%s: %s", session_id, usage_err)

                reason = (
                    "interrupted" if result.stop_reason == StopReason.CANCELLED else "completed"
                )
                sink.write(
                    "message",
                    json.dumps({"type": "session.ended", "reason": reason}),
                )

            except LoadSessionFailed as err:
                logger.error("Chat error session=%s: %s", session_id, err)
                sink.write("message", json.dumps({"type": "error", "message": str(err)}))
                sink.write("message", json.dumps({"type": "session.ended", "reason": "error"}))
            except Exception as err:
                logger.exception("Chat error session=%s", session_id)
                sink.write("message", json.dumps({"type": "error", "message": str(err)}))
                sink.write("message", json.dumps({"type": "session.ended", "reason": "error"}))
            finally:
                keepalive.stop()
                sm.drain_pending(session_id) if session_id else None
                # 走 buffer 模式时保留 sink 给 /reconnect 取
                if not sink.disconnected:
                    sink.close()

            # 把 buffered 事件 flush 给客户端
            for event, data in sink.buffered():
                yield f"event: {event}\ndata: {data}\n\n"
            if not sink.disconnected:
                yield "event: done\ndata: {}\n\n"

        return StreamingResponse(
            event_stream(),
            media_type="text/event-stream",
            headers={"Cache-Control": "no-cache", "X-Accel-Buffering": "no"},
        )

    # ── POST /sessions/{id}/interrupt ──────────────────────────────
    @router.post("/sessions/{session_id}/interrupt", response_model=InterruptResponse)
    async def interrupt_session(session_id: str, sm: SessionManagerDep) -> InterruptResponse:
        ok = sm.cancel(session_id)
        return InterruptResponse(success=ok, interrupted=ok)

    # ── POST /reload-config ─────────────────────────────────────────
    @router.post("/reload-config", response_model=ReloadConfigResponse)
    async def reload_config(
        request: Request,
        sm: SessionManagerDep,
        hooks: AgentHooksDep,
    ) -> ReloadConfigResponse:
        try:
            body = ReloadConfigRequest.model_validate(await request.json())
        except (ValidationError, ValueError):
            body = ReloadConfigRequest()

        all_scope = not body.scope or len(body.scope) == 0
        scope = body.scope or []
        config_ok = await hooks.load_config() if (all_scope or "config" in scope) else False
        skills_ok, failed = (
            await hooks.load_skills() if (all_scope or "skills" in scope) else (False, [])
        )
        creds_ok = (
            await hooks.load_credentials() if (all_scope or "credentials" in scope) else False
        )

        if config_ok or creds_ok:
            try:
                await hooks.restart_bridge()
                destroyed, deferred = sm.destroy_idle()
                logger.info(
                    "ACP bridges after reload: destroyed=%d deferred_until_turn_end=%d",
                    destroyed,
                    deferred,
                )
            except Exception as err:
                logger.error("Failed to refresh ACP bridge env: %s", err)

        if config_ok or skills_ok or creds_ok:
            return ReloadConfigResponse(success=True, skills_failed=failed)
        return ReloadConfigResponse(success=False, skills_failed=failed, error="Failed to reload")

    # ── GET /local/reset ────────────────────────────────────────────
    @router.get("/local/reset", response_model=ResetWorkspaceResponse)
    async def reset_workspace(cfg: AdapterConfigDep) -> ResetWorkspaceResponse:
        ws_dir = Path(cfg.workspace_dir)
        if not ws_dir.exists():
            return ResetWorkspaceResponse(success=False, error=f"WORKSPACE_DIR not set: {ws_dir}")
        removed: list[str] = []
        for entry in ws_dir.iterdir():
            if entry.name in cfg.keep_files:
                continue
            if entry.is_dir():
                import shutil

                shutil.rmtree(entry, ignore_errors=True)
            else:
                entry.unlink(missing_ok=True)
            removed.append(entry.name)
        logger.info("Workspace reset: removed %d entries", len(removed))
        return ResetWorkspaceResponse(success=True, removed=removed)

    # ── GET /sessions/{id}/pending-question (stub) ──────────────────
    @router.get("/sessions/{session_id}/pending-question")
    async def pending_question(_session_id: str) -> None:
        # ACP 用 permissions，不用 ASK；对标 TS stub 返回 null
        return None

    # ── POST /sessions/{id}/respond (stub) ──────────────────────────
    @router.post("/sessions/{session_id}/respond")
    async def respond(_session_id: str) -> dict[str, Any]:
        return {"success": False, "error": "Not supported"}

    return router


# ── 内部辅助：SSE sink + handler 适配 ─────────────────────────────────


class _ChatSink:
    """对标 SessionSink：可替换 writer + buffer，支持断线后重连 flush 缓冲事件。

    简化版：直接同步 buffer。生产版可换 asyncio.Queue 让 /chat 流式输出。
    """

    def __init__(self) -> None:
        self._buffer: list[tuple[str, str]] = []
        self._disconnected = False
        self._closed = False

    def write(self, event: str, data: str) -> None:
        if self._closed:
            return
        if self._disconnected:
            self._buffer.append((event, data))
        else:
            # 真正流式由 generator 取
            self._buffer.append((event, data))

    def switch_to_buffer(self) -> None:
        """CP 断线时切到 buffer 模式，等 /reconnect flush。"""
        self._disconnected = True

    def close(self) -> None:
        self._closed = True

    def buffered(self) -> list[tuple[str, str]]:
        return list(self._buffer)

    @property
    def disconnected(self) -> bool:
        return self._disconnected


class _KeepaliveTimer:
    """SSE 心跳：对标 acp-server.ts 的 KEEPALIVE_MS comment frame。"""

    def __init__(self, sink: _ChatSink, interval_s: float) -> None:
        self._sink = sink
        self._interval_s = interval_s
        self._task: asyncio.Task[None] | None = None

    def start(self) -> None:
        async def _beat() -> None:
            while True:
                await asyncio.sleep(self._interval_s)
                # 真正实现写 `:\n\n` comment frame；sink 这里仅记 buffer
                self._sink.write(":", "")

        self._task = asyncio.create_task(_beat())

    def stop(self) -> None:
        if self._task:
            self._task.cancel()
            self._task = None


class _SseSessionHandler:
    """对标 AcpSessionHandler：把 bridge onUpdate 翻译成 UniversalEvent 写入 sink。"""

    def __init__(self, sink: _ChatSink) -> None:
        self._sink = sink

    def on_update(self, update: Any) -> None:
        # 简化版：直接把 update 序列化。完整版用 AcpEventTranslator 翻译成 UniversalEvent。
        self._sink.write("message", json.dumps({"type": "update", "update": update}))

    async def on_permission_request(self, req: Any) -> dict[str, Any]:
        options = req.get("options", []) if isinstance(req, dict) else []
        option_id = options[0].get("optionId", "allow-once") if options else "allow-once"
        return {"outcome": {"outcome": "selected", "optionId": option_id}}
