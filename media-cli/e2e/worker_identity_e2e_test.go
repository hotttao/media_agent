package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"
)

func TestWorkerIdentityGateway(t *testing.T) {
	addr, user, org := os.Getenv("MEDIA_GATEWAY_ADDR"), os.Getenv("MEDIA_API_TOKEN"), os.Getenv("MEDIA_E2E_ORGANIZATION_ID")
	if addr == "" || user == "" || org == "" {
		t.Skip("set MEDIA_GATEWAY_ADDR, MEDIA_API_TOKEN and MEDIA_E2E_ORGANIZATION_ID")
	}
	request := func(method, path, credential string, body any, want int) []byte {
		t.Helper()
		status, result := doJSONBearer(t, addr, method, path, credential, body)
		if status != want {
			t.Fatalf("%s %s status=%d want=%d", method, path, status, want)
		}
		return result
	}
	request("POST", "/api/worker-identities", user, map[string]any{"name": "invalid", "organization_id": "not-a-member"}, 403)
	request("POST", "/api/worker-identities", user, map[string]any{"name": " ", "organization_id": org}, 400)
	request("GET", "/api/worker-identities/unknown", user, nil, 404)
	// Explicit JSON tags avoid mistaking successful HTTP for correct authority.
	var created struct {
		Worker struct {
			ID           string `json:"id"`
			Owner        string `json:"owner_user_id"`
			Actor        string `json:"actor_id"`
			Organization string `json:"organization_id"`
			Status       string `json:"status"`
		} `json:"worker_identity"`
	}
	body := request("POST", "/api/worker-identities", user, map[string]any{"name": fmt.Sprintf("worker-http-%d", time.Now().UnixNano()), "organization_id": org, "owner_user_id": "forged-owner", "status": "deleted", "id": "forged-id"}, 201)
	if err := json.Unmarshal(body, &created); err != nil || created.Worker.ID == "" || created.Worker.Owner == "forged-owner" || created.Worker.Actor != "Worker:"+created.Worker.ID || created.Worker.Organization != org || created.Worker.Status != "active" {
		t.Fatal("identity authority mismatch")
	}
	path := "/api/worker-identities/" + created.Worker.ID
	t.Cleanup(func() { request("POST", path+"/delete", user, nil, 200) })
	request("GET", path, user, nil, 200)
	list := request("GET", "/api/worker-identities", user, nil, 200)
	if !bytes.Contains(list, []byte(created.Worker.ID)) {
		t.Fatal("created identity absent from owner list")
	}
	command := map[string]any{"name": "desktop", "expires_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339), "idempotency_key": created.Worker.ID,
		"actor_id": "Worker:forged", "subject_user_id": "forged-owner", "organization_id": "forged-org"}
	command["scopes"] = []string{"api.access"}
	request("POST", path+"/api-tokens", user, command, 400)
	command["scopes"] = []string{"workflow.worker.poll"}
	var credential struct {
		Secret string `json:"secret"`
		Token  struct {
			ID           string   `json:"id"`
			Owner        string   `json:"subject_user_id"`
			Actor        string   `json:"actor_id"`
			Organization string   `json:"organization_id"`
			Scopes       []string `json:"scopes"`
		} `json:"api_token"`
	}
	if err := json.Unmarshal(request("POST", path+"/api-tokens", user, command, 201), &credential); err != nil || credential.Secret == "" || credential.Token.ID == "" || credential.Token.Owner != created.Worker.Owner || credential.Token.Actor != created.Worker.Actor || credential.Token.Organization != org || len(credential.Token.Scopes) != 1 {
		t.Fatal("worker token authority mismatch")
	}
	t.Cleanup(func() { request("POST", "/api/api-tokens/"+credential.Token.ID+"/revoke", user, nil, 200) })
	duplicate := request("POST", path+"/api-tokens", user, command, 409)
	if bytes.Contains(duplicate, []byte(credential.Secret)) {
		t.Fatal("duplicate replayed Secret")
	}
	// A deliberately invalid registration is read-only yet proves that the
	// Worker Actor passed edge JWT exchange and reached the business validator.
	request("POST", "/api/worker/v1/registrations", credential.Secret, map[string]any{"worker_instance_id": "test-instance", "capabilities": []any{}}, 400)
	request("POST", "/api/worker/v1/registrations", user, map[string]any{"worker_instance_id": "test-instance", "capabilities": []any{}}, 401)
	status, _ := doJSONBearer(t, addr, "GET", "/api/auth/me", credential.Secret, nil)
	if status != 401 && status != 403 {
		t.Fatal("worker scope admitted ordinary User API")
	}
	request("POST", "/api/worker/v1/tasks/unknown/heartbeat", credential.Secret, map[string]any{"worker_instance_id": "test-instance", "attempt": 1}, 403)
	list = request("GET", "/api/api-tokens", user, nil, 200)
	if bytes.Contains(list, []byte(credential.Secret)) {
		t.Fatal("token list leaked Secret")
	}
	request("POST", path+"/disable", user, nil, 200)
	request("POST", path+"/disable", user, nil, 200)
	request("POST", "/api/worker/v1/registrations", credential.Secret, map[string]any{}, 401)
	request("POST", path+"/api-tokens", user, command, 403)
	request("POST", "/api/api-tokens/"+credential.Token.ID+"/rotate", user, nil, 403)
	// Observe background cleanup through the public metadata read, not DB writes.
	deadline := time.Now().Add(30 * time.Second)
	for {
		var records struct {
			Tokens []struct {
				ID     string `json:"id"`
				Status string `json:"status"`
			} `json:"api_tokens"`
		}
		if err := json.Unmarshal(request("GET", "/api/api-tokens", user, nil, 200), &records); err != nil {
			t.Fatal(err)
		}
		done := false
		for _, record := range records.Tokens {
			if record.ID == credential.Token.ID && record.Status == "revoked" {
				done = true
			}
		}
		if done {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("disabled Worker key did not converge to revoked")
		}
		time.Sleep(200 * time.Millisecond)
	}
	request("POST", path+"/delete", user, nil, 200)
	request("POST", path+"/delete", user, nil, 200)
	if !bytes.Contains(request("GET", path, user, nil, 200), []byte(`"deleted"`)) {
		t.Fatal("deleted identity history not retained")
	}
}
