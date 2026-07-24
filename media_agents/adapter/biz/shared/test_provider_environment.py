from __future__ import annotations

from adapter.biz.shared.provider_environment import apply_openai_compatible_env, clear_provider_env


def test_apply_openai_compatible_env_clears_stale_provider_values() -> None:
    env = {
        "ANTHROPIC_API_KEY": "old-anthropic",
        "OPENAI_API_KEY": "old-openai",
        "OPENAI_BASE_URL": "old-url",
    }

    result = apply_openai_compatible_env(
        {"base_url": "http://llm.test/v1"},
        api_key="sk-test",
        environ=env,
    )

    assert "ANTHROPIC_API_KEY" not in env
    assert env["OPENAI_API_KEY"] == "sk-test"
    assert env["OPENAI_BASE_URL"] == "http://llm.test/v1"
    assert result.applied == {
        "OPENAI_API_KEY": "sk-test",
        "OPENAI_BASE_URL": "http://llm.test/v1",
    }


def test_clear_provider_env_returns_removed_keys() -> None:
    env = {"OPENAI_API_KEY": "old", "OTHER": "keep"}

    removed = clear_provider_env(env)

    assert removed == {"OPENAI_API_KEY"}
    assert env == {"OTHER": "keep"}
