package apitoken

import (
	"context"
	"errors"
	"testing"
	"time"

	domain "media_agent/media_auth/biz/domain/api_token"
	"media_agent/media_auth/biz/shared/client/control"
)

type agentAuthorityStub struct {
	facts control.AgentIdentity
	err   error
	calls int
}

func (s *agentAuthorityStub) ResolveAgentIdentity(context.Context, string) (control.AgentIdentity, error) {
	s.calls++
	return s.facts, s.err
}
func validAgentFacts() control.AgentIdentity {
	return control.AgentIdentity{ConfigurationID: "config", ActorID: "Agent:config", OwnerUserID: "owner", OrganizationID: "org", TeamID: "team", WorkspaceID: "workspace", WorkspaceState: "running", OwnerAuthorized: true}
}

func TestAgentActorAuthorityFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*control.AgentIdentity)
	}{
		{"configuration mismatch", func(v *control.AgentIdentity) { v.ConfigurationID = "other" }},
		{"actor mismatch", func(v *control.AgentIdentity) { v.ActorID = "Agent:other" }},
		{"owner mismatch", func(v *control.AgentIdentity) { v.OwnerUserID = "other" }},
		{"organization mismatch", func(v *control.AgentIdentity) { v.OrganizationID = "other" }},
		{"missing team", func(v *control.AgentIdentity) { v.TeamID = "" }},
		{"missing workspace", func(v *control.AgentIdentity) { v.WorkspaceID = "" }},
		{"owner unauthorized", func(v *control.AgentIdentity) { v.OwnerAuthorized = false }},
		{"deleted", func(v *control.AgentIdentity) { v.WorkspaceState = "deleted" }},
		{"unknown state", func(v *control.AgentIdentity) { v.WorkspaceState = "unknown" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			facts := validAgentFacts()
			tc.change(&facts)
			if NewAgentActorValidator(&agentAuthorityStub{facts: facts}).Validate(t.Context(), "Agent:config", "owner", "org") {
				t.Fatal("invalid authority accepted")
			}
		})
	}
	authority := &agentAuthorityStub{facts: validAgentFacts()}
	validator := NewAgentActorValidator(authority)
	if !validator.Validate(t.Context(), "Agent:config", "owner", "org") {
		t.Fatal("valid identity rejected")
	}
	authority.facts.WorkspaceState = "stopped"
	if !validator.Validate(t.Context(), "Agent:config", "owner", "org") {
		t.Fatal("runtime stop incorrectly deleted identity")
	}
	before := authority.calls
	for _, actor := range []string{"config", "Agent:", "Worker:config", "User:owner"} {
		if validator.Validate(t.Context(), actor, "owner", "org") {
			t.Fatal("wrong actor admitted")
		}
	}
	if authority.calls != before {
		t.Fatal("wrong actor performed identity lookup")
	}
	authority.err = errors.New("Control unavailable")
	if validator.Validate(t.Context(), "Agent:config", "owner", "org") {
		t.Fatal("authority outage admitted")
	}
	var missing *AgentActorValidator
	if missing.Validate(t.Context(), "Agent:config", "owner", "org") || NewAgentActorValidator(nil).Validate(t.Context(), "Agent:config", "owner", "org") {
		t.Fatal("missing authority admitted")
	}
}

func TestAgentTokenCreateVerifyRotateRecheckAuthority(t *testing.T) {
	authority := &agentAuthorityStub{facts: validAgentFacts()}
	validator := NewAgentActorValidator(authority)
	repo, secrets := &fakeRepo{}, &fakeSecrets{}
	svc := NewService(repo, secrets)
	svc.SetSubjectValidator(func(context.Context, string) bool { return true })
	svc.SetMembershipValidator(func(context.Context, string, string) bool { return true })
	svc.SetActorValidator(func(ctx context.Context, kind domain.ActorType, actor, owner, org string) bool {
		return kind == domain.ActorAgent && validator.Validate(ctx, actor, owner, org)
	})
	svc.SetIssuer(&fakeIssuer{})
	cmd := CreateCommand{CreatorUserID: "owner", SubjectUserID: "owner", ActorType: domain.ActorAgent, ActorID: "Agent:config", OrganizationID: "org", Name: "agent", Scopes: []string{"api.access"}, ExpiresAt: time.Now().Add(time.Hour), TrustedActorCreation: true}
	result, err := svc.Create(t.Context(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	repo.byKey = map[string]domain.Token{"talos-1": result.Token}
	repo.created = result.Token
	secrets.verified = VerifiedSecret{KeyID: "talos-1", ActorID: cmd.ActorID, Scopes: cmd.Scopes}
	if _, err := svc.Verify(t.Context(), "redacted"); err != nil {
		t.Fatal(err)
	}
	authority.facts.OwnerAuthorized = false
	rotator := &fakeRotator{}
	svc.SetRotator(rotator)
	if _, err := svc.Create(t.Context(), cmd); !errors.Is(err, ErrForbidden) {
		t.Fatal("lost delegation still issued")
	}
	if _, err := svc.Verify(t.Context(), "redacted"); !errors.Is(err, ErrForbidden) {
		t.Fatal("lost delegation still verified")
	}
	if _, err := svc.Rotate(t.Context(), "owner", result.Token.ID, time.Time{}); !errors.Is(err, ErrForbidden) || rotator.token.ID != "" {
		t.Fatal("lost delegation still rotated")
	}
	authority.facts = validAgentFacts()
	cmd.OrganizationID = "other"
	if _, err := svc.Create(t.Context(), cmd); !errors.Is(err, ErrForbidden) {
		t.Fatal("cross-organization issuance accepted")
	}
	cmd.OrganizationID = "org"
	cmd.SubjectUserID = "other"
	if _, err := svc.Create(t.Context(), cmd); !errors.Is(err, ErrForbidden) {
		t.Fatal("cross-owner issuance accepted")
	}
}
