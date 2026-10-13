"""Verified request epoch, propagated to execution tasks without HTTP dependencies."""

from contextvars import ContextVar

expected_execution_epoch: ContextVar[int | None] = ContextVar(
    "expected_execution_epoch", default=None
)
