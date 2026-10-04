package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
)

func TestUntrustedBearerNeverCallsLegacyAuthOrFallsBackToCookie(t *testing.T) {
	var legacyReads atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/internal/teams:byUser":
			_, _ = w.Write([]byte(`{"team_ids":[]}`))
		case "/internal/organizations:byUser":
			_, _ = w.Write([]byte(`{"organization_ids":[]}`))
		case "/api/auth/me":
			legacyReads.Add(1)
			// An accepting old Auth must not influence this service anymore.
			_, _ = w.Write([]byte(`{"id":"owner","role":"admin"}`))
		default:
			w.WriteHeader(401)
		}
	}))
	defer upstream.Close()
	m := New(&Config{MediaAuthURL: upstream.URL, JWTSecret: "test-cookie"})
	cookie, err := m.signJWT(&User{Sub: "owner", Role: "user"})
	if err != nil {
		t.Fatal(err)
	}
	h := server.New()
	h.Use(m.Handler())
	var admitted int
	h.GET("/api/v1/protected", func(ctx context.Context, c *app.RequestContext) {
		if caller := CallerFromContext(ctx); caller == nil || caller.Sub != "owner" {
			t.Error("independent Cookie identity missing")
		}
		admitted++
		c.Status(200)
	})
	header := ut.Header{Key: "Cookie", Value: m.cfg.CookieName + "=" + cookie}
	if response := ut.PerformRequest(h.Engine, "GET", "/api/v1/protected", nil, header); response.Code != 200 || admitted != 1 {
		t.Fatal("independent Cookie route broken")
	}
	for _, raw := range []string{"Bearer tos_legacy", "Bearer unknown-api-token", "Bearer invalid.jwt.value", "bearer tos_legacy", "BEARER tos_legacy", "Basic untrusted"} {
		for attempt := 0; attempt < 2; attempt++ {
			response := ut.PerformRequest(h.Engine, "GET", "/api/v1/protected", nil, header, ut.Header{Key: "Authorization", Value: raw})
			if response.Code != 401 || admitted != 1 || legacyReads.Load() != 0 {
				t.Fatal("untrusted Bearer accepted, cached, or delegated to old Auth")
			}
		}
	}
}
