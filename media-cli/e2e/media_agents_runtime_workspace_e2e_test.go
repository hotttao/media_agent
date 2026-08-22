package e2e

import (
	"encoding/json"
	"net/http"
	"os"
	"testing"
	"time"
)

func TestMediaAgentsRuntimeWorkspaceE2E(t *testing.T) {
	agentAddr := os.Getenv("MEDIA_AGENTS_ADDR")
	wantWorkspaceID := os.Getenv("MEDIA_AGENTS_WORKSPACE_ID")
	if agentAddr == "" || wantWorkspaceID == "" {
		t.Skip("MEDIA_AGENTS_ADDR and MEDIA_AGENTS_WORKSPACE_ID are required")
	}
	client := &http.Client{Timeout: 10 * time.Second}
	status, body := doJSON(t, client, http.MethodGet, agentAddr+"/api/v1/runtime-workspace", nil)
	if status != http.StatusOK {
		t.Fatalf("runtime workspace: status=%d body=%s", status, body)
	}
	var identity struct {
		WorkspaceID string `json:"workspace_id"`
		StorageID   string `json:"storage_id"`
		MountPath   string `json:"mount_path"`
	}
	if err := json.Unmarshal(body, &identity); err != nil {
		t.Fatal(err)
	}
	if identity.WorkspaceID != wantWorkspaceID || identity.StorageID == "" || identity.MountPath != "/workspace" {
		t.Fatalf("unexpected runtime workspace: %+v", identity)
	}
	if wantStorageID := os.Getenv("MEDIA_AGENTS_STORAGE_ID"); wantStorageID != "" && identity.StorageID != wantStorageID {
		t.Fatalf("storage identity changed: got=%s want=%s", identity.StorageID, wantStorageID)
	}
}
