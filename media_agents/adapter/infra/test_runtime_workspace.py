import json

import pytest

from adapter.infra.runtime_workspace import RuntimeWorkspaceError, ensure_runtime_workspace


def test_runtime_workspace_reuses_identity_across_restart(tmp_path) -> None:
    first = ensure_runtime_workspace(tmp_path, "workspace-1")
    second = ensure_runtime_workspace(tmp_path, "workspace-1")

    assert second == first
    assert first.workspace_id == "workspace-1"
    assert first.mount_path == str(tmp_path.resolve())


def test_runtime_workspace_rejects_mount_owned_by_another_workspace(tmp_path) -> None:
    ensure_runtime_workspace(tmp_path, "workspace-1")

    with pytest.raises(RuntimeWorkspaceError, match="identity conflict"):
        ensure_runtime_workspace(tmp_path, "workspace-2")


def test_runtime_workspace_rejects_corrupt_identity(tmp_path) -> None:
    metadata = tmp_path / ".media-agent"
    metadata.mkdir()
    (metadata / "workspace.json").write_text(json.dumps({"version": 1}), encoding="utf-8")

    with pytest.raises(RuntimeWorkspaceError, match="invalid Runtime Workspace identity"):
        ensure_runtime_workspace(tmp_path, "workspace-1")
