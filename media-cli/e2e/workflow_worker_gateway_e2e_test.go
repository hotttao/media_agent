// workflow_worker_gateway_e2e_test.go verifies the client Worker authentication
// boundary over the unified Gateway. The tested servers are started externally;
// this package only sends HTTP requests, as required by harness/contributing/e2e.md.
//
// Run with:
//
//	MEDIA_AUTH_ADDR=http://127.0.0.1:8001 \
//	MEDIA_GATEWAY_ADDR=http://192.168.2.41:18080 \
//	MEDIA_AUTH_E2E_USER=1556824234@qq.com \
//	MEDIA_AUTH_E2E_PASS=st123456 \
//	go test ./e2e -run TestWorkflowWorkerGatewayServiceToken -v
package e2e

import (
	"encoding/json"
	"net/http"
	"os"
	"testing"
	"time"
)

func TestWorkflowWorkerGatewayServiceToken(t *testing.T) {
	authAddr := skipIfNoServer(t)
	gatewayAddr := os.Getenv("MEDIA_GATEWAY_ADDR")
	if gatewayAddr == "" {
		t.Skip("MEDIA_GATEWAY_ADDR 未设，跳过 Worker Gateway HTTP e2e（需先启动统一网关）")
	}

	user := envOr("MEDIA_AUTH_E2E_USER", "1556824234@qq.com")
	pass := envOr("MEDIA_AUTH_E2E_PASS", "st123456")
	authClient := httpClient(t)
	login(t, authClient, authAddr, user, pass)

	status, body := doJSON(t, authClient, http.MethodPost, authAddr+"/api/organizations", map[string]string{
		"name": "Worker Gateway E2E " + time.Now().Format("20060102-150405.000000000"),
	})
	if status != http.StatusCreated {
		t.Fatalf("create organization: status=%d body=%s", status, body)
	}
	var organization struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &organization); err != nil || organization.ID == "" {
		t.Fatalf("decode organization: err=%v body=%s", err, body)
	}

	expiresAt := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	status, body = doJSON(t, authClient, http.MethodPost, authAddr+"/api/service-tokens", map[string]any{
		"name":            "worker_gateway_e2e_" + time.Now().Format("150405"),
		"organization_id": organization.ID,
		"scopes": []string{
			"workflow.worker.poll",
			"workflow.worker.heartbeat",
			"workflow.worker.complete",
			"workflow.worker.fail",
		},
		"expires_at": expiresAt,
	})
	if status != http.StatusCreated {
		t.Fatalf("create worker service token: status=%d body=%s", status, body)
	}
	var tokenResponse struct {
		ID    string `json:"id"`
		Token string `json:"token"`
	}
	if err := json.Unmarshal(body, &tokenResponse); err != nil || tokenResponse.ID == "" || tokenResponse.Token == "" {
		t.Fatalf("decode worker service token: err=%v body=%s", err, body)
	}

	workerID := "worker-e2e-" + time.Now().Format("150405.000000000")
	registerBody := map[string]any{
		"worker_instance_id": workerID,
		"capabilities": []map[string]any{{
			"capability_key":     "media.image.model",
			"capability_version": 1,
			"concurrency":        1,
		}},
	}
	status, body = doJSONBearer(t, gatewayAddr, http.MethodPost, "/api/worker/v1/registrations", tokenResponse.Token, registerBody)
	if status != http.StatusOK {
		t.Fatalf("register worker through gateway: status=%d body=%s", status, body)
	}

	// With no workflow submitted for this fresh worker, the long poll must end
	// cleanly instead of exposing a Conductor-specific response or error.
	status, body = doJSONBearer(t, gatewayAddr, http.MethodPost, "/api/worker/v1/tasks:poll", tokenResponse.Token, map[string]any{
		"worker_instance_id": workerID,
		"capability_key":     "media.image.model",
		"capability_version": 1,
	})
	if status != http.StatusNoContent {
		t.Fatalf("poll empty worker queue through gateway: status=%d body=%s", status, body)
	}

	status, body = doJSON(t, authClient, http.MethodDelete, authAddr+"/api/service-tokens/"+tokenResponse.ID, nil)
	if status != http.StatusOK {
		t.Fatalf("revoke worker service token: status=%d body=%s", status, body)
	}

	status, body = doJSONBearer(t, gatewayAddr, http.MethodPost, "/api/worker/v1/registrations", tokenResponse.Token, registerBody)
	if status != http.StatusUnauthorized {
		t.Fatalf("revoked token must be rejected by gateway: status=%d body=%s", status, body)
	}
}
