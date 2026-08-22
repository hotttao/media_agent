package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"
)

func TestMediaControlAgentTemplateCatalogE2E(t *testing.T) {
	authAddr, controlAddr := os.Getenv("MEDIA_AUTH_ADDR"), os.Getenv("MEDIA_CONTROL_ADDR")
	if authAddr == "" || controlAddr == "" { t.Skip("MEDIA_AUTH_ADDR and MEDIA_CONTROL_ADDR are required") }
	client := httpClient(t)
	login(t, client, authAddr, envOr("MEDIA_AUTH_E2E_USER", "smoke"), envOr("MEDIA_AUTH_E2E_PASS", "smoke123"))
	name := fmt.Sprintf("template-%d", time.Now().UnixNano())
	status, body := doJSON(t, client, http.MethodPost, controlAddr+"/api/v1/agent-templates", map[string]string{"name": name, "configuration_json": `{"model":"gpt"}`})
	if status != http.StatusCreated { t.Fatalf("create template: status=%d body=%s", status, body) }
	var created struct { ID, Status string; Version int64 }
	if err := json.Unmarshal(body, &created); err != nil { t.Fatal(err) }
	if created.ID == "" || created.Status != "active" || created.Version != 1 { t.Fatalf("unexpected template: %+v", created) }
	status, body = doJSON(t, client, http.MethodPut, controlAddr+"/api/v1/agent-templates/"+created.ID, map[string]any{"name": name, "configuration_json": `{"model":"claude"}`, "expected_version": 1})
	if status != http.StatusOK { t.Fatalf("update template: status=%d body=%s", status, body) }
	var updated struct { Version int64 }
	_ = json.Unmarshal(body, &updated)
	if updated.Version != 2 { t.Fatalf("updated version=%d", updated.Version) }
	status, _ = doJSON(t, client, http.MethodPut, controlAddr+"/api/v1/agent-templates/"+created.ID, map[string]any{"name": name, "configuration_json": `{}`, "expected_version": 1})
	if status != http.StatusConflict { t.Fatalf("stale update status=%d, want 409", status) }
	status, body = doJSON(t, client, http.MethodPost, controlAddr+"/api/v1/agent-templates/"+created.ID+"/archive", map[string]any{"expected_version": 2})
	if status != http.StatusOK || !jsonContains(body, `"status":"archived"`) { t.Fatalf("archive template: status=%d body=%s", status, body) }
}
