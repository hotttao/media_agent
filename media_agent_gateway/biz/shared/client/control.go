package client

import (
	"context"
	"net/http"

	controlclient "media_agent/hertz_gen/media_control/workspace_service"
	controlpb "media_agent/hertz_gen/model/media_control/workspace"
	service "media_agent/media_agent_gateway/biz/service/agent_session"
)

type Control struct {
	BaseURL      string
	ServiceToken string
	Fallback     service.ContextProvider
}

func (c *Control) Default(ctx context.Context, userID string) (service.AgentContext, error) {
	return c.Fallback.Default(ctx, userID)
}
func (c *Control) Resolve(ctx context.Context, _ string, workspaceID, token string) (service.AgentContext, error) {
	cli, err := controlclient.NewWorkspaceServiceClient(c.BaseURL, controlclient.WithHeader(http.Header{
		"Cookie":                  []string{"token=" + token},
		"X-Control-Service-Token": []string{c.ServiceToken},
	}))
	if err != nil {
		return service.AgentContext{}, err
	}
	value, _, err := cli.GetRuntimeContext(ctx, &controlpb.GetRuntimeContextRequest{WorkspaceId: workspaceID})
	if err != nil {
		return service.AgentContext{}, err
	}
	return service.AgentContext{WorkspaceID: value.WorkspaceId, AgentID: value.AgentId, AgentType: value.AgentType, APIKey: value.ApiKey, ContextVersion: value.ContextVersion}, nil
}

func (c *Control) AuthorizeWorkspace(ctx context.Context, userID, workspaceID, token string) error {
	_, err := c.Resolve(ctx, userID, workspaceID, token)
	return err
}
