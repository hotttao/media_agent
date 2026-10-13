from pathlib import Path

from adapter.platform_prompt import write_platform_prompt


def test_gateway_platform_body_is_written_without_interpolation(tmp_path: Path) -> None:
    body = "Gateway {{literal}} 👋\nConfigured prompt remains separate.\n"
    write_platform_prompt(target_dir=tmp_path, filename="AGENTS.md", body=body)
    assert (tmp_path / "AGENTS.md").read_text() == body
