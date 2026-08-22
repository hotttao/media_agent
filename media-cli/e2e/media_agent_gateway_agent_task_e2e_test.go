package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"
)

func TestMediaAgentGatewayAgentTaskE2E(t *testing.T) {
	authAddr, gatewayAddr, controlAddr := os.Getenv("MEDIA_AUTH_ADDR"), os.Getenv("MEDIA_AGENT_GATEWAY_ADDR"), os.Getenv("MEDIA_CONTROL_ADDR")
	if authAddr == "" || gatewayAddr == "" || controlAddr == "" {
		t.Skip("MEDIA_AUTH_ADDR, MEDIA_AGENT_GATEWAY_ADDR and MEDIA_CONTROL_ADDR are required")
	}
	owner := registerGatewayUser(t, authAddr, "task-owner")
	status, body := doJSON(t, owner, http.MethodPost, controlAddr+"/api/v1/workspaces", map[string]string{
		"name": fmt.Sprintf("task-%d", time.Now().UnixNano()), "agent_type": "claude-code", "api_key": "task-e2e-secret",
	})
	if status != http.StatusCreated {
		t.Fatalf("workspace: %d %s", status, body)
	}
	var workspace struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &workspace); err != nil {
		t.Fatal(err)
	}
	status, body = doJSON(t, owner, http.MethodPost, gatewayAddr+"/api/chat/sessions", map[string]string{"workspace_id": workspace.ID, "title": "Task E2E"})
	if status != http.StatusCreated {
		t.Fatalf("session: %d %s", status, body)
	}
	var session struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &session); err != nil {
		t.Fatal(err)
	}

	request := map[string]string{
		"workspace_id": workspace.ID, "session_id": session.ID, "source": "manual",
		"source_resource_id": "request-complete", "idempotency_key": "task-key-complete",
		"input_json": `{"message":"hello-through-task"}`,
	}
	status, body = doJSON(t, owner, http.MethodPost, gatewayAddr+"/api/v1/agent-tasks", request)
	if status != http.StatusCreated {
		t.Fatalf("create task: %d %s", status, body)
	}
	var task struct {
		ID      string `json:"id"`
		Status  string `json:"status"`
		Attempt int32  `json:"attempt"`
	}
	if err := json.Unmarshal(body, &task); err != nil {
		t.Fatal(err)
	}
	if task.ID == "" || task.Status != "queued" || task.Attempt != 0 {
		t.Fatalf("unexpected task: %+v", task)
	}
	status, body = doJSON(t, owner, http.MethodPost, gatewayAddr+"/api/v1/agent-tasks", request)
	if status != http.StatusCreated {
		t.Fatalf("idempotent create: %d %s", status, body)
	}
	var repeated struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &repeated); err != nil {
		t.Fatal(err)
	}
	if repeated.ID != task.ID {
		t.Fatalf("idempotency returned %q, want %q", repeated.ID, task.ID)
	}

	other := registerGatewayUser(t, authAddr, "task-other")
	status, body = doJSON(t, other, http.MethodGet, gatewayAddr+"/api/v1/agent-tasks/"+task.ID, nil)
	if status != http.StatusForbidden {
		t.Fatalf("cross-user get: %d %s", status, body)
	}

	deadline := time.Now().Add(30 * time.Second)
	for task.Status != "completed" && time.Now().Before(deadline) {
		time.Sleep(250 * time.Millisecond)
		status, body = doJSON(t, owner, http.MethodGet, gatewayAddr+"/api/v1/agent-tasks/"+task.ID, nil)
		if status != http.StatusOK {
			t.Fatalf("get dispatched task: %d %s", status, body)
		}
		if err := json.Unmarshal(body, &task); err != nil {
			t.Fatal(err)
		}
		if task.Status == "failed" || task.Status == "cancelled" {
			t.Fatalf("dispatched task ended as %s: %s", task.Status, body)
		}
	}
	if task.Status != "completed" || task.Attempt < 1 {
		t.Fatalf("task did not converge: %+v", task)
	}
	status, body = doJSON(t, owner, http.MethodGet, gatewayAddr+"/api/chat/sessions/"+session.ID+"/messages", nil)
	if status != http.StatusOK || !bytes.Contains(body, []byte("Claude fixture: hello-through-task")) || bytes.Contains(body, []byte("task-e2e-secret")) {
		t.Fatalf("task message history: %d %s", status, body)
	}

	cancelRequest := map[string]string{
		"workspace_id": workspace.ID, "session_id": session.ID, "source": "manual",
		"source_resource_id": "request-cancel", "idempotency_key": "task-key-cancel", "input_json": `{"message":"cancel-me"}`,
	}
	status, body = doJSON(t, owner, http.MethodPost, gatewayAddr+"/api/v1/agent-tasks", cancelRequest)
	if status != http.StatusCreated {
		t.Fatalf("create cancellable task: %d %s", status, body)
	}
	var cancellable struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &cancellable); err != nil {
		t.Fatal(err)
	}
	status, body = doJSON(t, owner, http.MethodPost, gatewayAddr+"/api/v1/agent-tasks/"+cancellable.ID+"/cancel", map[string]any{})
	if status != http.StatusOK || !jsonContains(body, `"status":"cancelled"`) {
		t.Fatalf("cancel: %d %s", status, body)
	}
}
