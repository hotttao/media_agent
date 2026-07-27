"""Usage pull route."""

from __future__ import annotations

from collections.abc import Callable
from pathlib import Path

from fastapi import APIRouter

from adapter.biz.service.usage.usage import UsageRecord, UsageSweepResult, sweep_usage
from adapter.schemas.usage import (
    FileFingerprintModel,
    UsageRecordResponse,
    UsageRequest,
    UsageResponse,
)


def build_usage_router(
    *,
    home_dir: str | Path,
    fallback_model: Callable[[], str | None] | None = None,
) -> APIRouter:
    router = APIRouter(prefix="/usage")

    @router.post("", response_model=UsageResponse, response_model_by_alias=True)
    async def usage(body: UsageRequest | None = None) -> UsageResponse:
        request_body = body or UsageRequest()
        result = sweep_usage(
            home_dir=home_dir,
            cursors=request_body.cursor_dict(),
            max_files=request_body.max_files,
            fallback_model=fallback_model() if fallback_model else None,
        )
        return _usage_response(result)

    return router


def _usage_response(result: UsageSweepResult) -> UsageResponse:
    return UsageResponse(
        records=[_usage_record_response(record) for record in result.records],
        cursors={
            path: FileFingerprintModel.model_validate(fingerprint)
            for path, fingerprint in result.cursors.items()
        },
        has_more=result.has_more,
    )


def _usage_record_response(record: UsageRecord) -> UsageRecordResponse:
    return UsageRecordResponse(
        source=record.source,
        session_id=record.session_id,
        model=record.model,
        ts=record.ts,
        input_tokens=record.input_tokens,
        output_tokens=record.output_tokens,
        cache_read_tokens=record.cache_read_tokens,
        cache_creation_tokens=record.cache_creation_tokens,
        cache_creation_5m_tokens=record.cache_creation_5m_tokens,
        cache_creation_1h_tokens=record.cache_creation_1h_tokens,
        reasoning_tokens=record.reasoning_tokens,
        web_search_requests=record.web_search_requests,
        speed=record.speed,
        fields_incomplete=record.fields_incomplete,
        dedup_key=record.dedup_key,
    )