#!/usr/bin/env python3
"""Start Codex in this workspace, reading the wx_channel token privately."""
import json
import os
from pathlib import Path
import sys

root = Path(__file__).resolve().parents[1]
settings = json.loads((root / "workspace.local.json").read_text())
token_file = Path(os.environ.get("WX_CHANNEL_DOWNLOADS_DIR", settings["wx_downloads"])) / ".wx_channel_mcp_token"
if token_file.is_file():
    token = token_file.read_text().strip()
    if not token or len(token) > 4096:
        raise SystemExit("wx_channel MCP token file is invalid")
    os.environ["MEDIA_WX_CHANNEL_MCP_TOKEN"] = token
else:
    print("wx_channel token not found; start wx_channel with this downloads directory to enable its MCP tools.", file=sys.stderr)
os.chdir(root)
os.execvp("codex", ["codex", *sys.argv[1:]])
