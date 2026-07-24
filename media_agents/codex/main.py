"""Codex agent FastAPI entrypoint."""

from __future__ import annotations

import asyncio
import logging
from pathlib import Path

import uvicorn
from fastapi import FastAPI

from adapter.clients.cp import CpClient
from adapter.main import build_app
from codex.bridge import CodexAcpBridge
from codex.config import CodexHooks, load_codex_config

logger = logging.getLogger(__name__)

CONF_PATH = Path(__file__).resolve().parent / "conf" / "config.yaml"


def create_app() -> FastAPI:
    cfg = load_codex_config(CONF_PATH)
    hooks = CodexHooks(
        cp_client=CpClient(base_url=cfg.cp.url, workspace_id=cfg.cp.workspace_id),
        workspace_dir=cfg.workspace.dir,
        codex_config_dir=cfg.codex.config_dir,
    )

    def bridge_factory(session_id: str) -> CodexAcpBridge:
        bridge = CodexAcpBridge(cwd=cfg.workspace.dir)
        asyncio.get_event_loop().create_task(bridge.start())
        return bridge

    app = build_app(cfg, hooks=hooks, bridge_factory=bridge_factory)
    return app


app = create_app()


def main() -> None:
    uvicorn.run(app, host="0.0.0.0", port=cfg_port())


def cfg_port() -> int:
    return load_codex_config(CONF_PATH).agent.port


if __name__ == "__main__":
    main()
