package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"
)

func TestMediaControlSkillCatalogE2E(t *testing.T) {
	authAddr, controlAddr := os.Getenv("MEDIA_AUTH_ADDR"), os.Getenv("MEDIA_CONTROL_ADDR")
	if authAddr == "" || controlAddr == "" { t.Skip("MEDIA_AUTH_ADDR and MEDIA_CONTROL_ADDR are required") }
	client := httpClient(t)
	login(t, client, authAddr, envOr("MEDIA_AUTH_E2E_USER", "smoke"), envOr("MEDIA_AUTH_E2E_PASS", "smoke123"))
	name := fmt.Sprintf("skill-%d", time.Now().UnixNano())
	status, body := doJSON(t, client, http.MethodPost, controlAddr+"/api/v1/skills", map[string]string{"name": name, "content_path": "skills/demo.md", "checksum": "sha256:demo"})
	if status != http.StatusCreated { t.Fatalf("create skill: status=%d body=%s", status, body) }
	var created struct { ID, Status string; Version int64 }
	if err := json.Unmarshal(body, &created); err != nil { t.Fatal(err) }
	if created.ID == "" || created.Status != "active" || created.Version != 1 { t.Fatalf("unexpected skill: %+v", created) }
	status, body = doJSON(t, client, http.MethodPost, controlAddr+"/api/v1/skills/"+created.ID+"/status", map[string]any{"status": "disabled", "expected_version": 1})
	if status != http.StatusOK || !jsonContains(body, `"status":"disabled"`) { t.Fatalf("disable skill: status=%d body=%s", status, body) }
	status, body = doJSON(t, client, http.MethodPost, controlAddr+"/api/v1/skills/"+created.ID+"/status", map[string]any{"status": "active", "expected_version": 2})
	if status != http.StatusOK || !jsonContains(body, `"status":"active"`) { t.Fatalf("enable skill: status=%d body=%s", status, body) }
	status, body = doJSON(t, client, http.MethodPost, controlAddr+"/api/v1/skills/"+created.ID+"/archive", map[string]any{"expected_version": 3})
	if status != http.StatusOK || !jsonContains(body, `"status":"archived"`) { t.Fatalf("archive skill: status=%d body=%s", status, body) }
}
