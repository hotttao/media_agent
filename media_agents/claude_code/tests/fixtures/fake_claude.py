#!/usr/bin/env python3
"""Protocol fixture used by Docker/media-cli E2E; never selected in production."""

from __future__ import annotations

import json
import os
import sys
import time
import uuid
from pathlib import Path


def argument(name: str) -> str | None:
    if name not in sys.argv:
        return None
    index = sys.argv.index(name)
    return sys.argv[index + 1] if index + 1 < len(sys.argv) else None


session_id = argument("--resume") or argument("--session-id") or str(uuid.uuid4())
message = sys.argv[-1]
if message == "wait-for-interrupt":
    time.sleep(30)
    raise SystemExit(0)
if message.startswith("slow-success"):
    time.sleep(0.5)
if message == "fail-turn":
    print(
        json.dumps(
            {
                "type": "result",
                "subtype": "error_during_execution",
                "is_error": True,
                "result": "fixture failure",
            }
        ),
        flush=True,
    )
    raise SystemExit(0)

answer = f"Claude fixture: {message}"
payload = {
    "type": "assistant",
    "session_id": session_id,
    "message": {
        "id": str(uuid.uuid4()),
        "content": [{"type": "text", "text": answer}],
    },
}
print(json.dumps(payload, separators=(",", ":")), flush=True)
print(json.dumps({"type": "result", "subtype": "success"}), flush=True)

transcript = Path(os.environ["HOME"]) / ".claude" / "projects" / "fixture" / f"{session_id}.jsonl"
transcript.parent.mkdir(parents=True, exist_ok=True)
transcript.write_text(json.dumps(payload) + "\n", encoding="utf-8")
