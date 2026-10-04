package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tokenpb "media_agent/hertz_gen/model/media_auth/token"
)

func validAgentIntrospection() *tokenpb.VerifyApiTokenResponse {
	return &tokenpb.VerifyApiTokenResponse{Active: true, Sub: "User:owner", ActorType: "agent", ActorId: "Agent:configuration", CredentialId: "credential", OrganizationId: "org", Scope: "agent.gateway", Exp: time.Now().Add(time.Hour).Unix(), AuthSource: "api_token"}
}

func TestAgentCredentialIntrospectionAndRevocation(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := calls.Add(1)
		if r.URL.Path != "/internal/token/api-token:verify" || r.Method != "POST" || r.Header.Get("X-Media-Auth-Internal-Token") != "private-query" || r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" {
			t.Error("credential verification used an incorrect transport identity")
		}
		var request tokenpb.VerifyApiTokenRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.Token != "agent-secret" {
			t.Error("credential not sent to Auth verification")
		}
		w.Header().Set("Content-Type", "application/json")
		response := validAgentIntrospection()
		if count > 1 {
			response.Active = false
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()
	c, err := NewAgentCredentialClient(server.URL, "private-query")
	if err != nil {
		t.Fatal(err)
	}
	p, err := c.VerifyAgentCredential(context.Background(), "agent-secret")
	if err != nil || p.UserID != "owner" || p.ActorID != "Agent:configuration" || p.OrganizationID != "org" || p.CredentialID != "credential" || p.WorkspaceID != "" {
		t.Fatal("Agent identity not resolved correctly", err)
	}
	if _, err := c.VerifyAgentCredential(context.Background(), "agent-secret"); !errors.Is(err, ErrAgentCredentialRejected) {
		t.Fatal("revoked credential accepted from a cache")
	}
	if calls.Load() != 2 {
		t.Fatal("credential not introspected on every request")
	}
}

func TestAgentCredentialRejectsOtherIdentitiesAndMalformedClaims(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*tokenpb.VerifyApiTokenResponse)
	}{
		{"inactive", func(r *tokenpb.VerifyApiTokenResponse) { r.Active = false }},
		{"user", func(r *tokenpb.VerifyApiTokenResponse) { r.ActorType = "user"; r.ActorId = "User:owner" }},
		{"worker", func(r *tokenpb.VerifyApiTokenResponse) { r.ActorType = "personal_worker"; r.ActorId = "Worker:worker" }},
		{"missing actor", func(r *tokenpb.VerifyApiTokenResponse) { r.ActorId = "" }},
		{"wrong actor prefix", func(r *tokenpb.VerifyApiTokenResponse) { r.ActorId = "Worker:configuration" }},
		{"missing owner", func(r *tokenpb.VerifyApiTokenResponse) { r.Sub = "User:" }},
		{"service subject", func(r *tokenpb.VerifyApiTokenResponse) { r.Sub = "Service:owner" }},
		{"missing organization", func(r *tokenpb.VerifyApiTokenResponse) { r.OrganizationId = "" }},
		{"missing credential", func(r *tokenpb.VerifyApiTokenResponse) { r.CredentialId = "" }},
		{"wrong source", func(r *tokenpb.VerifyApiTokenResponse) { r.AuthSource = "service_token" }},
		{"wrong scope", func(r *tokenpb.VerifyApiTokenResponse) { r.Scope = "api.access" }},
		{"extra scope", func(r *tokenpb.VerifyApiTokenResponse) { r.Scope = "agent.gateway api.access" }},
		{"expired", func(r *tokenpb.VerifyApiTokenResponse) { r.Exp = time.Now().Add(-time.Minute).Unix() }},
		{"no expiry", func(r *tokenpb.VerifyApiTokenResponse) { r.Exp = 0 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response := validAgentIntrospection()
			tc.change(response)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(response)
			}))
			defer server.Close()
			c, err := NewAgentCredentialClient(server.URL, "private-query")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := c.VerifyAgentCredential(context.Background(), "agent-secret"); !errors.Is(err, ErrAgentCredentialRejected) {
				t.Fatal("untrusted credential accepted", err)
			}
		})
	}
}

func TestAgentCredentialFailureDoesNotExposeSecrets(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "private-secret-body", 503) }))
	defer server.Close()
	c, err := NewAgentCredentialClient(server.URL, "private-query")
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.VerifyAgentCredential(context.Background(), "agent-secret")
	if !errors.Is(err, ErrAgentTokenUnavailable) || strings.Contains(err.Error(), "private-secret-body") || strings.Contains(err.Error(), "agent-secret") {
		t.Fatal("unsafe verification failure")
	}
	for _, raw := range []string{"", "Bearer agent-secret", "secret\r\nheader", strings.Repeat("x", 8193)} {
		if _, err := c.VerifyAgentCredential(context.Background(), raw); !errors.Is(err, ErrAgentCredentialRejected) {
			t.Fatal("invalid raw credential accepted")
		}
	}
	for _, address := range []string{"http://host/base", "http://user:pass@host", "file:///tmp/auth", "http://host?query=1"} {
		if _, err := NewAgentCredentialClient(address, "query"); err == nil {
			t.Fatal("unsafe Auth address accepted")
		}
	}
	if _, err := NewAgentCredentialClient(server.URL, ""); err == nil {
		t.Fatal("missing internal identity accepted")
	}
}
