package workspace

import (
	"context"
	"errors"
	"github.com/cloudwego/hertz/pkg/app"
	dto "media_agent/hertz_gen/model/media_control/workspace"
	"media_agent/hertz_infra/authn/appcredential"
	connection "media_agent/media_control/biz/domain/model_connection"
	domain "media_agent/media_control/biz/domain/workspace"
	service "media_agent/media_control/biz/service/workspace"
	flow "media_agent/media_control/biz/workflow/application_agent"
)

var applicationAgents *flow.Flow
var applicationAuthority *appcredential.Client

func ConfigureApplications(f *flow.Flow, a *appcredential.Client) {
	applicationAgents = f
	applicationAuthority = a
}
func applicationPrincipal(ctx context.Context, c *app.RequestContext, scope string) (appcredential.Principal, bool) {
	c.Header("Cache-Control", "no-store")
	if applicationAgents == nil {
		c.JSON(503, map[string]string{"error": "application_service_unavailable"})
		return appcredential.Principal{}, false
	}
	p, e := applicationAuthority.Verify(ctx, appcredential.Bearer(string(c.GetHeader("Authorization"))), scope)
	if e != nil {
		status := 503
		if errors.Is(e, appcredential.ErrDenied) {
			status = 401
		}
		c.JSON(status, map[string]string{"error": "application_access_denied"})
		return p, false
	}
	return p, true
}
func applicationError(c *app.RequestContext, e error) {
	status := 503
	switch {
	case errors.Is(e, flow.ErrInvalid), errors.Is(e, connection.ErrInvalid), errors.Is(e, connection.ErrRevoked):
		status = 400
	case errors.Is(e, domain.ErrNotFound), errors.Is(e, connection.ErrNotFound):
		status = 404
	case errors.Is(e, domain.ErrConflict):
		status = 409
	case errors.Is(e, service.ErrForbidden):
		status = 403
	}
	c.JSON(status, map[string]string{"error": "application_agent_request_failed"})
}
func EnsureApplicationAgent(ctx context.Context, c *app.RequestContext) {
	p, ok := applicationPrincipal(ctx, c, "workspace.ensure")
	if !ok {
		return
	}
	var req dto.EnsureApplicationAgentRequest
	if c.BindAndValidate(&req) != nil {
		c.JSON(400, map[string]string{"error": "invalid_application_agent"})
		return
	}
	v, created, e := applicationAgents.Ensure(ctx, p, req.Name, service.CreateCommand{Name: req.DisplayName, AgentType: req.AgentType, APIKey: req.ApiKey, ModelConnectionID: req.ModelConnectionId, ModelProtocol: req.ModelProtocol, ModelName: req.ModelName, ModelBaseURL: req.ModelBaseUrl})
	if e != nil {
		applicationError(c, e)
		return
	}
	status := 200
	if created {
		status = 201
	}
	c.JSON(status, toResponse(v))
}
func GetApplicationAgent(ctx context.Context, c *app.RequestContext) {
	p, ok := applicationPrincipal(ctx, c, "workspace.read")
	if !ok {
		return
	}
	v, e := applicationAgents.Get(ctx, p, c.Param("name"))
	if e != nil {
		applicationError(c, e)
		return
	}
	c.JSON(200, toResponse(v))
}
func CheckApplicationWorkspace(ctx context.Context, c *app.RequestContext) {
	p, ok := applicationPrincipal(ctx, c, "session.chat")
	if !ok {
		return
	}
	v, e := applicationAgents.ByWorkspace(ctx, p, c.Param("workspace_id"))
	if e != nil {
		applicationError(c, e)
		return
	}
	c.JSON(200, toResponse(v))
}
