"""Goose AcpBridge：spawn `goose acp --with-builtin developer` 子进程。

对标 agents/goose/src/index.ts 的 setBridgeFactory：
    const b = new AcpBridge({
      program: 'goose',
      args: ['acp', '--with-builtin', 'developer'],
      cwd: WORKSPACE_DIR,
      env: { GOOSE_PATH_ROOT: <per-session-dir> },
    })

每个 session 独立 bridge，env.GOOSE_PATH_ROOT 隔离 goose 的 SQLite 数据目录。
"""

from __future__ import annotations

import asyncio
import json
import logging
import os
from collections.abc import Callable
from pathlib import Path
from typing import TypeAlias, cast

from adapter.biz.domain.acp_adapter.entity import ChatImageAttachment, PromptResult, StopReason
from adapter.biz.domain.acp_adapter.repository import AcpSessionHandler

logger = logging.getLogger(__name__)

JsonValue: TypeAlias = str | int | float | bool | None | list["JsonValue"] | dict[str, "JsonValue"]
JsonObject: TypeAlias = dict[str, JsonValue]
ExtNotificationHandler: TypeAlias = Callable[[str, str, JsonObject], None]


class GooseAcpBridge:
    """AcpBridge Protocol 的 goose 实现。结构同 CodexAcpBridge，差异：

    - 程序 `goose acp --with-builtin developer`
    - env 注入 GOOSE_PATH_ROOT=<per-session-dir>，隔离 SQLite 数据
    - ext notification `_goose/unstable/session/update` 由 trackExtNotification 拦截
      累计 token，喂给 GooseHooks.record_usage
    """

    def __init__(
        self,
        *,
        cwd: str = "/workspace",
        goose_path_root: str | None = None,
        session_id: str | None = None,
        on_ext_notification: ExtNotificationHandler | None = None,
    ) -> None:
        self._cwd = cwd
        self._goose_path_root = goose_path_root
        self._session_id = session_id
        self._on_ext = on_ext_notification
        self._proc: asyncio.subprocess.Process | None = None
        self._handlers: dict[str, AcpSessionHandler] = {}
        self._reader_task: asyncio.Task[None] | None = None
        self._next_id = 0
        self._pending: dict[int, asyncio.Future[JsonValue]] = {}

    async def start(self) -> None:
        env = dict(os.environ)
        if self._goose_path_root and self._session_id:
            per_session_dir = Path(self._goose_path_root) / self._session_id
            per_session_dir.mkdir(parents=True, exist_ok=True)
            env["GOOSE_PATH_ROOT"] = str(per_session_dir)
        self._proc = await asyncio.create_subprocess_exec(
            "goose",
            "acp",
            "--with-builtin",
            "developer",
            stdin=asyncio.subprocess.PIPE,
            stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.PIPE,
            cwd=self._cwd,
            env=env,
        )
        logger.info("goose acp started pid=%s session=%s", self._proc.pid, self._session_id)
        self._reader_task = asyncio.create_task(self._read_loop())

    def is_alive(self) -> bool:
        return self._proc is not None and self._proc.returncode is None

    async def create_session(self, *, mcp_servers: list[JsonObject] | None = None) -> str:
        return cast(str, await self._call("createSession", {"mcpServers": mcp_servers or []}))

    async def load_session(
        self, session_id: str, *, mcp_servers: list[JsonObject] | None = None
    ) -> str:
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
        images: list[ChatImageAttachment] | None = None,
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
            logger.exception("goose acp read loop crashed")

    def _dispatch(self, msg: JsonObject) -> None:
        if "id" in msg and ("result" in msg or "error" in msg):
            fut = self._pending.pop(msg["id"], None)
            if fut and not fut.done():
                if "error" in msg:
                    fut.set_exception(RuntimeError(str(msg["error"])))
                else:
                    fut.set_result(msg["result"])
            return
        method = msg.get("method", "")
        params = msg.get("params") or {}
        sid = params.get("sessionId") or params.get("session_id") or self._session_id
        # goose ext notification 拦截
        if method == "_goose/unstable/session/update" and self._on_ext and sid:
            try:
                self._on_ext(sid, method, params)
            except Exception:
                logger.exception("on_ext_notification failed")
        if sid and sid in self._handlers:
            handler = self._handlers[sid]
            if method == "session/update":
                handler.on_update(params)
            elif method == "permission/request":
                asyncio.create_task(self._handle_permission(sid, params))

    async def _handle_permission(self, sid: str, params: JsonObject) -> None:
        handler = self._handlers.get(sid)
        if not handler:
            return
        result = await handler.on_permission_request(params)
        await self._notify("permission/response", {"sessionId": sid, "result": result})

    async def _call(self, method: str, params: JsonObject) -> JsonValue:
        assert self._proc and self._proc.stdin
        self._next_id += 1
        req_id = self._next_id
        fut: asyncio.Future[JsonValue] = asyncio.get_event_loop().create_future()
        self._pending[req_id] = fut
        payload = json.dumps({"jsonrpc": "2.0", "id": req_id, "method": method, "params": params})
        self._proc.stdin.write(payload.encode("utf-8") + b"\n")
        await self._proc.stdin.drain()
        return await fut

    async def _notify(self, method: str, params: JsonObject) -> None:
        assert self._proc and self._proc.stdin
        payload = json.dumps({"jsonrpc": "2.0", "method": method, "params": params})
        self._proc.stdin.write(payload.encode("utf-8") + b"\n")
        await self._proc.stdin.drain()
