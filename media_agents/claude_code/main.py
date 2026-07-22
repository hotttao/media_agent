"""Claude Code agent FastAPI 入口。

对标 agents/claude-code/src/server.ts + index.ts：不用 adapter 的 build_app
（不走 ACP bridge），而是 build_base_app 拿骨架，include 自己的全套 routes。
"""

from __future__ import annotations

import logging
from pathlib import Path

import uvicorn
from fastapi import FastAPI

from adapter.config.config import load_adapter_config
from adapter.main import build_base_app
from claude_code.routes import build_claude_code_router

logger = logging.getLogger(__name__)

CONF_PATH = Path(__file__).resolve().parent / "conf" / "config.yaml"


def create_app() -> FastAPI:
    """工厂：构造 claude_code agent FastAPI app。

    不调 build_app（不走 ACP router），而是用 build_base_app 拿共享骨架
    （CORS / lifespan / logging / app.state.adapter_config），然后 include
    自己的全套路由。
    """
    cfg = load_adapter_config(CONF_PATH)
    app = build_base_app(cfg)
    app.include_router(build_claude_code_router())
    return app


app = create_app()


def main() -> None:
    import os

    port = int(os.environ.get("PORT", "3003"))
    uvicorn.run(app, host="0.0.0.0", port=port)


if __name__ == "__main__":
    main()
