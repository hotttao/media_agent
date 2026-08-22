package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"
)

func TestMediaControlScheduleE2E(t *testing.T) {
	a, ctrl := os.Getenv("MEDIA_AUTH_ADDR"), os.Getenv("MEDIA_CONTROL_ADDR")
	if a == "" || ctrl == "" {
		t.Skip("MEDIA_AUTH_ADDR and MEDIA_CONTROL_ADDR are required")
	}
	c := httpClient(t)
	login(t, c, a, envOr("MEDIA_AUTH_E2E_USER", "smoke"), envOr("MEDIA_AUTH_E2E_PASS", "smoke123"))
	s, b := doJSON(t, c, http.MethodPost, ctrl+"/api/v1/workspaces", map[string]string{"name": fmt.Sprintf("schedule-%d", time.Now().UnixNano()), "agent_type": "codex"})
	if s != http.StatusCreated {
		t.Fatalf("workspace: %d %s", s, b)
	}
	var w struct{ ID string }
	json.Unmarshal(b, &w)
	s, b = doJSON(t, c, http.MethodPost, ctrl+"/api/v1/schedules", map[string]string{"workspace_id": w.ID, "agent_config_id": "default", "rule": "0 * * * *", "timezone": "UTC"})
	if s != http.StatusCreated {
		t.Fatalf("schedule: %d %s", s, b)
	}
	var v struct {
		ID      string
		Version int64
	}
	json.Unmarshal(b, &v)
	s, b = doJSON(t, c, http.MethodPost, ctrl+"/api/v1/schedules/"+v.ID+"/run-now", map[string]any{})
	if s != http.StatusCreated || !jsonContains(b, "\"status\":\"pending\"") {
		t.Fatalf("run now: %d %s", s, b)
	}
	var trigger struct {
		ID    string
		DueAt int64
	}
	json.Unmarshal(b, &trigger)
	s, b = doJSON(t, c, http.MethodPost, ctrl+"/api/v1/schedule-triggers/"+trigger.ID+"/submission", map[string]string{"status": "submitted", "agent_task_id": "task-1"})
	if s != http.StatusOK || !jsonContains(b, "\"status\":\"submitted\"") {
		t.Fatalf("submission: %d %s", s, b)
	}
	s, b = doJSON(t, c, http.MethodPost, ctrl+"/api/v1/schedules/"+v.ID+"/claim-due", map[string]any{"due_at": trigger.DueAt + 60000})
	if s != http.StatusCreated || !jsonContains(b, "\"status\":\"pending\"") {
		t.Fatalf("claim due: %d %s", s, b)
	}
}
