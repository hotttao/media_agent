"""Managed Runtime APIs accept signed Gateway requests; probes remain native."""

from __future__ import annotations

from starlette.requests import Request
from starlette.responses import JSONResponse
from starlette.types import ASGIApp, Message, Receive, Scope, Send

from adapter.biz.domain.runtime_session.ownership import InstanceScope
from adapter.biz.service.runtime_session.fence import expected_execution_epoch
from adapter.infra.runtime_auth import RuntimeAssertionRejected, RuntimeAssertionVerifier


class RuntimeAuthMiddleware:
    def __init__(self, app: ASGIApp) -> None:
        self.app = app

    async def __call__(self, scope: Scope, receive: Receive, send: Send) -> None:
        if scope["type"] != "http":
            await self.app(scope, receive, send)
            return
        application = scope["app"]
        instance: InstanceScope | None = getattr(application.state, "instance_scope", None)
        if instance is None:
            await self.app(scope, receive, send)
            return
        path = scope["path"]
        protected = path.startswith(("/api/v1/runtime-sessions", "/api/v1/runtime-instance"))
        if not protected:
            if path.startswith("/acp"):
                await JSONResponse({"detail": "managed_runtime_required"}, 403)(
                    scope, receive, send
                )
            else:
                await self.app(scope, receive, send)
            return
        request = Request(scope, receive)
        chunks: list[bytes] = []
        size = 0
        async for chunk in request.stream():
            size += len(chunk)
            if size > 2 * 1024 * 1024:
                await JSONResponse({"detail": "runtime_request_too_large"}, 413)(
                    scope, receive, send
                )
                return
            chunks.append(chunk)
        body = b"".join(chunks)
        query = scope.get("query_string", b"").decode("ascii")
        verifier: RuntimeAssertionVerifier = application.state.runtime_assertion_verifier
        try:
            epoch = verifier.verify(
                request.headers.get("X-Runtime-Assertion", ""),
                instance,
                scope["method"],
                path + ("?" + query if query else ""),
                body,
            )
        except RuntimeAssertionRejected:
            await JSONResponse({"detail": "runtime_assertion_rejected"}, 401)(scope, receive, send)
            return
        control = path.startswith("/api/v1/runtime-instance") or path.endswith(":transfer")
        if epoch == 0 and scope["method"] != "GET" and not control:
            await JSONResponse({"detail": "execution_epoch_required"}, 403)(scope, receive, send)
            return
        token = expected_execution_epoch.set(epoch or None)
        delivered = False

        async def replay() -> Message:
            nonlocal delivered
            if not delivered:
                delivered = True
                return {"type": "http.request", "body": body, "more_body": False}
            return await receive()

        try:
            await self.app(scope, replay, send)
        finally:
            expected_execution_epoch.reset(token)
