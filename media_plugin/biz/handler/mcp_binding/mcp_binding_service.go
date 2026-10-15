package mcp_binding

import (
	"context"
	"errors"
	"github.com/cloudwego/hertz/pkg/app"
	dto "media_agent/hertz_gen/model/media_plugin/mcp_binding"
	d "media_agent/media_plugin/biz/domain/plugin_binding"
	"media_agent/media_plugin/biz/middleware"
	"media_agent/media_plugin/biz/workflow"
)

var application *workflow.Bindings

func Configure(value *workflow.Bindings) { application = value }
func response(v *d.Binding) *dto.Binding {
	return &dto.Binding{BindingId: v.ID, WorkspaceId: v.WorkspaceID, DefinitionId: v.DefinitionID, DefinitionVersion: v.DefinitionVersion, Enabled: v.Enabled, AllowedMethods: v.AllowedMethods, AllowedTools: v.AllowedTools, AllowedPrompts: v.AllowedPrompts, AllowedResources: v.AllowedResources, Version: v.Version, CredentialHeader: v.CredentialHeader, CredentialConfigured: v.CredentialID != "", Checksum: v.Checksum}
}
func failure(c *app.RequestContext, err error) {
	status := 503
	switch {
	case errors.Is(err, d.ErrInvalid):
		status = 400
	case errors.Is(err, d.ErrForbidden):
		status = 403
	case errors.Is(err, d.ErrNotFound):
		status = 404
	case errors.Is(err, d.ErrConflict):
		status = 409
	}
	c.JSON(status, map[string]string{"error": "mcp_binding_request_failed"})
}
func ready(c *app.RequestContext) bool {
	if application == nil {
		failure(c, d.ErrUnavailable)
		return false
	}
	c.Response.Header.Set("Cache-Control", "no-store")
	return true
}
func CreateBinding(ctx context.Context, c *app.RequestContext) { put(ctx, c, true) }
func UpdateBinding(ctx context.Context, c *app.RequestContext) { put(ctx, c, false) }
func put(ctx context.Context, c *app.RequestContext, create bool) {
	if !ready(c) {
		return
	}
	var req dto.BindingCommand
	if c.BindAndValidate(&req) != nil || create && req.ExpectedVersion != 0 || !create && req.ExpectedVersion < 1 {
		failure(c, d.ErrInvalid)
		return
	}
	id := ""
	if !create {
		id = c.Param("binding_id")
	}
	value, err := application.Put(ctx, workflow.Command{UserID: middleware.User(ctx), ExpectedVersion: req.ExpectedVersion, Credential: req.Credential, Binding: d.Binding{ID: id, WorkspaceID: c.Param("workspace_id"), DefinitionID: req.DefinitionId, DefinitionVersion: req.DefinitionVersion, Enabled: req.Enabled, AllowedMethods: req.AllowedMethods, AllowedTools: req.AllowedTools, AllowedPrompts: req.AllowedPrompts, AllowedResources: req.AllowedResources, CredentialHeader: req.CredentialHeader}})
	if err != nil {
		failure(c, err)
		return
	}
	status := 200
	if create {
		status = 201
	}
	c.JSON(status, response(value))
}
func GetBinding(ctx context.Context, c *app.RequestContext)         { get(ctx, c, false) }
func GetSelectedBinding(ctx context.Context, c *app.RequestContext) { get(ctx, c, true) }
func get(ctx context.Context, c *app.RequestContext, selecting bool) {
	if !ready(c) {
		return
	}
	var req dto.BindingQuery
	if c.BindAndValidate(&req) != nil || req.Version < 0 || selecting && req.Version < 1 {
		failure(c, d.ErrInvalid)
		return
	}
	value, err := application.Get(ctx, middleware.User(ctx), c.Param("workspace_id"), c.Param("binding_id"), req.Version, selecting)
	if err != nil {
		failure(c, err)
		return
	}
	out := response(value)
	if !selecting {
		out.ConnectionStatus, out.OauthAuthorizeUrl, err = application.Connection(ctx, middleware.User(ctx), value)
		if err != nil {
			failure(c, err)
			return
		}
	}
	c.JSON(200, out)
}
func ListBindings(ctx context.Context, c *app.RequestContext) {
	if !ready(c) {
		return
	}
	values, err := application.List(ctx, middleware.User(ctx), c.Param("workspace_id"))
	if err != nil {
		failure(c, err)
		return
	}
	out := &dto.Bindings{Bindings: []*dto.Binding{}}
	for _, value := range values {
		out.Bindings = append(out.Bindings, response(&value))
	}
	c.JSON(200, out)
}
func ListBindingVersions(ctx context.Context, c *app.RequestContext) {
	if !ready(c) {
		return
	}
	values, err := application.Versions(ctx, middleware.User(ctx), c.Param("workspace_id"), c.Param("binding_id"))
	if err != nil {
		failure(c, err)
		return
	}
	out := &dto.Bindings{Bindings: []*dto.Binding{}}
	for _, value := range values {
		out.Bindings = append(out.Bindings, response(&value))
	}
	c.JSON(200, out)
}
