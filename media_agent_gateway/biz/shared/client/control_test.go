package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestControlResolveSendsUserCookieAndServiceToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/workspaces/workspace-1/runtime-context" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if r.Header.Get("Cookie") != "token=user-token" {
			t.Fatalf("unexpected Cookie %q", r.Header.Get("Cookie"))
		}
		if r.Header.Get("X-Control-Service-Token") != "service-token" {
			t.Fatalf("unexpected service token %q", r.Header.Get("X-Control-Service-Token"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"workspace_id": "workspace-1", "agent_id": "claude-code",
			"agent_type": "claude-code", "api_key": "provider-key", "context_version": 3,
		})
	}))
	defer server.Close()
	control := &Control{BaseURL: server.URL, ServiceToken: "service-token"}

	value, err := control.Resolve(context.Background(), "user-1", "workspace-1", "user-token")
	if err != nil {
		t.Fatal(err)
	}
	if value.AgentType != "claude-code" || value.APIKey != "provider-key" || value.ContextVersion != 3 {
		t.Fatalf("unexpected context: %+v", value)
	}
}
