#!/usr/bin/env python3
"""Codex HTTP headers helper: stdout goes directly to MCP authentication."""
import json
import os
from pathlib import Path
import sys

root = Path(__file__).resolve().parents[1]
try:
    settings = json.loads((root / "workspace.local.json").read_text(encoding="utf-8"))
    directory = Path(os.environ.get("WX_CHANNEL_DOWNLOADS_DIR", settings["wx_downloads"]))
    token = (directory / ".wx_channel_mcp_token").read_text(encoding="utf-8").strip()
    if not token or len(token) > 4096 or "\n" in token or "\r" in token:
        raise ValueError("invalid token")
except (OSError, ValueError, KeyError):
    print("wx_channel MCP token unavailable; start wx_channel and check its downloads directory", file=sys.stderr)
    raise SystemExit(1)
print(json.dumps({"Authorization": "Bearer " + token}))
