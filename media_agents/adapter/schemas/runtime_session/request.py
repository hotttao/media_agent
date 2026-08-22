from pydantic import BaseModel, ConfigDict, Field


class RegisterRuntimeSessionRequest(BaseModel):
    model_config = ConfigDict(extra="forbid")

    session_id: str = Field(min_length=1, max_length=64)
    context_version: int = Field(ge=1)
    context_ref: str = Field(min_length=1, max_length=512)


class SubmitTurnRequest(BaseModel):
    model_config = ConfigDict(extra="forbid")

    idempotency_key: str = Field(min_length=1, max_length=128)
    context_version: int = Field(ge=1)
    input: dict[str, object]
    api_key: str | None = Field(default=None, max_length=4096, repr=False)
    session_token: str | None = Field(default=None, max_length=4096, repr=False)
