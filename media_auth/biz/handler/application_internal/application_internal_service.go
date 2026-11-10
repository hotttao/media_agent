package application_internal

import (
	"context"
	"crypto/subtle"
	"errors"
	"github.com/cloudwego/hertz/pkg/app"
	pb "media_agent/hertz_gen/model/media_auth/application_internal"
	d "media_agent/media_auth/biz/domain/application"
	svc "media_agent/media_auth/biz/service/application"
)

var Service *svc.Service
var InternalSecret string

func VerifyApplicationAccess(ctx context.Context, c *app.RequestContext) {
	c.Header("Cache-Control", "no-store")
	if InternalSecret == "" || subtle.ConstantTimeCompare(c.GetHeader("X-Media-Auth-Internal-Token"), []byte(InternalSecret)) != 1 {
		c.JSON(401, map[string]string{"error": "service authentication required"})
		return
	}
	if Service == nil {
		c.JSON(503, map[string]string{"error": "authority unavailable"})
		return
	}
	var req pb.VerifyApplicationAccessRequest
	if c.BindAndValidate(&req) != nil {
		c.JSON(400, map[string]string{"error": "invalid request"})
		return
	}
	p, e := Service.VerifyBusiness(ctx, req.Token, req.Scope)
	if e != nil {
		status := 503
		if errors.Is(e, d.ErrUnauthorized) {
			status = 401
		}
		if errors.Is(e, d.ErrForbidden) {
			status = 403
		}
		c.JSON(status, map[string]string{"error": "application access unavailable"})
		return
	}
	c.JSON(200, &pb.VerifiedApplicationAccess{ApplicationId: p.ApplicationID, UserId: p.UserID, ExternalId: p.ExternalID, TeamId: p.TeamID, OrganizationId: p.OrganizationID, Scopes: p.Scopes, ExpiresAt: p.ExpiresAt.Unix()})
}

func ResolveApplicationCaller(ctx context.Context, c *app.RequestContext) {
	c.Header("Cache-Control", "no-store")
	if InternalSecret == "" || subtle.ConstantTimeCompare(c.GetHeader("X-Media-Auth-Internal-Token"), []byte(InternalSecret)) != 1 {
		c.JSON(401, map[string]string{"error": "service authentication required"})
		return
	}
	if Service == nil {
		c.JSON(503, map[string]string{"error": "authority unavailable"})
		return
	}
	var req pb.ResolveApplicationCallerRequest
	if c.BindAndValidate(&req) != nil {
		c.JSON(400, map[string]string{"error": "invalid request"})
		return
	}
	p, enabled, e := Service.ResolveCaller(ctx, req.DefinitionId, req.Endpoint, req.UserId, req.OrganizationId, req.TeamId)
	if e != nil {
		status := 503
		if errors.Is(e, d.ErrForbidden) {
			status = 403
		}
		c.JSON(status, map[string]string{"error": "caller unavailable"})
		return
	}
	c.JSON(200, &pb.ResolvedApplicationCaller{Enabled: enabled, ApplicationId: p.ApplicationID, UserId: p.UserID, ExternalId: p.ExternalID})
}
