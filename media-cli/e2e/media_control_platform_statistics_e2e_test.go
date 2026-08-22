package e2e

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

func TestMediaControlPlatformStatisticsE2E(t *testing.T) {
	authAddress, controlAddress := os.Getenv("MEDIA_AUTH_ADDR"), os.Getenv("MEDIA_CONTROL_ADDR")
	if authAddress == "" || controlAddress == "" {
		t.Skip("MEDIA_AUTH_ADDR and MEDIA_CONTROL_ADDR are required")
	}
	metricKey := fmt.Sprintf("e2e.workspace.active.%d", time.Now().UnixNano())
	ordinary := registerGatewayUser(t, authAddress, "statistics-reader")
	status, body := doJSON(t, ordinary, http.MethodGet, controlAddress+"/api/v1/admin/platform-statistics", nil)
	if status != http.StatusForbidden {
		t.Fatalf("ordinary reader accepted: %d %s", status, body)
	}

	internalToken := envOr("CONTROL_INTERNAL_TOKEN", "dev-control-internal-token")
	adminHeaders := map[string]string{"X-Control-Service-Token": internalToken, "X-Control-Acting-User-ID": "e2e-admin", "X-Control-Acting-Role": "admin"}
	systemHeaders := map[string]string{"X-Control-Service-Token": internalToken, "X-Control-Acting-User-ID": "e2e-statistics", "X-Control-Acting-Role": "system"}
	admin, system := httpClient(t), httpClient(t)
	reportURL := controlAddress + "/api/v1/admin/platform-statistics/report"
	periodStart, refreshedAt := time.Now().UTC().Add(-time.Hour).UnixMilli(), time.Now().UTC().UnixMilli()
	report := map[string]any{"metric_key": metricKey, "period_start": periodStart, "dimensions_json": `{"region":"cn","tier":"gpu"}`, "value": "10.50", "refreshed_at": refreshedAt}
	status, body = doJSONWithHeaders(t, admin, http.MethodPost, reportURL, report, adminHeaders)
	if status != http.StatusForbidden {
		t.Fatalf("non-system reporter accepted: %d %s", status, body)
	}
	status, body = doJSONWithHeaders(t, system, http.MethodPost, reportURL, report, systemHeaders)
	if status != http.StatusOK || !jsonContains(body, `"value":"10.50"`) {
		t.Fatalf("system report: %d %s", status, body)
	}

	late := map[string]any{"metric_key": metricKey, "period_start": periodStart, "dimensions_json": `{"tier":"gpu","region":"cn"}`, "value": "1", "refreshed_at": refreshedAt - 1000}
	status, body = doJSONWithHeaders(t, system, http.MethodPost, reportURL, late, systemHeaders)
	if status != http.StatusOK || !jsonContains(body, `"value":"10.50"`) {
		t.Fatalf("late refresh overwrote statistic: %d %s", status, body)
	}
	newer := map[string]any{"metric_key": metricKey, "period_start": periodStart, "dimensions_json": `{"region":"cn","tier":"gpu"}`, "value": "12", "refreshed_at": refreshedAt + 1000}
	status, body = doJSONWithHeaders(t, system, http.MethodPost, reportURL, newer, systemHeaders)
	if status != http.StatusOK || !jsonContains(body, `"value":"12"`) {
		t.Fatalf("new refresh: %d %s", status, body)
	}

	query := url.Values{"metric_key": []string{metricKey}, "period_start_from": []string{fmt.Sprint(periodStart - 1)}, "period_start_to": []string{fmt.Sprint(periodStart + 1)}}
	status, body = doJSONWithHeaders(t, admin, http.MethodGet, controlAddress+"/api/v1/admin/platform-statistics?"+query.Encode(), nil, adminHeaders)
	if status != http.StatusOK || !strings.Contains(string(body), metricKey) || !jsonContains(body, `"value":"12"`) {
		t.Fatalf("admin filtered list: %d %s", status, body)
	}
}
