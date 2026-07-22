"""Codex agent FastAPI 入口。

对标 agents/codex/src/index.ts + server.ts：load_adapter_config + build_app
+ include codex 特有路由（skills / usage TODO 留接口）+ uvicorn.run。
"""

from __future__ import annotations

import logging
from pathlib import Path

import uvicorn
from fastapi import FastAPI

from adapter.config.config import load_adapter_config
from adapter.main import build_app
from codex.bridge import CodexAcpBridge
from codex.config import CodexHooks

logger = logging.getLogger(__name__)

CONF_PATH = Path(__file__).resolve().parent / "conf" / "config.yaml"


def create_app() -> FastAPI:
    """工厂：构造 codex agent FastAPI app。"""
    cfg = load_adapter_config(CONF_PATH)
    hooks = CodexHooks(cp_url=cfg.cp_url, workspace_id=cfg.workspace_id)

    def bridge_factory(session_id: str) -> CodexAcpBridge:
        bridge = CodexAcpBridge(cwd=cfg.workspace_dir)
        # bridge.start 是 async，但 BridgeFactory Protocol 是同步签名 -> 用 fire-and-forget
        # 在 event loop 里调度。完整实现见 _ensure_started。
        import asyncio

        loop = asyncio.get_event_loop()
        loop.create_task(bridge.start())
        return bridge

    app = build_app(cfg, hooks=hooks, bridge_factory=bridge_factory)

    # codex 特有路由（skills 管理 / token-usage pull）TODO：
    # from .routes import build_codex_router
    # app.include_router(build_codex_router(hooks))
    return app


app = create_app()


def main() -> None:
    import os

    port = int(os.environ.get("PORT", "3001"))
    uvicorn.run(app, host="0.0.0.0", port=port)


if __name__ == "__main__":
    main()
