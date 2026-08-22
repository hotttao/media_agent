package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func TestMediaControlClusterProjectionE2E(t *testing.T) {
	authAddress, controlAddress := os.Getenv("MEDIA_AUTH_ADDR"), os.Getenv("MEDIA_CONTROL_ADDR")
	if authAddress == "" || controlAddress == "" {
		t.Skip("MEDIA_AUTH_ADDR and MEDIA_CONTROL_ADDR are required")
	}
	clusterID := fmt.Sprintf("e2e-cluster-%d", time.Now().UnixNano())
	ordinary := registerGatewayUser(t, authAddress, "cluster-reader")
	status, body := doJSON(t, ordinary, http.MethodGet, controlAddress+"/api/v1/admin/clusters", nil)
	if status != http.StatusForbidden {
		t.Fatalf("ordinary reader accepted: %d %s", status, body)
	}

	internalToken := envOr("CONTROL_INTERNAL_TOKEN", "dev-control-internal-token")
	adminHeaders := map[string]string{"X-Control-Service-Token": internalToken, "X-Control-Acting-User-ID": "e2e-admin", "X-Control-Acting-Role": "admin"}
	systemHeaders := map[string]string{"X-Control-Service-Token": internalToken, "X-Control-Acting-User-ID": "e2e-observer", "X-Control-Acting-Role": "system"}
	admin, system := httpClient(t), httpClient(t)
	reportURL := controlAddress + "/api/v1/admin/clusters/" + clusterID + "/report"
	observedAt := time.Now().UTC().UnixMilli()
	report := map[string]any{"status": "online", "capacity_json": `{"cpu":8}`, "observed_at": observedAt}
	status, body = doJSONWithHeaders(t, admin, http.MethodPut, reportURL, report, adminHeaders)
	if status != http.StatusForbidden {
		t.Fatalf("non-system reporter accepted: %d %s", status, body)
	}
	status, body = doJSONWithHeaders(t, system, http.MethodPut, reportURL, report, systemHeaders)
	if status != http.StatusOK || !jsonContains(body, `"status":"online"`) {
		t.Fatalf("system report: %d %s", status, body)
	}

	late := map[string]any{"status": "offline", "capacity_json": `{"cpu":0}`, "observed_at": observedAt - 1000}
	status, body = doJSONWithHeaders(t, system, http.MethodPut, reportURL, late, systemHeaders)
	if status != http.StatusOK || !jsonContains(body, `"status":"online"`) {
		t.Fatalf("late report overwrote projection: %d %s", status, body)
	}
	newer := map[string]any{"status": "degraded", "capacity_json": `{"cpu":4}`, "observed_at": observedAt + 1000}
	status, body = doJSONWithHeaders(t, system, http.MethodPut, reportURL, newer, systemHeaders)
	if status != http.StatusOK || !jsonContains(body, `"status":"degraded"`) {
		t.Fatalf("newer report: %d %s", status, body)
	}

	status, body = doJSONWithHeaders(t, admin, http.MethodGet, controlAddress+"/api/v1/admin/clusters/"+clusterID, nil, adminHeaders)
	var response struct {
		CapacityJSON string `json:"capacity_json"`
	}
	var capacity map[string]any
	decodeErr := json.Unmarshal(body, &response)
	capacityErr := json.Unmarshal([]byte(response.CapacityJSON), &capacity)
	if status != http.StatusOK || decodeErr != nil || capacityErr != nil || capacity["cpu"] != float64(4) {
		t.Fatalf("admin get: %d %s", status, body)
	}
	status, body = doJSONWithHeaders(t, admin, http.MethodGet, controlAddress+"/api/v1/admin/clusters", nil, adminHeaders)
	if status != http.StatusOK || !strings.Contains(string(body), clusterID) {
		t.Fatalf("admin list: %d %s", status, body)
	}
}
