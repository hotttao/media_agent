"""Runtime Session domain errors."""


class RuntimeSessionError(Exception):
    """Base error exposed by the Runtime Session domain."""


class InvalidRuntimeSession(RuntimeSessionError):
    """Registration data violates an aggregate invariant."""


class InvalidTurn(RuntimeSessionError):
    """Submitted Turn violates an aggregate invariant."""


class RuntimeSessionNotFound(RuntimeSessionError):
    def __init__(self, session_id: str) -> None:
        super().__init__(f"runtime session not found: {session_id}")


class RuntimeSessionAlreadyExists(RuntimeSessionError):
    def __init__(self, session_id: str) -> None:
        super().__init__(f"runtime session already exists: {session_id}")


class ContextVersionConflict(RuntimeSessionError):
    def __init__(self, session_id: str, existing: int, requested: int) -> None:
        super().__init__(
            f"runtime session {session_id} already uses context version "
            f"{existing}, requested {requested}"
        )


class StaleExecutionOwner(RuntimeSessionError):
    def __init__(self) -> None:
        super().__init__("stale_execution_owner")


class InstanceDraining(RuntimeSessionError):
    def __init__(self) -> None:
        super().__init__("instance_draining")


class TransferConflict(RuntimeSessionError):
    def __init__(self) -> None:
        super().__init__("session_transfer_conflict")


class ExecutionCredentialsDeferred(RuntimeSessionError):
    def __init__(self) -> None:
        super().__init__("execution_credentials_deferred")


class ExecutionCredentialsRejected(RuntimeSessionError):
    def __init__(self) -> None:
        super().__init__("execution_credentials_rejected")
