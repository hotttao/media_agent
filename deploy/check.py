#!/usr/bin/env python3
"""Validate deployment metadata in temp directories, without installs or a Docker daemon."""
import ast
import os
from pathlib import Path
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parent

def run(*args, env=None, expected=0):
    result = subprocess.run(args, env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
    if result.returncode != expected:
        raise SystemExit(f"Failed: {args[0:2]}\n{result.stderr}")
    return result

for path in ROOT.rglob("*.sh"):
    run("bash", "-n", str(path))
for path in ROOT.rglob("*.py"):
    ast.parse(path.read_text(), filename=str(path))

with tempfile.TemporaryDirectory(prefix="media-deploy-check-") as temp:
    env = dict(os.environ, DEPLOY_ROOT=temp, DATA_ROOT="/data")
    # Do not let host service environment or credentials affect parsing.
    clean = {k: v for k, v in env.items() if k in ("PATH", "HOME", "DEPLOY_ROOT", "DATA_ROOT", "LANG")}
    run("python3", str(ROOT / "penpot/prepare.py"), env=clean)
    directory = Path(temp) / "penpot"
    assert (directory / ".env").stat().st_mode & 0o777 == 0o600
    run("docker", "compose", "--project-directory", str(directory), "--env-file", str(directory / ".env"), "-f", str(directory / "compose.yaml"), "config", "--quiet", env=clean)
    run("python3", str(ROOT / "penpot/prepare.py"), env=clean, expected=2)
print("Deployment metadata valid: script syntax, Penpot Compose, private env mode, overwrite refusal; no service started.")
