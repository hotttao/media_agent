from typing import Protocol, runtime_checkable

from adapter.agent_environment import AcpMcpServerDeclaration
from adapter.biz.domain.acp_adapter.bridge import AcpBridge


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

    def load_mcp_servers(self, session_token: str | None) -> list[AcpMcpServerDeclaration]:
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

    def record_usage(self, session_id: str, usage: object) -> None:
        """单 turn 完成后记录 token 用量。失败应吞掉异常并日志记录。"""
        ...
