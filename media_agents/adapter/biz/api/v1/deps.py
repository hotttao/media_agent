"""FastAPI 依赖注入：adapter config / acp server / agent hooks。

对标 acp-server.ts 用闭包共享 bridgeFactory / sessionBridges 的模式；Python 用
Depends 注入 AcpServer / AgentHooks / AdapterConfig，便于测试。
"""

from __future__ import annotations

from typing import Annotated, cast

from fastapi import Depends, Request

from adapter.biz.domain.acp_adapter import AcpServer
from adapter.biz.domain.acp_adapter.acp_abs import AgentHooks
from adapter.config.config import AdapterConfig


def get_adapter_config(request: Request) -> AdapterConfig:
    """从 app.state 取 AdapterConfig（main.py lifespan 注入）。"""
    return cast(AdapterConfig, request.app.state.adapter_config)


def get_acp_server(request: Request) -> AcpServer:
    """从 app.state 取 AcpServer（main.py build_app lifespan 注入）。"""
    return cast(AcpServer, request.app.state.acp_server)


def get_hooks(request: Request) -> AgentHooks:
    return cast(AgentHooks, request.app.state.hooks)


AdapterConfigDep = Annotated[AdapterConfig, Depends(get_adapter_config)]
AcpServerDep = Annotated[AcpServer, Depends(get_acp_server)]
AgentHooksDep = Annotated[AgentHooks, Depends(get_hooks)]
