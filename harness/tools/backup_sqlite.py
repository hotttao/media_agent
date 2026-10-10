"""Online backup of explicitly selected repository SQLite databases."""
import argparse
import hashlib
import os
from pathlib import Path
import sqlite3

parser = argparse.ArgumentParser()
parser.add_argument("directory", type=Path)
parser.add_argument("--only-talos", action="store_true")
args = parser.parse_args()
if not args.directory.is_absolute() or not args.directory.is_dir():
    raise SystemExit("Existing absolute private backup directory required")
sources = [("talos", "/data/talos/talos.db")]
if not args.only_talos:
    sources.insert(0, ("conductor", "/data/conductor/conductor.db"))
for name, source in sources:
    target = args.directory / f"{name}.db"
    fd = os.open(target, os.O_CREAT | os.O_EXCL | os.O_WRONLY, 0o600)
    os.close(fd)
    with sqlite3.connect(f"file:{source}?mode=ro", uri=True, timeout=30) as origin:
        with sqlite3.connect(target) as destination:
            origin.backup(destination)
            if destination.execute("PRAGMA integrity_check").fetchall() != [("ok",)]:
                raise SystemExit(f"{name} backup integrity check failed")
    with target.open("rb") as input_file:
        digest = hashlib.file_digest(input_file, "sha256").hexdigest()
    manifest = args.directory / f"{name.upper()}_SHA256SUMS"
    fd = os.open(manifest, os.O_CREAT | os.O_EXCL | os.O_WRONLY, 0o600)
    with os.fdopen(fd, "w") as output:
        output.write(f"{digest}  {name}.db\n")
    print(f"SQLite backup verified: {name}")
