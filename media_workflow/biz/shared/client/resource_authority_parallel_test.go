package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"media_agent/media_workflow/biz/shared"
)

func TestResourceFactsOverlapLiveSourcesBeforeIntersection(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	started, release := make(chan string, 3), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/team-authorization/check" {
			started <- r.URL.Path
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
		}
		switch r.URL.Path {
		case "/internal/organizations:byUser":
			json.NewEncoder(w).Encode(map[string]any{"organization_ids": []string{"org"}})
		case "/internal/teams:byUser":
			json.NewEncoder(w).Encode(map[string]any{"team_ids": []string{"dept"}})
		case "/internal/team-authorization/check":
			json.NewEncoder(w).Encode(map[string]any{"team_id": "dept", "organization_id": "org", "team_active": true, "organization_active": true, "team_role": "member", "organization_role": "member", "allowed": true})
		case "/api/v1/workspaces":
			json.NewEncoder(w).Encode(map[string]any{"workspaces": []map[string]string{{"id": "workspace", "organization_id": "org", "team_id": "dept"}, {"id": "foreign", "organization_id": "other", "team_id": "dept"}}})
		default:
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	defer close(release)
	c := NewResourceAuthorityClient(server.URL, "auth", NewControlClient(server.URL, "control"))
	done := make(chan struct{})
	var facts shared.ResourceFacts
	var err error
	go func() { facts, err = c.Facts(ctx, "user"); close(done) }()
	seen := map[string]bool{}
	for range 3 {
		select {
		case source := <-started:
			seen[source] = true
		case <-ctx.Done():
			t.Fatal("independent live authority sources serialized")
		}
	}
	// Unblock without closing twice; deferred close also handles assertion errors.
	for range 3 {
		release <- struct{}{}
	}
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("facts did not settle")
	}
	if len(seen) != 3 || err != nil || len(facts.WorkspaceIDs) != 1 || facts.WorkspaceIDs[0] != "workspace" {
		t.Fatal("authority intersection or live reads changed")
	}
}
