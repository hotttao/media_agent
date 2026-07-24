"""Claude Code agent FastAPI entrypoint."""

from __future__ import annotations

import logging
from pathlib import Path

import uvicorn
from fastapi import FastAPI

from adapter.main import build_base_app
from claude_code.config import load_claude_code_config
from claude_code.routes import build_claude_code_router

logger = logging.getLogger(__name__)

CONF_PATH = Path(__file__).resolve().parent / "conf" / "config.yaml"


def create_app() -> FastAPI:
    cfg = load_claude_code_config(CONF_PATH)
    app = build_base_app(cfg)
    app.include_router(build_claude_code_router())
    return app


app = create_app()


def main() -> None:
    uvicorn.run(app, host="0.0.0.0", port=cfg_port())


def cfg_port() -> int:
    return load_claude_code_config(CONF_PATH).agent.port


if __name__ == "__main__":
    main()
