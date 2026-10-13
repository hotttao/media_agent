from pydantic import BaseModel, ConfigDict, Field


class RegisterRuntimeSessionRequest(BaseModel):
    model_config = ConfigDict(extra="forbid")

    session_id: str = Field(min_length=1, max_length=64)
    context_version: int = Field(ge=1)
    context_ref: str = Field(min_length=1, max_length=512)
    workspace_id: str = Field(default="", max_length=128)
    agent_config_id: str = Field(default="", max_length=128)
    initial_epoch: int = Field(default=1, ge=1)
    transfer_id: str = Field(default="", max_length=128)


class SubmitTurnRequest(BaseModel):
    model_config = ConfigDict(extra="forbid")

    idempotency_key: str = Field(min_length=1, max_length=128)
    context_version: int = Field(ge=1)
    input: dict[str, object]
    api_key: str | None = Field(default=None, max_length=4096, repr=False)
    session_token: str | None = Field(default=None, max_length=4096, repr=False)


class ResolveAgentRequestRequest(BaseModel):
    model_config = ConfigDict(extra="forbid")
    response: dict[str, object]


class TransferSessionRequest(BaseModel):
    model_config = ConfigDict(extra="forbid")
    transfer_id: str = Field(min_length=1, max_length=128)
    source_replica_id: str = Field(min_length=1, max_length=128)
    source_incarnation_id: str = Field(min_length=1, max_length=128)
    expected_epoch: int = Field(ge=1)
    termination_confirmed: bool = False


class DrainInstanceRequest(BaseModel):
    model_config = ConfigDict(extra="forbid")
    operation_id: str = Field(min_length=1, max_length=128)
    expected_incarnation: str = Field(min_length=1, max_length=128)


class ResumeSessionRequest(BaseModel):
    model_config = ConfigDict(extra="forbid")
    operation_id: str = Field(min_length=1, max_length=128)


class QuiesceSessionRequest(BaseModel):
    model_config = ConfigDict(extra="forbid")
    transfer_id: str = Field(min_length=1, max_length=128)
