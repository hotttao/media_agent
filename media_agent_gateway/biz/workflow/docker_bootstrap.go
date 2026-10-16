package workflow

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	contract "media_agent/hertz_infra/agentworkload"
	"media_agent/media_agent_gateway/biz/shared/client"
	"time"
)

type BootstrapSecretStore interface {
	GetOrCreate(context.Context, string, func() (client.BootstrapCredential, error)) (client.BootstrapCredential, error)
}
type DockerBootstrap struct {
	Authority AgentTokenAuthority
	Issuer    AgentTokenIssuer
	Store     BootstrapSecretStore
}

func (b DockerBootstrap) Get(ctx context.Context, spec contract.Spec) (string, error) {
	if b.Authority == nil || b.Issuer == nil || b.Store == nil || spec.Driver != "docker" || spec.Replicas != 1 {
		return "", client.ErrBootstrap
	}
	facts, err := b.Authority.ResolveAgentIdentity(ctx, spec.AgentConfigID)
	if err != nil {
		return "", err
	}
	if facts.ConfigurationID != spec.AgentConfigID || facts.WorkspaceID != spec.WorkspaceID || facts.ActorID != "Agent:"+spec.AgentConfigID || facts.OwnerUserID == "" || facts.OrganizationID == "" || facts.TeamID == "" || !facts.OwnerAuthorized || facts.State != "running" {
		return "", client.ErrAgentTokenForbidden
	}
	scope, _ := json.Marshal([]string{spec.EnvironmentID, spec.WorkspaceID, spec.AgentConfigID, facts.OwnerUserID, facts.OrganizationID})
	value, err := b.Store.GetOrCreate(ctx, string(scope), func() (client.BootstrapCredential, error) {
		key := make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			return client.BootstrapCredential{}, client.ErrBootstrap
		}
		expires := time.Now().UTC().Add(365 * 24 * time.Hour).Truncate(time.Second)
		issued, err := b.Issuer.Issue(ctx, client.AgentTokenCommand{UserID: facts.OwnerUserID, ConfigurationID: spec.AgentConfigID, Name: "Docker Agent", ExpiresAt: expires.Format(time.RFC3339), IdempotencyKey: "docker-bootstrap:" + hex.EncodeToString(key)})
		if err != nil {
			return client.BootstrapCredential{}, err
		}
		if issued.Secret == "" || issued.Token.OrganizationID != facts.OrganizationID {
			return client.BootstrapCredential{}, client.ErrBootstrap
		}
		return client.BootstrapCredential{Secret: issued.Secret, ExpiresAt: expires}, nil
	})
	if err != nil {
		return "", err
	}
	return value.Secret, nil
}
