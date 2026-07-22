"""adapter FastAPI app 工厂。

对标 acp-server.ts 的 createAcpAgentApp + TS agent 的 server.ts 包装层。
agent 包在自己的 main.py 调用 build_app(cfg, hooks, bridge_factory) 得到 FastAPI 实例，
然后 `app.include_router(build_agent_router(...))` 追加自己的路由（skills / usage 等）。

main.py 是 wiring 层：只做 app 创建 / middleware 注册 / router include / lifespan /
日志初始化。禁止业务逻辑。
"""

from __future__ import annotations

import logging
from collections.abc import AsyncIterator
from contextlib import asynccontextmanager

from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware

from adapter.biz.api.v1.routes.acp.router import build_acp_router
from adapter.biz.domain.acp.repository import AgentHooks, BridgeFactory
from adapter.biz.service.acp.service import SessionManager
from adapter.config.config import AdapterConfig
from adapter.infra.logging import setup_logging

logger = logging.getLogger(__name__)


def build_base_app(cfg: AdapterConfig) -> FastAPI:
    """构造共享 FastAPI 骨架：lifespan + CORS + 日志 + app.state。

    不 include 任何 router。codex/goose 通过 build_app() 在此基础上挂 ACP 路由；
    claude_code 不用 ACP router，直接调此函数拿到骨架再 include 自己的 routes。
    """

    @asynccontextmanager
    async def lifespan(app: FastAPI) -> AsyncIterator[None]:
        setup_logging()
        app.state.adapter_config = cfg
        app.state.session_manager = None  # claude_code 不用 SessionManager
        app.state.hooks = None
        logger.info(
            "adapter base app started: agent=%s workspace=%s",
            cfg.agent_type,
            cfg.workspace_dir,
        )
        yield
        logger.info("adapter base app shutdown")

    app = FastAPI(lifespan=lifespan)
    app.add_middleware(
        CORSMiddleware,
        allow_origins=["*"],
        allow_methods=["GET", "POST", "PUT", "DELETE", "MKCOL", "MOVE", "OPTIONS"],
        allow_headers=["Content-Type", "Destination"],
    )
    return app


def build_app(
    cfg: AdapterConfig,
    *,
    hooks: AgentHooks,
    bridge_factory: BridgeFactory,
) -> FastAPI:
    """构造共享 FastAPI app（含 ACP router）。

    Args:
        cfg: AdapterConfig（omegaconf 从 yaml 加载成的 Pydantic 实例）。
        hooks: agent 回调实现（load_config / load_skills / restart_bridge / record_usage）。
        bridge_factory: bridge 工厂，每个 session 调用一次返回启动好的 AcpBridge。

    agent 包在 main.py 用法：
        cfg = load_adapter_config("conf/config.yaml")
        app = build_app(cfg, hooks=MyHooks(), bridge_factory=make_bridge)
        app.include_router(build_codex_router())  # agent 特有路由
        uvicorn.run(app, host="0.0.0.0", port=3001)
    """

    @asynccontextmanager
    async def lifespan(app: FastAPI) -> AsyncIterator[None]:
        setup_logging()
        sm = SessionManager(
            bridge_factory=bridge_factory,
            hooks=hooks,
            idle_ttl_ms=cfg.bridge_ttl.idle_ttl_ms,
            max_count=cfg.bridge_ttl.max_count,
        )
        sm.start_evictor(cfg.bridge_ttl.evict_interval_ms)
        app.state.adapter_config = cfg
        app.state.session_manager = sm
        app.state.hooks = hooks
        logger.info(
            "adapter app started: agent=%s workspace=%s",
            cfg.agent_type,
            cfg.workspace_dir,
        )
        try:
            yield
        finally:
            sm.stop_evictor()
            logger.info("adapter app shutdown")

    app = FastAPI(lifespan=lifespan)

    app.add_middleware(
        CORSMiddleware,
        allow_origins=["*"],
        allow_methods=["GET", "POST", "PUT", "DELETE", "MKCOL", "MOVE", "OPTIONS"],
        allow_headers=["Content-Type", "Destination"],
    )

    app.include_router(build_acp_router())
    return app
