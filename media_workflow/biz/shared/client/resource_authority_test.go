package client

import (
	"context"
	"encoding/json"
	"errors"
	"media_agent/hertz_infra/authn/internaljwt"
	"media_agent/media_workflow/biz/shared"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestResourceFactsRecheckMembershipAndHideUpstreamErrors(t *testing.T) {
	var revoked, outage atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if outage.Load() {
			w.WriteHeader(500)
			w.Write([]byte("private downstream credentials"))
			return
		}
		if r.URL.Path == "/api/v1/workspaces" {
			if r.Header.Get("X-Control-Service-Token") != "control" || r.Header.Get("X-Control-Acting-User-ID") != "user" {
				t.Error("untrusted workspace caller")
			}
			json.NewEncoder(w).Encode(map[string]any{"workspaces": []map[string]string{{"id": "workspace", "organization_id": "org", "team_id": "dept"}}})
			return
		}
		if r.Header.Get("X-Media-Auth-Internal-Token") != "internal" {
			t.Error("internal identity credential missing")
		}
		switch r.URL.Path {
		case "/internal/organizations:byUser":
			json.NewEncoder(w).Encode(map[string]any{"organization_ids": []string{"org"}})
		case "/internal/teams:byUser":
			json.NewEncoder(w).Encode(map[string]any{"team_ids": []string{"dept"}})
		case "/internal/team-authorization/check":
			role := "member"
			if revoked.Load() {
				role = ""
			}
			json.NewEncoder(w).Encode(map[string]any{"team_id": "dept", "organization_id": "org", "team_active": true, "organization_active": true, "team_role": role, "organization_role": "member", "allowed": !revoked.Load()})
		default:
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	client := NewResourceAuthorityClient(server.URL, "internal", NewControlClient(server.URL, "control"))
	templateOnly := NewResourceAuthorityClient(server.URL, "internal", nil)
	if facts, err := templateOnly.TemplateAuthority().Facts(context.Background(), "user"); err != nil || len(facts.OrganizationIDs) != 1 {
		t.Fatal("enterprise template unnecessarily depends on Control", err)
	}
	facts, err := client.Facts(context.Background(), "user")
	if err != nil || len(facts.TeamIDs) != 1 || len(facts.WorkspaceIDs) != 1 || len(facts.WorkflowOrganizationIDs) != 1 {
		t.Fatal("current membership not returned", err)
	}
	limited := internaljwt.ContextWithAgentPrincipal(context.Background(), internaljwt.AgentPrincipal{UserID: "user", OrganizationID: "org", WorkspaceID: "different"})
	if facts, err := client.Facts(limited, "user"); err != nil || len(facts.WorkspaceIDs) != 0 {
		t.Fatal("existing delegated Workspace ceiling expanded", err)
	}
	revoked.Store(true)
	facts, err = client.Facts(context.Background(), "user")
	if err != nil || len(facts.TeamIDs) != 0 || len(facts.WorkspaceIDs) != 0 || len(facts.WorkflowOrganizationIDs) != 0 {
		t.Fatal("revoked member retained cached grant", err)
	}
	// An owner may grant another same-enterprise department without being its member.
	org, err := client.TeamOrganization(context.Background(), "user", "dept")
	if err != nil || org != "org" {
		t.Fatal("grant target incorrectly requires department membership", err)
	}
	outage.Store(true)
	if _, err := client.Facts(context.Background(), "user"); !errors.Is(err, shared.ErrResourceAuthorityUnavailable) {
		t.Fatal("upstream failure exposed or treated as membership")
	}
}

func TestResourceWorkspaceDeniedIsNotAuthorityOutage(t *testing.T) {
	for _, status := range []int{403, 404, 500} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status) }))
			defer server.Close()
			client := NewResourceAuthorityClient(server.URL, "internal", NewControlClient(server.URL, "control"))
			_, err := client.GetWorkspaceForUser(context.Background(), "workspace", "user")
			want := shared.ErrResourceAccessDenied
			if status == 500 {
				want = shared.ErrResourceAuthorityUnavailable
			}
			if !errors.Is(err, want) {
				t.Fatal("incorrect denial/outage distinction", err)
			}
		})
	}
}
