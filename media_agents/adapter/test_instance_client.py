from __future__ import annotations

import json
from pathlib import Path

import httpx
import pytest
from pydantic import SecretStr

from adapter.biz.domain.runtime_session.errors import (
    ExecutionCredentialsDeferred,
    ExecutionCredentialsRejected,
)
from adapter.clients.instance import GatewayInstanceClient, InstanceClientSettings


def settings(tmp_path: Path) -> InstanceClientSettings:
    proof = tmp_path / "proof"
    proof.write_text("native-proof")
    return InstanceClientSettings(
        "http://gateway",
        SecretStr("agent-token"),
        "env",
        "ws",
        "cfg",
        "0",
        "boot",
        "ns",
        "agent-0",
        "",
        "codex",
        str(proof),
    )


@pytest.mark.asyncio
async def test_bootstrap_accepts_protobuf_omitted_initial_version_and_scopes_credentials(
    tmp_path: Path,
) -> None:
    requests: list[httpx.Request] = []

    def respond(request: httpx.Request) -> httpx.Response:
        requests.append(request)
        assert request.headers["Authorization"] == "Bearer agent-token"
        if request.url.path.endswith("/identity"):
            assert request.headers["X-Runtime-Bootstrap-Proof"] == "native-proof"
            return httpx.Response(
                200,
                json={
                    "environment_id": "env",
                    "resource_uid": "uid",
                    "resource_generation": "generation",
                    "agent_type": "codex",
                },
            )
        if request.url.path.endswith("/register"):
            assert json.loads(request.content)["expected_version"] == 0
            return httpx.Response(
                200,
                json={
                    "incarnation_id": "boot",
                    "admission": "active",
                    "instance_credential": "instance-token",
                    "registration_version": 1,
                },
            )
        assert request.headers["X-Agent-Instance-Credential"] == "instance-token"
        assert json.loads(request.content)["execution_epoch"] == 3
        return httpx.Response(
            200,
            json={"execution_epoch": 3, "session_token": "st1.test.mac", "api_key": "provider-key"},
        )

    client = GatewayInstanceClient(settings(tmp_path))
    await client._http.aclose()
    client._http = httpx.AsyncClient(
        base_url="http://gateway",
        headers={"Authorization": "Bearer agent-token"},
        transport=httpx.MockTransport(respond),
    )
    try:
        scope = await client.register()
        assert scope.workspace_id == "ws" and scope.incarnation_id == "boot"
        value = await client.credentials("session", "turn", 3)
        assert value.session_token == "st1.test.mac" and value.api_key == "provider-key"
        assert len(requests) == 3
    finally:
        await client.close()


@pytest.mark.asyncio
async def test_execution_credentials_distinguish_transport_retry_and_revocation(
    tmp_path: Path,
) -> None:
    client = GatewayInstanceClient(settings(tmp_path))
    await client._http.aclose()
    status = 503

    def respond(request: httpx.Request) -> httpx.Response:
        return httpx.Response(status, json={"error": "unavailable"})

    client._http = httpx.AsyncClient(
        base_url="http://gateway", transport=httpx.MockTransport(respond)
    )
    try:
        with pytest.raises(ExecutionCredentialsDeferred):
            await client.credentials("session", "turn", 3)
        status = 403
        with pytest.raises(ExecutionCredentialsRejected):
            await client.credentials("session", "turn", 3)
    finally:
        await client.close()
