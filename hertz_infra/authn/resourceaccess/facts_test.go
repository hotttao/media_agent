package resourceaccess

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestCurrentFactsFilterForeignInactiveAndNonMemberTeams(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Media-Auth-Internal-Token") != "internal" {
			t.Error("missing service identity")
		}
		var in map[string]string
		_ = json.NewDecoder(r.Body).Decode(&in)
		switch r.URL.Path {
		case "/internal/organizations:byUser":
			_ = json.NewEncoder(w).Encode(map[string]any{"organization_ids": []string{"org"}})
		case "/internal/teams:byUser":
			_ = json.NewEncoder(w).Encode(map[string]any{"team_ids": []string{"active", "foreign", "inactive", "org-admin-only"}})
		default:
			org := "org"
			if in["team_id"] == "foreign" {
				org = "other"
			}
			role := "member"
			if in["team_id"] == "org-admin-only" {
				role = ""
			}
			_ = json.NewEncoder(w).Encode(Team{TeamID: in["team_id"], OrganizationID: org, TeamRole: role, TeamActive: in["team_id"] != "inactive", OrganizationActive: true, Allowed: true})
		}
	}))
	defer server.Close()
	facts, err := New(server.URL, "internal", nil).Resolve(context.Background(), "user", "org")
	if err != nil || !facts.Member || !reflect.DeepEqual(facts.TeamIDs, []string{"active"}) {
		t.Fatal(facts, err)
	}
	facts, err = New(server.URL, "internal", nil).Resolve(context.Background(), "user", "other")
	if err != nil || facts.Member || len(facts.TeamIDs) != 0 {
		t.Fatal(facts, err)
	}
}
func TestFailureDoesNotBecomeMembership(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(503) }))
	defer server.Close()
	facts, err := New(server.URL, "internal", nil).Resolve(context.Background(), "user", "org")
	if err == nil || facts.Member {
		t.Fatal(facts, err)
	}
}

func TestOrganizationRoleRequiresCurrentActiveMembership(t *testing.T) {
	for _, tc := range []struct {
		name               string
		ids                []string
		status, role, want string
	}{
		{"admin", []string{"org"}, "active", "admin", "admin"},
		{"member", []string{"org"}, "active", "member", "member"},
		{"removed", nil, "active", "admin", ""},
		{"disabled", []string{"org"}, "disabled", "admin", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/internal/organizations:byUser" {
					_ = json.NewEncoder(w).Encode(map[string]any{"organization_ids": tc.ids, "organizations": []map[string]string{{"organization_id": "org", "role": tc.role, "status": tc.status}}})
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"team_ids": []string{}})
			}))
			defer server.Close()
			got, err := New(server.URL, "token", nil).Resolve(context.Background(), "user", "org")
			if err != nil || got.OrganizationRole != tc.want {
				t.Fatal(got, err)
			}
		})
	}
}
