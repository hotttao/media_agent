package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"
)

func TestMediaControlProviderCatalogE2E(t *testing.T) {
	authAddr, controlAddr := os.Getenv("MEDIA_AUTH_ADDR"), os.Getenv("MEDIA_CONTROL_ADDR")
	if authAddr == "" || controlAddr == "" { t.Skip("MEDIA_AUTH_ADDR and MEDIA_CONTROL_ADDR are required") }
	client := httpClient(t)
	login(t, client, authAddr, envOr("MEDIA_AUTH_E2E_USER", "smoke"), envOr("MEDIA_AUTH_E2E_PASS", "smoke123"))
	name := fmt.Sprintf("provider-%d", time.Now().UnixNano())
	status, body := doJSON(t, client, http.MethodPost, controlAddr+"/api/v1/providers", map[string]string{"name": name, "provider_type": "model", "endpoint": "https://provider.example", "capabilities_json": `{"models":["demo"]}`})
	if status != http.StatusCreated { t.Fatalf("create provider: status=%d body=%s", status, body) }
	var created struct { ID, Status string; Version int64 }
	if err := json.Unmarshal(body, &created); err != nil { t.Fatal(err) }
	if created.ID == "" || created.Status != "active" || created.Version != 1 { t.Fatalf("unexpected provider: %+v", created) }
	status, body = doJSON(t, client, http.MethodPost, controlAddr+"/api/v1/providers/"+created.ID+"/status", map[string]any{"status":"disabled","expected_version":1})
	if status != http.StatusOK || !jsonContains(body, `"status":"disabled"`) { t.Fatalf("disable provider: status=%d body=%s", status, body) }
	status, _ = doJSON(t, client, http.MethodPut, controlAddr+"/api/v1/providers/"+created.ID, map[string]any{"name":name,"provider_type":"model","endpoint":"https://provider.example/v2","capabilities_json":`{}`,"expected_version":1})
	if status != http.StatusConflict { t.Fatalf("stale provider update status=%d", status) }
}
