package e2e_test

import (
	"context"
	"errors"
	"io"
	sdk "media_agent/packages/platform-go-sdk"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestRelayBindingOriginAndEvents(t *testing.T) {
	var writes atomic.Int32
	platform := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/token"):
			respond(w, `{"access_token":"`+token+`","expires_in":3600}`)
		case r.URL.Path == "/apps/v1/chat/sessions":
			respond(w, `{"sessions":[{"id":"owned","workspace_id":"trusted"}]}`)
		case r.URL.Path == "/apps/v1/chat/sessions/owned/turns":
			writes.Add(1)
			respond(w, `{"turn_id":"turn"}`)
		case r.URL.Path == "/apps/v1/chat/sessions/owned/events":
			if r.URL.Query().Get("after_sequence") != "12" {
				t.Error("cursor not forwarded")
			}
			if r.Header.Get("Authorization") != "Bearer "+token {
				t.Error("browser credential forwarded")
			}
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "id: 13\ndata: {}\n\n")
		default:
			t.Error("unexpected platform call", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer platform.Close()
	u := user(t, client(t, platform.URL))
	relay, err := sdk.NewRelay(sdk.RelayConfig{Prefix: "/api/chat", Origin: "https://app.example", Authenticate: func(r *http.Request) (context.Context, error) {
		if r.Header.Get("Cookie") != "session=valid" {
			return nil, errors.New("not logged in")
		}
		return r.Context(), nil
	}, Resolve: func(ctx context.Context, key string) (*sdk.UserClient, string, error) {
		if key != "assistant" {
			return nil, "", errors.New("unmapped")
		}
		return u, "trusted", nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		path, method, origin, cookie, body string
		status                             int
	}{
		{"/workspaces/assistant/sessions/owned/turns", "POST", "https://app.example", "session=valid", `{"message":"hi","idempotency_key":"key"}`, 200},
		{"/workspaces/assistant/sessions/other/turns", "POST", "https://app.example", "session=valid", `{}`, 404},
		{"/workspaces/assistant/sessions/owned/turns", "POST", "https://evil.example", "session=valid", `{}`, 403},
		{"/workspaces/assistant/sessions/owned/turns", "POST", "https://app.example", "", `{}`, 401},
		{"/workspaces/assistant/sessions/owned/turns", "POST", "https://app.example", "session=valid", `{"message":"hi","idempotency_key":"key","workspace_id":"attacker"}`, 400},
		{"/workspaces/assistant/sessions/owned/turns", "POST", "https://app.example", "session=valid", `{} {}`, 400},
		{"/workspaces/assistant/sessions", "POST", "https://app.example", "session=valid", `{"title":"` + strings.Repeat("x", 1<<20) + `"}`, 400},
		{"/workspaces/assistant/sessions/owned/turns", "GET", "", "session=valid", ``, 405},
		{"/workspaces/assistant/sessions/owned%2fother/events", "GET", "", "session=valid", ``, 400},
		{"/workspaces/assistant/sessions/owned/events?after_sequence=-1", "GET", "", "session=valid", ``, 400},
	} {
		req := httptest.NewRequest(test.method, "/api/chat"+test.path, strings.NewReader(test.body))
		req.Header.Set("Origin", test.origin)
		req.Header.Set("Cookie", test.cookie)
		res := httptest.NewRecorder()
		relay.ServeHTTP(res, req)
		if res.Code != test.status {
			t.Errorf("%s: got %d want %d", test.path, res.Code, test.status)
		}
	}
	req := httptest.NewRequest("GET", "/api/chat/workspaces/assistant/sessions/owned/events", nil)
	req.Header.Set("Cookie", "session=valid")
	req.Header.Set("Last-Event-ID", "12")
	req.Header.Set("Authorization", "Bearer browser-secret")
	res := httptest.NewRecorder()
	relay.ServeHTTP(res, req)
	if res.Code != 200 || res.Body.String() != "id: 13\ndata: {}\n\n" {
		t.Fatalf("stream %d %s", res.Code, res.Body.String())
	}
	if writes.Load() != 1 {
		t.Fatal("unauthorized writes", writes.Load())
	}
}
