import os
from dataclasses import replace
from pathlib import Path

import pytest

from adapter.biz.domain.acp_adapter.errors import AuthenticationRequired
from adapter.biz.workflow.execute_turn_flow import RuntimeCredentials
from goose.authentication import configure_model
from goose.bridge import GooseAcpBridge


def test_model_protocol_applies_to_one_bridge_without_global_state(tmp_path: Path) -> None:
    credentials = RuntimeCredentials(
        api_key="private-key",
        model_protocol="openai",
        model_name="deepseek/model",
        model_base_url="https://api.example.test/v1",
    )
    before = dict(os.environ)
    first = GooseAcpBridge(cwd=str(tmp_path))
    configure_model(first, credentials, "session-one")
    assert first.env["GOOSE_PROVIDER"] == "openai"
    assert first.env["OPENAI_HOST"] == "https://api.example.test"
    assert first.env["OPENAI_BASE_PATH"] == "v1/chat/completions"
    second = GooseAcpBridge(cwd=str(tmp_path))
    configure_model(
        second,
        replace(
            credentials,
            model_protocol="anthropic",
            api_key="other-private-key",
            model_base_url="https://anthropic.example.test",
        ),
        "session-two",
    )
    assert second.env["ANTHROPIC_API_KEY"] == "other-private-key"
    assert "OPENAI_API_KEY" not in second.env
    assert first.env["OPENAI_API_KEY"] == "private-key"
    assert dict(os.environ) == before
    assert "private-key" not in repr(credentials)


def test_missing_key_cannot_borrow_another_account(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    monkeypatch.setenv("OPENAI_API_KEY", "ambient-private-key")
    with pytest.raises(AuthenticationRequired):
        configure_model(
            GooseAcpBridge(cwd=str(tmp_path)), RuntimeCredentials(model_name="model"), "session"
        )
