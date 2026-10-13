"""Goose agent FastAPI entrypoint."""

from __future__ import annotations

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
        workspace_dir=cfg.workspace.dir,
        goose_config_dir=cfg.goose.config_dir,
    )

    def bridge_factory(session_id: str, api_key: str | None = None) -> GooseAcpBridge:
        bridge = GooseAcpBridge(
            program=cfg.goose.program,
            args=cfg.goose.args or None,
            api_key=api_key,
            cwd=cfg.workspace.dir,
            goose_path_root=cfg.goose.path_root,
            session_id=session_id,
            on_ext_notification=hooks.track_ext_notification,
        )
        return bridge

    def configure(bridge: object, bundle: Path, session_id: str) -> None:
        if not isinstance(bridge, GooseAcpBridge):
            raise TypeError("unexpected Goose bridge")
        bridge.configure_metadata(bundle, session_id)

    app = build_app(
        cfg, hooks=hooks, bridge_factory=bridge_factory, metadata_bridge_setup=configure
    )
    return app


app = create_app()


def main() -> None:
    uvicorn.run(app, host="0.0.0.0", port=cfg_port())


def cfg_port() -> int:
    return load_goose_config(CONF_PATH).agent.port


if __name__ == "__main__":
    main()
