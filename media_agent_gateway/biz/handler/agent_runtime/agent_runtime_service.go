package agent_runtime

import (
	"context"
	"errors"
	"github.com/cloudwego/hertz/pkg/app"
	dto "media_agent/hertz_gen/model/media_agent_gateway/agent_runtime"
	instance "media_agent/media_agent_gateway/biz/domain/agent_instance"
	session "media_agent/media_agent_gateway/biz/domain/agent_session"
	"media_agent/media_agent_gateway/biz/middleware"
	"media_agent/media_agent_gateway/biz/shared/client"
	"media_agent/media_agent_gateway/biz/workflow"
	"strings"
)

var registration *workflow.InstanceRegistration
var execution *workflow.ExecutionContext

func Configure(reg *workflow.InstanceRegistration, exec *workflow.ExecutionContext) {
	registration, execution = reg, exec
}
func bearer(c *app.RequestContext) string {
	value := string(c.GetHeader("Authorization"))
	if !strings.HasPrefix(value, "Bearer ") {
		return ""
	}
	return strings.TrimPrefix(value, "Bearer ")
}
func locator(req *dto.InstanceLocator) instance.ResourceLocator {
	return instance.ResourceLocator{EnvironmentID: req.EnvironmentId, WorkspaceID: req.WorkspaceId, AgentConfigID: req.AgentConfigId, ReplicaID: req.ReplicaId, Namespace: req.Namespace, ResourceName: req.ResourceName, ResourceUID: req.ResourceUid}
}
func ready(c *app.RequestContext) bool {
	if registration == nil || execution == nil {
		fail(c, workflow.ErrMetadataUnavailable)
		return false
	}
	return true
}
func ResolveInstanceIdentity(ctx context.Context, c *app.RequestContext) {
	if !ready(c) {
		return
	}
	var req dto.InstanceLocator
	if err := c.BindAndValidate(&req); err != nil {
		c.JSON(400, map[string]string{"error": "invalid_runtime_request"})
		return
	}
	value, err := registration.Resolve(ctx, bearer(c), string(c.GetHeader("X-Runtime-Bootstrap-Proof")), locator(&req))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(200, &dto.InstanceIdentity{EnvironmentId: value.Facts.Locator.EnvironmentID, ResourceUid: value.Facts.Locator.ResourceUID, ResourceGeneration: value.Facts.Generation, RegistrationVersion: value.Version, AgentType: value.Facts.AgentType})
}
func RegisterInstance(ctx context.Context, c *app.RequestContext) {
	if !ready(c) {
		return
	}
	var req dto.InstanceLocator
	if err := c.BindAndValidate(&req); err != nil {
		c.JSON(400, map[string]string{"error": "invalid_runtime_request"})
		return
	}
	value, err := registration.Register(ctx, bearer(c), string(c.GetHeader("X-Runtime-Bootstrap-Proof")), workflow.RegistrationCommand{Locator: locator(&req), IncarnationID: req.IncarnationId, Generation: req.ResourceGeneration, ExpectedVersion: req.ExpectedVersion})
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(200, &dto.InstanceRegistration{InstanceCredential: value.Credential, RegistrationVersion: value.Instance.Version, Admission: value.Instance.Admission, IncarnationId: value.Instance.IncarnationID, DrainOperationId: value.Instance.DrainOperationID})
}
func GetExecutionContext(ctx context.Context, c *app.RequestContext) {
	if !ready(c) {
		return
	}
	var req dto.ExecutionContextRequest
	if err := c.BindAndValidate(&req); err != nil {
		c.JSON(400, map[string]string{"error": "invalid_runtime_request"})
		return
	}
	value, err := execution.Get(ctx, bearer(c), string(c.GetHeader("X-Agent-Instance-Credential")), workflow.ExecutionContextCommand{SessionID: req.SessionId, WorkspaceID: req.WorkspaceId, AgentConfigID: req.AgentConfigId, ReplicaID: req.ReplicaId, IncarnationID: req.IncarnationId, TurnID: req.TurnId, ExecutionEpoch: req.ExecutionEpoch})
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(200, &dto.ExecutionContext{ApiKey: value.APIKey, CodexAuthJson: value.CodexAuthJSON, ModelConnectionId: value.ModelConnectionID, ModelConnectionVersion: value.ModelConnectionVersion, SessionToken: value.SessionToken, ExecutionEpoch: value.ExecutionEpoch, ContextVersion: value.ContextVersion})
}
func ResumeSession(ctx context.Context, c *app.RequestContext) {
	if !ready(c) {
		return
	}
	var req dto.SessionResumeRequest
	if err := c.BindAndValidate(&req); err != nil {
		c.JSON(400, map[string]string{"error": "invalid_runtime_request"})
		return
	}
	value, err := execution.Resume(ctx, req.SessionId, middleware.UserID(ctx), req.OperationId, req.ExpectedEpoch)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(200, &dto.SessionResumeResponse{SessionId: value.SessionID, Status: value.Status, ExecutionEpoch: value.ExecutionEpoch})
}
func fail(c *app.RequestContext, err error) {
	status, code := 503, "runtime_unavailable"
	switch {
	case errors.Is(err, client.ErrModelAuthenticationRequired):
		status, code = 409, "model_authentication_required"
	case errors.Is(err, client.ErrAgentCredentialRejected):
		status, code = 401, "agent_credential_rejected"
	case errors.Is(err, workflow.ErrMetadataForbidden), errors.Is(err, session.ErrForbidden), errors.Is(err, client.ErrNativeIdentity):
		status, code = 403, "runtime_forbidden"
	case errors.Is(err, instance.ErrIncarnationConflict):
		status, code = 403, "native_generation_conflict"
	case errors.Is(err, instance.ErrInvalid):
		status, code = 400, "invalid_runtime_request"
	case errors.Is(err, instance.ErrConflict), errors.Is(err, session.ErrBindingConflict):
		status, code = 409, "runtime_conflict"
	case errors.Is(err, instance.ErrNotFound), errors.Is(err, session.ErrNotFound):
		status, code = 404, "runtime_not_found"
	}
	c.JSON(status, map[string]string{"error": code})
}
