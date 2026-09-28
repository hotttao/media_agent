package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"
)

// TestAPITokenGateway uses only the provisioned API Token; no browser or login.
func TestAPITokenGateway(t *testing.T) {
	addr, token := os.Getenv("MEDIA_GATEWAY_ADDR"), os.Getenv("MEDIA_API_TOKEN")
	if addr == "" || token == "" {
		t.Skip("set MEDIA_GATEWAY_ADDR and MEDIA_API_TOKEN")
	}
	org := os.Getenv("MEDIA_E2E_ORGANIZATION_ID")
	request := func(method, path, credential string, body any, want int) []byte {
		t.Helper()
		status, result := doJSONBearer(t, addr, method, path, credential, body)
		if status != want {
			t.Fatalf("%s %s: status=%d, want=%d", method, path, status, want)
		}
		return result
	}
	request("GET", "/api/auth/me", token, nil, 200)
	request("GET", "/api/auth/me", "invalid-api-token", nil, 401)
	request("GET", "/api/v1/workflows?page_size=1", token, nil, 200)
	request("POST", "/api/api-tokens", token, map[string]any{
		"name": "expired", "scopes": []string{"api.access"},
		"expires_at": time.Now().Add(-time.Hour).UTC().Format(time.RFC3339),
	}, 400)
	request("POST", "/api/api-tokens", token, map[string]any{
		"name": "outside-organization", "scopes": []string{"api.access"}, "organization_id": "not-a-member",
		"expires_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
	}, 403)
	if org != "" {
		request("GET", "/api/v1/materials?page_size=1&organization_id="+org, token, nil, 200)
		request("GET", "/api/v1/materials?organization_id=not-a-member", token, nil, 404)
	}
	create := func(scopes []string) (string, string) {
		body := request("POST", "/api/api-tokens", token, map[string]any{
			"name": fmt.Sprintf("http-e2e-%d", time.Now().UnixNano()), "scopes": scopes,
			"organization_id": org, "expires_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		}, 201)
		var result struct {
			Secret string `json:"secret"`
			Token  struct {
				ID string `json:"id"`
			} `json:"api_token"`
		}
		if err := json.Unmarshal(body, &result); err != nil || result.Secret == "" || result.Token.ID == "" {
			t.Fatal("create did not return one-time credential")
		}
		t.Cleanup(func() { request("POST", "/api/api-tokens/"+result.Token.ID+"/revoke", token, nil, 200) })
		return result.Token.ID, result.Secret
	}
	id, secret := create([]string{"api.access"})
	request("GET", "/api/auth/me", secret, nil, 200)
	list := request("GET", "/api/api-tokens", token, nil, 200)
	var records map[string]json.RawMessage
	if err := json.Unmarshal(list, &records); err != nil {
		t.Fatal(err)
	}
	if _, ok := records["secret"]; ok {
		t.Fatal("list leaked a secret")
	}
	if bytes.Contains(list, []byte(secret)) {
		t.Fatal("list included a credential value")
	}
	rotation := request("POST", "/api/api-tokens/"+id+"/rotate", token, map[string]any{}, 200)
	var rotated struct {
		Secret string `json:"secret"`
	}
	if err := json.Unmarshal(rotation, &rotated); err != nil || rotated.Secret == "" || rotated.Secret == secret {
		t.Fatal("rotation did not replace secret")
	}
	request("GET", "/api/auth/me", secret, nil, http.StatusUnauthorized)
	request("GET", "/api/auth/me", rotated.Secret, nil, 200)
	request("POST", "/api/api-tokens/"+id+"/revoke", token, nil, 200)
	request("GET", "/api/auth/me", rotated.Secret, nil, http.StatusUnauthorized)
	_, restricted := create([]string{"workflow.worker.poll"})
	status, _ := doJSONBearer(t, addr, "GET", "/api/auth/me", restricted, nil)
	if status != 401 && status != 403 {
		t.Fatalf("missing api.access accepted: status=%d", status)
	}
}
