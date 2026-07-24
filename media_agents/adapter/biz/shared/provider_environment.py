"""Provider environment helpers shared by agent packages."""

from __future__ import annotations

import os
from collections.abc import Mapping, MutableMapping
from dataclasses import dataclass, field

ProviderEnv = MutableMapping[str, str]

DEFAULT_PROVIDER_ENV_KEYS = {
    "ANTHROPIC_API_KEY",
    "ANTHROPIC_BASE_URL",
    "OPENAI_API_KEY",
    "OPENAI_BASE_URL",
    "CODEX_API_KEY",
    "GOOSE_PROVIDER",
    "GOOSE_MODEL",
    "GOOSE_SMALL_MODEL",
    "GOOSE_CLI_MIN_PRIORITY",
}


@dataclass(frozen=True)
class ProviderEnvResult:
    cleared: set[str] = field(default_factory=set)
    applied: dict[str, str] = field(default_factory=dict)


def clear_provider_env(
    environ: ProviderEnv | None = None,
    *,
    keys: set[str] | None = None,
) -> set[str]:
    """Remove provider-specific env vars and return the keys that existed."""
    env = environ if environ is not None else os.environ
    removed: set[str] = set()
    for key in keys or DEFAULT_PROVIDER_ENV_KEYS:
        if key in env:
            removed.add(key)
            env.pop(key, None)
    return removed


def apply_openai_compatible_env(
    runtime: Mapping[str, object] | None,
    *,
    api_key: str | None,
    environ: ProviderEnv | None = None,
    clear_keys: set[str] | None = None,
) -> ProviderEnvResult:
    """Apply env vars for OpenAI-compatible agents such as codex-acp."""
    env = environ if environ is not None else os.environ
    cleared = clear_provider_env(env, keys=clear_keys)
    applied: dict[str, str] = {}
    if not runtime or not api_key:
        return ProviderEnvResult(cleared=cleared, applied=applied)

    env["OPENAI_API_KEY"] = api_key
    applied["OPENAI_API_KEY"] = api_key
    base_url = str(runtime.get("base_url") or "")
    if base_url:
        env["OPENAI_BASE_URL"] = base_url
        applied["OPENAI_BASE_URL"] = base_url
    return ProviderEnvResult(cleared=cleared, applied=applied)
