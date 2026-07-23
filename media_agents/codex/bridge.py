"""Codex AcpBridge：spawn codex-acp 子进程 + stdio JSON-RPC。

对标 agents/codex/src/index.ts 的 setBridgeFactory：
    const BRIDGE_OPTS = {
      program: 'codex-acp',
      args: [],
      cwd: WORKSPACE_DIR,
      env: { INITIAL_AGENT_MODE: 'agent-full-access' },
    }

简化版：用 asyncio.subprocess 启动 codex-acp，stdin/stdout 走 JSON-RPC。
完整实现可后续补：handle 跟 load/create/prompt/cancel 协议帧。
"""

from __future__ import annotations

import asyncio
import json
import logging
import os
from typing import Any, cast

from adapter.biz.domain.acp_adapter.entity import PromptResult, StopReason
from adapter.biz.domain.acp_adapter.repository import AcpSessionHandler

logger = logging.getLogger(__name__)

# codex-acp 1.1.x picks the sandbox from the INITIAL_AGENT_MODE env var.
# Force full access so agents can persist memory and exchange files as the
# platform contract promises.
BRIDGE_ENV = {"INITIAL_AGENT_MODE": "agent-full-access"}


class CodexAcpBridge:
    """对标 TS AcpBridge 的 codex 实现：一个 session 一个 codex-acp 子进程。

    实现了 AcpBridge Protocol 的鸭子类型（不需要显式继承）。完整 JSON-RPC
    帧编解码、会话 handler 派发逻辑在 _Reader / _Writer 协程里。
    """

    def __init__(
        self,
        *,
        program: str = "codex-acp",
        args: list[str] | None = None,
        cwd: str = "/workspace",
    ) -> None:
        self._program = program
        self._args = args or []
        self._cwd = cwd
        self._proc: asyncio.subprocess.Process | None = None
        self._handlers: dict[str, AcpSessionHandler] = {}
        self._reader_task: asyncio.Task[None] | None = None
        self._next_id = 0
        self._pending: dict[int, asyncio.Future[Any]] = {}

    async def start(self) -> None:
        env = {**os.environ, **BRIDGE_ENV}
        self._proc = await asyncio.create_subprocess_exec(
            self._program,
            *self._args,
            stdin=asyncio.subprocess.PIPE,
            stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.PIPE,
            cwd=self._cwd,
            env=env,
        )
        logger.info("codex-acp started pid=%s", self._proc.pid)
        self._reader_task = asyncio.create_task(self._read_loop())

    def is_alive(self) -> bool:
        return self._proc is not None and self._proc.returncode is None

    async def create_session(self, *, mcp_servers: list[Any] | None = None) -> str:
        return cast(str, await self._call("createSession", {"mcpServers": mcp_servers or []}))

    async def load_session(self, session_id: str, *, mcp_servers: list[Any] | None = None) -> str:
        return cast(
            str,
            await self._call(
                "loadSession",
                {"sessionId": session_id, "mcpServers": mcp_servers or []},
            ),
        )

    async def wait_for_mcp_ready(self) -> None:
        await self._call("waitForMcpReady", {})

    def register_handler(self, session_id: str, handler: AcpSessionHandler) -> None:
        self._handlers[session_id] = handler

    def unregister_handler(self, session_id: str) -> None:
        self._handlers.pop(session_id, None)

    async def prompt(
        self,
        session_id: str,
        message: str,
        images: list[Any] | None = None,
    ) -> PromptResult:
        result = await self._call(
            "prompt",
            {"sessionId": session_id, "message": message, "images": images or []},
        )
        stop = (
            StopReason.CANCELLED
            if result.get("stopReason") == "cancelled"
            else StopReason.COMPLETED
        )
        return PromptResult(stop_reason=stop, usage=result.get("usage"))

    def cancel(self, session_id: str) -> None:
        asyncio.create_task(self._notify("cancel", {"sessionId": session_id}))

    def destroy(self) -> None:
        if self._reader_task:
            self._reader_task.cancel()
            self._reader_task = None
        if self._proc and self._proc.returncode is None:
            self._proc.terminate()
        self._handlers.clear()

    # ── 内部 JSON-RPC ──────────────────────────────────────────────

    async def _read_loop(self) -> None:
        assert self._proc and self._proc.stdout
        try:
            while True:
                line = await self._proc.stdout.readline()
                if not line:
                    break
                try:
                    msg = json.loads(line.decode("utf-8"))
                except json.JSONDecodeError:
                    continue
                self._dispatch(msg)
        except asyncio.CancelledError:
            raise
        except Exception:
            logger.exception("codex-acp read loop crashed")

    def _dispatch(self, msg: dict[str, Any]) -> None:
        if "id" in msg and "result" in msg:
            fut = self._pending.pop(msg["id"], None)
            if fut and not fut.done():
                if "error" in msg:
                    fut.set_exception(RuntimeError(str(msg["error"])))
                else:
                    fut.set_result(msg["result"])
            return
        # notification -> 路由到 session handler
        method = msg.get("method", "")
        params = msg.get("params") or {}
        sid = params.get("sessionId") or params.get("session_id")
        if sid and sid in self._handlers:
            handler = self._handlers[sid]
            if method == "session/update":
                handler.on_update(params)
            elif method == "permission/request":
                asyncio.create_task(self._handle_permission(sid, params))

    async def _handle_permission(self, sid: str, params: dict[str, Any]) -> None:
        handler = self._handlers.get(sid)
        if not handler:
            return
        result = await handler.on_permission_request(params)
        await self._notify("permission/response", {"sessionId": sid, "result": result})

    async def _call(self, method: str, params: dict[str, Any]) -> Any:
        assert self._proc and self._proc.stdin
        self._next_id += 1
        req_id = self._next_id
        fut: asyncio.Future[Any] = asyncio.get_event_loop().create_future()
        self._pending[req_id] = fut
        payload = json.dumps({"jsonrpc": "2.0", "id": req_id, "method": method, "params": params})
        self._proc.stdin.write(payload.encode("utf-8") + b"\n")
        await self._proc.stdin.drain()
        return await fut

    async def _notify(self, method: str, params: dict[str, Any]) -> None:
        assert self._proc and self._proc.stdin
        payload = json.dumps({"jsonrpc": "2.0", "method": method, "params": params})
        self._proc.stdin.write(payload.encode("utf-8") + b"\n")
        await self._proc.stdin.drain()
