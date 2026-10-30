#!/usr/bin/env python3
"""Compatibility entry; deployment installer lives in deploy/image-use."""
from pathlib import Path
import runpy

if __name__ == "__main__":
    runpy.run_path(str(Path(__file__).resolve().parents[2] / "deploy/image-use/install.py"), run_name="__main__")
