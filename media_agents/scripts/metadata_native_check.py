"""Optional native CLI smoke: local configuration discovery, without model requests."""

from __future__ import annotations

import argparse
import asyncio
import json
import os
import shutil
import tempfile
from pathlib import Path
from typing import Any

from adapter.metadata_artifacts import MetadataArtifacts
from adapter.runtime_metadata_files import codex_home
from adapter.test_metadata_update import skill_zip, snapshot


async def codex_skill(binary: str, root: Path, bundle: Path, session: str) -> str:
    home = codex_home(bundle, session, root, root / "no-auth")
    environment = {
        **os.environ,
        "HOME": str(root / ".home"),
        "CODEX_HOME": str(home),
        "OPENAI_API_KEY": "sk-metadata-smoke-no-model-requests",
    }
    process = await asyncio.create_subprocess_exec(
        binary,
        "app-server",
        cwd=root,
        env=environment,
        stdin=asyncio.subprocess.PIPE,
        stdout=asyncio.subprocess.PIPE,
        stderr=asyncio.subprocess.DEVNULL,
    )

    async def request(identity: int, method: str, params: dict[str, object]) -> dict[str, Any]:
        assert process.stdin is not None and process.stdout is not None
        frame = {"id": identity, "method": method, "params": params}
        process.stdin.write((json.dumps(frame) + "\n").encode())
        await process.stdin.drain()
        async with asyncio.timeout(45):
            while True:
                line = await process.stdout.readline()
                if not line:
                    raise RuntimeError("native Codex process stopped")
                value: dict[str, Any] = json.loads(line)
                if value.get("id") == identity:
                    return value

    try:
        initialized = await request(
            1, "initialize", {"clientInfo": {"name": "metadata-smoke", "version": "1.0"}}
        )
        assert initialized["result"]["codexHome"] == str(home)
        response = await request(2, "skills/list", {"cwds": [str(root)], "forceReload": True})
        skills = [
            skill
            for row in response["result"]["data"]
            for skill in row["skills"]
            if skill["name"] == "skill"
        ]
        assert len(skills) == 1 and skills[0]["enabled"]
        path = Path(skills[0]["path"])
        assert path.is_relative_to(home)
        return path.read_text().splitlines()[-1]
    finally:
        if process.returncode is None:
            process.terminate()
        await process.wait()


async def check(codex: str, claude: str) -> None:
    with tempfile.TemporaryDirectory(prefix="metadata-native-") as name:
        root = Path(name)
        (root / ".home").mkdir()
        store = MetadataArtifacts(root, "config", "0", "boot")
        bundles = []
        for label, revision in [("old", 2), ("new", 3)]:
            content = skill_zip(label)
            selected = snapshot(label, revision, label, content)
            bundles.append(store.prepare(selected, {selected.resources[0].checksum: content}))
        values = [
            await codex_skill(codex, root, bundles[0], "old-session"),
            await codex_skill(codex, root, bundles[1], "new-session"),
            await codex_skill(codex, root, bundles[0], "old-session"),
        ]
        assert values == ["old", "new", "old"]
        print("PASS native Codex skills/list: old, new, restored old Session")
        content = skill_zip("claude")
        selected = snapshot("claude", 4, "Claude", content, agent_type="claude-code")
        bundle = store.prepare(selected, {selected.resources[0].checksum: content})
        process = await asyncio.create_subprocess_exec(
            claude,
            "plugin",
            "validate",
            "--strict",
            str(bundle),
            env={**os.environ, "HOME": str(root / ".home")},
            stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.PIPE,
        )
        try:
            async with asyncio.timeout(45):
                output, error = await process.communicate()
        finally:
            if process.returncode is None:
                process.terminate()
            await process.wait()
        assert process.returncode == 0, (output + error).decode()
        print("PASS native Claude plugin validation (strict)")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--codex-bin", default=shutil.which("codex"))
    parser.add_argument("--claude-bin", default=shutil.which("claude"))
    args = parser.parse_args()
    if not args.codex_bin or not args.claude_bin:
        parser.error("installed Codex and Claude CLIs are required")
    asyncio.run(check(args.codex_bin, args.claude_bin))
