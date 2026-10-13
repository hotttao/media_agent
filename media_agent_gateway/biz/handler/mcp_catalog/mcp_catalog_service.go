package mcp_catalog

import (
	"context"
	"errors"
	"github.com/cloudwego/hertz/pkg/app"
	dto "media_agent/hertz_gen/model/media_agent_gateway/mcp_catalog"
	d "media_agent/media_agent_gateway/biz/domain/mcp_catalog"
	"media_agent/media_agent_gateway/biz/middleware"
	service "media_agent/media_agent_gateway/biz/service/mcp_catalog"
	"net/http"
)

var catalog *service.Service
var protocolHandler http.Handler

func Configure(value *service.Service, protocol http.Handler) {
	catalog, protocolHandler = value, protocol
}
func response(v *d.Definition) *dto.Definition {
	return &dto.Definition{DefinitionId: v.ID, ServiceName: v.ServiceName, Endpoint: v.Endpoint, Transport: v.Transport, AuthenticationType: v.AuthenticationType, Status: v.Status, Version: v.Version}
}
func ready(c *app.RequestContext) bool {
	if catalog == nil {
		failure(c, errors.New("catalog unavailable"))
		return false
	}
	return true
}
func RegisterDefinition(ctx context.Context, c *app.RequestContext) { put(ctx, c, true) }
func UpdateDefinition(ctx context.Context, c *app.RequestContext)   { put(ctx, c, false) }
func put(ctx context.Context, c *app.RequestContext, create bool) {
	if !ready(c) {
		return
	}
	var req dto.DefinitionCommand
	if e := c.BindAndValidate(&req); e != nil {
		failure(c, d.ErrInvalid)
		return
	}
	if !create {
		req.DefinitionId = c.Param("definition_id")
	}
	if create && req.ExpectedVersion != 0 || !create && req.ExpectedVersion < 1 {
		failure(c, d.ErrInvalid)
		return
	}
	v, e := catalog.Put(ctx, d.Definition{ID: req.DefinitionId, ServiceName: req.ServiceName, Endpoint: req.Endpoint, Transport: req.Transport, AuthenticationType: req.AuthenticationType, Status: req.Status}, req.ExpectedVersion, middleware.ManagementUser(ctx).Role == "admin")
	if e != nil {
		failure(c, e)
		return
	}
	status := 200
	if create {
		status = 201
	}
	c.JSON(status, response(v))
}
func GetDefinition(ctx context.Context, c *app.RequestContext) {
	if !ready(c) {
		return
	}
	if middleware.ManagementUser(ctx).Role != "admin" {
		failure(c, d.ErrForbidden)
		return
	}
	v, e := catalog.Get(ctx, c.Param("definition_id"))
	if e != nil {
		failure(c, e)
		return
	}
	c.JSON(200, response(v))
}
func ListDefinitions(ctx context.Context, c *app.RequestContext) {
	if !ready(c) {
		return
	}
	if middleware.ManagementUser(ctx).Role != "admin" {
		failure(c, d.ErrForbidden)
		return
	}
	var req dto.DefinitionQuery
	if e := c.BindAndValidate(&req); e != nil {
		failure(c, d.ErrInvalid)
		return
	}
	limit := int(req.Limit)
	if limit == 0 {
		limit = 100
	}
	rows, e := catalog.List(ctx, req.AfterId, limit)
	if e != nil {
		failure(c, e)
		return
	}
	out := &dto.Definitions{}
	for _, v := range rows {
		out.Definitions = append(out.Definitions, response(&v))
	}
	if len(rows) == limit {
		out.NextAfterId = rows[len(rows)-1].ID
	}
	c.JSON(200, out)
}
func HandleMCP(ctx context.Context, c *app.RequestContext) {
	if protocolHandler == nil {
		failure(c, errors.New("MCP unavailable"))
		return
	}
	serveProtocol(ctx, c, protocolHandler)
}
func failure(c *app.RequestContext, e error) {
	status := 503
	switch {
	case errors.Is(e, d.ErrInvalid):
		status = 400
	case errors.Is(e, d.ErrForbidden):
		status = 403
	case errors.Is(e, d.ErrConflict):
		status = 409
	case errors.Is(e, d.ErrNotFound):
		status = 404
	}
	c.JSON(status, map[string]string{"error": "mcp_catalog_request_failed"})
}

func GetInternalDefinition(ctx context.Context, c *app.RequestContext) {
	if !ready(c) {
		return
	}
	var req dto.DefinitionQuery
	if c.BindAndValidate(&req) != nil || req.Version < 0 {
		failure(c, d.ErrInvalid)
		return
	}
	var v *d.Definition
	var e error
	if req.Version > 0 {
		v, e = catalog.Version(ctx, c.Param("definition_id"), req.Version)
	} else {
		v, e = catalog.Get(ctx, c.Param("definition_id"))
	}
	if e != nil {
		failure(c, e)
		return
	}
	c.JSON(200, response(v))
}
func ListInternalDefinitions(ctx context.Context, c *app.RequestContext) {
	if !ready(c) {
		return
	}
	var req dto.DefinitionQuery
	if c.BindAndValidate(&req) != nil {
		failure(c, d.ErrInvalid)
		return
	}
	limit := int(req.Limit)
	if limit == 0 {
		limit = 100
	}
	rows, e := catalog.List(ctx, req.AfterId, limit)
	if e != nil {
		failure(c, e)
		return
	}
	out := &dto.Definitions{}
	for _, v := range rows {
		out.Definitions = append(out.Definitions, response(&v))
	}
	if len(rows) == limit {
		out.NextAfterId = rows[len(rows)-1].ID
	}
	c.JSON(200, out)
}
