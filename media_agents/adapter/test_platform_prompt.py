from __future__ import annotations

from pathlib import Path

from adapter.platform_prompt import (
    load_platform_prompt_template,
    normalize_memory_attachments,
    render_platform_prompt,
    render_template,
    write_platform_prompt,
)


def test_render_platform_prompt_uses_agent_specific_prompt_file_and_capabilities() -> None:
    codex = render_platform_prompt(agent_kind="codex", workspace_id="ws-1", user_name="Alice")
    claude = render_platform_prompt(agent_kind="claude-code", workspace_id="ws-1")

    assert 'id: ws-1), serving user "Alice"' in codex
    assert "image generation" in codex
    assert "`/workspace/AGENTS.md` is platform-managed" in codex
    assert "image generation" not in claude
    assert "`/workspace/CLAUDE.md` is platform-managed" in claude


def test_render_platform_prompt_includes_memory_attachments() -> None:
    attachments = normalize_memory_attachments(
        [
            {
                "store_id": "mem1",
                "store_name": "Project Memory",
                "store_description": "notes",
                "access": "read_write",
                "instructions": "Prefer recent notes.",
                "index_content": "# Index\n- a.md",
            }
        ]
    )

    prompt = render_platform_prompt(
        agent_kind="goose",
        workspace_id=None,
        memory_attachments=attachments,
    )

    assert "id: unknown" in prompt
    assert "`/mnt/memory/mem1/` (read_write) - Project Memory: notes" in prompt
    assert "Workspace-specific guidance: Prefer recent notes." in prompt
    assert "# Index\n- a.md" in prompt


def test_write_platform_prompt_creates_target_file(tmp_path: Path) -> None:
    prompt = write_platform_prompt(
        agent_kind="codex",
        target_dir=tmp_path / ".codex",
        filename="AGENTS.md",
        workspace_id="ws-1",
    )

    assert (tmp_path / ".codex" / "AGENTS.md").read_text(encoding="utf-8") == prompt

def test_render_template_supports_sections_lists_and_triple_variables() -> None:
    template = "{{#items}}{{name}}={{#enabled}}{{{body}}}{{/enabled}};{{/items}}"

    assert render_template(
        template,
        {"items": [{"name": "a", "enabled": True, "body": "<raw>"}]},
    ) == "a=<raw>;"


def test_default_template_is_loaded_from_package_file() -> None:
    template = load_platform_prompt_template()
    prompt = render_platform_prompt(agent_kind="codex", workspace_id="ws-1")

    assert "{{workspaceId}}" in template
    assert "{{#codex}}" in template
    assert "{{workspaceId}}" not in prompt
    assert "id: ws-1" in prompt