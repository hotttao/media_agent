package mcp_access

import (
	"context"
	"errors"
	"github.com/cloudwego/hertz/pkg/app"
	dto "media_agent/hertz_gen/model/media_plugin/mcp_access"
	d "media_agent/media_plugin/biz/domain/plugin_binding"
	"media_agent/media_plugin/biz/workflow"
)

var application *workflow.Bindings

func Configure(value *workflow.Bindings) { application = value }
func ResolveMCPAccess(ctx context.Context, c *app.RequestContext) {
	c.Response.Header.Set("Cache-Control", "no-store")
	if application == nil {
		c.JSON(503, map[string]string{"error": "mcp_unavailable"})
		return
	}
	var req dto.AccessRequest
	if c.BindAndValidate(&req) != nil {
		c.JSON(400, map[string]string{"error": "invalid_scope"})
		return
	}
	value, err := application.Resolve(ctx, workflow.Scope{UserID: req.UserId, WorkspaceID: c.Param("workspace_id"), AgentConfigID: req.AgentConfigId, ContextVersion: req.ContextVersion, Target: req.TargetContextVersion, SessionID: req.SessionId})
	if err != nil {
		status := 503
		if errors.Is(err, d.ErrForbidden) {
			status = 403
		}
		if errors.Is(err, d.ErrInvalid) {
			status = 400
		}
		c.JSON(status, map[string]string{"error": "mcp_access_unavailable"})
		return
	}
	result := &dto.AccessResponse{WorkspaceId: req.WorkspaceId, AgentConfigId: req.AgentConfigId, ContextVersion: req.ContextVersion, SnapshotVersion: value.SnapshotVersion, AuthorizationRevision: value.AuthorizationRevision, Grants: []*dto.Grant{}}
	for _, grant := range value.Grants {
		b := grant.Binding
		result.Grants = append(result.Grants, &dto.Grant{DefinitionId: b.DefinitionID, DefinitionVersion: b.DefinitionVersion, AllowedMethods: b.AllowedMethods, AllowedTools: b.AllowedTools, AllowedPrompts: b.AllowedPrompts, AllowedResources: b.AllowedResources, UpstreamHeaders: grant.Headers, BindingId: b.ID, BindingVersion: b.Version, OauthProxy: grant.OAuthProxy})
	}
	c.JSON(200, result)
}
