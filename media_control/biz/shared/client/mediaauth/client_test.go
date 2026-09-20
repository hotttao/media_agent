package mediaauth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientUsesInternalCredentialAndDecodesAuthorization(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Media-Auth-Internal-Token") != "secret" {
			t.Fatal("missing internal credential")
		}
		switch r.URL.Path {
		case "/internal/teams:byUser":
			_ = json.NewEncoder(w).Encode(map[string]any{"team_ids": []string{"team-1"}})
		case "/internal/team-authorization/check":
			_ = json.NewEncoder(w).Encode(map[string]any{"allowed": true, "fact_allowed": true, "team_id": "team-1", "organization_id": "org-1"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := New(server.URL, "secret", server.Client())
	teams, err := client.ListTeams(context.Background(), "user-1")
	if err != nil || len(teams) != 1 || teams[0] != "team-1" {
		t.Fatalf("teams=%v err=%v", teams, err)
	}
	decision, err := client.Authorize(context.Background(), "user-1", "team-1", "create_workspace")
	if err != nil || !decision.Allowed || decision.OrganizationID != "org-1" {
		t.Fatalf("decision=%+v err=%v", decision, err)
	}
}

func TestClientFailsClosedWhenInternalCredentialMissing(t *testing.T) {
	client := New("http://media-auth", "", nil)
	if _, err := client.ListTeams(context.Background(), "user-1"); err == nil {
		t.Fatal("missing internal credential must fail")
	}
}
