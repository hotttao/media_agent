
from __future__ import annotations

from typing import Any

from adapter.biz.domain.acp.entity import PromptResult
from adapter.biz.domain.acp.session import AcpSession


class AcpBridge:
    """对标 AcpBridge：一个 ACP session 对应一个子进程 bridge。

    Agent 实现自己的 AcpBridge（codex-acp / goose-acp 子进程），注入到
    SessionManager 的 bridge_factory。adapter 不关心具体子进程，只通过此
    Protocol 调用。
    """

    async def start(self) -> None:
        """启动子进程并完成 JSON-RPC 握手。"""
        ...

    def is_alive(self) -> bool:
        """子进程是否存活（进程级检查，不能发现 session 级 poison）。"""
        ...

    async def create_session(self, *, mcp_servers: list[Any] | None = None) -> AcpSession:
        """创建新 ACP session，返回 session_id。"""
        ...

    async def load_session(self, session_id: str, *, mcp_servers: list[Any] | None = None) -> AcpSession:
        """加载已持久化的 session（rollout / SQLite）。失败抛 LoadSessionFailed。"""
        ...

    async def wait_for_mcp_ready(self) -> None:
        """等待 MCP 服务器启动完成（仅 has_mcp_servers 为真时调用）。"""
        ...

    async def prompt(
        self, session_id: str, message: str, images: list[Any] | None = None
    ) -> PromptResult:
        """发送一个 prompt turn，等待完成。"""
        ...

    def cancel(self, session_id: str) -> None:
        """中断正在执行的 turn。"""
        ...

    def destroy(self) -> None:
        """销毁子进程。"""
        ...

