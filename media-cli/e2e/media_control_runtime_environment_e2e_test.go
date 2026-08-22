package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"
)

func TestMediaControlRuntimeEnvironmentE2E(t *testing.T) {
	a, ctrl := os.Getenv("MEDIA_AUTH_ADDR"), os.Getenv("MEDIA_CONTROL_ADDR")
	if a == "" || ctrl == "" {
		t.Skip("MEDIA_AUTH_ADDR and MEDIA_CONTROL_ADDR are required")
	}
	c := httpClient(t)
	login(t, c, a, envOr("MEDIA_AUTH_E2E_USER", "smoke"), envOr("MEDIA_AUTH_E2E_PASS", "smoke123"))
	s, b := doJSON(t, c, http.MethodPost, ctrl+"/api/v1/runtime-environments", map[string]string{"name": fmt.Sprintf("runtime-%d", time.Now().UnixNano()), "endpoint": "http://runtime-controller", "capacity_json": `{"cpu":8}`})
	if s != http.StatusCreated {
		t.Fatalf("environment: %d %s", s, b)
	}
	var env struct {
		ID      string
		Status  string
		Version int64
	}
	if err := json.Unmarshal(b, &env); err != nil {
		t.Fatal(err)
	}
	if env.ID == "" || env.Status != "active" || env.Version != 1 {
		t.Fatalf("unexpected environment: %+v", env)
	}
	s, b = doJSON(t, c, http.MethodPost, ctrl+"/api/v1/workspaces", map[string]string{"name": fmt.Sprintf("runtime-ws-%d", time.Now().UnixNano()), "agent_type": "codex"})
	if s != http.StatusCreated {
		t.Fatalf("workspace: %d %s", s, b)
	}
	var ws struct{ ID string }
	if err := json.Unmarshal(b, &ws); err != nil {
		t.Fatal(err)
	}
	s, b = doJSON(t, c, http.MethodPost, ctrl+"/api/v1/workspaces/"+ws.ID+"/placement/reconcile", map[string]any{})
	if s != http.StatusOK {
		t.Fatalf("reconcile: %d %s", s, b)
	}
	var placement struct {
		Status        string `json:"status"`
		EnvironmentID string `json:"environment_id"`
		VolumeRef     string `json:"volume_ref"`
	}
	if err := json.Unmarshal(b, &placement); err != nil {
		t.Fatal(err)
	}
	if placement.Status != "placed" || placement.EnvironmentID != env.ID || placement.VolumeRef != "/workspace/"+ws.ID {
		t.Fatalf("unexpected placement: %+v body=%s", placement, b)
	}
	s, b = doJSON(t, c, http.MethodPost, ctrl+"/api/v1/workspaces/"+ws.ID+"/runtime-directory/ensure", map[string]any{})
	if s != http.StatusOK || !jsonContains(b, `"volume_ref":"/workspace/`) {
		t.Fatalf("ensure runtime directory: %d %s", s, b)
	}
}
