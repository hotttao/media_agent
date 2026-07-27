from __future__ import annotations

from pathlib import Path

import pytest

from adapter.config.config import AdapterConfig, load_agent_config


def test_load_agent_config_merges_common_defaults(tmp_path: Path) -> None:
    override = tmp_path / "config.yaml"
    override.write_text(
        """
agent:
  type: test
  port: 3100
capabilities:
  system_prompt: true
  mcp: false
  skills: false
  questions: false
  reconnect: true
  permissions: false
  streaming_deltas: true
""",
        encoding="utf-8",
    )

    cfg = load_agent_config(override, AdapterConfig)

    assert cfg.agent_type == "test"
    assert cfg.agent.port == 3100
    assert cfg.workspace_dir == "/workspace"
    assert cfg.bridge_ttl.max_count == 10


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