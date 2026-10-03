package workflow

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	api "media_agent/media_auth/biz/domain/api_token"
	service "media_agent/media_auth/biz/service/api_token"
)

type AgentTokenCreator interface {
	Create(context.Context, service.CreateCommand) (service.CreateResult, error)
}
type AgentTokenProvisioning struct {
	authority service.AgentAuthority
	tokens    AgentTokenCreator
}

func NewAgentTokenProvisioning(authority service.AgentAuthority, tokens AgentTokenCreator) *AgentTokenProvisioning {
	return &AgentTokenProvisioning{authority: authority, tokens: tokens}
}

type CreateAgentTokenCommand struct {
	RequesterUserID, ConfigurationID, Name, IdempotencyKey string
	Scopes                                                 []string
	ExpiresAt                                              time.Time
}

func (w *AgentTokenProvisioning) Create(ctx context.Context, cmd CreateAgentTokenCommand) (service.CreateResult, error) {
	cmd.IdempotencyKey = strings.TrimSpace(cmd.IdempotencyKey)
	if cmd.RequesterUserID == "" || cmd.ConfigurationID == "" || cmd.IdempotencyKey == "" || len(cmd.IdempotencyKey) > 200 || strings.TrimSpace(cmd.Name) == "" || cmd.ExpiresAt.IsZero() {
		return service.CreateResult{}, service.ErrInvalidRequest
	}
	if w == nil || w.authority == nil || w.tokens == nil {
		return service.CreateResult{}, service.ErrInactive
	}
	scopes := []string{"agent.gateway"}
	if len(cmd.Scopes) > 0 {
		for _, scope := range cmd.Scopes {
			if strings.TrimSpace(scope) != "agent.gateway" {
				return service.CreateResult{}, service.ErrInvalidRequest
			}
		}
	}
	facts, err := w.authority.ResolveAgentIdentity(ctx, cmd.ConfigurationID)
	if err != nil {
		return service.CreateResult{}, err
	}
	if facts.ConfigurationID != cmd.ConfigurationID || facts.ActorID != "Agent:"+cmd.ConfigurationID || facts.OwnerUserID != cmd.RequesterUserID || facts.OrganizationID == "" || facts.WorkspaceID == "" || facts.TeamID == "" || !facts.OwnerAuthorized || (facts.WorkspaceState != "running" && facts.WorkspaceState != "stopped") {
		return service.CreateResult{}, service.ErrForbidden
	}
	// Delimit every component to prevent concatenation collisions and isolate
	// Agent provisioning from User/Worker client keys in the creator namespace.
	digest := sha256.Sum256([]byte(cmd.ConfigurationID + "\x00agent-gateway\x00" + cmd.IdempotencyKey))
	return w.tokens.Create(ctx, service.CreateCommand{CreatorUserID: cmd.RequesterUserID, SubjectUserID: facts.OwnerUserID, ActorType: api.ActorAgent, ActorID: facts.ActorID, OrganizationID: facts.OrganizationID, Name: cmd.Name, Scopes: scopes, ExpiresAt: cmd.ExpiresAt, IdempotencyKey: "agent:" + hex.EncodeToString(digest[:]), TrustedActorCreation: true})
}
