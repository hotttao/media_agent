from __future__ import annotations

import json
import sys
import tomllib
from dataclasses import replace
from datetime import UTC, datetime
from pathlib import Path
from typing import Any, cast

import pytest

from adapter.biz.domain.acp_adapter import AcpServer
from adapter.biz.domain.acp_adapter.bridge import AcpBridge
from adapter.biz.domain.runtime_session.entity import RuntimeSession, Turn, TurnStatus
from adapter.biz.workflow.acp_turn_runtime import AcpTurnRuntime
from adapter.biz.workflow.execute_turn_flow import RuntimeCredentials
from adapter.biz.workflow.metadata_update_flow import MetadataUpdateFlow
from adapter.metadata_artifacts import MetadataArtifacts
from adapter.runtime_metadata_files import codex_home
from adapter.test_metadata_update import Gateway, skill_zip, snapshot
from codex.bridge import CodexAcpBridge


class Hooks:
    def load_mcp_servers(self, token: str | None) -> list[Any]:
        return []

    def has_mcp_servers(self) -> bool:
        return False


def turn(session_id: str) -> Turn:
    return Turn(
        "turn", session_id, "key", 1, TurnStatus.RUNNING, {"message": "hello"}, 1, datetime.now(UTC)
    )


@pytest.mark.asyncio
async def test_codex_acp_child_and_resume_keep_fixed_metadata(tmp_path: Path) -> None:
    first_content, second_content = skill_zip("old"), skill_zip("new")
    first = snapshot("old-version", 2, "Old 👋", first_content)
    second = snapshot("new-version", 3, "New", second_content)
    gateway = Gateway(
        first,
        {first.resources[0].checksum: first_content, second.resources[0].checksum: second_content},
    )
    flow = MetadataUpdateFlow(gateway, MetadataArtifacts(tmp_path, "config", "0", "boot"), "codex")
    fixture = Path(__file__).parent / "tests/fixtures/metadata_acp_agent.py"
    capture = tmp_path / "capture.jsonl"

    def factory(session_id: str, api_key: str | None = None) -> CodexAcpBridge:
        bridge = CodexAcpBridge(
            program=sys.executable, args=[str(fixture)], cwd=str(tmp_path), session_id=session_id
        )
        bridge.env["METADATA_CAPTURE"] = str(capture)
        return bridge

    def configure(bridge: object, bundle: Path, session_id: str) -> None:
        assert isinstance(bridge, AcpBridge)
        bridge.env["CODEX_HOME"] = str(codex_home(bundle, session_id, tmp_path, tmp_path / "auth"))

    server = AcpServer(cast(Any, Hooks()), factory)
    runtime = AcpTurnRuntime(server, flow, configure)
    old = RuntimeSession.register(
        session_id="old-session", context_version=2, context_ref="metadata:config:old-version"
    )
    handles = []
    processes = []
    try:
        prepared = await runtime.prepare(old, turn(old.session_id), RuntimeCredentials())
        handles.append(prepared.handle)
        processes.append(server.bridges[prepared.handle].proc)
        old = replace(old, runtime_session_id=prepared.handle)
        gateway.current = second
        gateway.history[second.target_context_version] = second
        await flow.reconcile()
        new = RuntimeSession.register(
            session_id="new-session", context_version=3, context_ref="metadata:config:new-version"
        )
        prepared_new = await runtime.prepare(new, turn(new.session_id), RuntimeCredentials())
        handles.append(prepared_new.handle)
        processes.append(server.bridges[prepared_new.handle].proc)
        server._destroy(prepared.handle)
        resumed = await runtime.prepare(old, turn(old.session_id), RuntimeCredentials())
        processes.append(server.bridges[resumed.handle].proc)
        assert resumed.handle == prepared.handle and not resumed.is_new
        records = [json.loads(line) for line in capture.read_text().splitlines()]
        assert [v["configured"] for v in records] == ["Old 👋", "New", "Old 👋"]
        assert [v["skill"].splitlines()[-1] for v in records] == ["old", "new", "old"]
        assert all(v["cwd"] == str(tmp_path) for v in records)
        native = tomllib.loads(
            (tmp_path / ".metadata/sessions/new-session/codex/config.toml").read_text()
        )
        assert native["features"] == {
            "apps": False,
            "plugins": False,
            "remote_plugin": False,
            "skill_mcp_dependency_install": False,
        }
    finally:
        for handle in handles:
            server._destroy(handle)
        for process in processes:
            if process is not None:
                await process.wait()
