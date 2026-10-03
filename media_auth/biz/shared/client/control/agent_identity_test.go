package control

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel/trace"
	"media_agent/hertz_infra/tracecontext"
)

func TestIdentityClientServiceBoundary(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/internal/v1/agent-identities/config" || r.Header.Get("X-Control-Service-Token") != "private-control-secret" || r.Header.Get("X-Control-Acting-User-ID") != "media-auth" || r.Header.Get("X-Control-Acting-Role") != "system" || r.Header.Get("Authorization") != "" || r.Header.Get("traceparent") == "" {
			t.Error("internal boundary/trace headers mismatch")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"agent_configuration_id":"config","actor_id":"Agent:config","workspace_id":"ws","owner_user_id":"owner","organization_id":"org","team_id":"team","workspace_state":"running","owner_authorized":true}`))
	}))
	defer server.Close()
	client, err := New(server.URL, "private-control-secret", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	ctx, span := tracecontext.Start(t.Context(), "test", trace.SpanKindServer)
	defer span.End()
	facts, err := client.ResolveAgentIdentity(ctx, "config")
	if err != nil || facts.ConfigurationID != "config" || facts.ActorID != "Agent:config" || facts.OwnerUserID != "owner" || facts.OrganizationID != "org" || facts.TeamID != "team" || facts.WorkspaceID != "ws" || facts.WorkspaceState != "running" || !facts.OwnerAuthorized {
		t.Fatalf("unexpected authority projection: %v", err)
	}
}

func TestIdentityClientErrorsAreSafe(t *testing.T) {
	for _, status := range []int{302, 401, 404, 503} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				w.Write([]byte(`{"secret":"private-control-secret","diagnostic":"unsafe-db-body"}`))
			}))
			defer server.Close()
			client, err := New(server.URL, "private-control-secret", time.Second)
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.ResolveAgentIdentity(t.Context(), "config")
			if !errors.Is(err, ErrUnavailable) || strings.Contains(err.Error(), "private-control-secret") || strings.Contains(err.Error(), "unsafe-db-body") {
				t.Fatalf("unsafe error: %v", err)
			}
		})
	}
	client, err := New("http://127.0.0.1:1", "private-control-secret", 20*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = client.ResolveAgentIdentity(t.Context(), "config"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("unreachable authority accepted: %v", err)
	}
	for _, id := range []string{"", "../secret", "config/other", "id?secret=token", strings.Repeat("a", 129)} {
		if _, err := client.ResolveAgentIdentity(t.Context(), id); !errors.Is(err, ErrUnavailable) {
			t.Fatal("invalid ID accepted")
		}
	}
	for _, url := range []string{"", "file:///secret", "http://user:secret@localhost", "http://localhost?secret=token"} {
		if _, err := New(url, "private-control-secret", time.Second); !errors.Is(err, ErrInvalid) {
			t.Fatal("unsafe configuration accepted")
		}
	}
}

func TestIdentityClientDeadlineAndCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{}`))
	}))
	defer server.Close()
	client, err := New(server.URL, "private-control-secret", 10*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.ResolveAgentIdentity(t.Context(), "config"); !errors.Is(err, ErrUnavailable) {
		t.Fatal("timeout accepted")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := client.ResolveAgentIdentity(ctx, "config"); !errors.Is(err, ErrUnavailable) {
		t.Fatal("canceled lookup accepted")
	}
}
