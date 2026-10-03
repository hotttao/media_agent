package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"
)

func TestAgentTokenGatewayProvisioning(t *testing.T) {
	addr, user, workspace := os.Getenv("MEDIA_GATEWAY_ADDR"), os.Getenv("MEDIA_API_TOKEN"), os.Getenv("MEDIA_E2E_WORKSPACE_ID")
	if addr == "" || user == "" || workspace == "" {
		t.Skip("configure User API Token, gateway and Workspace")
	}
	public := func(method, path, token string, body any, want int) []byte {
		t.Helper()
		status, result := doJSONBearer(t, addr, method, path, token, body)
		if status != want {
			t.Fatalf("%s %s status=%d want=%d", method, path, status, want)
		}
		return result
	}
	var w struct {
		Config string `json:"agent_configuration_id"`
		Owner  string `json:"created_by"`
		Org    string `json:"organization_id"`
	}
	if err := json.Unmarshal(public("GET", "/api/v1/workspaces/"+workspace, user, nil, 200), &w); err != nil || w.Config == "" || w.Owner == "" || w.Org == "" {
		t.Fatal("Workspace missing Agent identity")
	}
	path := "/api/v1/agent-clients/" + w.Config + "/api-tokens"
	command := map[string]any{"name": "Agent Gateway HTTP gate", "expires_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339), "idempotency_key": fmt.Sprintf("gateway-http-%d", time.Now().UnixNano()), "owner_user_id": "forged", "organization_id": "forged", "actor_id": "Worker:forged", "scopes": []string{"api.access"}, "agent_configuration_id": "body-forged"}
	public("POST", path, "", command, 401)
	public("POST", "/api/v1/agent-clients/missing-config/api-tokens", user, command, 404)
	public("POST", path, user, map[string]string{"name": "invalid"}, 400)
	encoded, err := json.Marshal(command)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest("POST", addr+path, bytes.NewReader(encoded))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+user)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Media-Auth-Acting-User-ID", "forged")
	httpClient := &http.Client{Timeout: 25 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		t.Fatalf("Gateway issuance status=%d", resp.StatusCode)
	}
	if resp.Header.Get("Cache-Control") != "no-store" {
		t.Error("Secret response cacheable")
	}
	var issued struct {
		Secret string `json:"secret"`
		Record struct {
			ID     string   `json:"id"`
			Actor  string   `json:"actor_id"`
			Owner  string   `json:"subject_user_id"`
			Org    string   `json:"organization_id"`
			Type   string   `json:"actor_type"`
			Scopes []string `json:"scopes"`
		} `json:"api_token"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&issued); err != nil || issued.Record.ID == "" {
		t.Fatal("Gateway issuance metadata missing")
	}
	t.Cleanup(func() { public("POST", "/api/api-tokens/"+issued.Record.ID+"/revoke", user, nil, 200) })
	if issued.Secret == "" || issued.Record.Actor != "Agent:"+w.Config || issued.Record.Owner != w.Owner || issued.Record.Org != w.Org || issued.Record.Type != "agent" || len(issued.Record.Scopes) != 1 || issued.Record.Scopes[0] != "agent.gateway" {
		t.Fatal("Gateway allowed forged scope/identity")
	}
	if bytes.Contains(public("POST", path, user, command, 409), []byte(issued.Secret)) {
		t.Fatal("duplicate returned Secret")
	}
	status, _ := doJSONBearer(t, addr, "POST", path, issued.Secret, command)
	if status != 401 && status != 403 {
		t.Fatal("Agent Token could manage Agent credentials")
	}
	if bytes.Contains(public("GET", "/api/api-tokens", user, nil, 200), []byte(issued.Secret)) {
		t.Fatal("Token list exposed Secret")
	}
	public("POST", "/api/api-tokens/"+issued.Record.ID+"/revoke", user, nil, 200)
}
