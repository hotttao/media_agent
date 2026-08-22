package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

func TestMediaControlAdminWorkspaceE2E(t *testing.T) {
	authAddress, controlAddress := os.Getenv("MEDIA_AUTH_ADDR"), os.Getenv("MEDIA_CONTROL_ADDR")
	if authAddress == "" || controlAddress == "" {
		t.Skip("MEDIA_AUTH_ADDR and MEDIA_CONTROL_ADDR are required")
	}
	firstClient, firstOwner := registerAdminWorkspaceUser(t, authAddress, "admin-workspace-first")
	secondClient, _ := registerAdminWorkspaceUser(t, authAddress, "admin-workspace-second")
	firstWorkspace := createAdminWorkspaceFixture(t, firstClient, controlAddress, "codex", "admin-query-secret-one")
	secondWorkspace := createAdminWorkspaceFixture(t, secondClient, controlAddress, "claude-code", "admin-query-secret-two")
	status, body := doJSON(t, firstClient, http.MethodGet, controlAddress+"/api/v1/admin/workspaces", nil)
	if status != http.StatusForbidden {
		t.Fatalf("ordinary user accepted: %d %s", status, body)
	}

	_, _ = doJSON(t, firstClient, http.MethodPost, controlAddress+"/api/v1/workspaces/"+firstWorkspace+"/placement/reconcile", map[string]any{})
	adminHeaders := map[string]string{
		"X-Control-Service-Token":  envOr("CONTROL_INTERNAL_TOKEN", "dev-control-internal-token"),
		"X-Control-Acting-User-ID": "e2e-admin-workspace",
		"X-Control-Acting-Role":    "admin",
	}
	query := url.Values{
		"owner_user_id": []string{firstOwner},
		"desired_state": []string{"running"},
		"agent_type":    []string{"codex"},
		"page_size":     []string{"10"},
		"page_offset":   []string{"0"},
	}
	status, body = doJSONWithHeaders(t, httpClient(t), http.MethodGet, controlAddress+"/api/v1/admin/workspaces?"+query.Encode(), nil, adminHeaders)
	if status != http.StatusOK {
		t.Fatalf("admin workspace list: %d %s", status, body)
	}
	var response struct {
		Workspaces []struct {
			WorkspaceID     string `json:"workspace_id"`
			OwnerUserID     string `json:"owner_user_id"`
			AgentType       string `json:"agent_type"`
			PlacementStatus string `json:"placement_status"`
			ContextVersion  int64  `json:"context_version"`
		} `json:"workspaces"`
		Total      int64 `json:"total"`
		PageSize   int32 `json:"page_size"`
		PageOffset int32 `json:"page_offset"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatal(err)
	}
	if response.Total != 1 || len(response.Workspaces) != 1 || response.Workspaces[0].WorkspaceID != firstWorkspace || response.Workspaces[0].OwnerUserID != firstOwner || response.Workspaces[0].AgentType != "codex" || response.Workspaces[0].ContextVersion < 1 || response.Workspaces[0].PlacementStatus == "" {
		t.Fatalf("unexpected admin workspace projection: %+v", response)
	}
	unsafeBody := string(body)
	for _, forbidden := range []string{"admin-query-secret-one", "admin-query-secret-two", "encrypted_api_key", secondWorkspace} {
		if strings.Contains(unsafeBody, forbidden) {
			t.Fatalf("admin response leaked or crossed filter %q: %s", forbidden, body)
		}
	}
	status, body = doJSONWithHeaders(t, httpClient(t), http.MethodGet, controlAddress+"/api/v1/admin/workspaces?page_size=201", nil, adminHeaders)
	if status != http.StatusBadRequest {
		t.Fatalf("invalid pagination accepted: %d %s", status, body)
	}
}

func registerAdminWorkspaceUser(t *testing.T, authAddress, prefix string) (*http.Client, string) {
	t.Helper()
	client := httpClient(t)
	username := fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	status, body := doJSON(t, client, http.MethodPost, authAddress+"/api/auth/register", map[string]string{"username": username, "password": "workspace123", "display_name": "Admin Workspace E2E"})
	if status != http.StatusCreated {
		t.Fatalf("register %s: %d %s", prefix, status, body)
	}
	var response struct {
		User struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	if err := json.Unmarshal(body, &response); err != nil || response.User.ID == "" {
		t.Fatalf("decode registered user: %+v err=%v", response, err)
	}
	return client, response.User.ID
}

func createAdminWorkspaceFixture(t *testing.T, client *http.Client, controlAddress, agentType, apiKey string) string {
	t.Helper()
	status, body := doJSON(t, client, http.MethodPost, controlAddress+"/api/v1/workspaces", map[string]string{"name": fmt.Sprintf("admin-query-%d", time.Now().UnixNano()), "agent_type": agentType, "api_key": apiKey})
	if status != http.StatusCreated {
		t.Fatalf("create workspace: %d %s", status, body)
	}
	var response struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &response); err != nil || response.ID == "" {
		t.Fatalf("decode workspace: %+v err=%v", response, err)
	}
	return response.ID
}
