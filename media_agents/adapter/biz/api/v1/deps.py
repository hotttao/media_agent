"""FastAPI 依赖注入：DB session / auth subject / adapter config / session manager。

对标 acp-server.ts 用闭包共享 bridgeFactory / sessionBridges 的模式；Python 用
Depends 注入 SessionManager / AgentHooks / AdapterConfig，便于测试。
"""

from __future__ import annotations

from typing import Annotated, cast

from fastapi import Depends, Request

from adapter.biz.domain.acp.repository import AgentHooks
from adapter.biz.service.acp.service import SessionManager
from adapter.config.config import AdapterConfig


def get_adapter_config(request: Request) -> AdapterConfig:
    """从 app.state 取 AdapterConfig（main.py lifespan 注入）。"""
    return cast(AdapterConfig, request.app.state.adapter_config)


def get_session_manager(request: Request) -> SessionManager:
    return cast(SessionManager, request.app.state.session_manager)


def get_hooks(request: Request) -> AgentHooks:
    return cast(AgentHooks, request.app.state.hooks)


AdapterConfigDep = Annotated[AdapterConfig, Depends(get_adapter_config)]
SessionManagerDep = Annotated[SessionManager, Depends(get_session_manager)]
AgentHooksDep = Annotated[AgentHooks, Depends(get_hooks)]
