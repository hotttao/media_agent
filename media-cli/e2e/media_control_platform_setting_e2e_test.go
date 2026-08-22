package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func TestMediaControlPlatformSettingE2E(t *testing.T) {
	authAddress, controlAddress := os.Getenv("MEDIA_AUTH_ADDR"), os.Getenv("MEDIA_CONTROL_ADDR")
	if authAddress == "" || controlAddress == "" {
		t.Skip("MEDIA_AUTH_ADDR and MEDIA_CONTROL_ADDR are required")
	}
	key := fmt.Sprintf("e2e.platform.setting.%d", time.Now().UnixNano())
	ordinary := registerGatewayUser(t, authAddress, "platform-setting-user")
	status, body := doJSON(t, ordinary, http.MethodPost, controlAddress+"/api/v1/admin/platform-settings", map[string]string{"key": key, "value_json": `{}`})
	if status != http.StatusForbidden {
		t.Fatalf("ordinary user was not rejected: %d %s", status, body)
	}

	admin := httpClient(t)
	headers := map[string]string{
		"X-Control-Service-Token":  envOr("CONTROL_INTERNAL_TOKEN", "dev-control-internal-token"),
		"X-Control-Acting-User-ID": "e2e-platform-admin",
		"X-Control-Acting-Role":    "admin",
	}
	request := func(method, path string, value any) (int, []byte) {
		return doJSONWithHeaders(t, admin, method, controlAddress+path, value, headers)
	}
	status, body = request(http.MethodPost, "/api/v1/admin/platform-settings", map[string]string{"key": key, "value_json": ` {"enabled":true} `})
	if status != http.StatusCreated || !jsonContains(body, `"version":1`) || !jsonContains(body, `"updated_by":"e2e-platform-admin"`) {
		t.Fatalf("create setting: %d %s", status, body)
	}
	status, duplicate := request(http.MethodPost, "/api/v1/admin/platform-settings", map[string]string{"key": key, "value_json": `{}`})
	if status != http.StatusConflict {
		t.Fatalf("duplicate setting: %d %s", status, duplicate)
	}
	status, body = request(http.MethodGet, "/api/v1/admin/platform-settings/"+key, nil)
	var fetched struct {
		ValueJSON string `json:"value_json"`
	}
	var settingValue map[string]any
	decodeErr := json.Unmarshal(body, &fetched)
	valueErr := json.Unmarshal([]byte(fetched.ValueJSON), &settingValue)
	if status != http.StatusOK || decodeErr != nil || valueErr != nil || settingValue["enabled"] != true {
		t.Fatalf("get setting: %d %s", status, body)
	}
	status, body = request(http.MethodPut, "/api/v1/admin/platform-settings/"+key, map[string]any{"value_json": `{"enabled":false}`, "expected_version": 1})
	if status != http.StatusOK || !jsonContains(body, `"version":2`) {
		t.Fatalf("update setting: %d %s", status, body)
	}
	status, stale := request(http.MethodPut, "/api/v1/admin/platform-settings/"+key, map[string]any{"value_json": `{}`, "expected_version": 1})
	if status != http.StatusConflict {
		t.Fatalf("stale update: %d %s", status, stale)
	}
	status, body = request(http.MethodGet, "/api/v1/admin/platform-settings", nil)
	if status != http.StatusOK || !strings.Contains(string(body), key) {
		t.Fatalf("list settings: %d %s", status, body)
	}
}

func doJSONWithHeaders(t *testing.T, client *http.Client, method, url string, body any, headers map[string]string) (int, []byte) {
	t.Helper()
	var requestBody io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		requestBody = bytes.NewReader(encoded)
	}
	request, err := http.NewRequest(method, url, requestBody)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	responseBody, _ := io.ReadAll(response.Body)
	return response.StatusCode, responseBody
}
