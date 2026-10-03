package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"
)

// TestWorkflowWorkerGatewayAPIToken exercises the public Worker boundary without
// logging in, submitting Jobs, polling queues or altering existing credentials.
// The servers are started externally, per harness/contributing/e2e.md.
func TestWorkflowWorkerGatewayAPIToken(t *testing.T) {
	addr, user, org := os.Getenv("MEDIA_GATEWAY_ADDR"), os.Getenv("MEDIA_API_TOKEN"), os.Getenv("MEDIA_E2E_ORGANIZATION_ID")
	if addr == "" || user == "" || org == "" {
		t.Skip("set MEDIA_GATEWAY_ADDR, MEDIA_API_TOKEN and MEDIA_E2E_ORGANIZATION_ID")
	}
	request := func(method, path, credential string, body any, want int) []byte {
		t.Helper()
		status, result := doJSONBearer(t, addr, method, path, credential, body)
		if status != want {
			t.Fatalf("%s %s: status=%d want=%d", method, path, status, want)
		}
		return result
	}
	var identity struct {
		Worker struct {
			ID           string `json:"id"`
			Actor        string `json:"actor_id"`
			Organization string `json:"organization_id"`
		} `json:"worker_identity"`
	}
	name := fmt.Sprintf("worker-boundary-%d", time.Now().UnixNano())
	if err := json.Unmarshal(request("POST", "/api/worker-identities", user, map[string]any{
		"name": name, "organization_id": org,
	}, http.StatusCreated), &identity); err != nil || identity.Worker.ID == "" || identity.Worker.Actor != "Worker:"+identity.Worker.ID || identity.Worker.Organization != org {
		t.Fatal("created Worker identity authority mismatch")
	}
	path := "/api/worker-identities/" + identity.Worker.ID
	t.Cleanup(func() { request("POST", path+"/delete", user, nil, http.StatusOK) })
	var credential struct {
		Secret string `json:"secret"`
		Token  struct {
			ID    string `json:"id"`
			Actor string `json:"actor_id"`
		} `json:"api_token"`
	}
	if err := json.Unmarshal(request("POST", path+"/api-tokens", user, map[string]any{
		"name": name, "idempotency_key": name,
		"scopes":     []string{"workflow.worker.poll"},
		"expires_at": time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
	}, http.StatusCreated), &credential); err != nil || credential.Secret == "" || credential.Token.ID == "" || credential.Token.Actor != identity.Worker.Actor {
		t.Fatal("issued Worker API Token authority mismatch")
	}
	t.Cleanup(func() { request("POST", "/api/api-tokens/"+credential.Token.ID+"/revoke", user, nil, http.StatusOK) })
	instance := name + "-instance"
	registration := map[string]any{
		"worker_instance_id": instance,
		"capabilities": []map[string]any{{
			"capability_key": "media.image.model", "capability_version": 1, "concurrency": 1,
		}},
	}
	var registered struct {
		Instance string `json:"worker_instance_id"`
		At       int64  `json:"registered_at"`
	}
	if err := json.Unmarshal(request("POST", "/api/worker/v1/registrations", credential.Secret, registration, http.StatusOK), &registered); err != nil || registered.Instance != instance || registered.At == 0 {
		t.Fatal("valid Worker registration was not acknowledged")
	}
	// User credentials cannot impersonate a Worker; poll-only credentials
	// cannot heartbeat a task even after passing edge identity exchange.
	request("POST", "/api/worker/v1/registrations", user, registration, http.StatusUnauthorized)
	request("POST", "/api/worker/v1/registrations", "invalid-api-token", registration, http.StatusUnauthorized)
	request("POST", "/api/worker/v1/tasks/unknown/heartbeat", credential.Secret, map[string]any{
		"worker_instance_id": instance, "attempt": 1,
	}, http.StatusForbidden)
	request("POST", "/api/api-tokens/"+credential.Token.ID+"/revoke", user, nil, http.StatusOK)
	request("POST", "/api/worker/v1/registrations", credential.Secret, registration, http.StatusUnauthorized)
	request("POST", "/api/worker/v1/tasks:poll", credential.Secret, map[string]any{
		"worker_instance_id": instance, "capability_key": "media.image.model", "capability_version": 1,
	}, http.StatusUnauthorized)
}
