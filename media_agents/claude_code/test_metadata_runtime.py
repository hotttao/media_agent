from __future__ import annotations

import json
from datetime import UTC, datetime
from pathlib import Path

import pytest

from adapter.biz.domain.runtime_session.entity import RuntimeSession, Turn, TurnStatus
from adapter.biz.workflow.execute_turn_flow import RuntimeCredentials
from adapter.biz.workflow.metadata_update_flow import MetadataUpdateFlow
from adapter.metadata_artifacts import MetadataArtifacts
from adapter.test_metadata_update import Gateway, skill_zip, snapshot
from claude_code.agent import ClaudeCodeAgent
from claude_code.test_agent import _fake_claude, _ignore_event, _ignore_request
from claude_code.turn_runtime import ClaudeCodeTurnRuntime


@pytest.mark.asyncio
async def test_claude_child_and_resume_read_pinned_prompt_and_skill(tmp_path: Path) -> None:
    first_content, second_content = skill_zip("old"), skill_zip("new")
    first = snapshot("old-version", 2, "Old", first_content, agent_type="claude-code")
    second = snapshot("new-version", 3, "New", second_content, agent_type="claude-code")
    gateway = Gateway(
        first,
        {first.resources[0].checksum: first_content, second.resources[0].checksum: second_content},
    )
    flow = MetadataUpdateFlow(
        gateway, MetadataArtifacts(tmp_path, "config", "0", "boot"), "claude-code"
    )
    executable = _fake_claude(
        tmp_path,
        """import json, sys
from pathlib import Path
prompt = Path(sys.argv[sys.argv.index("--append-system-prompt-file") + 1]).read_text()
plugin = Path(sys.argv[sys.argv.index("--plugin-dir") + 1])
assert json.loads((plugin / ".claude-plugin/plugin.json").read_text())["name"]
assert "--strict-mcp-config" in sys.argv
mcp = json.loads(sys.argv[sys.argv.index("--mcp-config") + 1])
record = {"mcp":mcp,"prompt": prompt, "skill": (plugin / "skills/skill/SKILL.md").read_text(),
          "resumed": "--resume" in sys.argv}
with Path("capture.jsonl").open("a") as output:
    output.write(json.dumps(record) + "\\n")
print(json.dumps({"type":"result", "subtype":"success"}), flush=True)
""",
    )
    runtime = ClaudeCodeTurnRuntime(ClaudeCodeAgent(str(executable), str(tmp_path)), flow)
    old = RuntimeSession.register(
        session_id="old-session", context_version=2, context_ref="metadata:config:old-version"
    )
    new = RuntimeSession.register(
        session_id="new-session", context_version=3, context_ref="metadata:config:new-version"
    )
    for index, session in enumerate((old, new, old)):
        if index == 1:
            gateway.current = second
            gateway.history[second.target_context_version] = second
            await flow.reconcile()
        if index == 2:
            session = session.attach_runtime(session.session_id, now=datetime.now(UTC))
        turn = Turn(
            "turn",
            session.session_id,
            "key",
            1,
            TurnStatus.RUNNING,
            {"message": "hello"},
            1,
            datetime.now(UTC),
        )
        prepared = await runtime.prepare(session, turn, RuntimeCredentials())
        credentials = RuntimeCredentials(
            mcp_url="http://gateway/mcp", mcp_headers={"X-Agent-Turn-ID": str(index)}
        )
        result = await runtime.execute(
            prepared, session, turn, credentials, _ignore_event, _ignore_request
        )
        assert result.status is TurnStatus.COMPLETED
    records = [json.loads(line) for line in (tmp_path / "capture.jsonl").read_text().splitlines()]
    assert [v["prompt"].split("\n\n")[-1] for v in records] == ["Old", "New", "Old"]
    assert [v["skill"].splitlines()[-1] for v in records] == ["old", "new", "old"]
    assert [v["resumed"] for v in records] == [False, False, True]

    assert [v["mcp"] for v in records] == [
        {
            "mcpServers": {
                "media-agent-gateway": {
                    "type": "http",
                    "url": "http://gateway/mcp",
                    "headers": {"X-Agent-Turn-ID": str(i)},
                }
            }
        }
        for i in range(3)
    ]
