package client

import (
	"context"
	"encoding/json"
	"errors"
	"media_agent/hertz_infra/authn/internaljwt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
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

func TestAgentWorkspaceAccessCannotChangeOwnerTenantOrWorkspace(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte(`{"id":"ws-1","organization_id":"org-1","team_id":"team-1"}`))
	}))
	defer server.Close()
	c := NewControlClient(server.URL, "internal")
	p := internaljwt.AgentPrincipal{UserID: "owner", ActorID: "Agent:configuration", OrganizationID: "org-1", WorkspaceID: "ws-1", CredentialID: "credential"}
	ctx := internaljwt.ContextWithAgentPrincipal(context.Background(), p)
	if _, err := c.GetWorkspaceForUser(ctx, "ws-1", "owner"); err != nil {
		t.Fatal("valid delegated Workspace rejected", err)
	}
	for _, request := range []struct{ workspace, user string }{{"ws-2", "owner"}, {"ws-1", "other"}} {
		if _, err := c.GetWorkspaceForUser(ctx, request.workspace, request.user); !errors.Is(err, ErrWorkspaceDelegationDenied) {
			t.Fatal("changed Workspace or User accepted")
		}
	}
	if calls.Load() != 1 {
		t.Fatal("identity mismatch reached Control")
	}
	p.OrganizationID = "other-org"
	ctx = internaljwt.ContextWithAgentPrincipal(context.Background(), p)
	if _, err := c.GetWorkspaceForUser(ctx, "ws-1", "owner"); !errors.Is(err, ErrWorkspaceDelegationDenied) {
		t.Fatal("foreign Organization accepted")
	}
	if allowed, err := c.CanUseWorkflow(ctx, "ws-1", "owner"); allowed || !errors.Is(err, ErrWorkspaceDelegationDenied) {
		t.Fatal("cross-tenant execution allowed")
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
