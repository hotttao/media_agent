package agent_identity

import (
	"context"
	"errors"
	"testing"

	domain "media_agent/media_control/biz/domain/workspace"
)

type readerStub struct {
	value *domain.AgentIdentity
	err   error
}

func (r readerStub) FindAgentIdentity(context.Context, string) (*domain.AgentIdentity, error) {
	return r.value, r.err
}

type teamStub struct {
	decision            domain.TeamAuthorization
	err                 error
	owner, team, action string
}

func (r *teamStub) ListTeams(context.Context, string) ([]string, error) { return nil, nil }
func (r *teamStub) Authorize(_ context.Context, owner, team, action string) (domain.TeamAuthorization, error) {
	r.owner, r.team, r.action = owner, team, action
	return r.decision, r.err
}

func TestResolveStableIdentityAndCurrentAuthority(t *testing.T) {
	for _, tc := range []struct {
		name, state, organization, team string
		allowed, want                   bool
	}{
		{"running", "running", "org", "team", true, true},
		{"stopped keeps identity", "stopped", "org", "team", true, true},
		{"owner no longer authorized", "running", "org", "team", false, false},
		{"different organization", "running", "other", "team", true, false},
		{"different team", "running", "org", "other", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := &domain.AgentIdentity{ConfigurationID: "config", WorkspaceID: "workspace", OwnerUserID: "owner", OrganizationID: "org", TeamID: "team", WorkspaceState: tc.state}
			teams := &teamStub{decision: domain.TeamAuthorization{Allowed: tc.allowed, OrganizationID: tc.organization, TeamID: tc.team}}
			got, err := New(readerStub{value: v}, teams).Resolve(t.Context(), "config")
			if err != nil || got.ActorID() != "Agent:config" || got.OwnerAuthorized != tc.want || teams.owner != "owner" || teams.team != "team" || teams.action != "view" {
				t.Fatalf("identity authority mismatch: %v", err)
			}
			if v.OwnerAuthorized {
				t.Fatal("query mutated repository facts")
			}
		})
	}
}

func TestResolveFailsClosed(t *testing.T) {
	valid := domain.AgentIdentity{ConfigurationID: "config", WorkspaceID: "workspace", OwnerUserID: "owner", OrganizationID: "org", TeamID: "team", WorkspaceState: "running"}
	for _, tc := range []struct {
		name   string
		change func(*domain.AgentIdentity)
		want   error
	}{
		{"configuration mismatch", func(v *domain.AgentIdentity) { v.ConfigurationID = "other" }, ErrIdentityUnavailable},
		{"missing workspace", func(v *domain.AgentIdentity) { v.WorkspaceID = "" }, ErrIdentityUnavailable},
		{"missing owner", func(v *domain.AgentIdentity) { v.OwnerUserID = "" }, ErrIdentityUnavailable},
		{"missing organization", func(v *domain.AgentIdentity) { v.OrganizationID = "" }, ErrIdentityUnavailable},
		{"missing team", func(v *domain.AgentIdentity) { v.TeamID = "" }, ErrIdentityUnavailable},
		{"deleted", func(v *domain.AgentIdentity) { v.WorkspaceState = "deleted" }, domain.ErrNotFound},
		{"unknown state", func(v *domain.AgentIdentity) { v.WorkspaceState = "unknown" }, ErrIdentityUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := valid
			tc.change(&v)
			if _, err := New(readerStub{value: &v}, &teamStub{}).Resolve(t.Context(), "config"); !errors.Is(err, tc.want) {
				t.Fatalf("invalid facts accepted: %v", err)
			}
		})
	}
	if _, err := New(readerStub{}, &teamStub{}).Resolve(t.Context(), " "); !errors.Is(err, ErrInvalidID) {
		t.Fatal("empty ID accepted")
	}
	if _, err := New(readerStub{}, &teamStub{}).Resolve(t.Context(), "config"); !errors.Is(err, ErrIdentityUnavailable) {
		t.Fatal("nil facts accepted")
	}
	upstream := errors.New("authorization unavailable")
	if _, err := New(readerStub{value: &valid}, &teamStub{err: upstream}).Resolve(t.Context(), "config"); !errors.Is(err, upstream) {
		t.Fatal("authorization outage hidden")
	}
	if _, err := New(readerStub{err: upstream}, &teamStub{}).Resolve(t.Context(), "config"); !errors.Is(err, upstream) {
		t.Fatal("repository outage hidden")
	}
}
