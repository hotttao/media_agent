package workflow

import (
	"context"
	"errors"
	"testing"
	"time"

	api "media_agent/media_auth/biz/domain/api_token"
	service "media_agent/media_auth/biz/service/api_token"
	"media_agent/media_auth/biz/shared/client/control"
)

type agentAuthority struct {
	facts control.AgentIdentity
	err   error
}

func (a *agentAuthority) ResolveAgentIdentity(context.Context, string) (control.AgentIdentity, error) {
	return a.facts, a.err
}

func TestAgentProvisioningDerivesAuthorityAndIdempotency(t *testing.T) {
	a := &agentAuthority{facts: control.AgentIdentity{ConfigurationID: "config", ActorID: "Agent:config", WorkspaceID: "ws", OwnerUserID: "owner", OrganizationID: "org", TeamID: "team", WorkspaceState: "running", OwnerAuthorized: true}}
	tokens := &workerTokenCreator{}
	flow := NewAgentTokenProvisioning(a, tokens)
	cmd := CreateAgentTokenCommand{RequesterUserID: "owner", ConfigurationID: "config", Name: "agent", ExpiresAt: time.Now().Add(time.Hour), IdempotencyKey: "once"}
	if _, err := flow.Create(t.Context(), cmd); err != nil {
		t.Fatal(err)
	}
	got := tokens.command
	if got.CreatorUserID != "owner" || got.SubjectUserID != "owner" || got.ActorType != api.ActorAgent || got.ActorID != "Agent:config" || got.OrganizationID != "org" || !got.TrustedActorCreation || len(got.Scopes) != 1 || got.Scopes[0] != "agent.gateway" || len(got.IdempotencyKey) > 200 {
		t.Fatal("issuer did not receive derived authority")
	}
	if _, err := flow.Create(t.Context(), cmd); err != nil || tokens.command.IdempotencyKey != got.IdempotencyKey {
		t.Fatal("retry changed client key")
	}
	cmd.ConfigurationID = "other"
	a.facts.ConfigurationID = "other"
	a.facts.ActorID = "Agent:other"
	if _, err := flow.Create(t.Context(), cmd); err != nil || tokens.command.IdempotencyKey == got.IdempotencyKey {
		t.Fatal("different Agent reused client key")
	}
	for _, scopes := range [][]string{{"api.access"}, {"workflow.worker.poll"}, {"agent.gateway", "api.access"}, {""}} {
		cmd.Scopes = scopes
		before := tokens.calls
		if _, err := flow.Create(t.Context(), cmd); !errors.Is(err, service.ErrInvalidRequest) || tokens.calls != before {
			t.Fatal("scope escalation reached issuer")
		}
	}
	cmd.Scopes = []string{" agent.gateway "}
	cmd.RequesterUserID = "other"
	if _, err := flow.Create(t.Context(), cmd); !errors.Is(err, service.ErrForbidden) {
		t.Fatal("non-owner issued Agent token")
	}
	cmd.RequesterUserID = "owner"
	a.facts.OwnerAuthorized = false
	if _, err := flow.Create(t.Context(), cmd); !errors.Is(err, service.ErrForbidden) {
		t.Fatal("unauthorized owner issued Agent token")
	}
	a.facts.OwnerAuthorized = true
	a.err = control.ErrUnavailable
	if _, err := flow.Create(t.Context(), cmd); !errors.Is(err, control.ErrUnavailable) {
		t.Fatal("authority outage hidden")
	}
	a.err = nil
	tokens.err = api.ErrConflict
	if _, err := flow.Create(t.Context(), cmd); !errors.Is(err, api.ErrConflict) {
		t.Fatal("persistent issuance conflict hidden")
	}
	cmd.IdempotencyKey = ""
	if _, err := flow.Create(t.Context(), cmd); !errors.Is(err, service.ErrInvalidRequest) {
		t.Fatal("missing client key accepted")
	}
}
