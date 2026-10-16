import json
from dataclasses import replace
from pathlib import Path

import pytest

from adapter.biz.domain.acp_adapter.errors import AuthenticationRequired
from adapter.biz.workflow.execute_turn_flow import RuntimeCredentials
from codex.authentication import configure_authentication
from codex.bridge import CodexAcpBridge


def test_selected_auth_is_private_and_native_refresh_survives_until_revision_changes(
    tmp_path: Path,
) -> None:
    raw = json.dumps(
        {"tokens": {"access_token": "private-access", "refresh_token": "private-refresh"}}
    )
    credentials = RuntimeCredentials(
        codex_auth_json=raw, model_connection_id="connection", model_connection_version=1
    )
    first = CodexAcpBridge()
    configure_authentication(first, credentials, "session-one", tmp_path)
    home = Path(first.env["CODEX_HOME"])
    target = home / "auth.json"
    assert target.read_text() == raw
    assert target.stat().st_mode & 0o777 == 0o600
    assert "private-access" not in repr(credentials)
    refreshed = raw.replace("private-access", "native-refreshed")
    target.write_text(refreshed)
    configure_authentication(first, credentials, "session-one", tmp_path)
    assert target.read_text() == refreshed
    configure_authentication(
        first, replace(credentials, model_connection_version=2), "session-one", tmp_path
    )
    assert target.read_text() == raw
    other = CodexAcpBridge()
    configure_authentication(
        other, RuntimeCredentials(api_key="new-api-key"), "session-two", tmp_path
    )
    assert Path(other.env["CODEX_HOME"]) != home
    assert not (Path(other.env["CODEX_HOME"]) / "auth.json").exists()
    configure_authentication(
        first, RuntimeCredentials(api_key="new-api-key"), "session-one", tmp_path
    )
    assert not target.exists()
    assert not (home / "model-connection").exists()


def test_auth_requires_explicit_connection_scope(tmp_path: Path) -> None:
    with pytest.raises(AuthenticationRequired):
        configure_authentication(
            CodexAcpBridge(),
            RuntimeCredentials(codex_auth_json='{"tokens":{}}'),
            "session",
            tmp_path,
        )
