package workflow

import (
	"bytes"
	"context"
	"errors"
	contract "media_agent/hertz_infra/agentworkload"
	"media_agent/hertz_infra/sessioncredential"
	"media_agent/media_agent_gateway/biz/shared/client"
	"path/filepath"
	"testing"
)

func TestDockerBootstrapRechecksCurrentIdentityBeforeReuse(t *testing.T) {
	keys, err := sessioncredential.New("k1", map[string][]byte{"k1": bytes.Repeat([]byte{2}, 32)})
	if err != nil {
		t.Fatal(err)
	}
	authority := &agentTokenGateFake{facts: client.AgentIdentity{ConfigurationID: "config", ActorID: "Agent:config", WorkspaceID: "workspace", OwnerUserID: "owner", OrganizationID: "org", TeamID: "team", OwnerAuthorized: true, State: "running"}}
	b := DockerBootstrap{Authority: authority, Issuer: authority, Store: client.BootstrapFiles{Root: filepath.Join(t.TempDir(), "secrets"), Keys: keys}}
	spec := contract.Spec{Driver: "docker", EnvironmentID: "local", WorkspaceID: "workspace", AgentConfigID: "config", Replicas: 1}
	for range 2 {
		secret, err := b.Get(context.Background(), spec)
		if err != nil || secret != "one-time" {
			t.Fatalf("bootstrap: %v", err)
		}
	}
	if authority.calls != 1 || authority.command.UserID != "owner" {
		t.Fatal("bootstrap duplicated or changed owner")
	}
	authority.facts.OwnerAuthorized = false
	if _, err = b.Get(context.Background(), spec); !errors.Is(err, client.ErrAgentTokenForbidden) || authority.calls != 1 {
		t.Fatal("cached credential bypassed revoked owner")
	}
	authority.facts.OwnerAuthorized = true
	spec.WorkspaceID = "foreign"
	if _, err = b.Get(context.Background(), spec); !errors.Is(err, client.ErrAgentTokenForbidden) {
		t.Fatal("cross workspace bootstrap accepted")
	}
}
