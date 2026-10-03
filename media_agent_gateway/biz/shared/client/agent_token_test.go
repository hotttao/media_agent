package client

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"media_agent/hertz_infra/authn/internaljwt"
)

func TestAgentManagementIdentity(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, subject, actor, scope, credential, source, authUser string
		want                                                      error
	}{
		{"user", "User:owner", "User:owner", "api.access", "key", "api_token", "owner", nil},
		{"browser", "kratos-id", "", "", "", "", "owner", nil},
		{"delegated Agent", "User:owner", "Agent:config", "api.access", "key", "api_token", "owner", ErrAgentTokenUnauthorized},
		{"delegated Worker", "User:owner", "Worker:config", "api.access", "key", "api_token", "owner", ErrAgentTokenUnauthorized},
		{"no credential", "User:owner", "User:owner", "api.access", "", "api_token", "owner", ErrAgentTokenUnauthorized},
		{"wrong scope", "User:owner", "User:owner", "worker.poll", "key", "api_token", "owner", ErrAgentTokenUnauthorized},
		{"mismatched user", "User:owner", "User:owner", "api.access", "key", "api_token", "another", ErrAgentTokenUnauthorized},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/jwks" {
					_ = json.NewEncoder(w).Encode(map[string]any{"keys": []any{map[string]string{"kty": "RSA", "use": "sig", "alg": "RS256", "kid": "test", "n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()), "e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes())}}})
					return
				}
				calls.Add(1)
				if r.URL.Path != "/api/auth/me" || r.Header.Get("X-Media-Auth-Acting-User-ID") != "" {
					t.Error("identity bridge leaked acting header")
				}
				_ = json.NewEncoder(w).Encode(map[string]string{"id": tc.authUser})
			}))
			defer srv.Close()
			c, err := NewAgentTokenClient(srv.URL, srv.URL, "control", "issuer", internaljwt.NewVerifier(srv.URL+"/jwks", "issuer", "internal-api", nil))
			if err != nil {
				t.Fatal(err)
			}
			claims := internaljwt.Claims{PrincipalType: "user", AuthSource: tc.source, CredentialID: tc.credential, OrganizationID: "org-a", Scope: tc.scope, RegisteredClaims: jwt.RegisteredClaims{Subject: tc.subject, Issuer: "issuer", Audience: jwt.ClaimStrings{"internal-api"}, IssuedAt: jwt.NewNumericDate(time.Now()), ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute))}}
			if tc.actor != "" {
				claims.Actor = &internaljwt.Actor{Sub: tc.actor}
			}
			token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
			token.Header["kid"] = "test"
			raw, err := token.SignedString(key)
			if err != nil {
				t.Fatal(err)
			}
			user, err := c.Authenticate(context.Background(), "Bearer "+raw, "session=browser")
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v want %v", err, tc.want)
			}
			if tc.want == nil {
				if user.UserID != "owner" || calls.Load() != 1 {
					t.Fatal("wrong authenticated owner")
				}
				if tc.source == "api_token" && user.OrganizationID != "org-a" {
					t.Fatal("enterprise restriction lost")
				}
			}
			calls.Store(0)
			if _, err := c.Authenticate(context.Background(), "Bearer invalid", "token=otherwise-valid"); !errors.Is(err, ErrAgentTokenUnauthorized) || calls.Load() != 0 {
				t.Fatal("invalid assertion fell back to Cookie")
			}
		})
	}
}

func TestAgentTokenIssueBoundary(t *testing.T) {
	for _, tc := range []struct {
		status int
		want   error
	}{{201, nil}, {400, ErrAgentTokenInvalid}, {401, ErrAgentTokenUnavailable}, {403, ErrAgentTokenForbidden}, {404, ErrAgentTokenNotFound}, {409, ErrAgentTokenConflict}, {500, ErrAgentTokenUnavailable}} {
		t.Run(http.StatusText(tc.status), func(t *testing.T) {
			var calls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.URL.Path != "/internal/v1/agent-api-tokens" || r.Header.Get("X-Agent-Gateway-Internal-Token") != "private" || r.Header.Get("X-Media-Auth-Acting-User-ID") != "owner" || r.Header.Get("Authorization") != "" {
					t.Error("invalid trusted issuance headers")
				}
				var body struct {
					Scopes []string `json:"scopes"`
					Config string   `json:"agent_configuration_id"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Scopes) != 1 || body.Scopes[0] != "agent.gateway" || body.Config != "config" {
					t.Error("invalid issuance parameters")
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_ = json.NewEncoder(w).Encode(map[string]any{"secret": "private-secret", "api_token": map[string]any{"id": "key", "status": "active", "subject_user_id": "owner", "actor_type": "agent", "actor_id": "Agent:config", "organization_id": "org", "scopes": []string{"agent.gateway"}}})
			}))
			defer srv.Close()
			c, err := NewAgentTokenClient(srv.URL, srv.URL, "control", "private", internaljwt.NewVerifier(srv.URL, "issuer", "internal-api", nil))
			if err != nil {
				t.Fatal(err)
			}
			result, err := c.Issue(context.Background(), AgentTokenCommand{UserID: "owner", ConfigurationID: "config", Name: "test", ExpiresAt: "2027-01-01T00:00:00Z", IdempotencyKey: "key"})
			if !errors.Is(err, tc.want) || calls.Load() != 1 {
				t.Fatalf("err=%v want=%v calls=%d", err, tc.want, calls.Load())
			}
			if err != nil && (result.Secret != "" || strings.Contains(err.Error(), "private-secret")) {
				t.Fatal("error response exposed Secret")
			}
		})
	}
}
