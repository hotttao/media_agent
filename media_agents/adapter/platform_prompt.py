"""Load the Gateway's platform body into native runtime files without rendering."""

from pathlib import Path


def write_platform_prompt(*, target_dir: str | Path, filename: str, body: str) -> None:
    target = Path(target_dir).expanduser()
    target.mkdir(parents=True, exist_ok=True)
    (target / filename).write_text(body, encoding="utf-8")
