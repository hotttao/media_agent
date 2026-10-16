"""Apply fixed Workspace model settings to one Goose bridge, never global env."""

from urllib.parse import urlsplit

from adapter.biz.domain.acp_adapter.errors import AuthenticationRequired
from adapter.biz.domain.runtime_session.errors import InvalidTurn
from adapter.biz.workflow.execute_turn_flow import RuntimeCredentials
from goose.bridge import GooseAcpBridge


def configure_model(bridge: object, credentials: RuntimeCredentials, session_id: str) -> None:
    if not isinstance(bridge, GooseAcpBridge):
        raise TypeError("unexpected Goose bridge")
    if not credentials.api_key:
        raise AuthenticationRequired("Model authentication is required")
    if not credentials.model_name:
        raise InvalidTurn("Model name is required")
    protocol = credentials.model_protocol or "openai"
    if protocol not in {"openai", "anthropic"}:
        raise InvalidTurn("Unsupported model protocol")
    bridge.env.update(
        {
            "GOOSE_PROVIDER": protocol,
            "GOOSE_MODEL": credentials.model_name,
            "GOOSE_MODE": "approve",
            "GOOSE_DISABLE_KEYRING": "1",
            "GOOSE_DISABLE_SESSION_NAMING": "1",
            "GOOSE_DISABLE_TOOL_CALL_SUMMARY": "1",
        }
    )
    if protocol == "anthropic":
        bridge.env.pop("OPENAI_API_KEY", None)
        bridge.env["ANTHROPIC_API_KEY"] = credentials.api_key
        if credentials.model_base_url:
            bridge.env["ANTHROPIC_HOST"] = credentials.model_base_url
    else:
        bridge.env["OPENAI_API_KEY"] = credentials.api_key
        if credentials.model_base_url:
            endpoint = urlsplit(credentials.model_base_url)
            bridge.env["OPENAI_HOST"] = f"{endpoint.scheme}://{endpoint.netloc}"
            path = endpoint.path.strip("/") or "v1"
            bridge.env["OPENAI_BASE_PATH"] = path + "/chat/completions"
