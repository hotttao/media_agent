package e2e_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	sdk "media_agent/packages/platform-go-sdk"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const token = "mac_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func client(t *testing.T, url string) *sdk.Client {
	t.Helper()
	c, err := sdk.New(sdk.Config{BaseURL: url, ClientID: "app", ClientSecret: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	return c
}
func user(t *testing.T, c *sdk.Client) *sdk.UserClient {
	t.Helper()
	u, err := c.As("external", "team", sdk.SessionChat, sdk.WorkspaceRead, sdk.WorkspaceEnsure)
	if err != nil {
		t.Fatal(err)
	}
	return u
}
func respond(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, body)
}
func TestGeneratedClientsAndCredentialBoundaries(t *testing.T) {
	var exchanges atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/apps/v1/users/") {
			id, secret, ok := r.BasicAuth()
			if !ok || id != "app" || secret != "secret" {
				t.Error("basic credentials missing")
			}
		} else if r.Header.Get("Authorization") != "Bearer "+token {
			t.Error("wrong bearer")
		}
		switch r.URL.Path {
		case "/apps/v1/users/external/token":
			exchanges.Add(1)
			var body struct {
				Team   string   `json:"team_id"`
				Scopes []string `json:"scopes"`
			}
			if json.NewDecoder(r.Body).Decode(&body) != nil || body.Team != "team" || len(body.Scopes) != 3 {
				t.Error("exchange DTO mismatch")
			}
			respond(w, `{"access_token":"`+token+`","expires_in":3600}`)
		case "/apps/v1/users/external":
			respond(w, `{"id":"user","external_id":"external","display_name":"Name"}`)
		case "/apps/v1/me":
			respond(w, `{"id":"user","external_id":"external"}`)
		case "/apps/v1/agents/assistant":
			respond(w, `{"id":"workspace","team_id":"team","name":"assistant"}`)
		case "/apps/v1/chat/sessions":
			if r.Method == "GET" {
				if r.URL.Query().Get("workspace_id") != "workspace" {
					t.Error("workspace query missing")
				}
				respond(w, `{"sessions":[{"id":"session","workspace_id":"workspace"}]}`)
			} else {
				respond(w, `{"id":"session","workspace_id":"workspace"}`)
			}
		case "/apps/v1/chat/sessions/session/turns":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["idempotency_key"] != "key" || body["message"] != "hello" {
				t.Error("turn DTO mismatch")
			}
			respond(w, `{"turn_id":"turn","status":"running"}`)
		case "/apps/v1/chat/sessions/session/messages":
			respond(w, `{"messages":[{"id":"message","sequence":7,"content_json":"{}"}]}`)
		case "/apps/v1/chat/sessions/session/events":
			if r.URL.Query().Get("after_sequence") != "7" {
				t.Error("cursor missing")
			}
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "id: 8\ndata: {}\n\n")
		default:
			t.Errorf("unexpected route %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	c := client(t, server.URL)
	u := user(t, c)
	ctx := context.Background()
	if p, e := c.UpsertUser(ctx, "external", sdk.UserProfileInput{DisplayName: "Name"}); e != nil || p.ID != "user" {
		t.Fatalf("profile %+v %v", p, e)
	}
	if p, e := u.Me(ctx); e != nil || p.ID != "user" {
		t.Fatalf("me %+v %v", p, e)
	}
	if w, e := u.EnsureAgent(ctx, "assistant", sdk.AgentConfiguration{}); e != nil || w.ID != "workspace" {
		t.Fatalf("workspace %+v %v", w, e)
	}
	if s, e := u.CreateSession(ctx, "workspace", "title"); e != nil || s.ID != "session" {
		t.Fatalf("session %+v %v", s, e)
	}
	if rows, e := u.ListSessions(ctx, "workspace"); e != nil || len(rows) != 1 {
		t.Fatalf("sessions %v %v", rows, e)
	}
	if turn, e := u.RunTurn(ctx, "session", "hello", "key"); e != nil || turn.TurnID != "turn" {
		t.Fatalf("turn %+v %v", turn, e)
	}
	if rows, e := u.ListMessages(ctx, "session"); e != nil || len(rows) != 1 || rows[0].Sequence != 7 {
		t.Fatalf("messages %v %v", rows, e)
	}
	res, e := u.Events(ctx, "session", 7)
	if e != nil {
		t.Fatal(e)
	}
	data, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if string(data) != "id: 8\ndata: {}\n\n" {
		t.Fatal("SSE mismatch")
	}
	if exchanges.Load() != 1 {
		t.Fatalf("exchanges %d", exchanges.Load())
	}
	for _, id := range []string{"../admin", "%2e%2e", "foo?other=x"} {
		if _, e := u.GetAgent(ctx, id); e == nil {
			t.Error("unsafe path accepted")
		}
	}
}
func TestRejectionInvalidatesWithoutReplaying(t *testing.T) {
	var exchanges, writes atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/token") {
			exchanges.Add(1)
			respond(w, `{"access_token":"`+token+`","expires_in":3600}`)
			return
		}
		writes.Add(1)
		w.WriteHeader(401)
		_, _ = io.WriteString(w, "secret must never appear in errors")
	}))
	defer server.Close()
	u := user(t, client(t, server.URL))
	for range 2 {
		_, err := u.RunTurn(context.Background(), "session", "hello", "key")
		var pe *sdk.Error
		if !errors.As(err, &pe) || pe.StatusCode != 401 || strings.Contains(err.Error(), "secret") {
			t.Fatalf("error %v", err)
		}
	}
	if exchanges.Load() != 2 || writes.Load() != 2 {
		t.Fatalf("exchanges=%d writes=%d", exchanges.Load(), writes.Load())
	}
}
func TestRedirectDoesNotForwardCredentials(t *testing.T) {
	var leaked atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaked.Store(true) }))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 307) }))
	defer server.Close()
	_, err := client(t, server.URL).GetUser(context.Background(), "external")
	var pe *sdk.Error
	if !errors.As(err, &pe) || pe.StatusCode != 307 || leaked.Load() {
		t.Fatalf("redirect %v leaked=%v", err, leaked.Load())
	}
}

func TestCanceledContextNeverSendsApplicationCredentials(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); respond(w, `{}`) }))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := client(t, server.URL).GetUser(ctx, "external")
	if !errors.Is(err, context.Canceled) || calls.Load() != 0 {
		t.Fatalf("canceled request %v calls=%d", err, calls.Load())
	}
}

func TestEventCancellationClosesUpstream(t *testing.T) {
	closed := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/token") {
			respond(w, `{"access_token":"`+token+`","expires_in":3600}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "id: 1\ndata: {}\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		close(closed)
	}))
	defer server.Close()
	u := user(t, client(t, server.URL))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	response, err := u.Events(ctx, "session", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	cancel()
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("upstream not canceled")
	}
}
func TestMalformedSuccessIsNotAnEmptyProfile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "<html>proxy error</html>") }))
	defer server.Close()
	if _, err := client(t, server.URL).GetUser(context.Background(), "external"); err == nil {
		t.Fatal("accepted non-JSON response")
	}
}

func TestInstancesDoNotShareApplicationTokens(t *testing.T) {
	var exchanges atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/token") {
			app, _, _ := r.BasicAuth()
			exchanges.Add(1)
			digit := "a"
			if app == "second" {
				digit = "b"
			}
			respond(w, `{"access_token":"mac_`+strings.Repeat(digit, 64)+`","expires_in":3600}`)
			return
		}
		id := "first"
		if r.Header.Get("Authorization") == "Bearer mac_"+strings.Repeat("b", 64) {
			id = "second"
		}
		respond(w, `{"id":"`+id+`"}`)
	}))
	defer server.Close()
	first := user(t, client(t, server.URL))
	secondClient, err := sdk.New(sdk.Config{BaseURL: server.URL, ClientID: "second", ClientSecret: "other-secret"})
	if err != nil {
		t.Fatal(err)
	}
	second := user(t, secondClient)
	for _, v := range []struct {
		u  *sdk.UserClient
		id string
	}{{first, "first"}, {second, "second"}, {first, "first"}, {second, "second"}} {
		p, e := v.u.Me(context.Background())
		if e != nil || p.ID != v.id {
			t.Fatalf("instance isolation: %+v %v", p, e)
		}
	}
	if exchanges.Load() != 2 {
		t.Fatal("shared or uncached exchange", exchanges.Load())
	}
}
