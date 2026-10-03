package internaljwt

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func delegationFixture(t *testing.T) (*AgentSigner, AgentPrincipal, AgentRequest) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := NewAgentSigner(key, "urn:media:agent-gateway", nil)
	if err != nil {
		t.Fatal(err)
	}
	return signer, AgentPrincipal{UserID: "owner", ActorID: "Agent:configuration", OrganizationID: "org", CredentialID: "credential"}, AgentRequest{Method: "POST", URI: "/api/v1/jobs?workspace_id=workspace", Body: []byte(`{"workflow_id":"wf"}`)}
}

func TestAgentDelegationRoundTripAndRequestBinding(t *testing.T) {
	signer, principal, request := delegationFixture(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _ = json.NewEncoder(w).Encode(signer.PublicJWKS()) }))
	defer server.Close()
	raw, err := signer.Sign(principal, "media-workflow-agent", request)
	if err != nil {
		t.Fatal(err)
	}
	verifier := NewVerifier(server.URL, signer.issuer, "media-workflow-agent", server.Client())
	claims, err := verifier.Verify(context.Background(), raw)
	if err != nil {
		t.Fatal("valid delegated signature rejected")
	}
	got, err := ValidateAgentRequest(claims, request)
	if err != nil || got != principal {
		t.Fatal("delegated identity lost")
	}
	if len(claims.Audience) != 1 || claims.Role != "user" || claims.ExpiresAt.Time.Sub(claims.IssuedAt.Time) != time.Minute || claims.ID == "" {
		t.Fatal("assertion restrictions missing")
	}
	for _, changed := range []AgentRequest{
		{Method: "GET", URI: request.URI, Body: request.Body},
		{Method: request.Method, URI: "/api/v1/jobs/other", Body: request.Body},
		{Method: request.Method, URI: "/api/v1/jobs?workspace_id=other", Body: request.Body},
		{Method: request.Method, URI: request.URI, Body: []byte(`{"workflow_id":"other"}`)},
	} {
		if _, err := ValidateAgentRequest(claims, changed); !errors.Is(err, ErrAgentDelegation) {
			t.Fatal("changed target request authorized")
		}
	}
	if _, err := NewVerifier(server.URL, signer.issuer, "media-materials-agent", server.Client()).Verify(context.Background(), raw); err == nil {
		t.Fatal("cross-service audience accepted")
	}
	if _, err := NewVerifier(server.URL, "wrong-issuer", "media-workflow-agent", server.Client()).Verify(context.Background(), raw); err == nil {
		t.Fatal("wrong issuer accepted")
	}
	ordinary := NewVerifier(server.URL, signer.issuer, "internal-api", server.Client())
	if _, err := ordinary.Verify(context.Background(), raw); err == nil {
		t.Fatal("Agent assertion accepted as ordinary User JWT")
	}
	public, _ := json.Marshal(signer.PublicJWKS())
	for _, field := range []string{`"d":`, `"p":`, `"q":`, `"dp":`, `"dq":`, `"qi":`} {
		if strings.Contains(string(public), field) {
			t.Fatal("JWKS exposed private key")
		}
	}
	rawAgain, err := signer.Sign(principal, "media-workflow-agent", request)
	if err != nil || rawAgain == raw {
		t.Fatal("assertions missing unique request ID")
	}
}

func TestAgentDelegationRejectsInvalidClaims(t *testing.T) {
	signer, principal, request := delegationFixture(t)
	raw, err := signer.Sign(principal, "media-workflow-agent", request)
	if err != nil {
		t.Fatal(err)
	}
	parser := jwt.NewParser()
	base := &Claims{}
	if _, _, err := parser.ParseUnverified(raw, base); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		change func(*Claims)
	}{
		{"worker", func(c *Claims) { c.Actor = &Actor{Sub: "Worker:configuration"} }},
		{"user actor", func(c *Claims) { c.Actor = &Actor{Sub: "User:owner"} }},
		{"missing actor", func(c *Claims) { c.Actor = nil }},
		{"non User subject", func(c *Claims) { c.Subject = "Service:owner" }},
		{"missing org", func(c *Claims) { c.OrganizationID = "" }},
		{"missing credential", func(c *Claims) { c.CredentialID = "" }},
		{"extra scope", func(c *Claims) { c.Scope = "agent.gateway api.access" }},
		{"admin role", func(c *Claims) { c.Role = "admin" }},
		{"wrong source", func(c *Claims) { c.AuthSource = "api_token" }},
		{"missing iat", func(c *Claims) { c.IssuedAt = nil }},
		{"missing nbf", func(c *Claims) { c.NotBefore = nil }},
		{"missing exp", func(c *Claims) { c.ExpiresAt = nil }},
		{"missing jti", func(c *Claims) { c.ID = "" }},
		{"multi audience", func(c *Claims) {
			c.Audience = append(jwt.ClaimStrings{}, "media-workflow-agent", "media-materials-agent")
		}},
		{"too long TTL", func(c *Claims) { c.ExpiresAt = jwt.NewNumericDate(c.IssuedAt.Time.Add(2 * time.Minute)) }},
		{"invalid TTL", func(c *Claims) { c.ExpiresAt = jwt.NewNumericDate(c.IssuedAt.Time) }},
		{"shifted nbf", func(c *Claims) { c.NotBefore = jwt.NewNumericDate(c.IssuedAt.Time.Add(-time.Minute)) }},
		{"missing digest", func(c *Claims) { c.RequestDigest = "" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			copy := *base
			tc.change(&copy)
			if _, err := ValidateAgentRequest(&copy, request); !errors.Is(err, ErrAgentDelegation) {
				t.Fatal("invalid delegation accepted")
			}
		})
	}
	signer.now = func() time.Time { return time.Now().Add(-2 * time.Minute) }
	expired, err := signer.Sign(principal, "media-workflow-agent", request)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _ = json.NewEncoder(w).Encode(signer.PublicJWKS()) }))
	defer server.Close()
	if _, err := NewVerifier(server.URL, signer.issuer, "media-workflow-agent", server.Client()).Verify(context.Background(), expired); err == nil {
		t.Fatal("expired assertion accepted")
	}
}

func TestAgentDelegationSignerRejectsInvalidInputs(t *testing.T) {
	signer, principal, request := delegationFixture(t)
	for _, bad := range []AgentPrincipal{{}, {UserID: "owner", ActorID: "Worker:config", OrganizationID: "org", CredentialID: "key"}, {UserID: "owner", ActorID: "Agent:../config", OrganizationID: "org", CredentialID: "key"}} {
		if _, err := signer.Sign(bad, "media-workflow-agent", request); !errors.Is(err, ErrAgentDelegation) {
			t.Fatal("invalid Actor signed")
		}
	}
	for _, uri := range []string{"https://outside.test/api/v1/jobs", "//outside.test/api/v1/jobs", "/internal/token/platform", "/api/v1/../auth/me", "/api/v1/%2e%2e/auth/me", "/api/v1/jobs%20extra", "/api/v1/jobs#fragment", "/api/v1/jobs\r\nX-Forged: value"} {
		if _, err := signer.Sign(principal, "media-workflow-agent", AgentRequest{Method: "GET", URI: uri}); !errors.Is(err, ErrAgentDelegation) {
			t.Fatal("invalid target signed")
		}
	}
	if _, err := signer.Sign(principal, "internal-api", request); !errors.Is(err, ErrAgentDelegation) {
		t.Fatal("broad User audience signed")
	}
	if _, err := signer.Sign(principal, "media-workflow-agent", AgentRequest{Method: "CONNECT", URI: request.URI}); !errors.Is(err, ErrAgentDelegation) {
		t.Fatal("tunnel method signed")
	}
	if _, err := signer.Sign(principal, "media-workflow-agent", AgentRequest{Method: "POST", URI: request.URI, Body: make([]byte, (8<<20)+1)}); !errors.Is(err, ErrAgentDelegation) {
		t.Fatal("oversized body signed")
	}
	if _, err := NewAgentSigner(nil, "issuer", nil); !errors.Is(err, ErrAgentDelegation) {
		t.Fatal("nil key accepted")
	}
	weak, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewAgentSigner(weak, "issuer", nil); !errors.Is(err, ErrAgentDelegation) {
		t.Fatal("weak key accepted")
	}
	if _, err := NewAgentSigner(signer.key, "", nil); !errors.Is(err, ErrAgentDelegation) {
		t.Fatal("empty issuer accepted")
	}
}

func TestAgentSigningKeyParsingAndPrivateFile(t *testing.T) {
	signer, _, _ := delegationFixture(t)
	der, err := x509.MarshalPKCS8PrivateKey(signer.key)
	if err != nil {
		t.Fatal(err)
	}
	raw := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	parsed, err := ParseAgentSigningKey(raw)
	if err != nil || parsed.N.Cmp(signer.key.N) != 0 {
		t.Fatal("PKCS8 parse failed")
	}
	pkcs1 := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(signer.key)})
	if _, err := ParseAgentSigningKey(pkcs1); err != nil {
		t.Fatal("PKCS1 parse failed")
	}
	for _, bad := range [][]byte{nil, []byte("secret-text"), append(append([]byte{}, raw...), raw...), pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})} {
		if _, err := ParseAgentSigningKey(bad); !errors.Is(err, ErrAgentDelegation) {
			t.Fatal("invalid key material accepted")
		}
	}
	file := filepath.Join(t.TempDir(), "signing.pem")
	// Test fixtures use ephemeral keys; no persistent credential is created.
	if err := os.WriteFile(file, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadAgentSigner(file, signer.issuer); err != nil {
		t.Fatal("private key file rejected")
	}
	if err := os.Chmod(file, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadAgentSigner(file, signer.issuer); !errors.Is(err, ErrAgentDelegation) {
		t.Fatal("public-readable key accepted")
	}
	if _, err := LoadAgentSigner(t.TempDir(), signer.issuer); !errors.Is(err, ErrAgentDelegation) {
		t.Fatal("directory accepted as signing key")
	}
}
