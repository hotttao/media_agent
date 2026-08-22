package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func TestMediaControlTeamworkRuntimeContextE2E(t *testing.T) {
	authAddress, controlAddress := os.Getenv("MEDIA_AUTH_ADDR"), os.Getenv("MEDIA_CONTROL_ADDR")
	if authAddress == "" || controlAddress == "" {
		t.Skip("MEDIA_AUTH_ADDR and MEDIA_CONTROL_ADDR are required")
	}
	client, ownerID := registerAdminWorkspaceUser(t, authAddress, "teamwork-runtime")
	foreignClient, _ := registerAdminWorkspaceUser(t, authAddress, "teamwork-runtime-foreign")
	workspaceID, configurationID := createTeamworkWorkspace(t, client, controlAddress, "teamwork-runtime-secret")
	_, foreignConfigurationID := createTeamworkWorkspace(t, foreignClient, controlAddress, "foreign-secret")

	foreignMembers := fmt.Sprintf(`[{"member_id":"writer","agent_config_id":%q,"name":"Writer"}]`, foreignConfigurationID)
	status, body := doJSON(t, client, http.MethodPost, controlAddress+"/api/v1/teamwork-topologies", map[string]string{
		"workspace_id": workspaceID, "name": "invalid", "entry_member_id": "writer", "members_json": foreignMembers, "edges_json": `[]`,
	})
	if status != http.StatusBadRequest {
		t.Fatalf("foreign configuration accepted: %d %s", status, body)
	}

	members := fmt.Sprintf(`[{"member_id":"writer","agent_config_id":%q,"name":"Writer"},{"member_id":"reviewer","agent_config_id":%q,"name":"Reviewer"}]`, configurationID, configurationID)
	edges := `[{"from_member_id":"writer","to_member_id":"reviewer","mode":"delegate"}]`
	status, body = doJSON(t, client, http.MethodPost, controlAddress+"/api/v1/teamwork-topologies", map[string]string{
		"workspace_id": workspaceID, "name": "content-review", "entry_member_id": "writer", "members_json": members, "edges_json": edges,
	})
	if status != http.StatusCreated {
		t.Fatalf("create topology: %d %s", status, body)
	}
	var topology struct {
		ID      string `json:"id"`
		Version int64  `json:"version"`
	}
	if err := json.Unmarshal(body, &topology); err != nil || topology.ID == "" || topology.Version != 1 {
		t.Fatalf("decode topology: %+v err=%v", topology, err)
	}

	status, body = doJSON(t, client, http.MethodGet, controlAddress+"/api/v1/teamwork-topologies/"+topology.ID+"/runtime-context", nil)
	if status != http.StatusForbidden {
		t.Fatalf("ordinary session read internal context: %d %s", status, body)
	}

	headers := map[string]string{
		"X-Control-Service-Token":  envOr("CONTROL_INTERNAL_TOKEN", "dev-control-internal-token"),
		"X-Control-Acting-User-ID": ownerID,
	}
	status, body = doJSONWithHeaders(t, httpClient(t), http.MethodGet, controlAddress+"/api/v1/teamwork-topologies/"+topology.ID+"/runtime-context", nil, headers)
	if status != http.StatusOK {
		t.Fatalf("runtime context: %d %s", status, body)
	}
	var contextResponse struct {
		TopologyID      string `json:"topology_id"`
		WorkspaceID     string `json:"workspace_id"`
		TopologyVersion int64  `json:"topology_version"`
		EntryMemberID   string `json:"entry_member_id"`
		MembersJSON     string `json:"members_json"`
		EdgesJSON       string `json:"edges_json"`
	}
	if err := json.Unmarshal(body, &contextResponse); err != nil || contextResponse.TopologyID != topology.ID || contextResponse.WorkspaceID != workspaceID || contextResponse.TopologyVersion != 1 || contextResponse.EntryMemberID != "writer" || !strings.Contains(contextResponse.MembersJSON, configurationID) || !strings.Contains(contextResponse.EdgesJSON, "delegate") {
		t.Fatalf("unexpected runtime context: %+v err=%v", contextResponse, err)
	}
	for _, forbidden := range []string{"teamwork-runtime-secret", "foreign-secret", "api_key", "encrypted_api_key"} {
		if strings.Contains(string(body), forbidden) {
			t.Fatalf("runtime context leaked %q: %s", forbidden, body)
		}
	}
}

func createTeamworkWorkspace(t *testing.T, client *http.Client, controlAddress, apiKey string) (string, string) {
	t.Helper()
	status, body := doJSON(t, client, http.MethodPost, controlAddress+"/api/v1/workspaces", map[string]string{
		"name": fmt.Sprintf("teamwork-runtime-%d", time.Now().UnixNano()), "agent_type": "codex", "api_key": apiKey,
	})
	if status != http.StatusCreated {
		t.Fatalf("create teamwork workspace: %d %s", status, body)
	}
	var response struct {
		ID                   string `json:"id"`
		AgentConfigurationID string `json:"agent_configuration_id"`
	}
	if err := json.Unmarshal(body, &response); err != nil || response.ID == "" || response.AgentConfigurationID == "" {
		t.Fatalf("decode teamwork workspace: %+v err=%v body=%s", response, err, body)
	}
	return response.ID, response.AgentConfigurationID
}
