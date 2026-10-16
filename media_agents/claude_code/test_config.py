from __future__ import annotations

from pathlib import Path

from claude_code.config import ClaudeCodeConfig, load_claude_code_config

ROOT = Path(__file__).resolve().parents[1]


def test_load_claude_code_config_has_agent_specific_section() -> None:
    cfg = load_claude_code_config(ROOT / "claude_code" / "conf" / "config.yaml")

    assert isinstance(cfg, ClaudeCodeConfig)
    assert cfg.agent_type == "claude-code"
    assert cfg.agent.port == 3003
    assert cfg.skills.enabled is True
    assert cfg.capabilities.system_prompt is True
    assert cfg.capabilities.skills is True
    assert cfg.capabilities.permissions is True
    assert cfg.capabilities.questions is True
    assert cfg.capabilities.reconnect is True
    assert cfg.capabilities.mcp is True
    assert cfg.claude_code.bin == "claude"
