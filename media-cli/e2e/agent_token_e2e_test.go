package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"
)

// The internal Auth endpoint remains private. This gate creates/revokes only its
// own temporary Agent Token; it never starts an Agent runtime or changes owner.
func TestAgentTokenInternalProvisioning(t *testing.T) {
	addr, user, workspace := os.Getenv("MEDIA_GATEWAY_ADDR"), os.Getenv("MEDIA_API_TOKEN"), os.Getenv("MEDIA_E2E_WORKSPACE_ID")
	internal, gatewayCredential, authCredential := os.Getenv("MEDIA_AUTH_INTERNAL_ADDR"), os.Getenv("MEDIA_AUTH_AGENT_GATEWAY_TOKEN"), os.Getenv("MEDIA_E2E_AUTH_INTERNAL_TOKEN")
	if addr == "" || user == "" || workspace == "" || internal == "" || gatewayCredential == "" || authCredential == "" {
		t.Skip("configure gateway/User/workspace and private Auth test credentials")
	}
	public := func(method, path string, body any, want int) []byte {
		t.Helper()
		status, result := doJSONBearer(t, addr, method, path, user, body)
		if status != want {
			t.Fatalf("%s %s status=%d want=%d", method, path, status, want)
		}
		return result
	}
	var w struct {
		ConfigurationID string `json:"agent_configuration_id"`
		Owner           string `json:"created_by"`
		Org             string `json:"organization_id"`
	}
	if err := json.Unmarshal(public("GET", "/api/v1/workspaces/"+url.PathEscape(workspace), nil, 200), &w); err != nil || w.ConfigurationID == "" || w.Owner == "" || w.Org == "" {
		t.Fatal("Workspace missing identity references")
	}
	client := &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	private := func(path string, body any, credential, actingUser string, want int) []byte {
		t.Helper()
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		req, err := http.NewRequest("POST", internal+path, bytes.NewReader(encoded))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		if path == "/internal/v1/agent-api-tokens" {
			req.Header.Set("X-Agent-Gateway-Internal-Token", credential)
			req.Header.Set("X-Media-Auth-Acting-User-ID", actingUser)
		} else {
			req.Header.Set("X-Media-Auth-Internal-Token", credential)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != want {
			t.Fatalf("private %s status=%d want=%d", path, resp.StatusCode, want)
		}
		result, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	command := map[string]any{"agent_configuration_id": w.ConfigurationID, "name": "Agent Token HTTP gate", "expires_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339), "idempotency_key": fmt.Sprintf("agent-http-%d", time.Now().UnixNano()), "actor_id": "Worker:forged", "owner_user_id": "forged", "organization_id": "forged"}
	path := "/internal/v1/agent-api-tokens"
	private(path, command, "", w.Owner, 401)
	private(path, command, authCredential, w.Owner, 401)
	private(path, command, gatewayCredential, "", 401)
	private(path, command, gatewayCredential, "not-the-owner", 403)
	command["scopes"] = []string{"api.access"}
	private(path, command, gatewayCredential, w.Owner, 400)
	delete(command, "scopes")
	command["agent_configuration_id"] = "missing-agent"
	private(path, command, gatewayCredential, w.Owner, 404)
	command["agent_configuration_id"] = w.ConfigurationID
	var issued struct {
		Secret string `json:"secret"`
		Token  struct {
			ID     string   `json:"id"`
			Actor  string   `json:"actor_id"`
			Owner  string   `json:"subject_user_id"`
			Org    string   `json:"organization_id"`
			Type   string   `json:"actor_type"`
			Scopes []string `json:"scopes"`
		} `json:"api_token"`
	}
	if err := json.Unmarshal(private(path, command, gatewayCredential, w.Owner, 201), &issued); err != nil || issued.Token.ID == "" {
		t.Fatal("issuance missing token metadata")
	}
	t.Cleanup(func() { public("POST", "/api/api-tokens/"+issued.Token.ID+"/revoke", nil, 200) })
	if issued.Secret == "" || issued.Token.Actor != "Agent:"+w.ConfigurationID || issued.Token.Owner != w.Owner || issued.Token.Org != w.Org || issued.Token.Type != "agent" || len(issued.Token.Scopes) != 1 || issued.Token.Scopes[0] != "agent.gateway" {
		t.Fatal("issuance did not derive Agent authority")
	}
	if bytes.Contains(private(path, command, gatewayCredential, w.Owner, 409), []byte(issued.Secret)) {
		t.Fatal("duplicate replayed Secret")
	}
	var verified struct {
		Active bool   `json:"active"`
		Sub    string `json:"sub"`
		Actor  string `json:"actor_id"`
		Org    string `json:"organization_id"`
		Scope  string `json:"scope"`
	}
	if err := json.Unmarshal(private("/internal/token/api-token:verify", map[string]string{"token": issued.Secret}, authCredential, "", 200), &verified); err != nil || !verified.Active || verified.Sub != "User:"+w.Owner || verified.Actor != issued.Token.Actor || verified.Org != w.Org || verified.Scope != "agent.gateway" {
		t.Fatal("real Talos/Control verification mismatch")
	}
	for _, target := range []struct {
		method, path string
		body         any
	}{{"GET", "/api/auth/me", nil}, {"POST", "/api/worker/v1/registrations", map[string]any{}}} {
		status, _ := doJSONBearer(t, addr, target.method, target.path, issued.Secret, target.body)
		if status != 401 && status != 403 {
			t.Fatal("Agent credential escaped Gateway-only scope")
		}
	}
	if bytes.Contains(public("GET", "/api/api-tokens", nil, 200), []byte(issued.Secret)) {
		t.Fatal("list leaked Secret")
	}
	public("POST", "/api/api-tokens/"+issued.Token.ID+"/revoke", nil, 200)
	var revoked struct {
		Active bool `json:"active"`
	}
	if err := json.Unmarshal(private("/internal/token/api-token:verify", map[string]string{"token": issued.Secret}, authCredential, "", 200), &revoked); err != nil || revoked.Active {
		t.Fatal("revoked Agent Token remained valid")
	}
}
