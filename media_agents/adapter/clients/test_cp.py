from __future__ import annotations

import httpx
import pytest

from adapter.clients.cp import CpClient


@pytest.mark.asyncio
async def test_fetch_workspace_config_skips_when_unconfigured() -> None:
    client = CpClient(base_url=None, workspace_id=None)

    assert await client.fetch_workspace_config() is None


@pytest.mark.asyncio
async def test_fetch_workspace_config_uses_workspace_endpoint() -> None:
    seen_urls: list[str] = []

    def handler(request: httpx.Request) -> httpx.Response:
        seen_urls.append(str(request.url))
        return httpx.Response(200, json={"model": "gpt-test"})

    async with httpx.AsyncClient(transport=httpx.MockTransport(handler)) as http_client:
        client = CpClient(
            base_url="http://cp.test/",
            workspace_id="workspace-1",
            http_client=http_client,
        )
        payload = await client.fetch_workspace_config()

    assert payload == {"model": "gpt-test"}
    assert seen_urls == ["http://cp.test/_cp/workspaces/workspace-1/config"]


@pytest.mark.asyncio
async def test_fetch_credentials_accepts_wrapped_list() -> None:
    def handler(request: httpx.Request) -> httpx.Response:
        return httpx.Response(200, json={"credentials": [{"type": "env", "name": "A"}]})

    async with httpx.AsyncClient(transport=httpx.MockTransport(handler)) as http_client:
        client = CpClient(
            base_url="http://cp.test",
            workspace_id="workspace-1",
            http_client=http_client,
        )
        payload = await client.fetch_credentials()

    assert payload == [{"type": "env", "name": "A"}]


def test_mcp_headers_include_session_token() -> None:
    client = CpClient(base_url="http://cp.test", workspace_id="workspace-1")

    assert client.mcp_url() == "http://cp.test/mcp"
    assert client.mcp_headers("token") == {
        "X-Workspace-ID": "workspace-1",
        "X-Agent-ID": "workspace-1",
        "X-Session-Token": "token",
    }
