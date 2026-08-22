"""Durable identity for a Workspace-scoped Agent bind mount."""

from __future__ import annotations

import json
import os
import tempfile
import uuid
from dataclasses import dataclass
from datetime import UTC, datetime
from pathlib import Path


class RuntimeWorkspaceError(RuntimeError):
    """The mounted Workspace directory is missing, corrupt, or belongs elsewhere."""


@dataclass(frozen=True, slots=True)
class RuntimeWorkspaceIdentity:
    workspace_id: str
    storage_id: str
    created_at: str
    mount_path: str


def ensure_runtime_workspace(
    workspace_dir: str | Path, workspace_id: str
) -> RuntimeWorkspaceIdentity:
    root = Path(workspace_dir).resolve()
    root.mkdir(parents=True, exist_ok=True)
    metadata_dir = root / ".media-agent"
    metadata_dir.mkdir(mode=0o700, parents=True, exist_ok=True)
    marker = metadata_dir / "workspace.json"
    try:
        _publish_identity(marker, workspace_id)
    except FileExistsError:
        pass
    return _read_identity(marker, workspace_id, root)


def _publish_identity(marker: Path, workspace_id: str) -> None:
    payload = {
        "version": 1,
        "workspace_id": workspace_id,
        "storage_id": str(uuid.uuid4()),
        "created_at": datetime.now(UTC).isoformat(),
    }
    fd, temporary_name = tempfile.mkstemp(prefix="workspace-", suffix=".tmp", dir=marker.parent)
    temporary = Path(temporary_name)
    try:
        os.fchmod(fd, 0o600)
        with os.fdopen(fd, "w", encoding="utf-8") as stream:
            json.dump(payload, stream, separators=(",", ":"), sort_keys=True)
            stream.flush()
            os.fsync(stream.fileno())
        os.link(temporary, marker)
    finally:
        temporary.unlink(missing_ok=True)


def _read_identity(
    marker: Path, expected_workspace_id: str, root: Path
) -> RuntimeWorkspaceIdentity:
    try:
        payload = json.loads(marker.read_text(encoding="utf-8"))
        version = payload["version"]
        workspace_id = payload["workspace_id"]
        storage_id = payload["storage_id"]
        created_at = payload["created_at"]
    except (OSError, json.JSONDecodeError, KeyError, TypeError) as error:
        raise RuntimeWorkspaceError(f"invalid Runtime Workspace identity: {marker}") from error
    if version != 1 or not all(
        isinstance(value, str) and value for value in (workspace_id, storage_id, created_at)
    ):
        raise RuntimeWorkspaceError(f"invalid Runtime Workspace identity: {marker}")
    if workspace_id != expected_workspace_id:
        raise RuntimeWorkspaceError(
            "Runtime Workspace identity conflict: "
            f"mounted={workspace_id}, configured={expected_workspace_id}"
        )
    return RuntimeWorkspaceIdentity(
        workspace_id=workspace_id,
        storage_id=storage_id,
        created_at=created_at,
        mount_path=str(root),
    )
