from pathlib import Path

import pytest
from claude_code.config import ClaudeCodeConfig, load_claude_code_config
from codex.config import CodexConfig, load_codex_config
from goose.config import GooseConfig, load_goose_config

from adapter.config.config import AdapterConfig, load_agent_config

ROOT = Path(__file__).resolve().parents[2]


def test_load_codex_config_merges_common_defaults() -> None:
    cfg = load_codex_config(ROOT / "codex" / "conf" / "config.yaml")

    assert isinstance(cfg, CodexConfig)
    assert cfg.agent_type == "codex"
    assert cfg.agent.port == 3001
    assert cfg.workspace_dir == "/workspace"
    assert cfg.bridge_ttl.max_count == 10
    assert cfg.codex.program == "codex-acp"


def test_load_goose_config_has_agent_specific_section() -> None:
    cfg = load_goose_config(ROOT / "goose" / "conf" / "config.yaml")

    assert isinstance(cfg, GooseConfig)
    assert cfg.agent_type == "goose"
    assert cfg.agent.port == 3002
    assert cfg.goose.program == "goose"
    assert cfg.goose.path_root is None


def test_load_claude_code_config_has_agent_specific_section() -> None:
    cfg = load_claude_code_config(ROOT / "claude_code" / "conf" / "config.yaml")

    assert isinstance(cfg, ClaudeCodeConfig)
    assert cfg.agent_type == "claude-code"
    assert cfg.agent.port == 3003
    assert cfg.skills.enabled is True
    assert cfg.claude_code.bin == "claude"


def test_cp_url_and_workspace_id_must_be_configured_together(tmp_path: Path) -> None:
    override = tmp_path / "config.yaml"
    override.write_text(
        """
agent:
  type: test
capabilities:
  system_prompt: true
  mcp: true
  skills: false
  questions: false
  reconnect: true
  permissions: true
  streaming_deltas: true
cp:
  url: http://cp.test
""",
        encoding="utf-8",
    )

    with pytest.raises(ValueError, match="configured together"):
        load_agent_config(override, AdapterConfig)


def test_runtime_paths_must_stay_inside_workspace(tmp_path: Path) -> None:
    override = tmp_path / "config.yaml"
    override.write_text(
        """
agent:
  type: test
capabilities:
  system_prompt: true
  mcp: true
  skills: false
  questions: false
  reconnect: true
  permissions: true
  streaming_deltas: true
runtime:
  file: ../runtime.json
""",
        encoding="utf-8",
    )

    with pytest.raises(ValueError, match="runtime.file"):
        load_agent_config(override, AdapterConfig)