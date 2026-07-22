"""Goose agent FastAPI 入口。

对标 agents/goose/src/index.ts + server.ts：load_adapter_config + build_app
+ bridge factory（注入 on_ext_notification 回调累计 token）+ uvicorn.run。
"""

from __future__ import annotations

import asyncio
import logging
from pathlib import Path

import uvicorn
from fastapi import FastAPI

from adapter.config.config import load_adapter_config
from adapter.main import build_app
from goose.bridge import GooseAcpBridge
from goose.config import GooseHooks

logger = logging.getLogger(__name__)

CONF_PATH = Path(__file__).resolve().parent / "conf" / "config.yaml"


def create_app() -> FastAPI:
    """工厂：构造 goose agent FastAPI app。"""
    cfg = load_adapter_config(CONF_PATH)
    hooks = GooseHooks(
        cp_url=cfg.cp_url,
        workspace_id=cfg.workspace_id,
        goose_path_root=getattr(cfg, "goose_path_root", None),
    )

    def bridge_factory(session_id: str) -> GooseAcpBridge:
        bridge = GooseAcpBridge(
            cwd=cfg.workspace_dir,
            goose_path_root=getattr(cfg, "goose_path_root", None),
            session_id=session_id,
            on_ext_notification=hooks.track_ext_notification,
        )
        loop = asyncio.get_event_loop()
        loop.create_task(bridge.start())
        return bridge

    app = build_app(cfg, hooks=hooks, bridge_factory=bridge_factory)
    # goose 特有路由（skills / usage）TODO
    return app


app = create_app()


def main() -> None:
    import os

    port = int(os.environ.get("PORT", "3002"))
    uvicorn.run(app, host="0.0.0.0", port=port)


if __name__ == "__main__":
    main()
