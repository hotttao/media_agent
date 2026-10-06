package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/golang-jwt/jwt/v5"
	"media_agent/hertz_infra/authn/internaljwt"
)

func TestUserAPITokenPreservesTrustedOrganization(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/jwks" {
			_ = json.NewEncoder(w).Encode(map[string]any{"keys": []any{jwkForTest(&key.PublicKey)}})
			return
		}
		_, _ = w.Write([]byte(`{"team_ids":[]}`))
	}))
	defer upstream.Close()
	issuer := "http://gateway.test/"
	raw := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss": issuer, "aud": []string{"media-workflow"}, "sub": "User:user-1", "principal_type": "user",
		"organization_id": "org-1", "credential_id": "credential-1", "auth_source": "api_token",
		"iat": time.Now().Add(-time.Second).Unix(), "exp": time.Now().Add(time.Minute).Unix(),
	})
	raw.Header["kid"] = "test-key"
	signed, err := raw.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	m := New(&Config{MediaAuthURL: upstream.URL, InternalJWTVerifier: internaljwt.NewVerifier(upstream.URL+"/jwks", issuer, "media-workflow", upstream.Client())})
	h := server.New()
	h.Use(m.Handler())
	var caller *User
	h.GET("/api/v1/probe", func(ctx context.Context, c *app.RequestContext) {
		caller = CallerFromContext(ctx)
		c.Status(200)
	})
	response := ut.PerformRequest(h.Engine, "GET", "/api/v1/probe?organization_id=foreign", nil,
		ut.Header{Key: "Authorization", Value: "Bearer " + signed},
		ut.Header{Key: "X-Organization-ID", Value: "foreign"})
	if response.Code != 200 || caller == nil || caller.Sub != "user-1" || caller.OrganizationID != "org-1" {
		t.Fatal("User identity lost trusted enterprise or accepted client enterprise headers")
	}
}
