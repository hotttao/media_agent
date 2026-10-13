"""Gateway-authorized instance startup and fresh, non-persisted execution credentials."""

from __future__ import annotations

import asyncio
from dataclasses import dataclass
from pathlib import Path

import httpx
from pydantic import SecretStr

from adapter.biz.domain.runtime_session.errors import (
    ExecutionCredentialsDeferred,
    ExecutionCredentialsRejected,
)
from adapter.biz.domain.runtime_session.ownership import InstanceScope
from adapter.biz.workflow.execute_turn_flow import RuntimeCredentials


@dataclass(frozen=True, slots=True)
class InstanceClientSettings:
    gateway_url: str
    agent_api_token: SecretStr
    environment_id: str
    workspace_id: str
    agent_config_id: str
    replica_id: str
    incarnation_id: str
    namespace: str
    resource_name: str
    resource_uid: str
    agent_type: str
    bootstrap_proof_file: str


class InstanceRegistrationRejected(ValueError):
    pass


class GatewayInstanceClient:
    def __init__(self, config: InstanceClientSettings) -> None:
        self._config = config
        self.instance_credential = SecretStr("")
        self.admission = "active"
        self.drain_operation_id = ""
        self._http = httpx.AsyncClient(
            base_url=config.gateway_url.rstrip("/"),
            timeout=10,
            follow_redirects=False,
            headers={"Authorization": "Bearer " + config.agent_api_token.get_secret_value()},
        )

    async def register(self) -> InstanceScope:
        cfg = self._config
        locator: dict[str, object] = {
            "environment_id": cfg.environment_id,
            "workspace_id": cfg.workspace_id,
            "agent_config_id": cfg.agent_config_id,
            "replica_id": cfg.replica_id,
            "incarnation_id": cfg.incarnation_id,
            "namespace": cfg.namespace,
            "resource_name": cfg.resource_name,
            "resource_uid": cfg.resource_uid,
        }
        # Retain the resolved native generation across transport retries; never let an
        # old process silently register itself as a newly restarted container.
        resolved = False
        delay = 1
        while True:
            proof = Path(cfg.bootstrap_proof_file).read_text().strip()
            headers = {"X-Runtime-Bootstrap-Proof": proof}
            try:
                identity = await self._http.post(
                    "/api/agent/v1/instances/identity", json=locator, headers=headers
                )
                if identity.status_code >= 500:
                    raise ExecutionCredentialsDeferred()
                identity.raise_for_status()
                value = identity.json()
                if value["agent_type"] != cfg.agent_type:
                    raise InstanceRegistrationRejected("native_runtime_type_mismatch")
                if value["environment_id"] != cfg.environment_id:
                    raise InstanceRegistrationRejected("instance_environment_mismatch")
                if resolved and (locator["resource_uid"], locator["resource_generation"]) != (
                    value["resource_uid"],
                    value["resource_generation"],
                ):
                    raise InstanceRegistrationRejected("native_generation_changed")
                locator.update(
                    resource_uid=value["resource_uid"],
                    resource_generation=value["resource_generation"],
                    expected_version=value.get("registration_version", 0),
                )
                resolved = True
                response = await self._http.post(
                    "/api/agent/v1/instances/register", json=locator, headers=headers
                )
                if response.status_code >= 500 or response.status_code == 409:
                    raise ExecutionCredentialsDeferred()
                response.raise_for_status()
                registration = response.json()
                if registration["incarnation_id"] != cfg.incarnation_id:
                    raise InstanceRegistrationRejected("instance_incarnation_mismatch")
                self.admission = registration["admission"]
                self.drain_operation_id = registration.get("drain_operation_id") or ""
                if self.admission not in {"active", "draining"} or (
                    self.admission == "draining" and not self.drain_operation_id
                ):
                    raise InstanceRegistrationRejected("invalid_registered_admission")
                self.instance_credential = SecretStr(registration["instance_credential"])
                self._http.headers["X-Agent-Instance-Credential"] = (
                    self.instance_credential.get_secret_value()
                )
                return InstanceScope(
                    cfg.workspace_id,
                    cfg.agent_config_id,
                    cfg.replica_id,
                    cfg.incarnation_id,
                )
            except (httpx.TransportError, ExecutionCredentialsDeferred):
                await asyncio.sleep(delay)
                delay = min(delay * 2, 30)

    async def credentials(self, session_id: str, turn_id: str, epoch: int) -> RuntimeCredentials:
        cfg = self._config
        try:
            response = await self._http.post(
                f"/api/agent/v1/sessions/{session_id}/execution-context",
                json={
                    "workspace_id": cfg.workspace_id,
                    "agent_config_id": cfg.agent_config_id,
                    "replica_id": cfg.replica_id,
                    "incarnation_id": cfg.incarnation_id,
                    "execution_epoch": epoch,
                    "turn_id": turn_id,
                },
            )
            if response.status_code >= 500 or response.status_code == 429:
                raise ExecutionCredentialsDeferred()
            if response.status_code != 200:
                raise ExecutionCredentialsRejected()
            value = response.json()
            if value["execution_epoch"] != epoch or not value["session_token"]:
                raise ExecutionCredentialsRejected()
            return RuntimeCredentials(
                api_key=value.get("api_key") or None,
                session_token=value["session_token"],
                mcp_url=cfg.gateway_url.rstrip("/") + "/mcp",
                mcp_headers={
                    "Authorization": "Bearer " + cfg.agent_api_token.get_secret_value(),
                    "X-Session-Token": value["session_token"],
                    "X-Agent-Instance-Credential": self.instance_credential.get_secret_value(),
                    "X-Agent-Replica-ID": cfg.replica_id,
                    "X-Agent-Incarnation-ID": cfg.incarnation_id,
                    "X-Agent-Turn-ID": turn_id,
                    "X-Agent-Execution-Epoch": str(epoch),
                },
            )
        except httpx.TransportError as error:
            raise ExecutionCredentialsDeferred() from error

    async def close(self) -> None:
        await self._http.aclose()
