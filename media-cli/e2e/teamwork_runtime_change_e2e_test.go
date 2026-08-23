package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestTeamworkRuntimeChangeE2E(t *testing.T) {
	authAddress, controlAddress, gatewayAddress := os.Getenv("MEDIA_AUTH_ADDR"), os.Getenv("MEDIA_CONTROL_ADDR"), os.Getenv("MEDIA_AGENT_GATEWAY_ADDR")
	if authAddress == "" || controlAddress == "" || gatewayAddress == "" {
		t.Skip("MEDIA_AUTH_ADDR, MEDIA_CONTROL_ADDR and MEDIA_AGENT_GATEWAY_ADDR are required")
	}
	client := registerGatewayUser(t, authAddress, "teamworkchange")
	workspaceID, configurationID := createTeamworkWorkspace(t, client, controlAddress, "teamwork-change-secret")
	members := fmt.Sprintf(`[{"member_id":"writer","agent_config_id":%q,"name":"Writer"}]`, configurationID)
	status, body := doJSON(t, client, http.MethodPost, controlAddress+"/api/v1/teamwork-topologies", map[string]string{
		"workspace_id": workspaceID, "name": "runtime-change", "entry_member_id": "writer", "members_json": members, "edges_json": `[]`,
	})
	if status != http.StatusCreated {
		t.Fatalf("create topology: %d %s", status, body)
	}
	var topology struct {
		ID      string `json:"id"`
		Version int64  `json:"version"`
	}
	if err := json.Unmarshal(body, &topology); err != nil || topology.ID == "" || topology.Version != 1 {
		t.Fatalf("topology=%+v err=%v", topology, err)
	}

	// The first accepted Task proves Control's create Outbox reached Gateway.
	probe := awaitTopologyTaskAccepted(t, client, gatewayAddress, workspaceID, topology.ID, 1, "probe")
	awaitTaskStatus(t, client, gatewayAddress, probe, "failed", 15*time.Second)

	queued := createTopologyTask(t, client, gatewayAddress, workspaceID, topology.ID, 1, "stale-queued")
	internalURL := gatewayAddress + "/api/v1/internal/teamwork-topology-events"
	eventID := uuid.NewString()
	event := map[string]any{"event_id": eventID, "workspace_id": workspaceID, "topology_id": topology.ID, "topology_version": 2, "status": "active"}
	status, body = doJSON(t, httpClient(t), http.MethodPost, internalURL, event)
	if status != http.StatusForbidden {
		t.Fatalf("public internal-event request accepted: %d %s", status, body)
	}
	headers := map[string]string{"X-Control-Service-Token": envOr("CONTROL_INTERNAL_TOKEN", "dev-control-internal-token")}
	status, body = doJSONWithHeaders(t, httpClient(t), http.MethodPost, internalURL, event, headers)
	if status != http.StatusOK || !jsonContains(body, `"applied":true`) || !jsonContains(body, `"cancelled_task_count":1`) {
		t.Fatalf("apply version 2: %d %s", status, body)
	}
	awaitTaskStatus(t, client, gatewayAddress, queued, "cancelled", 5*time.Second)

	status, body = doJSON(t, client, http.MethodPost, gatewayAddress+"/api/v1/agent-tasks", topologyTaskRequest(workspaceID, topology.ID, 1, "stale-version"))
	if status != http.StatusConflict {
		t.Fatalf("stale topology version accepted: %d %s", status, body)
	}
	status, body = doJSONWithHeaders(t, httpClient(t), http.MethodPost, internalURL, event, headers)
	if status != http.StatusOK || runtimeChangeApplied(t, body) {
		t.Fatalf("duplicate event was not idempotent: %d %s", status, body)
	}
	late := map[string]any{"event_id": uuid.NewString(), "workspace_id": workspaceID, "topology_id": topology.ID, "topology_version": 1, "status": "active"}
	status, body = doJSONWithHeaders(t, httpClient(t), http.MethodPost, internalURL, late, headers)
	if status != http.StatusOK || runtimeChangeApplied(t, body) {
		t.Fatalf("late event changed projection: %d %s", status, body)
	}

	// Version 2 is already present from the explicit event. Control emits it as a duplicate,
	// then version 3 proves update Outbox delivery advances the Gateway projection.
	for expected := int64(1); expected <= 2; expected++ {
		status, body = doJSON(t, client, http.MethodPut, controlAddress+"/api/v1/teamwork-topologies/"+topology.ID, map[string]any{
			"name": fmt.Sprintf("runtime-change-v%d", expected+1), "entry_member_id": "writer", "members_json": members, "edges_json": `[]`, "expected_version": expected,
		})
		if status != http.StatusOK {
			t.Fatalf("update topology from version %d: %d %s", expected, status, body)
		}
	}
	_ = awaitTopologyTaskAccepted(t, client, gatewayAddress, workspaceID, topology.ID, 3, "control-update")
	status, body = doJSON(t, client, http.MethodPost, gatewayAddress+"/api/v1/agent-tasks", topologyTaskRequest(workspaceID, topology.ID, 2, "version-two-after-three"))
	if status != http.StatusConflict {
		t.Fatalf("version 2 admitted after version 3: %d %s", status, body)
	}
}

func runtimeChangeApplied(t *testing.T, body []byte) bool {
	t.Helper()
	var response struct {
		Applied            bool  `json:"applied"`
		CancelledTaskCount int32 `json:"cancelled_task_count"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatalf("decode runtime change response: %v body=%s", err, body)
	}
	if !response.Applied && response.CancelledTaskCount != 0 {
		t.Fatalf("unapplied event cancelled tasks: %+v", response)
	}
	return response.Applied
}

func topologyTaskRequest(workspaceID, topologyID string, version int64, suffix string) map[string]any {
	return map[string]any{
		"workspace_id": workspaceID, "source": "teamwork-e2e", "source_resource_id": suffix,
		"idempotency_key": fmt.Sprintf("%s-%d", suffix, time.Now().UnixNano()), "input_json": `{}`,
		"topology_id": topologyID, "topology_version": version,
	}
}

func createTopologyTask(t *testing.T, client *http.Client, gatewayAddress, workspaceID, topologyID string, version int64, suffix string) string {
	t.Helper()
	status, body := doJSON(t, client, http.MethodPost, gatewayAddress+"/api/v1/agent-tasks", topologyTaskRequest(workspaceID, topologyID, version, suffix))
	if status != http.StatusCreated {
		t.Fatalf("create topology task: %d %s", status, body)
	}
	var task struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &task); err != nil || task.ID == "" {
		t.Fatalf("task=%+v err=%v", task, err)
	}
	return task.ID
}

func awaitTopologyTaskAccepted(t *testing.T, client *http.Client, gatewayAddress, workspaceID, topologyID string, version int64, suffix string) string {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for {
		status, body := doJSON(t, client, http.MethodPost, gatewayAddress+"/api/v1/agent-tasks", topologyTaskRequest(workspaceID, topologyID, version, suffix))
		if status == http.StatusCreated {
			var task struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal(body, &task); err != nil || task.ID == "" {
				t.Fatalf("task=%+v err=%v", task, err)
			}
			return task.ID
		}
		if status != http.StatusConflict || time.Now().After(deadline) {
			t.Fatalf("topology version %d was not admitted: %d %s", version, status, body)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func awaitTaskStatus(t *testing.T, client *http.Client, gatewayAddress, taskID, expected string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		status, body := doJSON(t, client, http.MethodGet, gatewayAddress+"/api/v1/agent-tasks/"+taskID, nil)
		if status != http.StatusOK {
			t.Fatalf("get task: %d %s", status, body)
		}
		var task struct {
			Status    string `json:"status"`
			ErrorCode string `json:"error_code"`
		}
		if err := json.Unmarshal(body, &task); err != nil {
			t.Fatal(err)
		}
		if task.Status == expected {
			if expected == "cancelled" && task.ErrorCode != "topology_version_superseded" {
				t.Fatalf("cancel reason=%q body=%s", task.ErrorCode, body)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("task status=%q, want %q: %s", task.Status, expected, body)
		}
		time.Sleep(100 * time.Millisecond)
	}
}
