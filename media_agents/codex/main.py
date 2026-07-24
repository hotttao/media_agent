"""Codex agent FastAPI entrypoint."""

from __future__ import annotations

from collections.abc import AsyncIterator
from contextlib import asynccontextmanager
from pathlib import Path

import uvicorn
from fastapi import FastAPI

from adapter.agent_app import build_app
from adapter.agent_environment import read_runtime_config
from adapter.agent_lifespan import agent_lifespan
from adapter.biz.api.v1.routes.usage.router import build_usage_router
from adapter.biz.shared.managed_process import ManagedProcessSpec
from adapter.clients.cp import CpClient
from codex.bridge import CodexAcpBridge
from codex.config import CodexHooks, load_codex_config

CONF_PATH = Path(__file__).resolve().parent / "conf" / "config.yaml"
AFS_MOUNT_BASE = "/mnt/afs"


def create_app() -> FastAPI:
    cfg = load_codex_config(CONF_PATH)
    hooks = CodexHooks(
        cp_client=CpClient(base_url=cfg.cp.url, workspace_id=cfg.cp.workspace_id),
        workspace_dir=cfg.workspace.dir,
        codex_config_dir=cfg.codex.config_dir,
    )

    def bridge_factory(session_id: str) -> CodexAcpBridge:
        return CodexAcpBridge(
            program=cfg.codex.program,
            cwd=cfg.workspace.dir,
            initial_agent_mode=cfg.codex.initial_agent_mode,
            session_id=session_id,
        )

    @asynccontextmanager
    async def codex_lifespan(app: FastAPI) -> AsyncIterator[None]:
        async with agent_lifespan(app, hooks=hooks, processes=_codex_processes(cfg.workspace.dir)):
            yield

    app = build_app(
        cfg,
        hooks=hooks,
        bridge_factory=bridge_factory,
        extra_lifespan=codex_lifespan,
    )
    app.include_router(
        build_usage_router(
            home_dir=Path(cfg.codex.config_dir).expanduser().parent,
            fallback_model=lambda: (read_runtime_config(cfg.workspace.dir) or {}).get("model"),
        )
    )
    return app


app = create_app()


def main() -> None:
    uvicorn.run(app, host="0.0.0.0", port=cfg_port())


def cfg_port() -> int:
    return load_codex_config(CONF_PATH).agent.port


def _codex_processes(workspace_dir: str) -> list[ManagedProcessSpec]:
    return [
        ManagedProcessSpec(
            label="ttyd",
            program="ttyd",
            args=(
                "-W",
                "-a",
                "-p",
                "7681",
                "tmux",
                "-f",
                "/etc/tmux.conf",
                "new-session",
                "-A",
                "-s",
            ),
            cwd=workspace_dir,
        ),
        ManagedProcessSpec(
            label="dufs-workspace",
            program="dufs",
            args=(workspace_dir, "-A", "--allow-symlink", "--port", "8000"),
        ),
        ManagedProcessSpec(
            label="dufs-afs",
            program="dufs",
            args=(AFS_MOUNT_BASE, "-A", "--allow-symlink", "--port", "8001"),
        ),
    ]


if __name__ == "__main__":
    main()
