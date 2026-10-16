package model_connection

import (
	"context"
	"errors"
	"github.com/cloudwego/hertz/pkg/app"
	p "media_agent/hertz_gen/model/media_control/model_connection"
	d "media_agent/media_control/biz/domain/model_connection"
	"media_agent/media_control/biz/middleware"
	s "media_agent/media_control/biz/service/model_connection"
)

var service *s.Service

func Configure(v *s.Service) { service = v }
func respond(c *app.RequestContext, v *d.Connection, e error, success int) {
	if e != nil {
		status := 400
		if errors.Is(e, d.ErrNotFound) {
			status = 404
		}
		if errors.Is(e, d.ErrConflict) {
			status = 409
		}
		c.JSON(status, map[string]string{"error": e.Error()})
		return
	}
	c.JSON(success, dto(v))
}
func dto(v *d.Connection) *p.Connection {
	return &p.Connection{Id: v.ID, Name: v.Name, Kind: v.Kind, Status: v.Status, Version: v.Version, CreatedAt: v.CreatedAt.UnixMilli(), UpdatedAt: v.UpdatedAt.UnixMilli()}
}
func CreateConnection(ctx context.Context, c *app.RequestContext) {
	var req p.Command
	if c.BindAndValidate(&req) != nil {
		c.JSON(400, map[string]string{"error": "invalid model connection"})
		return
	}
	v, e := service.Create(ctx, middleware.UserID(ctx), req.Name, req.AuthJson)
	respond(c, v, e, 201)
}
func ListConnections(ctx context.Context, c *app.RequestContext) {
	values, e := service.List(ctx, middleware.UserID(ctx))
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
func change(ctx context.Context, c *app.RequestContext, revoke bool) {
	var req p.Command
	if c.BindAndValidate(&req) != nil {
		c.JSON(400, map[string]string{"error": "invalid model connection"})
		return
	}
	v, e := service.Update(ctx, req.ConnectionId, middleware.UserID(ctx), req.Name, req.AuthJson, req.ExpectedVersion, revoke)
	respond(c, v, e, 200)
}
func UpdateConnection(ctx context.Context, c *app.RequestContext) { change(ctx, c, false) }
func RevokeConnection(ctx context.Context, c *app.RequestContext) { change(ctx, c, true) }
