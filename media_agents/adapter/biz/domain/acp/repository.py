"""ACP domain 仓库接口（Protocol）。

对标 internal/acp-adapter/acp-bridge.ts 的 AcpBridge + AcpSessionHandler。
domain 不依赖具体子进程实现，仅声明 Protocol 契约。
"""

from __future__ import annotations

from typing import Any, Protocol, runtime_checkable

from adapter.biz.domain.acp.entity import PromptResult


@runtime_checkable
class AcpSessionHandler(Protocol):
    """对标 AcpSessionHandler：bridge 回调上层处理事件。"""

    def on_update(self, update: Any) -> None:
        """ACP session/update 通知翻译为 UniversalEvent 投递给 SSE sink。"""
        ...

    async def on_permission_request(self, req: Any) -> dict[str, Any]:
        """ACP permission/request 回调。默认接受第一个选项。"""
        ...


@runtime_checkable
class AcpBridge(Protocol):
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

    async def create_session(self, *, mcp_servers: list[Any] | None = None) -> str:
        """创建新 ACP session，返回 session_id。"""
        ...

    async def load_session(self, session_id: str, *, mcp_servers: list[Any] | None = None) -> str:
        """加载已持久化的 session（rollout / SQLite）。失败抛 LoadSessionFailed。"""
        ...

    async def wait_for_mcp_ready(self) -> None:
        """等待 MCP 服务器启动完成（仅 has_mcp_servers 为真时调用）。"""
        ...

    def register_handler(self, session_id: str, handler: AcpSessionHandler) -> None:
        """注册 onUpdate / onPermissionRequest 回调。"""
        ...

    def unregister_handler(self, session_id: str) -> None: ...

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


@runtime_checkable
class BridgeFactory(Protocol):
    """bridge 工厂：每个 session 独立 bridge，避免 SQLite contention。"""

    def __call__(self, session_id: str) -> AcpBridge:
        """接收 platform session id（draft UUID），返回启动好的 bridge。"""
        ...


@runtime_checkable
class AgentHooks(Protocol):
    """agent 回调契约。

    对标 TS AcpAgentServerConfig 里的回调集合：load_mcp_servers / has_mcp_servers /
    load_config / load_skills / load_credentials / restart_bridge / record_usage。
    agent 包实现一个具体类注入到 build_acp_router。静态配置（agent_type /
    capabilities / workspace_dir 等）经 AdapterConfig 提供，不在 hooks 内。
    """

    def load_mcp_servers(self, session_token: str | None) -> list[Any]:
        """ACP session 要挂载的 MCP 服务器列表。HTTP 类走 cp 代理 + X-Session-Token。"""
        ...

    def has_mcp_servers(self) -> bool:
        """是否在 config.toml 注册了 MCP（决定是否等 wait_for_mcp_ready）。"""
        ...

    async def load_config(self) -> bool:
        """从 CP 拉取 workspace config，写 runtime.json / config.toml / AGENTS.md。"""
        ...

    async def load_skills(self) -> tuple[bool, list[str]]:
        """从 CP 同步 skills。返回 (ok, failed_names)。"""
        ...

    async def load_credentials(self) -> bool:
        """从 CP 注入 credentials 到 env / 文件。"""
        ...

    async def restart_bridge(self) -> None:
        """config/credentials 重载后刷新 provider env，新 bridge 接管。"""
        ...

    def record_usage(self, session_id: str, usage: Any) -> None:
        """单 turn 完成后记录 token 用量。失败应吞掉异常并日志记录。"""
        ...
