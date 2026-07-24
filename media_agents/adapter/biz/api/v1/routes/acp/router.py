"""ACP FastAPI routes.

Routes intentionally stay thin: HTTP parsing and response mapping live here,
while session, bridge, stream buffering, reconnect, and eviction are handled by
the ACP domain server.
"""

from __future__ import annotations

import asyncio
import json
import logging
from collections.abc import AsyncIterator
from pathlib import Path

from fastapi import APIRouter, HTTPException, Request
from fastapi.responses import StreamingResponse
from pydantic import ValidationError

from adapter.biz.api.v1.deps import AcpServerDep, AdapterConfigDep, AgentHooksDep
from adapter.biz.domain.acp_adapter.entity import Chat, PromptResult
from adapter.biz.domain.acp_adapter.errors import BridgeNotFound, LoadSessionFailed
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
    router = APIRouter()

    @router.get("/health", response_model=HealthResponse)
    async def health(cfg: AdapterConfigDep, sm: AcpServerDep) -> HealthResponse:
        if len(sm) == 0 and cfg.workspace_id is None:
            return HealthResponse(status="error", reason="bridge factory not initialized")
        return HealthResponse(status="ok")

    @router.get("/info", response_model=AgentInfo)
    async def info(cfg: AdapterConfigDep) -> AgentInfo:
        runtime: dict[str, object] = {}
        runtime_path = Path(cfg.workspace_dir) / "runtime.json"
        if runtime_path.exists():
            try:
                payload = json.loads(runtime_path.read_text(encoding="utf-8"))
                runtime = payload if isinstance(payload, dict) else {}
            except json.JSONDecodeError:
                logger.warning("runtime.json malformed at %s", runtime_path)
        model = runtime.get("model")
        return AgentInfo(
            agent_type=cfg.agent_type,
            model=model if isinstance(model, str) else "default",
            capabilities=cfg.capabilities,
        )

    @router.post("/chat")
    async def chat(
        body: ChatRequest,
        request: Request,
        sm: AcpServerDep,
    ) -> StreamingResponse:
        if not body.message:
            raise HTTPException(status_code=400, detail="Message is required")

        async def event_stream() -> AsyncIterator[str]:
            session_id: str | None = None
            writer_id: int | None = None
            turn_task: asyncio.Task[PromptResult] | None = None
            try:
                session, _ = await sm.chat(Chat.model_validate(body.model_dump()))
                session_id = session.session_id
                writer_id = session.stream.writer_id
                turn_task = asyncio.create_task(
                    sm.run_turn(session, body.message, body.images)
                )
                turn_task.add_done_callback(_log_task_error)

                async for chunk in session.read(keepalive_interval=KEEPALIVE_INTERVAL_S):
                    if await request.is_disconnected():
                        sm.detach_stream(session_id, writer_id)
                        return
                    yield chunk
            except LoadSessionFailed as err:
                logger.error("Chat load failed session=%s: %s", session_id, err)
                yield _sse("message", json.dumps({"type": "error", "message": str(err)}))
                yield _sse("message", json.dumps({"type": "session.ended", "reason": "error"}))
            except Exception as err:
                logger.exception("Chat error session=%s", session_id)
                yield _sse("message", json.dumps({"type": "error", "message": str(err)}))
                yield _sse("message", json.dumps({"type": "session.ended", "reason": "error"}))
            finally:
                if session_id and writer_id is not None and await request.is_disconnected():
                    sm.detach_stream(session_id, writer_id)
                if turn_task is not None and turn_task.done():
                    _consume_task_error(turn_task)

        return _event_response(event_stream())

    @router.post("/sessions/{session_id}/reconnect")
    async def reconnect_session(
        session_id: str,
        request: Request,
        sm: AcpServerDep,
    ) -> StreamingResponse:
        try:
            stream = await sm.reconnect(session_id)
        except BridgeNotFound as err:
            raise HTTPException(status_code=404, detail=str(err)) from err
        writer_id = stream.writer_id

        async def event_stream() -> AsyncIterator[str]:
            try:
                async for chunk in stream.read(keepalive_interval=KEEPALIVE_INTERVAL_S):
                    if await request.is_disconnected():
                        sm.detach_stream(session_id, writer_id)
                        return
                    yield chunk
            except asyncio.CancelledError:
                sm.detach_stream(session_id, writer_id)
                raise
            finally:
                if await request.is_disconnected():
                    sm.detach_stream(session_id, writer_id)

        return _event_response(event_stream())

    @router.post("/sessions/{session_id}/interrupt", response_model=InterruptResponse)
    async def interrupt_session(session_id: str, sm: AcpServerDep) -> InterruptResponse:
        ok = sm.cancel(session_id)
        return InterruptResponse(success=ok, interrupted=ok)

    @router.post("/reload-config", response_model=ReloadConfigResponse)
    async def reload_config(
        request: Request,
        sm: AcpServerDep,
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

    @router.get("/sessions/{session_id}/pending-question")
    async def pending_question(_session_id: str) -> None:
        return None

    @router.post("/sessions/{session_id}/respond")
    async def respond(_session_id: str) -> dict[str, object]:
        return {"success": False, "error": "Not supported"}

    return router


def _event_response(stream: AsyncIterator[str]) -> StreamingResponse:
    return StreamingResponse(
        stream,
        media_type="text/event-stream",
        headers={"Cache-Control": "no-cache", "X-Accel-Buffering": "no"},
    )


def _sse(event: str, data: str) -> str:
    return f"event: {event}\ndata: {data}\n\n"


def _log_task_error(task: asyncio.Task[PromptResult]) -> None:
    try:
        task.result()
    except asyncio.CancelledError:
        return
    except Exception:
        logger.exception("ACP turn task failed")


def _consume_task_error(task: asyncio.Task[PromptResult]) -> None:
    try:
        task.result()
    except asyncio.CancelledError:
        return
    except Exception:
        pass
