#!/usr/bin/env python3
"""Stage new standalone Compose metadata without touching data or starting services."""
import argparse
import os
from pathlib import Path
import secrets
import shutil

parser = argparse.ArgumentParser(description=__doc__)
parser.parse_args()
source = Path(__file__).resolve().parent
destination = Path(os.environ.get("DEPLOY_ROOT", "/home/tao/deploy")) / "penpot"
data = os.environ.get("DATA_ROOT", "/data")
if not destination.is_absolute() or not Path(data).is_absolute():
    parser.error("deployment/data roots must be absolute")
if any((destination / n).exists() or (destination / n).is_symlink() for n in ("compose.yaml", "docker-compose.yaml", "docker-compose.yml", ".env")):
    parser.error("existing deployment found; stage elsewhere and review, preserve existing secrets and volumes")
destination.mkdir(parents=True, exist_ok=True)
shutil.copyfile(source / "compose.yaml", destination / "compose.yaml")
text = (source / ".env.example").read_text().replace("DATA_ROOT=/data", "DATA_ROOT=" + data)
for key in ("PENPOT_SECRET_KEY", "POSTGRES_PASSWORD"):
    text = text.replace(key + "=\n", key + "=" + secrets.token_urlsafe(64) + "\n")
# Create private before writing; do not briefly expose generated secrets through umask.
fd = os.open(destination / ".env", os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
with os.fdopen(fd, "w") as out:
    out.write(text)
print(f"Prepared {destination}; no service started or data directory created.")
