package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"
)

func TestMediaControlClaudeCodeConfigurationE2E(t *testing.T) {
	authAddr := os.Getenv("MEDIA_AUTH_ADDR")
	controlAddr := os.Getenv("MEDIA_CONTROL_ADDR")
	if authAddr == "" || controlAddr == "" {
		t.Skip("MEDIA_AUTH_ADDR and MEDIA_CONTROL_ADDR are required")
	}
	client := httpClient(t)
	username := fmt.Sprintf("dualagent%d", time.Now().UnixNano())
	status, body := doJSON(t, client, http.MethodPost, authAddr+"/api/auth/register", map[string]string{
		"username": username, "password": "dualagent123", "display_name": "Dual Agent E2E",
	})
	if status != http.StatusCreated {
		t.Fatalf("register E2E user: status=%d body=%s", status, body)
	}
	secret := fmt.Sprintf("sk-ant-e2e-%d", time.Now().UnixNano())

	status, body = doJSON(t, client, http.MethodPost, controlAddr+"/api/v1/workspaces", map[string]string{
		"name": "Claude E2E", "agent_type": "claude-code", "api_key": secret,
	})
	if status != http.StatusCreated {
		t.Fatalf("create Claude workspace: status=%d body=%s", status, body)
	}
	if bytes.Contains(body, []byte(secret)) || bytes.Contains(body, []byte("encrypted_api_key")) {
		t.Fatalf("public response exposed API Key material: %s", body)
	}
	var workspace struct {
		ID                   string `json:"id"`
		AgentType            string `json:"agent_type"`
		APIKeyConfigured     bool   `json:"api_key_configured"`
		ConfigurationVersion int64  `json:"configuration_version"`
	}
	if err := json.Unmarshal(body, &workspace); err != nil {
		t.Fatal(err)
	}
	if workspace.ID == "" || workspace.AgentType != "claude-code" || !workspace.APIKeyConfigured {
		t.Fatalf("unexpected Workspace response: %+v", workspace)
	}

	status, body = doJSON(t, client, http.MethodPut, controlAddr+"/api/v1/workspaces/"+workspace.ID, map[string]string{
		"name": "Claude E2E renamed", "agent_type": "claude-code", "api_key": "",
	})
	if status != http.StatusOK {
		t.Fatalf("update Claude workspace: status=%d body=%s", status, body)
	}
	var updated struct {
		ConfigurationVersion int64 `json:"configuration_version"`
	}
	if err := json.Unmarshal(body, &updated); err != nil {
		t.Fatal(err)
	}
	if updated.ConfigurationVersion != workspace.ConfigurationVersion {
		t.Fatalf("blank API Key advanced configuration version: before=%d after=%d", workspace.ConfigurationVersion, updated.ConfigurationVersion)
	}

	runtimeURL := controlAddr + "/api/v1/workspaces/" + workspace.ID + "/runtime-context"
	status, body = doJSON(t, client, http.MethodGet, runtimeURL, nil)
	if status != http.StatusForbidden || bytes.Contains(body, []byte(secret)) {
		t.Fatalf("public runtime context was not protected: status=%d body=%s", status, body)
	}
	req, err := http.NewRequest(http.MethodGet, runtimeURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Control-Service-Token", envOr("CONTROL_INTERNAL_TOKEN", "dev-control-internal-token"))
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("internal runtime context: status=%d body=%s", resp.StatusCode, body)
	}
	var runtimeContext struct {
		AgentType string `json:"agent_type"`
		APIKey    string `json:"api_key"`
	}
	if err := json.Unmarshal(body, &runtimeContext); err != nil {
		t.Fatal(err)
	}
	if runtimeContext.AgentType != "claude-code" || runtimeContext.APIKey != secret {
		t.Fatalf("runtime context did not retain secret: agent=%q key_matches=%t", runtimeContext.AgentType, runtimeContext.APIKey == secret)
	}

	status, _ = doJSON(t, client, http.MethodPost, controlAddr+"/api/v1/workspaces", map[string]string{
		"name": "Invalid", "agent_type": "goose",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("unsupported Agent Type status=%d, want 400", status)
	}
}
