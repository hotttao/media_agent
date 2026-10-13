import httpx
import pytest

from adapter.clients.cp import CpClient


@pytest.mark.asyncio
async def test_missing_identity_skips_gateway_bootstrap() -> None:
    client = CpClient(base_url="http://gateway", workspace_id="ws")
    assert await client.fetch_workspace_config() is None
    assert await client.fetch_credentials() is None


@pytest.mark.asyncio
async def test_gateway_bootstrap_keeps_prompt_bodies_separate() -> None:
    def handler(request: httpx.Request) -> httpx.Response:
        assert request.url.path == "/api/agent/v1/runtime-context"
        assert request.url.params["agent_config_id"] == "cfg"
        assert request.headers["Authorization"] == "Bearer stable"
        assert request.headers["X-Agent-Instance-Credential"] == "instance"
        return httpx.Response(
            200,
            json={
                "platform_prompt": "Platform {{literal}}",
                "configured_prompt": "User",
                "api_key": "not-exposed",
                "mcp_config": {"remote": {"url": "http://backend"}},
            },
        )

    async with httpx.AsyncClient(transport=httpx.MockTransport(handler)) as http:
        client = CpClient(
            base_url="http://gateway",
            workspace_id="ws",
            agent_token="stable",
            instance_credential="instance",
            config_id="cfg",
            replica_id="0",
            incarnation_id="boot",
            http_client=http,
        )
        assert await client.fetch_workspace_config() == {
            "platform_prompt": "Platform {{literal}}",
            "prompt_content": "User",
        }
        assert await client.fetch_credentials() == []
        assert client.mcp_url() == "http://gateway/mcp"
        headers = client.mcp_headers("session")
        assert headers["X-Session-Token"] == "session"
        assert "X-Workspace-ID" not in headers
