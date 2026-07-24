"""Usage pull route."""

from __future__ import annotations

from collections.abc import Callable
from pathlib import Path

from fastapi import APIRouter, Request
from fastapi.responses import JSONResponse

from adapter.biz.service.usage.usage import sweep_usage


def build_usage_router(
    *,
    home_dir: str | Path,
    fallback_model: Callable[[], str | None] | None = None,
) -> APIRouter:
    router = APIRouter(prefix="/usage")

    @router.post("")
    async def usage(request: Request) -> JSONResponse:
        try:
            body = await request.json()
        except ValueError:
            body = {}
        cursors = body.get("cursors") if isinstance(body, dict) else None
        max_files = body.get("maxFiles") if isinstance(body, dict) else None
        result = sweep_usage(
            home_dir=home_dir,
            cursors=cursors if isinstance(cursors, dict) else None,  # type: ignore[arg-type]
            max_files=max_files if isinstance(max_files, int) else None,
            fallback_model=fallback_model() if fallback_model else None,
        )
        return JSONResponse(result.to_dict())

    return router
