from __future__ import annotations

import json
import sys
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
from adapter.test_metadata_update import Gateway, skill_zip, snapshot
from codex.bridge import CodexAcpBridge
from goose.bridge import GooseAcpBridge


class Hooks:
    def load_mcp_servers(self, token: str | None) -> list[Any]:
        raise AssertionError("managed execution loaded ambient MCP configuration")

    def has_mcp_servers(self) -> bool:
        return False


@pytest.mark.asyncio
@pytest.mark.parametrize("kind", ["codex", "goose"])
async def test_acp_resume_refreshes_turn_identity_and_keeps_fixed_prompt(
    tmp_path: Path,
    kind: str,
) -> None:
    artifact = skill_zip("pinned")
    metadata = snapshot("selected", 2, "Configured body", artifact, agent_type=kind)
    flow = MetadataUpdateFlow(
        Gateway(metadata, {metadata.resources[0].checksum: artifact}),
        MetadataArtifacts(tmp_path, "config", "0", "boot"),
        kind,
    )
    fixture = Path(__file__).with_name("mcp_acp_fixture.py")
    capture = tmp_path / "capture.jsonl"
    processes = []

    def factory(session_id: str, api_key: str | None = None) -> AcpBridge:
        cls = GooseAcpBridge if kind == "goose" else CodexAcpBridge
        bridge = cls(
            program=sys.executable, args=[str(fixture)], cwd=str(tmp_path), session_id=session_id
        )
        bridge.env["MCP_CAPTURE"] = str(capture)
        return bridge

    def configure(bridge: object, bundle: Path, session_id: str) -> None:
        if isinstance(bridge, GooseAcpBridge):
            bridge.configure_metadata(bundle, session_id)

    server = AcpServer(cast(Any, Hooks()), factory)
    runtime = AcpTurnRuntime(server, flow, configure)
    session = RuntimeSession.register(
        session_id="session", context_version=2, context_ref="metadata:config:selected"
    )
    try:
        handles = []
        for index in range(2):
            turn = Turn(
                f"turn-{index}",
                "session",
                f"key-{index}",
                index + 1,
                TurnStatus.RUNNING,
                {"message": "hello"},
                1,
                datetime.now(UTC),
            )
            credentials = RuntimeCredentials(
                session_token="secret",
                mcp_url="http://gateway/mcp",
                mcp_headers={"X-Agent-Turn-ID": turn.turn_id, "X-Agent-Execution-Epoch": "3"},
            )
            prepared = await runtime.prepare(session, turn, credentials)
            handles.append(prepared.handle)
            processes.append(server.bridges[prepared.handle].proc)
            session = replace(session, runtime_session_id=prepared.handle)
        assert handles[0] == handles[1]
        records = [json.loads(v) for v in capture.read_text().splitlines()]
        config_records = [v for v in records if v["operation"] in {"new", "load"}]
        assert [v["operation"] for v in config_records] == ["new", "load"]
        for index, record in enumerate(config_records):
            assert len(record["servers"]) == 1
            config = record["servers"][0]
            assert config["url"] == "http://gateway/mcp"
            assert config["name"] == "media-agent-gateway"
            assert {v["name"]: v["value"] for v in config["headers"]} == {
                "X-Agent-Turn-ID": f"turn-{index}",
                "X-Agent-Execution-Epoch": "3",
            }
        if kind == "goose":
            prompts = [v for v in records if v["operation"] == "prompt"]
            assert [v["text"] for v in prompts] == ["Platform selected", "Configured body"] * 2
            assert [v["key"] for v in prompts] == [
                "media-agent-platform",
                "media-agent-configured",
            ] * 2
            assert all(v["mode"] == "append" for v in prompts)
    finally:
        for handle in tuple(server.sessions):
            await server.detach_session(handle)
        for process in processes:
            if process is not None:
                await process.wait()
