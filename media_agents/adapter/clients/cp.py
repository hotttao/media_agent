"""Client for Control Plane workspace APIs."""

from __future__ import annotations

import logging
from collections.abc import Mapping
from typing import Any

import httpx

logger = logging.getLogger(__name__)


class CpClient:
    """Small async client for CP workspace configuration endpoints."""

    def __init__(
        self,
        *,
        base_url: str | None,
        workspace_id: str | None,
        http_client: httpx.AsyncClient | None = None,
    ) -> None:
        self._base_url = base_url.rstrip("/") if base_url else None
        self._workspace_id = workspace_id
        self._http_client = http_client

    @property
    def base_url(self) -> str | None:
        return self._base_url

    @property
    def workspace_id(self) -> str | None:
        return self._workspace_id

    def is_configured(self) -> bool:
        return bool(self._base_url and self._workspace_id)

    def mcp_url(self) -> str | None:
        if not self._base_url:
            return None
        return f"{self._base_url}/mcp"

    def mcp_headers(self, session_token: str | None = None) -> dict[str, str]:
        if not self._workspace_id:
            return {}
        headers = {
            "X-Workspace-ID": self._workspace_id,
            "X-Agent-ID": self._workspace_id,
        }
        if session_token:
            headers["X-Session-Token"] = session_token
        return headers

    async def fetch_workspace_config(self) -> dict[str, Any] | None:
        if not self.is_configured():
            logger.info("CP config skipped: cp.url/cp.workspace_id unset")
            return None
        url = f"{self._base_url}/_cp/workspaces/{self._workspace_id}/config"
        return await self._get_json(url, "workspace config")

    async def fetch_credentials(self) -> list[dict[str, Any]] | None:
        if not self.is_configured():
            logger.info("CP credentials skipped: cp.url/cp.workspace_id unset")
            return None
        url = f"{self._base_url}/_cp/workspaces/{self._workspace_id}/credentials"
        payload = await self._get_json(url, "credentials")
        if payload is None:
            return None
        if isinstance(payload, list):
            return [item for item in payload if isinstance(item, dict)]
        if isinstance(payload, Mapping):
            credentials = payload.get("credentials")
            if isinstance(credentials, list):
                return [item for item in credentials if isinstance(item, dict)]
        logger.error("CP credentials response has unexpected shape")
        return None

    async def _get_json(self, url: str, label: str) -> dict[str, Any] | list[Any] | None:
        try:
            if self._http_client is not None:
                response = await self._http_client.get(url)
            else:
                async with httpx.AsyncClient() as client:
                    response = await client.get(url)
        except httpx.HTTPError:
            logger.exception("CP %s fetch failed url=%s", label, url)
            return None
        if not response.is_success:
            logger.error(
                "CP %s fetch failed status=%s reason=%s url=%s",
                label,
                response.status_code,
                response.reason_phrase,
                url,
            )
            return None
        try:
            payload = response.json()
        except ValueError:
            logger.exception("CP %s response is not JSON url=%s", label, url)
            return None
        if isinstance(payload, (dict, list)):
            return payload
        logger.error("CP %s response has unexpected shape url=%s", label, url)
        return None
