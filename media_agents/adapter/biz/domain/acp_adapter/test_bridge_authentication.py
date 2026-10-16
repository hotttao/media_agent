from types import SimpleNamespace
from typing import cast
from unittest.mock import AsyncMock

import pytest
from acp import RequestError
from acp.client.connection import ClientSideConnection

from adapter.biz.domain.acp_adapter.bridge import AcpBridge
from adapter.biz.domain.acp_adapter.errors import AuthenticationRequired
from adapter.biz.domain.acp_adapter.session import AcpSession


@pytest.mark.asyncio
@pytest.mark.parametrize("operation", ["new", "load", "prompt"])
async def test_bridge_classifies_authentication_without_exposing_details(operation: str) -> None:
    call = AsyncMock(side_effect=RequestError.auth_required({"private": "secret-value"}))
    bridge = AcpBridge(program="unused")
    bridge.conn = cast(
        ClientSideConnection, SimpleNamespace(new_session=call, load_session=call, prompt=call)
    )
    bridge.session = AcpSession("session", bridge)
    with pytest.raises(AuthenticationRequired) as caught:
        if operation == "new":
            await bridge.create_session()
        elif operation == "load":
            await bridge.load_session("session")
        else:
            await bridge.prompt("session", "hello")
    assert "secret-value" not in str(caught.value)
