"""Convert an explicitly selected personal connection into private native state."""

from __future__ import annotations

import json
import os
import re
import tempfile
from pathlib import Path

from adapter.biz.domain.acp_adapter.errors import AuthenticationRequired
from adapter.biz.workflow.execute_turn_flow import RuntimeCredentials
from codex.bridge import CodexAcpBridge


def configure_authentication(
    bridge: object, credentials: RuntimeCredentials, session_id: str, workspace: Path
) -> None:
    if not isinstance(bridge, CodexAcpBridge):
        raise TypeError("unexpected Codex bridge")
    if not re.fullmatch(r"[A-Za-z0-9_-]{1,128}", session_id):
        raise AuthenticationRequired("Invalid native authentication scope")
    home = Path(bridge.env.get("CODEX_HOME") or workspace / ".runtime-auth" / session_id / "codex")
    home.mkdir(parents=True, exist_ok=True, mode=0o700)
    bridge.env["CODEX_HOME"] = str(home)
    if not (home / "config.toml").exists():
        (home / "config.toml").write_text(
            "[features]\napps = false\nplugins = false\n"
            "remote_plugin = false\nskill_mcp_dependency_install = false\n"
        )
    target = home / "auth.json"
    marker = home / "model-connection"
    if not credentials.codex_auth_json:
        target.unlink(missing_ok=True)
        marker.unlink(missing_ok=True)
        return
    if not credentials.model_connection_id or credentials.model_connection_version < 1:
        raise AuthenticationRequired("Invalid model connection")
    reference = json.dumps([credentials.model_connection_id, credentials.model_connection_version])
    if marker.is_file() and marker.read_text() == reference and target.is_file():
        return  # Preserve tokens refreshed by native Codex.
    raw = credentials.codex_auth_json
    if len(raw.encode()) > 65536:
        raise AuthenticationRequired("Invalid model authentication")
    try:
        value = json.loads(raw)
    except ValueError as error:
        raise AuthenticationRequired("Invalid model authentication") from error
    if not isinstance(value, dict) or not value:
        raise AuthenticationRequired("Invalid model authentication")
    fd, name = tempfile.mkstemp(prefix=".auth-", dir=home)
    try:
        with os.fdopen(fd, "w") as stream:
            stream.write(raw)
        os.replace(name, target)
    finally:
        Path(name).unlink(missing_ok=True)
    target.chmod(0o600)
    marker.write_text(reference)
    marker.chmod(0o600)
