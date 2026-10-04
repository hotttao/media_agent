package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"media_agent/hertz_infra/authn/internaljwt"
)

func TestAgentDelegationBindsRequestAndRestrictsOrganization(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := internaljwt.NewAgentSigner(key, "urn:media:agent-gateway", nil)
	if err != nil {
		t.Fatal(err)
	}
	var membership atomic.Bool
	membership.Store(true)
	var unavailable atomic.Bool
	var checks atomic.Int32
	authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/jwks":
			_ = json.NewEncoder(w).Encode(signer.PublicJWKS())
		case "/internal/organizations:byUser":
			checks.Add(1)
			if r.Header.Get("X-Media-Auth-Internal-Token") != "query-credential" {
				t.Error("membership query not authenticated")
			}
			if unavailable.Load() {
				w.WriteHeader(503)
				return
			}
			organizations := []string{"other-org"}
			if membership.Load() {
				organizations = append(organizations, "org")
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"organization_ids": organizations})
		case "/internal/teams:byUser":
			_, _ = w.Write([]byte(`{"team_ids":[]}`))
		default:
			w.WriteHeader(401)
		}
	}))
	defer authServer.Close()
	m := New(&Config{MediaAuthURL: authServer.URL, MediaAuthInternalToken: "query-credential", JWTSecret: "cookie-test-secret", AgentJWTVerifier: internaljwt.NewVerifier(authServer.URL+"/jwks", "urn:media:agent-gateway", "media-materials-agent", nil)})
	h := server.New()
	h.Use(m.Handler())
	terminal := func(ctx context.Context, c *app.RequestContext) {
		p, ok := internaljwt.AgentPrincipalFromContext(ctx)
		u := CallerFromContext(ctx)
		organizations := OrganizationsFromContext(ctx)
		if !ok || p.ActorID != "Agent:configuration" || p.WorkspaceID != "workspace" || u == nil || u.Sub != "owner" || u.Role != "user" || len(organizations) != 1 || organizations[0] != "org" {
			t.Error("delegated owner, actor or organization scope was lost")
		}
		c.JSON(200, map[string]bool{"ok": true})
	}
	h.POST("/api/v1/materials", terminal)
	h.GET("/api/v1/materials", terminal)
	bound := internaljwt.AgentRequest{Method: "POST", URI: "/api/v1/materials?organization_id=org", ContentType: "application/json", Body: []byte(`{"name":"material"}`)}
	p := internaljwt.AgentPrincipal{UserID: "owner", ActorID: "Agent:configuration", OrganizationID: "org", WorkspaceID: "workspace", CredentialID: "credential"}
	raw, err := signer.Sign(p, "media-materials-agent", bound)
	if err != nil {
		t.Fatal(err)
	}
	cookie, err := m.signJWT(&User{Sub: "owner", Role: "user"})
	if err != nil {
		t.Fatal(err)
	}
	perform := func(request internaljwt.AgentRequest, token string) *ut.ResponseRecorder {
		return ut.PerformRequest(h.Engine, request.Method, request.URI, &ut.Body{Body: strings.NewReader(string(request.Body)), Len: len(request.Body)}, ut.Header{Key: "Content-Type", Value: request.ContentType}, ut.Header{Key: "Authorization", Value: "Bearer " + token}, ut.Header{Key: "Cookie", Value: m.cfg.CookieName + "=" + cookie})
	}
	if response := perform(bound, raw); response.Code != 200 {
		t.Fatalf("valid delegation rejected: %d %s", response.Code, response.Body.String())
	}
	for _, change := range []func(*internaljwt.AgentRequest){
		func(r *internaljwt.AgentRequest) { r.Method = "GET" },
		func(r *internaljwt.AgentRequest) { r.URI = "/api/v1/materials?organization_id=other-org" },
		func(r *internaljwt.AgentRequest) { r.Body = []byte(`{"name":"changed"}`) },
		func(r *internaljwt.AgentRequest) { r.ContentType = "application/octet-stream" },
	} {
		request := bound
		change(&request)
		if response := perform(request, raw); response.Code != 401 {
			t.Fatal("modified request accepted or fell back to Cookie")
		}
	}
	wrongAudience, err := signer.Sign(p, "media-workflow-agent", bound)
	if err != nil {
		t.Fatal(err)
	}
	if response := perform(bound, wrongAudience); response.Code != 401 {
		t.Fatal("cross-service assertion accepted or fell back to Cookie")
	}
	membership.Store(false)
	if response := perform(bound, raw); response.Code != 403 {
		t.Fatal("removed membership remained authorized from cache")
	}
	unavailable.Store(true)
	if response := perform(bound, raw); response.Code != 503 {
		t.Fatal("unavailable authority did not fail closed")
	}
	if checks.Load() != 3 {
		t.Fatal("binding failures queried authority or membership was cached")
	}
}
