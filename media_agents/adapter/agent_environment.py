"""Reusable agent environment preparation helpers.

This module is intentionally stdlib-only so it can live in ``biz.shared`` under
adapter architecture rules. Callers pass already-fetched CP payloads and concrete
paths; this module only normalizes data and performs local filesystem/env work.
"""

from __future__ import annotations

import json
import logging
import os
from collections.abc import MutableMapping
from dataclasses import dataclass
from pathlib import Path
from typing import Any

logger = logging.getLogger(__name__)

McpServerConfig = dict[str, Any]
McpServerMap = dict[str, McpServerConfig]
RuntimeConfig = dict[str, str]


@dataclass(frozen=True)
class CredentialLoadResult:
    injected: int
    cleaned: int
    env_file: Path


def parse_mcp_servers(raw_mcp_config: Any) -> McpServerMap:
    """Parse CP mcp_config into a name -> config mapping.

    CP may return either ``{"mcpServers": {...}}`` or the raw mapping. Invalid
    or empty JSON is treated as no MCP servers, matching the TS agents.
    """
    if not raw_mcp_config:
        return {}
    if isinstance(raw_mcp_config, dict):
        candidate = raw_mcp_config.get("mcpServers", raw_mcp_config)
    elif isinstance(raw_mcp_config, str):
        try:
            parsed = json.loads(raw_mcp_config)
        except json.JSONDecodeError:
            logger.warning("CP mcp_config is not valid JSON")
            return {}
        candidate = parsed.get("mcpServers", parsed) if isinstance(parsed, dict) else {}
    else:
        return {}
    if not isinstance(candidate, dict):
        return {}
    return {str(name): cfg for name, cfg in candidate.items() if isinstance(cfg, dict)}


def runtime_from_workspace_config(config: dict[str, Any]) -> RuntimeConfig:
    return {
        "model": str(config.get("model") or ""),
        "provider_type": str(config.get("provider_type") or ""),
        "base_url": str(config.get("base_url") or ""),
        "small_model": str(config.get("small_model") or ""),
    }


def write_runtime_config(workspace_dir: str | Path, config: dict[str, Any]) -> RuntimeConfig:
    runtime = runtime_from_workspace_config(config)
    workspace = Path(workspace_dir)
    workspace.mkdir(parents=True, exist_ok=True)
    (workspace / "runtime.json").write_text(json.dumps(runtime), encoding="utf-8")
    return runtime


def read_runtime_config(workspace_dir: str | Path) -> RuntimeConfig | None:
    try:
        data = json.loads((Path(workspace_dir) / "runtime.json").read_text(encoding="utf-8"))
    except (FileNotFoundError, json.JSONDecodeError):
        return None
    if not isinstance(data, dict):
        return None
    return runtime_from_workspace_config(data)


def prompt_from_workspace_config(config: dict[str, Any]) -> str:
    return str(config.get("prompt_content") or config.get("system_prompt") or "")


def write_prompt_file(
    workspace_dir: str | Path,
    config: dict[str, Any],
    *,
    filename: str = "AGENTS.md",
) -> str:
    prompt = prompt_from_workspace_config(config)
    workspace = Path(workspace_dir)
    workspace.mkdir(parents=True, exist_ok=True)
    (workspace / filename).write_text(prompt, encoding="utf-8")
    return prompt


def write_mcp_json(config_dir: str | Path, mcp_servers: McpServerMap) -> None:
    target_dir = Path(config_dir).expanduser()
    target_dir.mkdir(parents=True, exist_ok=True)
    (target_dir / "mcp.json").write_text(
        json.dumps({"mcpServers": mcp_servers}, indent=2),
        encoding="utf-8",
    )


def build_acp_mcp_servers(
    *,
    platform_url: str | None,
    platform_headers: dict[str, str],
    user_mcp_servers: McpServerMap,
    session_token: str | None,
    include_user_http: bool = True,
    include_user_stdio: bool = False,
) -> list[dict[str, Any]]:
    """Build ACP-session MCP server declarations shared by agents.

    Platform-controlled headers override user headers on ``tos-platform``.
    User HTTP MCPs receive the session token so CP can translate it upstream.
    """
    if not platform_url:
        return []

    servers: list[dict[str, Any]] = []
    user_platform = user_mcp_servers.get("tos-platform", {})
    user_platform_headers = _string_headers(user_platform.get("headers"))
    headers = {**user_platform_headers, **platform_headers}
    servers.append(
        {"type": "http", "name": "tos-platform", "url": platform_url, "headers": headers}
    )

    for name, cfg in user_mcp_servers.items():
        if name == "tos-platform":
            continue
        if include_user_http and cfg.get("url"):
            entry_headers = _string_headers(cfg.get("headers"))
            if session_token:
                entry_headers["X-Session-Token"] = session_token
            servers.append(
                {"type": "http", "name": name, "url": cfg["url"], "headers": entry_headers}
            )
        elif include_user_stdio and cfg.get("command"):
            server: dict[str, Any] = {"name": name, "command": cfg["command"]}
            if isinstance(cfg.get("args"), list):
                server["args"] = cfg["args"]
            if isinstance(cfg.get("env"), dict):
                server["env"] = cfg["env"]
            servers.append(server)
    return servers


def apply_credentials(
    credentials: list[dict[str, Any]],
    *,
    home: str | Path,
    environ: MutableMapping[str, str] | None = None,
) -> CredentialLoadResult:
    """Inject CP credentials into process env, files, and shell startup files."""
    env = environ if environ is not None else os.environ
    home_path = Path(home).expanduser()
    env_file = home_path / ".agent-credentials.env"
    source_line = f'[ -f "{env_file}" ] && . "{env_file}"'
    injected = 0
    cleaned = 0
    env_lines: list[str] = []

    _ensure_ssh_config(home_path)

    for cred in credentials:
        name = str(cred.get("name") or "")
        inject = str(cred.get("inject") or "")
        status = str(cred.get("status") or "")
        path = cred.get("path")
        if status == "deleting":
            if inject == "env" and name:
                env.pop(name, None)
                cleaned += 1
            elif inject == "file" and isinstance(path, str) and path:
                _resolve_agent_path(path, home_path).unlink(missing_ok=True)
                cleaned += 1
            continue

        if inject == "env" and name:
            value = str(cred.get("value") or "")
            env[name] = value
            env_lines.append(f"export {name}={shell_quote(value)}")
            injected += 1
        elif inject == "file" and isinstance(path, str) and path:
            target = _resolve_agent_path(path, home_path)
            target.parent.mkdir(parents=True, exist_ok=True)
            parent_mode = 0o700 if _is_under(target, home_path / ".ssh") else None
            _chmod_best_effort(target.parent, parent_mode)
            target.write_text(str(cred.get("value") or ""), encoding="utf-8")
            _chmod_best_effort(target, _parse_mode(cred.get("mode")) or 0o600)
            injected += 1

    env_file.parent.mkdir(parents=True, exist_ok=True)
    env_file.write_text("\n".join(env_lines) + ("\n" if env_lines else ""), encoding="utf-8")
    _chmod_best_effort(env_file, 0o600)
    if env_lines:
        env["BASH_ENV"] = str(env_file)
        _ensure_shell_source_line(home_path / ".bashrc", source_line)
        _ensure_shell_source_line(home_path / ".profile", source_line)

    return CredentialLoadResult(injected=injected, cleaned=cleaned, env_file=env_file)


def shell_quote(value: str) -> str:
    return "'" + value.replace("'", "'\\''") + "'"


def _string_headers(value: Any) -> dict[str, str]:
    if not isinstance(value, dict):
        return {}
    return {str(k): v for k, v in value.items() if isinstance(v, str)}


def _ensure_ssh_config(home: Path) -> None:
    ssh_dir = home / ".ssh"
    ssh_dir.mkdir(parents=True, exist_ok=True)
    _chmod_best_effort(ssh_dir, 0o700)
    ssh_config = ssh_dir / "config"
    if not ssh_config.exists():
        ssh_config.write_text("StrictHostKeyChecking accept-new\n", encoding="utf-8")
        _chmod_best_effort(ssh_config, 0o644)


def _ensure_shell_source_line(path: Path, source_line: str) -> None:
    try:
        content = path.read_text(encoding="utf-8")
    except FileNotFoundError:
        content = ""
    if source_line in content:
        return
    sep = "" if not content or content.endswith("\n") else "\n"
    path.write_text(f"{content}{sep}{source_line}\n", encoding="utf-8")


def _resolve_agent_path(path: str, home: Path) -> Path:
    if path == "~":
        return home
    if path.startswith("~/") or path.startswith("~\\"):
        return home / path[2:]
    return Path(path)


def _parse_mode(value: Any) -> int | None:
    if value is None:
        return None
    if isinstance(value, int):
        return value
    if isinstance(value, str) and value:
        try:
            return int(value, 8)
        except ValueError:
            return None
    return None


def _chmod_best_effort(path: Path, mode: int | None) -> None:
    if mode is None:
        return
    try:
        path.chmod(mode)
    except OSError:
        logger.debug("chmod skipped path=%s mode=%o", path, mode)


def _is_under(path: Path, parent: Path) -> bool:
    try:
        path.resolve().relative_to(parent.resolve())
        return True
    except ValueError:
        return False
