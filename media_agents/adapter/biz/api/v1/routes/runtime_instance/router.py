from adapter.biz.api.v1.deps import ExecuteTurnFlowDep, RuntimeSessionServiceDep
from adapter.biz.domain.runtime_session.errors import StaleExecutionOwner
from adapter.schemas.runtime_session.request import DrainInstanceRequest
from fastapi import APIRouter


def build_runtime_instance_router() -> APIRouter:
    router = APIRouter(prefix="/api/v1/runtime-instance", tags=["runtime-instance"])

    @router.post(":drain")
    async def begin(
        body: DrainInstanceRequest, service: RuntimeSessionServiceDep
    ) -> dict[str, object]:
        if service.scope is None or service.scope.incarnation_id != body.expected_incarnation:
            raise StaleExecutionOwner()
        admission = await service.begin_drain(body.operation_id)
        return {
            "operation_id": admission.operation_id,
            "version": admission.version,
            "incarnation_id": admission.scope.incarnation_id,
            "state": admission.state,
        }

    @router.get("/drain")
    async def state(
        service: RuntimeSessionServiceDep, flow: ExecuteTurnFlowDep
    ) -> dict[str, object]:
        admission, sessions = await service.drain_state()
        running = sum(session.active_turn_id is not None for session in sessions)
        return {
            "operation_id": admission.operation_id,
            "version": admission.version,
            "incarnation_id": admission.scope.incarnation_id,
            "state": admission.state,
            "running_count": running,
            "session_ids": [s.session_id for s in sessions],
            "quiesced": admission.state == "draining" and running == 0 and flow.local_idle(),
        }

    return router
