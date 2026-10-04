package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"media_agent/hertz_infra/authn/internaljwt"
)

func TestAgentJWTBindingAndWorkerIsolation(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := internaljwt.NewAgentSigner(key, "urn:media:agent-gateway", nil)
	if err != nil {
		t.Fatal(err)
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/jwks":
			_ = json.NewEncoder(w).Encode(signer.PublicJWKS())
		case "/internal/teams:byUser":
			_, _ = w.Write([]byte(`{"team_ids":[]}`))
		default:
			w.WriteHeader(401)
		}
	}))
	defer upstream.Close()
	m := New(&Config{MediaAuthURL: upstream.URL, JWTSecret: "test-cookie", AgentJWTVerifier: internaljwt.NewVerifier(upstream.URL+"/jwks", "urn:media:agent-gateway", "media-workflow-agent", nil), WorkerJWTVerifier: internaljwt.NewVerifier(upstream.URL+"/jwks", "urn:media:agent-gateway", "media-workflow", nil)})
	h := server.New()
	h.Use(m.Handler())
	terminal := func(ctx context.Context, c *app.RequestContext) {
		p, ok := internaljwt.AgentPrincipalFromContext(ctx)
		u := CallerFromContext(ctx)
		if !ok || p.ActorID != "Agent:configuration" || p.WorkspaceID != "workspace" || p.OrganizationID != "org" || u == nil || u.Sub != "owner" || u.Role != "user" || WorkerPrincipalFromContext(ctx) != nil {
			t.Error("Agent identity was converted to a Worker or privileged User")
		}
		c.JSON(200, map[string]bool{"ok": true})
	}
	h.POST("/api/v1/jobs", terminal)
	h.GET("/api/v1/jobs", terminal)
	h.POST("/api/v1/step-runs/claim", terminal)
	p := internaljwt.AgentPrincipal{UserID: "owner", ActorID: "Agent:configuration", OrganizationID: "org", WorkspaceID: "workspace", CredentialID: "credential"}
	bound := internaljwt.AgentRequest{Method: "POST", URI: "/api/v1/jobs?workflow_id=wf", ContentType: "application/json", Body: []byte(`{"workflow_id":"wf"}`)}
	raw, err := signer.Sign(p, "media-workflow-agent", bound)
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
		t.Fatalf("valid Agent rejected: %d %s", response.Code, response.Body.String())
	}
	for _, change := range []func(*internaljwt.AgentRequest){func(r *internaljwt.AgentRequest) { r.Method = "GET" }, func(r *internaljwt.AgentRequest) { r.URI = "/api/v1/jobs?workflow_id=other" }, func(r *internaljwt.AgentRequest) { r.Body = []byte(`{}`) }, func(r *internaljwt.AgentRequest) { r.ContentType = "application/octet-stream" }} {
		request := bound
		change(&request)
		if perform(request, raw).Code != 401 {
			t.Fatal("changed request accepted or Cookie used as fallback")
		}
	}
	wrong, err := signer.Sign(p, "media-materials-agent", bound)
	if err != nil {
		t.Fatal(err)
	}
	if perform(bound, wrong).Code != 401 {
		t.Fatal("wrong service audience accepted")
	}
	claim := internaljwt.AgentRequest{Method: "POST", URI: "/api/v1/step-runs/claim", ContentType: "application/json", Body: []byte(`{}`)}
	claimJWT, err := signer.Sign(p, "media-workflow-agent", claim)
	if err != nil {
		t.Fatal(err)
	}
	if perform(claim, claimJWT).Code != 401 {
		t.Fatal("Agent accepted as Worker even with an exactly bound request")
	}
}
