"""Shared FastAPI app factories for ACP agent adapters."""

from __future__ import annotations

import logging
from collections.abc import AsyncIterator, Callable
from contextlib import AbstractAsyncContextManager, asynccontextmanager

from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware

from adapter.biz.api.v1.routes.acp.router import build_acp_router
from adapter.biz.api.v1.routes.runtime_session.router import build_runtime_session_router
from adapter.biz.api.v1.routes.runtime_workspace.router import build_runtime_workspace_router
from adapter.biz.dal.db import create_database
from adapter.biz.dal.repos.runtime_session.repository import SqlRuntimeSessionRepository
from adapter.biz.dal.session import TxRunner
from adapter.biz.domain.acp_adapter import AcpServer
from adapter.biz.domain.acp_adapter.acp_abs import AgentHooks, BridgeFactory
from adapter.biz.service.runtime_session.service import RuntimeSessionService
from adapter.biz.workflow.acp_turn_runtime import AcpTurnRuntime
from adapter.biz.workflow.execute_turn_flow import ExecuteTurnFlow, TurnRuntime
from adapter.biz.workflow.turn_event_sink import TurnEventSink
from adapter.config.config import AdapterConfig
from adapter.infra.logging import setup_logging
from adapter.infra.migrations import migrate_database
from adapter.infra.runtime_workspace import ensure_runtime_workspace
from adapter.session_event_bus import InMemorySessionEventBus, RedisSessionEventBus

logger = logging.getLogger(__name__)

ExtraLifespan = Callable[[FastAPI], AbstractAsyncContextManager[None]]


def _event_bus(cfg: AdapterConfig) -> InMemorySessionEventBus | RedisSessionEventBus:
    if cfg.event_bus.redis_url:
        return RedisSessionEventBus(cfg.event_bus.redis_url, cfg.event_bus.channel_prefix)
    return InMemorySessionEventBus()


def _execute_turn_flow(
    cfg: AdapterConfig, service: RuntimeSessionService, runtime: TurnRuntime
) -> tuple[ExecuteTurnFlow, InMemorySessionEventBus | RedisSessionEventBus]:
    bus = _event_bus(cfg)

    def sink_factory(session_id: str, turn_id: str) -> TurnEventSink:
        return TurnEventSink(
            service,
            bus,
            session_id,
            turn_id,
            tool_result_max_bytes=cfg.event_bus.tool_result_max_bytes,
            tool_result_window_ms=cfg.event_bus.tool_result_window_ms,
        )

    return ExecuteTurnFlow(service, runtime, sink_factory), bus


def _add_common_middleware(app: FastAPI) -> None:
    app.add_middleware(
        CORSMiddleware,
        allow_origins=["*"],
        allow_methods=["GET", "POST", "PUT", "DELETE", "MKCOL", "MOVE", "OPTIONS"],
        allow_headers=["Content-Type", "Destination"],
    )


def build_base_app(cfg: AdapterConfig, *, turn_runtime: TurnRuntime) -> FastAPI:
    """Build the shared FastAPI shell without ACP routes."""

    @asynccontextmanager
    async def lifespan(app: FastAPI) -> AsyncIterator[None]:
        setup_logging()
        app.state.runtime_workspace = ensure_runtime_workspace(cfg.workspace.dir, cfg.workspace.id)
        engine, session_factory = create_database(cfg.database.url, echo=cfg.database.echo)
        if cfg.database.migrate_on_start:
            await migrate_database(cfg.database.url)
        app.state.adapter_config = cfg
        app.state.acp_server = None
        app.state.session_manager = None
        app.state.hooks = None
        app.state.runtime_session_service = RuntimeSessionService(
            TxRunner(session_factory), SqlRuntimeSessionRepository()
        )
        app.state.execute_turn_flow, app.state.session_event_bus = _execute_turn_flow(
            cfg, app.state.runtime_session_service, turn_runtime
        )
        logger.info(
            "adapter base app started: agent=%s workspace=%s",
            cfg.agent_type,
            cfg.workspace_dir,
        )
        try:
            yield
        finally:
            await app.state.execute_turn_flow.close()
            await app.state.session_event_bus.close()
            await engine.dispose()
            logger.info("adapter base app shutdown")

    app = FastAPI(lifespan=lifespan)
    _add_common_middleware(app)
    app.include_router(build_runtime_session_router())
    app.include_router(build_runtime_workspace_router())
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
        app.state.runtime_workspace = ensure_runtime_workspace(cfg.workspace.dir, cfg.workspace.id)
        engine, session_factory = create_database(cfg.database.url, echo=cfg.database.echo)
        if cfg.database.migrate_on_start:
            await migrate_database(cfg.database.url)
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
        app.state.runtime_session_service = RuntimeSessionService(
            TxRunner(session_factory), SqlRuntimeSessionRepository()
        )
        app.state.execute_turn_flow, app.state.session_event_bus = _execute_turn_flow(
            cfg, app.state.runtime_session_service, AcpTurnRuntime(acp_server)
        )
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
            await app.state.execute_turn_flow.close()
            await app.state.session_event_bus.close()
            acp_server.stop_evictor()
            await engine.dispose()
            logger.info("adapter app shutdown")

    app = FastAPI(lifespan=lifespan)
    _add_common_middleware(app)
    app.include_router(build_acp_router())
    app.include_router(build_runtime_session_router())
    app.include_router(build_runtime_workspace_router())
    return app
