"""adapter 配置定义。

omegaconf 负责 yaml 加载 + ${oc.env:VAR,default} 环境变量插值，pydantic 负责
schema 校验与类型。OmegaConf.structured(AdapterConfig) 用 Pydantic BaseModel 作为
schema 模板，OmegaConf.to_object() 返回 Pydantic 实例。

对标 acp-server.ts 的 AcpAgentServerConfig 静态部分。运行时回调（load_config /
load_skills / restart_bridge / record_usage 等）由 AgentHooks Protocol 注入，
见 biz/domain/acp/repository.py。
"""

from __future__ import annotations

from pathlib import Path
from typing import cast

from omegaconf import OmegaConf
from pydantic import BaseModel, Field

from adapter.schemas.acp.response import AgentCapabilities


class BridgeTtlConfig(BaseModel):
    """bridge LRU 驱逐配置。对标 acp-server.ts BRIDGE_IDLE_TTL_MS / BRIDGE_MAX_COUNT。"""

    idle_ttl_ms: int = 10 * 60 * 1000  # 10 分钟空闲驱逐
    max_count: int = 10  # 最多保留 10 个并发 bridge（codex-acp ~127MB，2GiB 限制）
    evict_interval_ms: int = 60 * 1000  # 每分钟扫一次


class KeepaliveConfig(BaseModel):
    """SSE 心跳配置。对标 acp-server.ts KEEPALIVE_MS=15_000。"""

    interval_ms: int = 15_000


class AdapterConfig(BaseModel):
    """adapter 总配置。omegaconf 从 yaml 加载后转为此 Pydantic 实例。

    agent 包的 conf/config.yaml 只写 override 字段；未列出的字段用 BaseModel 默认值。
    """

    agent_type: str
    capabilities: AgentCapabilities
    workspace_dir: str = "/workspace"
    keep_files: set[str] = Field(default_factory=lambda: {"AGENTS.md", "runtime.json"})
    cp_url: str | None = None
    workspace_id: str | None = None
    bridge_ttl: BridgeTtlConfig = Field(default_factory=BridgeTtlConfig)
    keepalive: KeepaliveConfig = Field(default_factory=KeepaliveConfig)


def load_adapter_config(yaml_path: str | Path) -> AdapterConfig:
    """从 yaml 加载 AdapterConfig。

    用 OmegaConf.structured(AdapterConfig) 作为 schema 模板，合并 yaml 文件（含
    ${oc.env:VAR,default} 插值），最后 OmegaConf.to_object 转回 Pydantic 实例。
    """
    schema = OmegaConf.structured(AdapterConfig)
    yaml_cfg = OmegaConf.load(str(yaml_path))
    merged = OmegaConf.merge(schema, yaml_cfg)
    return cast(AdapterConfig, OmegaConf.to_object(merged))
