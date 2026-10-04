package e2e

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"testing"
	"time"
)

// Services are started externally. This gate verifies real deployed route and
// credential boundaries; no database, credential extraction or simulated login.
func TestLegacyTokenVerificationRetirement(t *testing.T) {
	authAddr, workflowAddr, materialsAddr := os.Getenv("MEDIA_AUTH_PRIVATE_ADDR"), os.Getenv("MEDIA_WORKFLOW_PRIVATE_ADDR"), os.Getenv("MEDIA_MATERIALS_PRIVATE_ADDR")
	internal := os.Getenv("MEDIA_AUTH_INTERNAL_TOKEN")
	if authAddr == "" || workflowAddr == "" || materialsAddr == "" || internal == "" {
		t.Skip("configure private Auth/Workflow/Materials addresses and internal introspection credential")
	}
	transport := &http.Transport{}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 10 * time.Second}
	request := func(method, addr, path, credential string, payload []byte, want int) []byte {
		t.Helper()
		req, err := http.NewRequest(method, addr+path, bytes.NewReader(payload))
		if err != nil {
			t.Fatal("invalid test request")
		}
		req.Header.Set("Content-Type", "application/json")
		if credential != "" {
			req.Header.Set("X-Media-Auth-Internal-Token", credential)
		}
		if method == http.MethodGet {
			req.Header.Set("Authorization", "Bearer tos_retired-test-credential")
			req.Header.Set("Cookie", "token=untrusted; mw_token=untrusted")
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal("private runtime request failed")
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(io.LimitReader(resp.Body, 65536))
		if err != nil || resp.StatusCode != want {
			t.Fatalf("retirement boundary status=%d want=%d", resp.StatusCode, want)
		}
		return body
	}
	for _, path := range []string{"/internal/token/verify", "/internal/token/platform"} {
		for _, credential := range []string{"", internal} {
			request("POST", authAddr, path, credential, []byte(`{"token":"tos_retired-test-credential","user_id":"unknown"}`), 404)
		}
	}
	for _, target := range []struct{ addr, path string }{{authAddr, "/api/auth/me"}, {workflowAddr, "/api/v1/jobs"}, {materialsAddr, "/api/v1/materials"}} {
		request("GET", target.addr, target.path, "", nil, 401)
	}
	// These two private capabilities must remain registered and fail closed.
	for _, path := range []string{"/internal/token/session", "/internal/token/api-token:verify"} {
		request("POST", authAddr, path, "", []byte(`{"token":"invalid"}`), 401)
		var result struct {
			Active bool `json:"active"`
		}
		body := request("POST", authAddr, path, internal, []byte(`{"token":"invalid"}`), 200)
		if json.Unmarshal(body, &result) != nil || result.Active {
			t.Fatal("invalid introspection principal accepted")
		}
	}
}

// Public routes must be absent even for an authenticated current principal.
// The provisioned token is borrowed, never created, logged or revoked here.
func TestLegacyTokenCRUDRetirement(t *testing.T) {
	addr, token := os.Getenv("MEDIA_GATEWAY_ADDR"), os.Getenv("MEDIA_API_TOKEN")
	if addr == "" || token == "" {
		t.Skip("configure unified gateway address and provisioned API Token")
	}
	status, _ := doJSONBearer(t, addr, http.MethodGet, "/api/auth/me", token, nil)
	if status != http.StatusOK {
		t.Fatalf("current API Token status=%d want=200", status)
	}
	for _, target := range []struct{ method, path string }{
		{http.MethodGet, "/api/service-tokens"},
		{http.MethodPost, "/api/service-tokens"},
		{http.MethodDelete, "/api/service-tokens/retired-route-test"},
	} {
		status, _ := doJSONBearer(t, addr, target.method, target.path, token, nil)
		if status != http.StatusNotFound {
			t.Fatalf("retired %s route status=%d want=404", target.method, status)
		}
	}
}
