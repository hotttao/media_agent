package model_connection

import (
	"context"
	"errors"
	"github.com/cloudwego/hertz/pkg/app"
	p "media_agent/hertz_gen/model/media_control/model_connection"
	"media_agent/hertz_infra/authn/appcredential"
	d "media_agent/media_control/biz/domain/model_connection"
	"media_agent/media_control/biz/middleware"
	s "media_agent/media_control/biz/service/model_connection"
)

var service *s.Service

func Configure(v *s.Service) { service = v }
func respond(c *app.RequestContext, v *d.Connection, e error, success int) {
	if e != nil {
		status := 503
		if errors.Is(e, d.ErrInvalid) || errors.Is(e, d.ErrRevoked) {
			status = 400
		}
		if errors.Is(e, d.ErrNotFound) {
			status = 404
		}
		if errors.Is(e, d.ErrConflict) {
			status = 409
		}
		c.JSON(status, map[string]string{"error": "model_connection_request_failed"})
		return
	}
	c.JSON(success, dto(v))
}
func dto(v *d.Connection) *p.Connection {
	return &p.Connection{Id: v.ID, Name: v.Name, Kind: v.Kind, Status: v.Status, Version: v.Version, CreatedAt: v.CreatedAt.UnixMilli(), UpdatedAt: v.UpdatedAt.UnixMilli()}
}
func CreateConnection(ctx context.Context, c *app.RequestContext) {
	create(ctx, c, middleware.UserID(ctx))
}
func create(ctx context.Context, c *app.RequestContext, owner string) {
	var req p.Command
	if c.BindAndValidate(&req) != nil {
		c.JSON(400, map[string]string{"error": "invalid model connection"})
		return
	}
	v, e := service.Create(ctx, owner, req.Name, req.AuthJson)
	respond(c, v, e, 201)
}
func ListConnections(ctx context.Context, c *app.RequestContext) {
	list(ctx, c, middleware.UserID(ctx))
}
func list(ctx context.Context, c *app.RequestContext, owner string) {
	values, e := service.List(ctx, owner)
	if e != nil {
		respond(c, nil, e, 200)
		return
	}
	out := &p.Connections{Connections: make([]*p.Connection, 0, len(values))}
	for i := range values {
		out.Connections = append(out.Connections, dto(&values[i]))
	}
	c.JSON(200, out)
}
func change(ctx context.Context, c *app.RequestContext, owner string, revoke bool) {
	var req p.Command
	if c.BindAndValidate(&req) != nil {
		c.JSON(400, map[string]string{"error": "invalid model connection"})
		return
	}
	v, e := service.Update(ctx, req.ConnectionId, owner, req.Name, req.AuthJson, req.ExpectedVersion, revoke)
	respond(c, v, e, 200)
}
func UpdateConnection(ctx context.Context, c *app.RequestContext) {
	change(ctx, c, middleware.UserID(ctx), false)
}
func RevokeConnection(ctx context.Context, c *app.RequestContext) {
	change(ctx, c, middleware.UserID(ctx), true)
}

var applicationAuthority *appcredential.Client

func ConfigureApplications(a *appcredential.Client) { applicationAuthority = a }
func applicationOwner(ctx context.Context, c *app.RequestContext, scope string) (string, bool) {
	c.Header("Cache-Control", "no-store")
	if applicationAuthority == nil || service == nil {
		c.JSON(503, map[string]string{"error": "application_service_unavailable"})
		return "", false
	}
	p, err := applicationAuthority.Verify(ctx, appcredential.Bearer(string(c.GetHeader("Authorization"))), scope)
	if err != nil {
		status := 503
		if errors.Is(err, appcredential.ErrDenied) {
			status = 401
		}
		c.JSON(status, map[string]string{"error": "application_access_denied"})
		return "", false
	}
	return p.UserID, true
}
func CreateApplicationConnection(ctx context.Context, c *app.RequestContext) {
	if owner, ok := applicationOwner(ctx, c, "workspace.ensure"); ok {
		create(ctx, c, owner)
	}
}
func ListApplicationConnections(ctx context.Context, c *app.RequestContext) {
	if owner, ok := applicationOwner(ctx, c, "workspace.read"); ok {
		list(ctx, c, owner)
	}
}
func UpdateApplicationConnection(ctx context.Context, c *app.RequestContext) {
	if owner, ok := applicationOwner(ctx, c, "workspace.ensure"); ok {
		change(ctx, c, owner, false)
	}
}
func RevokeApplicationConnection(ctx context.Context, c *app.RequestContext) {
	if owner, ok := applicationOwner(ctx, c, "workspace.ensure"); ok {
		change(ctx, c, owner, true)
	}
}
