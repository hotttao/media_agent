"""Isolated protocol fixture for the Go/Python ownership integration test.

Uses the production HTTP routes, ownership service, migrations, credential client
and RabbitMQ adapter. Native model execution is an explicit deterministic fixture.
"""

from __future__ import annotations

import asyncio
import os
from collections.abc import AsyncIterator
from contextlib import asynccontextmanager
from pathlib import Path

import uvicorn
from fastapi import FastAPI, Request
from fastapi.responses import JSONResponse

from adapter.biz.api.v1.routes.runtime_instance.router import build_runtime_instance_router
from adapter.biz.api.v1.routes.runtime_session.router import build_runtime_session_router
from adapter.biz.dal.db import create_database
from adapter.biz.dal.repos.runtime_session.repository import SqlRuntimeSessionRepository
from adapter.biz.dal.session import TxRunner
from adapter.biz.domain.runtime_session.entity import RuntimeSession, Turn, TurnStatus
from adapter.biz.domain.runtime_session.errors import RuntimeSessionError
from adapter.biz.middleware.runtime_auth import RuntimeAuthMiddleware
from adapter.biz.service.runtime_session.service import RuntimeSessionService
from adapter.biz.workflow.execute_turn_flow import (
    AgentRequestHandler,
    ExecuteTurnFlow,
    RuntimeCredentials,
    RuntimePreparation,
    RuntimeResult,
)
from adapter.biz.workflow.turn_event_sink import RuntimeEventHandler, TurnEventSink
from adapter.clients.instance import GatewayInstanceClient, InstanceClientSettings
from adapter.config.config import AdapterConfig
from adapter.infra.migrations import migrate_database
from adapter.infra.runtime_auth import RuntimeAssertionVerifier
from adapter.session_event_bus import RabbitMQSessionEventBus


class FixtureRuntime:
    async def prepare(
        self, session: RuntimeSession, turn: Turn, credentials: RuntimeCredentials
    ) -> RuntimePreparation:
        assert credentials.api_key == "fixed-version-provider-key"
        assert credentials.session_token and credentials.session_token.startswith("st1.")
        return RuntimePreparation(
            session.runtime_session_id or "native-" + session.session_id,
            session.runtime_session_id is None,
        )

    async def execute(
        self,
        preparation: RuntimePreparation,
        session: RuntimeSession,
        turn: Turn,
        credentials: RuntimeCredentials,
        event_handler: RuntimeEventHandler,
        request_handler: AgentRequestHandler,
    ) -> RuntimeResult:
        await event_handler({"type": "item.delta", "delta": "live"})
        if turn.input.get("message") == "hold":
            async with asyncio.timeout(90):
                while not Path(os.environ["TEST_RUNTIME_RELEASE"]).exists():
                    await asyncio.sleep(0.05)
        await event_handler(
            {
                "type": "item.completed",
                "item": {
                    "item_id": turn.turn_id,
                    "kind": "message",
                    "content": [{"type": "text", "text": "done"}],
                },
            }
        )
        return RuntimeResult(TurnStatus.COMPLETED)

    async def interrupt(self, handle: str) -> bool:
        return False


def application(ordinal: str) -> FastAPI:
    @asynccontextmanager
    async def lifespan(app: FastAPI) -> AsyncIterator[None]:
        cfg = AdapterConfig.model_validate(
            {
                "agent": {"type": "codex"},
                "capabilities": {
                    name: False
                    for name in (
                        "system_prompt",
                        "mcp",
                        "skills",
                        "questions",
                        "reconnect",
                        "permissions",
                        "streaming_deltas",
                    )
                },
                "workspace": {"id": "workspace", "dir": os.environ["TEST_RUNTIME_ROOT"]},
                "event_bus": {"rabbitmq_url": os.environ["MEDIA_AGENTS_TEST_RABBITMQ_URL"]},
                "metadata": {
                    "enabled": True,
                    "gateway_url": os.environ.get("TEST_GATEWAY_URL_" + ordinal, os.environ["TEST_GATEWAY_URL"]),
                    "agent_api_token": "agent-test-token",
                    "agent_config_id": "config",
                    "replica_id": ordinal,
                    "incarnation_id": "boot" + ordinal + "-" + os.environ["TEST_RUNTIME_BOOT_ID"],
                },
                "instance": {
                    "enabled": True,
                    "environment_id": "environment",
                    "namespace": "test",
                    "resource_name": "agents-" + ordinal,
                    "bootstrap_proof_file": os.environ["TEST_RUNTIME_PROOF"],
                    "public_keys_file": os.environ["TEST_RUNTIME_PUBLIC_KEYS"],
                },
            }
        )
        client = GatewayInstanceClient(
            InstanceClientSettings(
                gateway_url=cfg.metadata.gateway_url,
                agent_api_token=cfg.metadata.agent_api_token,
                environment_id=cfg.instance.environment_id,
                workspace_id=cfg.workspace.id,
                agent_config_id=cfg.metadata.agent_config_id,
                replica_id=ordinal,
                incarnation_id=cfg.metadata.incarnation_id,
                namespace=cfg.instance.namespace,
                resource_name=cfg.instance.resource_name,
                resource_uid=cfg.instance.resource_uid,
                agent_type=cfg.agent_type,
                bootstrap_proof_file=cfg.instance.bootstrap_proof_file,
            )
        )
        scope = await client.register()
        engine, factory = create_database(os.environ["MEDIA_AGENTS_TEST_DB_URL"])
        repo = SqlRuntimeSessionRepository()
        service = RuntimeSessionService(TxRunner(factory), repo, scope, repo)
        await service.initialize_instance(client.admission, client.drain_operation_id)
        bus = RabbitMQSessionEventBus(
            os.environ["MEDIA_AGENTS_TEST_RABBITMQ_URL"], "test.ownership.sessions"
        )

        def sink_factory(session_id: str, turn_id: str) -> TurnEventSink:
            async def guard() -> dict[str, object]:
                value = await service.execution(session_id, turn_id)
                return {
                    "owner_instance_id": value.session.owner_instance_id,
                    "owner_incarnation_id": value.session.owner_incarnation_id,
                    "execution_epoch": value.session.execution_epoch,
                }

            return TurnEventSink(service, bus, session_id, turn_id, live_guard=guard)

        flow = ExecuteTurnFlow(service, FixtureRuntime(), sink_factory, service, client.credentials)
        app.state.instance_scope = scope
        app.state.runtime_assertion_verifier = RuntimeAssertionVerifier(
            Path(cfg.instance.public_keys_file)
        )
        app.state.runtime_session_service = service
        app.state.execute_turn_flow = flow
        try:
            yield
        finally:
            await flow.close()
            await bus.close()
            await client.close()
            await engine.dispose()

    app = FastAPI(lifespan=lifespan)
    app.add_middleware(RuntimeAuthMiddleware)

    @app.exception_handler(RuntimeSessionError)
    async def domain_error(request: Request, error: RuntimeSessionError) -> JSONResponse:
        return JSONResponse({"detail": str(error)}, 409)

    @app.get("/health")
    async def health() -> dict[str, bool]:
        return {"ready": True}

    app.include_router(build_runtime_session_router())
    app.include_router(build_runtime_instance_router())
    return app


async def main() -> None:
    await migrate_database(os.environ["MEDIA_AGENTS_TEST_DB_URL"])
    servers = [
        uvicorn.Server(
            uvicorn.Config(
                application(str(i)),
                host="127.0.0.1",
                port=int(os.environ["TEST_RUNTIME_PORT_" + str(i)]),
                log_level="warning",
            )
        )
        for i in (0, 1)
    ]
    await asyncio.gather(*(server.serve() for server in servers))


if __name__ == "__main__":
    asyncio.run(main())
