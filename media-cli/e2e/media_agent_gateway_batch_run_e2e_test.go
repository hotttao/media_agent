package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"
)

func TestMediaAgentGatewayBatchRunE2E(t *testing.T) {
	authAddr, gatewayAddr, controlAddr := os.Getenv("MEDIA_AUTH_ADDR"), os.Getenv("MEDIA_AGENT_GATEWAY_ADDR"), os.Getenv("MEDIA_CONTROL_ADDR")
	if authAddr == "" || gatewayAddr == "" || controlAddr == "" {
		t.Skip("MEDIA_AUTH_ADDR, MEDIA_AGENT_GATEWAY_ADDR and MEDIA_CONTROL_ADDR are required")
	}
	owner := registerGatewayUser(t, authAddr, "batch-owner")
	status, body := doJSON(t, owner, http.MethodPost, controlAddr+"/api/v1/workspaces", map[string]string{"name": fmt.Sprintf("batch-%d", time.Now().UnixNano()), "agent_type": "claude-code"})
	if status != http.StatusCreated {
		t.Fatalf("workspace: %d %s", status, body)
	}
	var workspace struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &workspace); err != nil {
		t.Fatal(err)
	}
	status, body = doJSON(t, owner, http.MethodPost, gatewayAddr+"/api/chat/sessions", map[string]string{"workspace_id": workspace.ID, "title": "Batch E2E"})
	if status != http.StatusCreated {
		t.Fatalf("session: %d %s", status, body)
	}
	var session struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &session); err != nil {
		t.Fatal(err)
	}

	request := map[string]any{"workspace_id": workspace.ID, "idempotency_key": "batch-key", "items": []map[string]string{
		{"item_key": "one", "session_id": session.ID, "input_json": `{"message":"one"}`},
		{"item_key": "two", "session_id": session.ID, "input_json": `{"message":"two"}`},
	}}
	status, body = doJSON(t, owner, http.MethodPost, gatewayAddr+"/api/v1/batch-runs", request)
	if status != http.StatusCreated {
		t.Fatalf("create Batch Run: %d %s", status, body)
	}
	var batch struct {
		ID, Status     string
		ItemCount      int32 `json:"item_count"`
		NextCursor     int32 `json:"next_cursor"`
		CompletedCount int32 `json:"completed_count"`
		CancelledCount int32 `json:"cancelled_count"`
		Items          []struct {
			ItemKey, Status string
			AgentTaskID     string `json:"agent_task_id"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &batch); err != nil {
		t.Fatal(err)
	}
	if batch.ID == "" || batch.Status != "creating" || batch.ItemCount != 2 || len(batch.Items) != 2 {
		t.Fatalf("batch=%+v", batch)
	}
	status, body = doJSON(t, owner, http.MethodPost, gatewayAddr+"/api/v1/batch-runs", request)
	if status != http.StatusCreated {
		t.Fatalf("idempotent Batch Run: %d %s", status, body)
	}
	var repeated struct{ ID string }
	if err := json.Unmarshal(body, &repeated); err != nil {
		t.Fatal(err)
	}
	if repeated.ID != batch.ID {
		t.Fatalf("idempotent ID=%q want=%q", repeated.ID, batch.ID)
	}

	other := registerGatewayUser(t, authAddr, "batch-other")
	status, body = doJSON(t, other, http.MethodGet, gatewayAddr+"/api/v1/batch-runs/"+batch.ID, nil)
	if status != http.StatusForbidden {
		t.Fatalf("cross-user get: %d %s", status, body)
	}
	deadline := time.Now().Add(30 * time.Second)
	for batch.Status != "completed" && time.Now().Before(deadline) {
		time.Sleep(250 * time.Millisecond)
		status, body = doJSON(t, owner, http.MethodGet, gatewayAddr+"/api/v1/batch-runs/"+batch.ID, nil)
		if status != http.StatusOK {
			t.Fatalf("get dispatched Batch Run: %d %s", status, body)
		}
		if err := json.Unmarshal(body, &batch); err != nil {
			t.Fatal(err)
		}
		if batch.Status == "failed" || batch.Status == "partial_failed" || batch.Status == "cancelled" {
			t.Fatalf("Batch Run ended as %s: %s", batch.Status, body)
		}
	}
	if batch.Status != "completed" || batch.CompletedCount != 2 || batch.NextCursor != 2 || batch.Items[0].AgentTaskID == "" || batch.Items[1].AgentTaskID == "" {
		t.Fatalf("Batch Run did not converge: %+v", batch)
	}

	cancelRequest := map[string]any{"workspace_id": workspace.ID, "idempotency_key": "batch-cancel-key", "items": []map[string]string{
		{"item_key": "cancel-one", "session_id": session.ID, "input_json": `{"message":"cancel one"}`},
		{"item_key": "cancel-two", "session_id": session.ID, "input_json": `{"message":"cancel two"}`},
	}}
	status, body = doJSON(t, owner, http.MethodPost, gatewayAddr+"/api/v1/batch-runs", cancelRequest)
	if status != http.StatusCreated {
		t.Fatalf("create cancellable Batch Run: %d %s", status, body)
	}
	var cancellable struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &cancellable); err != nil {
		t.Fatal(err)
	}
	status, body = doJSON(t, owner, http.MethodPost, gatewayAddr+"/api/v1/batch-runs/"+cancellable.ID+"/cancel", map[string]any{})
	if status != http.StatusOK || !jsonContains(body, `"status":"cancelled"`) {
		t.Fatalf("cancel: %d %s", status, body)
	}
	deadline = time.Now().Add(15 * time.Second)
	for !jsonContains(body, `"cancelled_count":2`) && time.Now().Before(deadline) {
		time.Sleep(250 * time.Millisecond)
		status, body = doJSON(t, owner, http.MethodGet, gatewayAddr+"/api/v1/batch-runs/"+cancellable.ID, nil)
		if status != http.StatusOK {
			t.Fatalf("get cancelled Batch Run: %d %s", status, body)
		}
	}
	if !jsonContains(body, `"cancelled_count":2`) {
		t.Fatalf("cancel did not converge: %s", body)
	}
}
