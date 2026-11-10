package workflow

import (
	"context"
	"crypto/rand"
	"crypto/rsa"

	"errors"
	"media_agent/hertz_infra/authn/applicationcaller"
	"testing"
)

type callerAuthority struct {
	enabled bool
	err     error
	calls   int
}

func (a *callerAuthority) Resolve(_ context.Context, _, _ string, s MCPContext) (applicationcaller.Identity, bool, error) {
	a.calls++
	return applicationcaller.Identity{ApplicationID: "app", UserID: s.UserID, ExternalID: "external", OrganizationID: s.OrganizationID, TeamID: s.TeamID, WorkspaceID: s.WorkspaceID, SessionID: s.SessionID}, a.enabled, a.err
}
func TestApplicationCallerOnlineForEveryRequestAndFailsClosed(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	signer, e := applicationcaller.New(key, "gateway")
	if e != nil {
		t.Fatal(e)
	}
	a := &callerAuthority{enabled: true}
	c := &ApplicationCaller{Authority: a, Signer: signer}
	scope := MCPContext{Kind: "managed_agent", UserID: "user", OrganizationID: "org", TeamID: "team", SessionID: "session", WorkspaceID: "workspace"}
	headers := c.Headers(scope, "definition", "https://business.example/mcp")
	ctx := context.Background()
	for range 2 {
		h, e := headers(ctx)
		if e != nil || h[applicationcaller.Header] == "" {
			t.Fatal("missing signed Caller", e)
		}
	}
	if a.calls != 2 {
		t.Fatal("mapping cached across calls")
	}
	a.err = errors.New("disabled app")
	if _, e = headers(ctx); e == nil {
		t.Fatal("revoked caller accepted")
	}
	a.err = nil
	a.enabled = false
	if h, e := headers(ctx); e != nil || len(h) != 0 {
		t.Fatal("unregistered target assertion")
	}
	a.enabled = true
	c.Signer = nil
	if _, e = headers(ctx); e == nil {
		t.Fatal("missing signer accepted")
	}
	scope.Kind = "user"
	if _, e = c.Headers(scope, "definition", "https://business.example")(ctx); e == nil {
		t.Fatal("interactive user mistaken for application Agent")
	}
}
