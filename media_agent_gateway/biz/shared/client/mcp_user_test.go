package client

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"media_agent/hertz_infra/authn/internaljwt"
	"media_agent/hertz_infra/authn/mcpdelegation"
)

func TestMCPUserAudienceScopeActorAndLiveRevocation(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := mcpdelegation.NewSigner(key, "edge", nil)
	if err != nil {
		t.Fatal(err)
	}
	var member, allowed atomic.Bool
	member.Store(true)
	allowed.Store(true)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/jwks" {
			json.NewEncoder(w).Encode(signer.PublicJWKS())
			return
		}
		if r.Header.Get("X-Media-Auth-Internal-Token") != "service-secret" {
			t.Error("missing service authentication")
		}
		switch r.URL.Path {
		case "/internal/organizations:byUser":
			ids := []string{}
			if member.Load() {
				ids = append(ids, "org")
			}
			json.NewEncoder(w).Encode(map[string]any{"organization_ids": ids})
		case "/internal/teams:byUser":
			json.NewEncoder(w).Encode(map[string]any{"team_ids": []string{"team"}})
		case "/internal/team-authorization/check":
			json.NewEncoder(w).Encode(map[string]any{"organization_id": "org", "allowed": allowed.Load(), "team_active": true, "organization_active": true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	access := &MCPUserAccess{Verifier: internaljwt.NewVerifier(server.URL+"/jwks", "edge", "media-agent-gateway-mcp", nil), AuthURL: server.URL, AuthCredential: "service-secret"}
	token := func(scope, actor, organization, audience, source string) string {
		claims := internaljwt.Claims{PrincipalType: "user", AuthSource: source, Scope: scope, OrganizationID: organization, CredentialID: "credential", Actor: &internaljwt.Actor{Sub: actor}, RegisteredClaims: jwt.RegisteredClaims{Subject: "User:user", Issuer: "edge", Audience: jwt.ClaimStrings{audience}, IssuedAt: jwt.NewNumericDate(time.Now()), ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute))}}
		raw := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
		raw.Header["kid"] = signer.PublicJWKS().Keys[0].Kid
		value, err := raw.SignedString(key)
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	for _, c := range []struct{ scope, actor, org, aud, source string }{
		{"api.access", "User:user", "org", "media-agent-gateway-mcp", "api_token"},
		{"mcp.access", "Worker:worker", "org", "media-agent-gateway-mcp", "api_token"},
		{"mcp.access", "Agent:agent", "org", "media-agent-gateway-mcp", "api_token"},
		{"mcp.access", "User:user", "other", "media-agent-gateway-mcp", "api_token"},
		{"mcp.access", "User:user", "org", "internal-api", "api_token"},
		{"mcp.access", "User:user", "org", "media-agent-gateway-mcp", "browser_session"},
	} {
		if _, err := access.Authenticate(context.Background(), token(c.scope, c.actor, c.org, c.aud, c.source), "org", ""); !errors.Is(err, ErrMCPUserDenied) {
			t.Fatalf("invalid principal accepted: %+v %v", c, err)
		}
	}
	raw := token("mcp.access", "User:user", "org", "media-agent-gateway-mcp", "api_token")
	scope, err := access.Authenticate(context.Background(), raw, "org", "")
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := access.ModuleAllowed(context.Background(), scope, "workflow"); err != nil || !ok {
		t.Fatal("current module permission denied", err)
	}
	allowed.Store(false)
	if ok, err := access.ModuleAllowed(context.Background(), scope, "workflow"); err != nil || ok {
		t.Fatal("revoked module permission cached", err)
	}
	member.Store(false)
	if _, err := access.Authenticate(context.Background(), raw, "org", ""); !errors.Is(err, ErrMCPUserDenied) {
		t.Fatal("removed membership cached", err)
	}
}
