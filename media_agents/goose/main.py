"""Goose agent FastAPI entrypoint."""

from __future__ import annotations

import asyncio
import logging
from pathlib import Path

import uvicorn
from fastapi import FastAPI

from adapter.agent_app import build_app
from adapter.clients.cp import CpClient
from goose.bridge import GooseAcpBridge
from goose.config import GooseHooks, load_goose_config

logger = logging.getLogger(__name__)

CONF_PATH = Path(__file__).resolve().parent / "conf" / "config.yaml"


def create_app() -> FastAPI:
    cfg = load_goose_config(CONF_PATH)
    hooks = GooseHooks(
        cp_client=CpClient(base_url=cfg.cp.url, workspace_id=cfg.cp.workspace_id),
        goose_path_root=cfg.goose.path_root,
        workspace_dir=cfg.workspace.dir,
        goose_config_dir=cfg.goose.config_dir,
    )

    def bridge_factory(session_id: str) -> GooseAcpBridge:
        bridge = GooseAcpBridge(
            cwd=cfg.workspace.dir,
            goose_path_root=cfg.goose.path_root,
            session_id=session_id,
            on_ext_notification=hooks.track_ext_notification,
        )
        asyncio.get_event_loop().create_task(bridge.start())
        return bridge

    app = build_app(cfg, hooks=hooks, bridge_factory=bridge_factory)
    return app


app = create_app()


def main() -> None:
    uvicorn.run(app, host="0.0.0.0", port=cfg_port())


def cfg_port() -> int:
    return load_goose_config(CONF_PATH).agent.port


if __name__ == "__main__":
    main()
