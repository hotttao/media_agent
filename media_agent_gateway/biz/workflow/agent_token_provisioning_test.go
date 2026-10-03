package workflow

import (
	"context"
	"errors"
	client "media_agent/media_agent_gateway/biz/shared/client"
	"testing"
)

type agentTokenGateFake struct {
	facts   client.AgentIdentity
	err     error
	calls   int
	command client.AgentTokenCommand
}

func (f *agentTokenGateFake) ResolveAgentIdentity(context.Context, string) (client.AgentIdentity, error) {
	return f.facts, f.err
}
func (f *agentTokenGateFake) Issue(_ context.Context, c client.AgentTokenCommand) (client.AgentTokenResult, error) {
	f.calls++
	f.command = c
	return client.AgentTokenResult{Token: client.AgentTokenMetadata{OrganizationID: f.facts.OrganizationID}, Secret: "one-time"}, f.err
}

func TestAgentTokenGatewayPreservesOwnerAndOrganization(t *testing.T) {
	base := client.AgentIdentity{ConfigurationID: "config-1", ActorID: "Agent:config-1", OwnerUserID: "owner", OrganizationID: "org-a", WorkspaceID: "workspace", TeamID: "team", State: "stopped", OwnerAuthorized: true}
	command := client.AgentTokenCommand{UserID: "forged", ConfigurationID: "config-1", Name: "test", ExpiresAt: "2027-01-01T00:00:00Z", IdempotencyKey: " key "}
	for _, tc := range []struct {
		name   string
		mutate func(*client.AgentIdentity, *client.GatewayUser)
		want   error
	}{
		{"valid", func(*client.AgentIdentity, *client.GatewayUser) {}, nil},
		{"browser no restriction", func(_ *client.AgentIdentity, u *client.GatewayUser) { u.OrganizationID = "" }, nil},
		{"cross org", func(_ *client.AgentIdentity, u *client.GatewayUser) { u.OrganizationID = "org-b" }, client.ErrAgentTokenForbidden},
		{"wrong owner", func(f *client.AgentIdentity, _ *client.GatewayUser) { f.OwnerUserID = "another" }, client.ErrAgentTokenForbidden},
		{"unauthorized", func(f *client.AgentIdentity, _ *client.GatewayUser) { f.OwnerAuthorized = false }, client.ErrAgentTokenForbidden},
		{"deleted", func(f *client.AgentIdentity, _ *client.GatewayUser) { f.State = "deleted" }, client.ErrAgentTokenForbidden},
		{"wrong actor", func(f *client.AgentIdentity, _ *client.GatewayUser) { f.ActorID = "Worker:config-1" }, client.ErrAgentTokenForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			facts := base
			u := client.GatewayUser{UserID: "owner", OrganizationID: "org-a"}
			tc.mutate(&facts, &u)
			fake := &agentTokenGateFake{facts: facts}
			_, err := NewAgentTokenProvisioning(fake, fake).Create(context.Background(), u, command)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err=%v want=%v", err, tc.want)
			}
			if tc.want == nil {
				if fake.calls != 1 || fake.command.UserID != "owner" || fake.command.IdempotencyKey != "key" {
					t.Fatal("identity or idempotency overridden")
				}
			} else if fake.calls != 0 {
				t.Fatal("forbidden request issued token")
			}
		})
	}
	for _, bad := range []client.AgentTokenCommand{{ConfigurationID: "../config"}, {ConfigurationID: "config-1", Name: "n", ExpiresAt: "bad", IdempotencyKey: "key"}} {
		fake := &agentTokenGateFake{facts: base}
		if _, err := NewAgentTokenProvisioning(fake, fake).Create(context.Background(), client.GatewayUser{UserID: "owner"}, bad); !errors.Is(err, client.ErrAgentTokenInvalid) || fake.calls != 0 {
			t.Fatal("invalid command issued token")
		}
	}
	fake := &agentTokenGateFake{err: client.ErrAgentTokenUnavailable}
	if _, err := NewAgentTokenProvisioning(fake, fake).Create(context.Background(), client.GatewayUser{UserID: "owner"}, command); !errors.Is(err, client.ErrAgentTokenUnavailable) || fake.calls != 0 {
		t.Fatal("authority failure issued token")
	}
}
