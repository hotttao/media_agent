"""Shared FastAPI app factories for ACP agent adapters."""

from __future__ import annotations

import logging
from collections.abc import AsyncContextManager, AsyncIterator, Callable
from contextlib import asynccontextmanager

from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware

from adapter.biz.api.v1.routes.acp.router import build_acp_router
from adapter.biz.domain.acp_adapter import AcpServer
from adapter.biz.domain.acp_adapter.acp_abs import AgentHooks, BridgeFactory
from adapter.config.config import AdapterConfig
from adapter.infra.logging import setup_logging

logger = logging.getLogger(__name__)

ExtraLifespan = Callable[[FastAPI], AsyncContextManager[None]]


def _add_common_middleware(app: FastAPI) -> None:
    app.add_middleware(
        CORSMiddleware,
        allow_origins=["*"],
        allow_methods=["GET", "POST", "PUT", "DELETE", "MKCOL", "MOVE", "OPTIONS"],
        allow_headers=["Content-Type", "Destination"],
    )


def build_base_app(cfg: AdapterConfig) -> FastAPI:
    """Build the shared FastAPI shell without ACP routes."""

    @asynccontextmanager
    async def lifespan(app: FastAPI) -> AsyncIterator[None]:
        setup_logging()
        app.state.adapter_config = cfg
        app.state.acp_server = None
        app.state.session_manager = None
        app.state.hooks = None
        logger.info(
            "adapter base app started: agent=%s workspace=%s",
            cfg.agent_type,
            cfg.workspace_dir,
        )
        yield
        logger.info("adapter base app shutdown")

    app = FastAPI(lifespan=lifespan)
    _add_common_middleware(app)
    return app


def build_app(
    cfg: AdapterConfig,
    *,
    hooks: AgentHooks,
    bridge_factory: BridgeFactory,
    extra_lifespan: ExtraLifespan | None = None,
) -> FastAPI:
    """Build the shared FastAPI app with ACP routes.

    Agent packages can pass ``extra_lifespan`` for boot-time config loading,
    credential loading, skills sync, and auxiliary processes while keeping ACP
    routers thin.
    """

    @asynccontextmanager
    async def lifespan(app: FastAPI) -> AsyncIterator[None]:
        setup_logging()
        acp_server = AcpServer(
            agent_hooks=hooks,
            bridge_factory=bridge_factory,
            idle_ttl_ms=cfg.bridge_ttl.idle_ttl_ms,
            max_count=cfg.bridge_ttl.max_count,
        )
        acp_server.start_evictor(cfg.bridge_ttl.evict_interval_ms)
        app.state.adapter_config = cfg
        app.state.acp_server = acp_server
        app.state.hooks = hooks
        logger.info(
            "adapter app started: agent=%s workspace=%s",
            cfg.agent_type,
            cfg.workspace_dir,
        )
        try:
            if extra_lifespan is None:
                yield
            else:
                async with extra_lifespan(app):
                    yield
        finally:
            acp_server.stop_evictor()
            logger.info("adapter app shutdown")

    app = FastAPI(lifespan=lifespan)
    _add_common_middleware(app)
    app.include_router(build_acp_router())
    return app
