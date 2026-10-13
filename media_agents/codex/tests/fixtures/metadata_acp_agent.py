#!/usr/bin/env python3
"""Records native config selection, then delegates to the real ACP protocol fixture."""

from __future__ import annotations

import json
import os
import runpy
import tomllib
from pathlib import Path

home = Path(os.environ["CODEX_HOME"])
with Path(os.environ["METADATA_CAPTURE"]).open("a") as capture:
    capture.write(
        json.dumps(
            {
                "platform": (home / "AGENTS.md").read_text(),
                "configured": tomllib.loads((home / "config.toml").read_text())[
                    "developer_instructions"
                ],
                "skill": (home / "skills/skill/SKILL.md").read_text(),
                "cwd": str(Path.cwd()),
            }
        )
        + "\n"
    )
runpy.run_path(str(Path(__file__).with_name("fake_acp_agent.py")), run_name="__main__")
