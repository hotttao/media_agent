"""Legacy hook adapter for Gateway metadata; managed sessions use versioned clients."""

from __future__ import annotations

from collections.abc import Mapping
from typing import cast

import httpx

from adapter.agent_environment import CredentialPayload, WorkspaceConfigPayload


class CpClient:
    def __init__(
        self,
        *,
        base_url: str | None,
        workspace_id: str | None,
        agent_token: str = "",
        instance_credential: str = "",
        config_id: str = "",
        replica_id: str = "",
        incarnation_id: str = "",
        http_client: httpx.AsyncClient | None = None,
    ) -> None:
        self._base_url = base_url.rstrip("/") if base_url else None
        self._workspace_id = workspace_id
        self._agent_token = agent_token
        self._instance_credential = instance_credential
        self._scope = {
            "workspace_id": workspace_id or "",
            "agent_config_id": config_id,
            "replica_id": replica_id,
            "incarnation_id": incarnation_id,
        }
        self._http_client = http_client

    @property
    def workspace_id(self) -> str | None:
        return self._workspace_id

    def is_configured(self) -> bool:
        return bool(
            self._base_url
            and self._workspace_id
            and self._agent_token
            and self._instance_credential
            and all(self._scope.values())
        )

    def mcp_url(self) -> str | None:
        return f"{self._base_url}/mcp" if self._base_url else None

    def mcp_headers(self, session_token: str | None = None) -> dict[str, str]:
        headers = {
            "Authorization": "Bearer " + self._agent_token,
            "X-Agent-Instance-Credential": self._instance_credential,
            "X-Agent-Replica-ID": self._scope["replica_id"],
            "X-Agent-Incarnation-ID": self._scope["incarnation_id"],
        }
        if session_token:
            headers["X-Session-Token"] = session_token
        return headers

    async def fetch_workspace_config(self) -> WorkspaceConfigPayload | None:
        if not self.is_configured():
            return None
        url = f"{self._base_url}/api/agent/v1/runtime-context"
        headers = self.mcp_headers()
        if self._http_client is not None:
            response = await self._http_client.get(url, params=self._scope, headers=headers)
        else:
            async with httpx.AsyncClient(timeout=10, follow_redirects=False) as client:
                response = await client.get(url, params=self._scope, headers=headers)
        response.raise_for_status()
        value = response.json()
        if not isinstance(value, Mapping) or not isinstance(value.get("platform_prompt"), str):
            return None
        # No provider or MCP credentials are exposed by this metadata adapter.
        return cast(
            WorkspaceConfigPayload,
            {
                "platform_prompt": value["platform_prompt"],
                "prompt_content": value.get("configured_prompt", ""),
            },
        )

    async def fetch_credentials(self) -> list[CredentialPayload] | None:
        # Provider credentials are resolved for a running Turn by GatewayInstanceClient.
        return [] if self.is_configured() else None
