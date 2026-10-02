package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetWorkspaceForUserCarriesAuthenticatedActor(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/workspaces/ws-1" ||
			r.Header.Get("X-Control-Service-Token") != "service-secret" ||
			r.Header.Get("X-Control-Acting-User-ID") != "owner" {
			t.Errorf("unexpected workspace request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusForbidden)
			return
		}
		_, _ = w.Write([]byte(`{"id":"ws-1","organization_id":"org-1","team_id":"team-1"}`))
	}))
	defer server.Close()

	workspace, err := NewControlClient(server.URL, "service-secret").GetWorkspaceForUser(context.Background(), "ws-1", "owner")
	if err != nil || workspace.OrganizationID != "org-1" || workspace.TeamID != "team-1" {
		t.Fatalf("workspace validation failed: workspace=%+v err=%v", workspace, err)
	}
}

func TestGetWorkspaceForUserFailsClosed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()
	if _, err := NewControlClient(server.URL, "service-secret").GetWorkspaceForUser(context.Background(), "ws-1", "owner"); err == nil {
		t.Fatal("forbidden workspace was accepted")
	}
	if _, err := NewControlClient(server.URL, "").GetWorkspaceForUser(context.Background(), "ws-1", "owner"); err == nil {
		t.Fatal("missing internal credential was accepted")
	}
}

func TestCanUseWorkflowCarriesInternalIdentity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/internal/workspaces:check" ||
			r.Header.Get("X-Control-Service-Token") != "service-secret" ||
			r.Header.Get("X-Control-Acting-User-ID") != "owner" {
			t.Errorf("unexpected access request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusForbidden)
			return
		}
		var request struct {
			WorkspaceID string `json:"workspace_id"`
			Action      string `json:"action"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.WorkspaceID != "ws-1" || request.Action != "use_workflow" {
			t.Errorf("unexpected access body: %+v err=%v", request, err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(`{"allowed":true}`))
	}))
	defer server.Close()
	allowed, err := NewControlClient(server.URL, "service-secret").CanUseWorkflow(context.Background(), "ws-1", "owner")
	if err != nil || !allowed {
		t.Fatalf("workflow access failed closed unexpectedly: allowed=%v err=%v", allowed, err)
	}
}
