package workflow

import (
	"context"
	client "media_agent/media_agent_gateway/biz/shared/client"
	"strings"
	"time"
)

type AgentTokenAuthority interface {
	ResolveAgentIdentity(context.Context, string) (client.AgentIdentity, error)
}
type AgentTokenIssuer interface {
	Issue(context.Context, client.AgentTokenCommand) (client.AgentTokenResult, error)
}
type AgentTokenProvisioning struct {
	authority AgentTokenAuthority
	issuer    AgentTokenIssuer
}

func NewAgentTokenProvisioning(authority AgentTokenAuthority, issuer AgentTokenIssuer) *AgentTokenProvisioning {
	return &AgentTokenProvisioning{authority: authority, issuer: issuer}
}

func (w *AgentTokenProvisioning) Create(ctx context.Context, user client.GatewayUser, cmd client.AgentTokenCommand) (client.AgentTokenResult, error) {
	cmd.UserID = user.UserID
	cmd.IdempotencyKey = strings.TrimSpace(cmd.IdempotencyKey)
	if user.UserID == "" || !validConfigurationID(cmd.ConfigurationID) || strings.TrimSpace(cmd.Name) == "" || cmd.IdempotencyKey == "" || len(cmd.IdempotencyKey) > 200 {
		return client.AgentTokenResult{}, client.ErrAgentTokenInvalid
	}
	if _, err := time.Parse(time.RFC3339, cmd.ExpiresAt); err != nil {
		return client.AgentTokenResult{}, client.ErrAgentTokenInvalid
	}
	if w == nil || w.authority == nil || w.issuer == nil {
		return client.AgentTokenResult{}, client.ErrAgentTokenUnavailable
	}
	facts, err := w.authority.ResolveAgentIdentity(ctx, cmd.ConfigurationID)
	if err != nil {
		return client.AgentTokenResult{}, err
	}
	if facts.ConfigurationID != cmd.ConfigurationID || facts.ActorID != "Agent:"+cmd.ConfigurationID || facts.OwnerUserID != user.UserID || facts.OrganizationID == "" || facts.WorkspaceID == "" || facts.TeamID == "" || !facts.OwnerAuthorized || (facts.State != "running" && facts.State != "stopped") || (user.OrganizationID != "" && user.OrganizationID != facts.OrganizationID) {
		return client.AgentTokenResult{}, client.ErrAgentTokenForbidden
	}
	result, err := w.issuer.Issue(ctx, cmd)
	if err != nil {
		return client.AgentTokenResult{}, err
	}
	if result.Token.OrganizationID != facts.OrganizationID {
		return client.AgentTokenResult{}, client.ErrAgentTokenUnavailable
	}
	return result, nil
}
func validConfigurationID(id string) bool {
	if len(id) == 0 || len(id) > 128 {
		return false
	}
	for _, c := range id {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}
