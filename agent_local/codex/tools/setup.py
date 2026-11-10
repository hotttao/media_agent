#!/usr/bin/env python3
"""Prepare project-local Codex config without changing existing servers/secrets."""
import argparse
import json
import shlex
import sys
from pathlib import Path
import tomllib

ROOT = Path(__file__).resolve().parents[1]
WX = '''
[mcp_servers.wx-channel]
url = "http://127.0.0.1:2027/mcp"
http_headers_helper = {headers_helper}
startup_timeout_sec = 30
tool_timeout_sec = 120
required = false
'''


def setup(downloads):
    config = ROOT / ".codex" / "config.toml"
    config.parent.mkdir(parents=True, exist_ok=True)
    if config.is_symlink() and not config.exists():
        raise ValueError("existing config symlink is broken; restore its target first")
    text = config.read_text() if config.exists() else (ROOT / "config.example.toml").read_text()
    # The example uses env authentication for portability; a local setup knows
    # the exact helper path and works for Desktop without restarting its env.
    if not config.exists():
        text = text[:text.index("# launch.py")]
    parsed = tomllib.loads(text)
    if "wx-channel" not in parsed.get("mcp_servers", {}):
        text += WX.format(headers_helper=json.dumps(shlex.join([sys.executable, str(ROOT / "tools" / "wx_headers.py")]), ensure_ascii=False))
    tomllib.loads(text)
    config.write_text(text)
    config.chmod(0o600)
    (ROOT / "workspace.local.json").write_text(json.dumps({"wx_downloads": str(downloads.resolve())}) + "\n")
    print("Project config prepared; existing MCP settings preserved. The wx-channel server reads its local token with the configured headers helper.")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--wx-downloads", type=Path, required=True)
    args = parser.parse_args()
    setup(args.wx_downloads)
