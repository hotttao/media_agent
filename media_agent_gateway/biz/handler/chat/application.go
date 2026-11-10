package chat

import (
	"context"
	"errors"
	"github.com/cloudwego/hertz/pkg/app"
	dto "media_agent/hertz_gen/model/media_agent_gateway/chat"
	"media_agent/hertz_infra/authn/appcredential"
	"media_agent/media_agent_gateway/biz/middleware"
	"media_agent/media_agent_gateway/biz/workflow"
)

var applicationChat *workflow.ApplicationChat

func ConfigureApplications(f *workflow.ApplicationChat) { applicationChat = f }
func authorizeApplication(ctx context.Context, c *app.RequestContext, workspace string) (context.Context, bool) {
	c.Header("Cache-Control", "no-store")
	if applicationChat == nil {
		c.JSON(503, map[string]string{"error": "application_chat_unavailable"})
		return ctx, false
	}
	p, e := applicationChat.Authorize(ctx, appcredential.Bearer(string(c.GetHeader("Authorization"))), workspace, c.Param("session_id"))
	if e != nil {
		status := 503
		if errors.Is(e, appcredential.ErrDenied) {
			status = 403
		}
		c.JSON(status, map[string]string{"error": "application_chat_access_denied"})
		return ctx, false
	}
	// Application requests have no interactive session credential to forward.
	c.Request.Header.Del("Cookie")
	return middleware.WithApplicationUser(ctx, p.UserID), true
}
func ApplicationCreateSession(ctx context.Context, c *app.RequestContext) {
	var req dto.CreateSessionRequest
	if c.BindAndValidate(&req) != nil {
		c.JSON(400, map[string]string{"error": "invalid_session"})
		return
	}
	verified, ok := authorizeApplication(ctx, c, req.WorkspaceId)
	if ok {
		CreateSession(verified, c)
	}
}
func ApplicationListSessions(ctx context.Context, c *app.RequestContext) {
	verified, ok := authorizeApplication(ctx, c, c.Query("workspace_id"))
	if ok {
		ListSessions(verified, c)
	}
}
func ApplicationListMessages(ctx context.Context, c *app.RequestContext) {
	verified, ok := authorizeApplication(ctx, c, "")
	if ok {
		ListMessages(verified, c)
	}
}
func ApplicationRunTurn(ctx context.Context, c *app.RequestContext) {
	verified, ok := authorizeApplication(ctx, c, "")
	if ok {
		RunTurn(verified, c)
	}
}
func ApplicationInterruptTurn(ctx context.Context, c *app.RequestContext) {
	verified, ok := authorizeApplication(ctx, c, "")
	if ok {
		InterruptTurn(verified, c)
	}
}
func ApplicationSubscribeEvents(ctx context.Context, c *app.RequestContext) {
	verified, ok := authorizeApplication(ctx, c, "")
	if ok {
		SubscribeEvents(verified, c)
	}
}
func ApplicationListAgentRequests(ctx context.Context, c *app.RequestContext) {
	verified, ok := authorizeApplication(ctx, c, "")
	if ok {
		ListAgentRequests(verified, c)
	}
}
func ApplicationResolveAgentRequest(ctx context.Context, c *app.RequestContext) {
	verified, ok := authorizeApplication(ctx, c, "")
	if ok {
		ResolveAgentRequest(verified, c)
	}
}
