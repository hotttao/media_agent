"""Native runtime configuration conversion; never overwrite another Session's files."""

from __future__ import annotations

import json
import re
import shutil
from pathlib import Path

from adapter.biz.domain.runtime_metadata.entity import InvalidMetadata


def codex_home(bundle: Path, session_id: str, workspace: Path, auth_source: Path) -> Path:
    if re.fullmatch(r"[A-Za-z0-9_-]{1,128}", session_id) is None:
        raise InvalidMetadata("invalid_runtime_session_id")
    target = workspace / ".metadata" / "sessions" / session_id / "codex"
    identity = target / "metadata-ref"
    if identity.exists():
        if identity.read_text() != str(bundle):
            raise InvalidMetadata("session_metadata_changed")
        return target
    target.mkdir(parents=True, exist_ok=True, mode=0o700)
    # Separate native instruction mechanisms preserve both independently supplied bodies.
    (target / "AGENTS.md").write_bytes((bundle / "platform_prompt.md").read_bytes())
    configured = (bundle / "configured_prompt.md").read_text()
    (target / "config.toml").write_text(
        "developer_instructions = " + json.dumps(configured, ensure_ascii=False) + "\n"
        "[features]\n"
        "apps = false\n"
        "plugins = false\n"
        "remote_plugin = false\n"
        "skill_mcp_dependency_install = false\n"
    )
    if (bundle / "skills").exists():
        shutil.copytree(bundle / "skills", target / "skills", dirs_exist_ok=True)
    source = auth_source / "auth.json"
    if source.is_file() and not source.is_symlink():
        # Native OAuth state is private runtime state, excluded from release bundles/markers.
        shutil.copyfile(source, target / "auth.json")
        (target / "auth.json").chmod(0o600)
    identity.write_text(str(bundle))
    return target


def claude_args(bundle: Path) -> list[str]:
    args = ["--append-system-prompt-file", str(bundle / "claude" / "system_prompt_append.md")]
    if (bundle / "skills").exists():
        # Native --plugin-dir accepts a versioned local plugin with a skills/ subtree.
        args.extend(["--plugin-dir", str(bundle)])
    return args
