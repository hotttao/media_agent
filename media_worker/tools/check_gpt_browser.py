#!/usr/bin/env python3
"""Check the current Chrome relay without reading Codex credentials or sending a prompt."""
import argparse
import contextlib
import json
import os
from pathlib import Path
import image_use_bridge as bridge


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--image-use", type=Path, required=True)
    parser.add_argument("--open", action="store_true", help="Also open ChatGPT and verify its composer; never sends")
    args = parser.parse_args()
    browser, relay, composer, failure = None, False, None, None
    with open(os.devnull, "w") as null, contextlib.redirect_stdout(null), contextlib.redirect_stderr(null):
        try:
            module = bridge.load_upstream(args.image_use)
            browser = module._find_agent_browser()
            relay = bool(browser and module._relay_connected(browser))
            if relay:
                options = argparse.Namespace(upstream=str(args.image_use), prepare=args.open)
                composer = bridge.preflight(options)["ready"]
        except Exception:
            failure = "BROWSER_PREFLIGHT_UNCONFIRMED"
    print(json.dumps({"backend": "web", "chrome_use_installed": bool(browser), "current_chrome_connected": relay, "chatgpt_composer_ready": composer, "failure": failure}))
    return 0 if relay and composer and failure is None else 1


if __name__ == "__main__":
    raise SystemExit(main())
