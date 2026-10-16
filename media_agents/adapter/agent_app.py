"""Shared FastAPI app factories for ACP agent adapters."""

from __future__ import annotations

import asyncio
import logging
from collections.abc import AsyncIterator, Callable
from contextlib import AbstractAsyncContextManager, asynccontextmanager
from pathlib import Path

from fastapi import FastAPI, Request
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import JSONResponse

from adapter.biz.api.v1.routes.acp.router import build_acp_router
from adapter.biz.api.v1.routes.runtime_instance.router import build_runtime_instance_router
from adapter.biz.api.v1.routes.runtime_session.router import build_runtime_session_router
from adapter.biz.api.v1.routes.runtime_workspace.router import build_runtime_workspace_router
from adapter.biz.dal.db import create_database
from adapter.biz.dal.repos.runtime_session.repository import SqlRuntimeSessionRepository
from adapter.biz.dal.session import TxRunner
from adapter.biz.domain.acp_adapter import AcpServer
from adapter.biz.domain.acp_adapter.acp_abs import AgentHooks, BridgeFactory
from adapter.biz.domain.runtime_metadata.entity import VersionLoader
from adapter.biz.domain.runtime_session.errors import (
    ContextVersionConflict,
    InstanceDraining,
    RuntimeSessionError,
    RuntimeSessionNotFound,
    StaleExecutionOwner,
    TransferConflict,
)
from adapter.biz.middleware.runtime_auth import RuntimeAuthMiddleware
from adapter.biz.service.runtime_session.service import RuntimeSessionService
from adapter.biz.workflow.acp_turn_runtime import AcpTurnRuntime
from adapter.biz.workflow.execute_turn_flow import ExecuteTurnFlow, RuntimeCredentials, TurnRuntime
from adapter.biz.workflow.metadata_update_flow import MetadataUpdateFlow
from adapter.biz.workflow.turn_event_sink import TurnEventSink
from adapter.clients.instance import GatewayInstanceClient, InstanceClientSettings
from adapter.clients.metadata import GatewayMetadataClient
from adapter.config.config import AdapterConfig
from adapter.infra.logging import setup_logging
from adapter.infra.migrations import migrate_database
from adapter.infra.runtime_auth import RuntimeAssertionVerifier
from adapter.infra.runtime_workspace import ensure_runtime_workspace
from adapter.metadata_artifacts import MetadataArtifacts
from adapter.metadata_notifications import MetadataNotifications
from adapter.session_event_bus import InMemorySessionEventBus, RabbitMQSessionEventBus

logger = logging.getLogger(__name__)


def _metadata(
    cfg: AdapterConfig,
) -> tuple[MetadataUpdateFlow | None, GatewayMetadataClient | None, list[asyncio.Task[None]]]:
    if not cfg.metadata.enabled:
        return None, None, []
    client = GatewayMetadataClient(
        cfg.metadata.gateway_url,
        cfg.metadata.agent_api_token.get_secret_value(),
        cfg.workspace.id,
        cfg.metadata.agent_config_id,
        cfg.metadata.replica_id,
        cfg.metadata.incarnation_id,
        cfg.metadata.instance_credential.get_secret_value(),
    )
    store = MetadataArtifacts(
        Path(cfg.workspace.dir),
        cfg.metadata.agent_config_id,
        cfg.metadata.replica_id,
        cfg.metadata.incarnation_id,
    )
    flow = MetadataUpdateFlow(client, store, cfg.agent_type)
    notifications = MetadataNotifications(
        cfg.event_bus.rabbitmq_url or "",
        cfg.metadata.exchange,
        cfg.workspace.id,
        cfg.metadata.agent_config_id,
        flow,
    )
    return flow, client, [asyncio.create_task(flow.run()), asyncio.create_task(notifications.run())]


async def _close_metadata(
    client: GatewayMetadataClient | None, tasks: list[asyncio.Task[None]]
) -> None:
    for task in tasks:
        task.cancel()
    await asyncio.gather(*tasks, return_exceptions=True)
    if client is not None:
        await client.close()


ExtraLifespan = Callable[[FastAPI], AbstractAsyncContextManager[None]]


def _event_bus(cfg: AdapterConfig) -> InMemorySessionEventBus | RabbitMQSessionEventBus:
    if cfg.event_bus.rabbitmq_url:
        return RabbitMQSessionEventBus(cfg.event_bus.rabbitmq_url, cfg.event_bus.session_exchange)
    return InMemorySessionEventBus()


def _instance_client(cfg: AdapterConfig) -> GatewayInstanceClient | None:
    if not cfg.instance.enabled:
        return None
    return GatewayInstanceClient(
        InstanceClientSettings(
            gateway_url=cfg.metadata.gateway_url,
            agent_api_token=cfg.metadata.agent_api_token,
            environment_id=cfg.instance.environment_id,
            workspace_id=cfg.workspace.id,
            agent_config_id=cfg.metadata.agent_config_id,
            replica_id=cfg.metadata.replica_id,
            incarnation_id=cfg.metadata.incarnation_id,
            namespace=cfg.instance.namespace,
            resource_name=cfg.instance.resource_name,
            resource_uid=cfg.instance.resource_uid,
            agent_type=cfg.agent_type,
            bootstrap_proof_file=cfg.instance.bootstrap_proof_file,
        )
    )


def _execute_turn_flow(
    cfg: AdapterConfig,
    service: RuntimeSessionService,
    runtime: TurnRuntime,
    instance_client: GatewayInstanceClient | None = None,
) -> tuple[ExecuteTurnFlow, InMemorySessionEventBus | RabbitMQSessionEventBus]:
    bus = _event_bus(cfg)

    def sink_factory(session_id: str, turn_id: str) -> TurnEventSink:
        async def live_guard() -> dict[str, object]:
            execution = await service.execution(session_id, turn_id)
            return {
                "owner_instance_id": execution.session.owner_instance_id,
                "owner_incarnation_id": execution.session.owner_incarnation_id,
                "execution_epoch": execution.session.execution_epoch,
            }

        return TurnEventSink(
            service,
            bus,
            session_id,
            turn_id,
            live_guard=live_guard if service.scope is not None else None,
            tool_result_max_bytes=cfg.event_bus.tool_result_max_bytes,
            tool_result_window_ms=cfg.event_bus.tool_result_window_ms,
        )

    return ExecuteTurnFlow(
        service,
        runtime,
        sink_factory,
        service,
        instance_client.credentials if instance_client is not None else None,
    ), bus


def _add_common_middleware(app: FastAPI) -> None:
    app.add_middleware(RuntimeAuthMiddleware)
    app.add_exception_handler(RuntimeSessionError, _runtime_error)
    app.add_middleware(
        CORSMiddleware,
        allow_origins=["*"],
        allow_methods=["GET", "POST", "PUT", "DELETE", "MKCOL", "MOVE", "OPTIONS"],
        allow_headers=["Content-Type", "Destination"],
    )


async def _runtime_error(request: Request, error: Exception) -> JSONResponse:
    code = (
        404
        if isinstance(error, RuntimeSessionNotFound)
        else 409
        if isinstance(
            error, (ContextVersionConflict, StaleExecutionOwner, InstanceDraining, TransferConflict)
        )
        else 400
    )
    return JSONResponse({"detail": str(error)}, status_code=code)


def build_base_app(
    cfg: AdapterConfig,
    *,
    turn_runtime: TurnRuntime,
    metadata_runtime_factory: Callable[[VersionLoader | None], TurnRuntime] | None = None,
) -> FastAPI:
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
        instance_client = _instance_client(cfg)
        instance_scope = await instance_client.register() if instance_client is not None else None
        if instance_client is not None:
            cfg.metadata.instance_credential = instance_client.instance_credential
        app.state.instance_scope = instance_scope
        if instance_scope is not None:
            app.state.runtime_assertion_verifier = RuntimeAssertionVerifier(
                Path(cfg.instance.public_keys_file)
            )
        repository = SqlRuntimeSessionRepository()
        app.state.runtime_session_service = RuntimeSessionService(
            TxRunner(session_factory),
            repository,
            instance_scope,
            repository if instance_scope is not None else None,
        )
        await app.state.runtime_session_service.initialize_instance(
            instance_client.admission if instance_client is not None else None,
            instance_client.drain_operation_id if instance_client is not None else "",
        )
        if instance_client is not None and instance_client.admission == "draining":
            await app.state.runtime_session_service.begin_drain(instance_client.drain_operation_id)
        metadata_flow, metadata_client, metadata_tasks = _metadata(cfg)
        app.state.metadata_flow = metadata_flow
        app.state.execute_turn_flow, app.state.session_event_bus = _execute_turn_flow(
            cfg,
            app.state.runtime_session_service,
            metadata_runtime_factory(metadata_flow) if metadata_runtime_factory else turn_runtime,
            instance_client,
        )
        await app.state.execute_turn_flow.recover()
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
            await _close_metadata(metadata_client, metadata_tasks)
            if instance_client is not None:
                await instance_client.close()
            await engine.dispose()
            logger.info("adapter base app shutdown")

    app = FastAPI(lifespan=lifespan)
    _add_common_middleware(app)
    app.include_router(build_runtime_session_router())
    app.include_router(build_runtime_instance_router())
    app.include_router(build_runtime_workspace_router())
    return app


def build_app(
    cfg: AdapterConfig,
    *,
    hooks: AgentHooks,
    bridge_factory: BridgeFactory,
    extra_lifespan: ExtraLifespan | None = None,
    metadata_bridge_setup: Callable[[object, Path, str], None] | None = None,
    credential_bridge_setup: Callable[[object, RuntimeCredentials, str], None] | None = None,
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
        instance_client = _instance_client(cfg)
        instance_scope = await instance_client.register() if instance_client is not None else None
        if instance_client is not None:
            cfg.metadata.instance_credential = instance_client.instance_credential
        app.state.instance_scope = instance_scope
        if instance_scope is not None:
            app.state.runtime_assertion_verifier = RuntimeAssertionVerifier(
                Path(cfg.instance.public_keys_file)
            )
        repository = SqlRuntimeSessionRepository()
        app.state.runtime_session_service = RuntimeSessionService(
            TxRunner(session_factory),
            repository,
            instance_scope,
            repository if instance_scope is not None else None,
        )
        await app.state.runtime_session_service.initialize_instance(
            instance_client.admission if instance_client is not None else None,
            instance_client.drain_operation_id if instance_client is not None else "",
        )
        if instance_client is not None and instance_client.admission == "draining":
            await app.state.runtime_session_service.begin_drain(instance_client.drain_operation_id)
        metadata_flow, metadata_client, metadata_tasks = _metadata(cfg)
        app.state.metadata_flow = metadata_flow
        app.state.execute_turn_flow, app.state.session_event_bus = _execute_turn_flow(
            cfg,
            app.state.runtime_session_service,
            AcpTurnRuntime(
                acp_server, metadata_flow, metadata_bridge_setup, credential_bridge_setup
            ),
            instance_client,
        )
        await app.state.execute_turn_flow.recover()
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
            await _close_metadata(metadata_client, metadata_tasks)
            if instance_client is not None:
                await instance_client.close()
            acp_server.stop_evictor()
            await engine.dispose()
            logger.info("adapter app shutdown")

    app = FastAPI(lifespan=lifespan)
    _add_common_middleware(app)
    app.include_router(build_acp_router())
    app.include_router(build_runtime_session_router())
    app.include_router(build_runtime_instance_router())
    app.include_router(build_runtime_workspace_router())
    return app
