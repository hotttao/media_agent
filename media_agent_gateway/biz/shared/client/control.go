package client

import (
	"context"
	"errors"
	"net/http"

	controlclient "media_agent/hertz_gen/media_control/workspace_service"
	controlpb "media_agent/hertz_gen/model/media_control/workspace"
	service "media_agent/media_agent_gateway/biz/service/agent_session"
)

var ErrModelAuthenticationRequired = errors.New("Model connection is unavailable or revoked. Update model credentials before continuing.")

type Control struct {
	BaseURL      string
	ServiceToken string
	Fallback     service.ContextProvider
}

func (c *Control) Default(ctx context.Context, userID string) (service.AgentContext, error) {
	return c.Fallback.Default(ctx, userID)
}
func (c *Control) Resolve(ctx context.Context, userID, workspaceID, token string) (service.AgentContext, error) {
	return c.ResolveVersion(ctx, userID, workspaceID, token, 0)
}
func (c *Control) ResolveVersion(ctx context.Context, userID string, workspaceID, token string, version int64) (service.AgentContext, error) {
	cli, err := controlclient.NewWorkspaceServiceClient(c.BaseURL, controlclient.WithHeader(http.Header{
		"Cookie":                   []string{"token=" + token},
		"X-Control-Service-Token":  []string{c.ServiceToken},
		"X-Control-Acting-User-ID": []string{userID},
	}))
	if err != nil {
		return service.AgentContext{}, err
	}
	value, raw, err := cli.GetRuntimeContext(ctx, &controlpb.GetRuntimeContextRequest{WorkspaceId: workspaceID, ContextVersion: version})
	if raw != nil && raw.StatusCode() == 409 {
		return service.AgentContext{}, ErrModelAuthenticationRequired
	}
	if err != nil {
		return service.AgentContext{}, err
	}
	return service.AgentContext{WorkspaceID: value.WorkspaceId, AgentID: value.AgentId, AgentType: value.AgentType, APIKey: value.ApiKey, CodexAuthJSON: value.CodexAuthJson, ModelConnectionID: value.ModelConnectionId, ModelConnectionVersion: value.ModelConnectionVersion, ModelProtocol: value.ModelProtocol, ModelName: value.ModelName, ModelBaseURL: value.ModelBaseUrl, ContextVersion: value.ContextVersion, AgentConfigID: value.AgentConfigurationId, MetadataTargetContextVersion: value.MetadataTargetContextVersion}, nil
}

func (c *Control) AuthorizeWorkspace(ctx context.Context, userID, workspaceID, token string) error {
	_, err := c.Resolve(ctx, userID, workspaceID, token)
	return err
}
