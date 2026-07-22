"""Claude Code agent 自有端点。

对标 agents/claude-code/src/server.ts + agent.ts + web-tools.ts：
- GET    /health
- GET    /info
- POST   /chat                       SSE 流式
- POST   /sessions/{id}/interrupt
- POST   /sessions/{id}/reconnect    SSE
- POST   /reload-config
- GET    /sessions/{id}/pending-question
- POST   /sessions/{id}/respond
- POST   /sessions/{id}/exec         执行 shell 命令（对标 /exec）
- GET    /local/reset
- WS     /terminal/ws                ttyd 代理
- /files/*, /afs-files/*             dufs 代理

claude_code 不通过 ACP bridge 走 JSON-RPC，而是直接调 `claude` CLI 子进程
（agent.ts 里的 chat/interrupt 逻辑），所以不用 adapter 的 SessionManager。
"""

from __future__ import annotations

import asyncio
import json
import logging
import shutil
from pathlib import Path
from typing import Any

from fastapi import APIRouter, HTTPException, Request
from fastapi.responses import StreamingResponse

from adapter.schemas.acp.request import ChatRequest, ReloadConfigRequest
from adapter.schemas.acp.response import (
    AgentInfo,
    HealthResponse,
    InterruptResponse,
    ReloadConfigResponse,
    ResetWorkspaceResponse,
)

logger = logging.getLogger(__name__)


def build_claude_code_router() -> APIRouter:
    """工厂：构造 claude_code 专用路由。不依赖 adapter.SessionManager。"""
    router = APIRouter()

    @router.get("/health", response_model=HealthResponse)
    async def health() -> HealthResponse:
        return HealthResponse(status="ok")

    @router.get("/info", response_model=AgentInfo)
    async def info(request: Request) -> AgentInfo:
        cfg = request.app.state.adapter_config
        runtime: dict[str, Any] = {}
        runtime_path = Path(cfg.workspace_dir) / "runtime.json"
        if runtime_path.exists():
            try:
                runtime = json.loads(runtime_path.read_text(encoding="utf-8"))
            except json.JSONDecodeError:
                pass
        import os

        return AgentInfo(
            agent_type=cfg.agent_type,
            model=runtime.get("model") or os.environ.get("ANTHROPIC_MODEL", "default"),
            capabilities=cfg.capabilities,
        )

    @router.post("/chat")
    async def chat(body: ChatRequest, request: Request) -> StreamingResponse:
        """对标 agents/claude-code/src/agent.ts chat()：spawn `claude` 子进程流式输出。

        简化版：直接 SSE echo 一个 session.started + session.ended。完整实现调
        ClaudeCodeAgent.chat() 跟 `claude` CLI 交互。
        """
        if not body.message:
            raise HTTPException(status_code=400, detail="Message is required")

        async def event_stream() -> Any:
            yield (
                "event: message\ndata: "
                + json.dumps({"type": "session.started", "session_id": body.session_id or "draft"})
                + "\n\n"
            )
            # TODO: 调 ClaudeCodeAgent.chat(body) 跟 claude CLI 子进程交互
            await asyncio.sleep(0)
            yield (
                "event: message\ndata: "
                + json.dumps({"type": "session.ended", "reason": "completed"})
                + "\n\n"
            )

        return StreamingResponse(
            event_stream(),
            media_type="text/event-stream",
            headers={"Cache-Control": "no-cache", "X-Accel-Buffering": "no"},
        )

    @router.post("/sessions/{session_id}/interrupt", response_model=InterruptResponse)
    async def interrupt_session(session_id: str) -> InterruptResponse:
        # TODO: 调 ClaudeCodeAgent.interrupt(session_id)
        logger.info("Interrupt request session=%s", session_id)
        return InterruptResponse(success=True, interrupted=True)

    @router.post("/sessions/{session_id}/reconnect")
    async def reconnect(session_id: str, request: Request) -> StreamingResponse:
        """对标 /sessions/:id/reconnect：flush 缓冲事件后接管 sink。"""

        async def event_stream() -> Any:
            # TODO: 从 ClaudeCodeAgent 取 buffer
            yield (
                "event: message\ndata: "
                + json.dumps({"type": "reconnected", "session_id": session_id})
                + "\n\n"
            )

        return StreamingResponse(event_stream(), media_type="text/event-stream")

    @router.post("/reload-config", response_model=ReloadConfigResponse)
    async def reload_config(request: Request) -> ReloadConfigResponse:
        try:
            ReloadConfigRequest.model_validate(await request.json())
        except (ValueError, TypeError):
            pass

        # TODO: 调 ClaudeCodeHooks 的 load_config / load_skills / load_credentials
        return ReloadConfigResponse(success=True, skills_failed=[])

    @router.get("/sessions/{session_id}/pending-question")
    async def pending_question(_session_id: str) -> None:
        return None

    @router.post("/sessions/{session_id}/respond")
    async def respond(_session_id: str) -> dict[str, Any]:
        return {"success": False, "error": "Not supported"}

    @router.post("/sessions/{session_id}/exec")
    async def exec_cmd(session_id: str, request: Request) -> dict[str, Any]:
        """对标 agents/claude-code/src/web-tools.ts /exec：执行 shell 命令。"""
        body = await request.json()
        cmd = body.get("command") or ""
        if not cmd:
            raise HTTPException(status_code=400, detail="command is required")
        try:
            proc = await asyncio.create_subprocess_shell(
                cmd,
                stdout=asyncio.subprocess.PIPE,
                stderr=asyncio.subprocess.PIPE,
            )
            stdout, stderr = await proc.communicate()
            return {
                "success": proc.returncode == 0,
                "stdout": stdout.decode("utf-8", errors="replace"),
                "stderr": stderr.decode("utf-8", errors="replace"),
                "exit_code": proc.returncode,
            }
        except Exception as err:
            logger.exception("exec failed session=%s cmd=%s", session_id, cmd)
            return {"success": False, "error": str(err)}

    @router.get("/local/reset", response_model=ResetWorkspaceResponse)
    async def reset_workspace(request: Request) -> ResetWorkspaceResponse:
        cfg = request.app.state.adapter_config
        ws_dir = Path(cfg.workspace_dir)
        if not ws_dir.exists():
            return ResetWorkspaceResponse(success=False, error=f"WORKSPACE_DIR not set: {ws_dir}")
        removed: list[str] = []
        for entry in ws_dir.iterdir():
            if entry.name in cfg.keep_files:
                continue
            if entry.is_dir():
                shutil.rmtree(entry, ignore_errors=True)
            else:
                entry.unlink(missing_ok=True)
            removed.append(entry.name)
        logger.info("Workspace reset: removed %d entries", len(removed))
        return ResetWorkspaceResponse(success=True, removed=removed)

    return router
