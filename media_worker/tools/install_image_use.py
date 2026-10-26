#!/usr/bin/env python3
"""Install only the pinned upstream image-use script; never browser extensions."""
import argparse
import hashlib
import os
from pathlib import Path
import tempfile
import urllib.request
from image_use_bridge import UPSTREAM_COMMIT, UPSTREAM_SHA256


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--destination", type=Path, default=Path.home() / ".local/share/media-worker/image-use" / UPSTREAM_COMMIT / "image-use")
    parser.add_argument("--source", type=Path, help="Optional previously downloaded source; the same hash is required")
    args = parser.parse_args()
    if args.source:
        data = args.source.read_bytes()
    else:
        url = f"https://raw.githubusercontent.com/leeguooooo/image-use/{UPSTREAM_COMMIT}/image-use"
        with urllib.request.urlopen(url, timeout=30) as response:
            data = response.read((2 << 20) + 1)
    if hashlib.sha256(data).hexdigest() != UPSTREAM_SHA256:
        raise SystemExit("image-use source checksum mismatch; nothing installed")
    destination = args.destination.expanduser().absolute()
    if destination.is_symlink():
        raise SystemExit("destination must not be a symlink")
    destination.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
    fd, temp = tempfile.mkstemp(prefix=".image-use-", dir=destination.parent)
    try:
        with os.fdopen(fd, "wb") as out:
            out.write(data)
            out.flush()
            os.fsync(out.fileno())
            os.fchmod(out.fileno(), 0o755)
        os.replace(temp, destination)
        print(destination)
    finally:
        if os.path.exists(temp): os.unlink(temp)


if __name__ == "__main__":
    main()
