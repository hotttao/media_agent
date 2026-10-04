package e2e

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"
)

// Exercises the public proxy without starting Runtime/Worker or submitting Jobs.
func TestAgentProxyGateway(t *testing.T) {
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
	if err := json.Unmarshal(public("GET", "/api/v1/workspaces/"+workspace, user, nil, 200), &w); err != nil || w.Config == "" || w.Org == "" || w.Owner == "" {
		t.Fatal("Workspace lacks current Agent identity")
	}
	var issued struct {
		Secret string `json:"secret"`
		Record struct {
			ID string `json:"id"`
		} `json:"api_token"`
	}
	command := map[string]any{"name": "Agent proxy HTTP gate", "expires_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339), "idempotency_key": fmt.Sprintf("proxy-http-%d", time.Now().UnixNano())}
	if err := json.Unmarshal(public("POST", "/api/v1/agent-clients/"+w.Config+"/api-tokens", user, command, 201), &issued); err != nil || issued.Secret == "" || issued.Record.ID == "" {
		t.Fatal("Agent test credential unavailable")
	}
	t.Cleanup(func() { public("POST", "/api/api-tokens/"+issued.Record.ID+"/revoke", user, nil, 200) })
	type envelope struct {
		Status int    `json:"status_code"`
		Body   []byte `json:"body"`
		Trace  string `json:"trace_id"`
	}
	proxy := func(service, method, uri string, body any, want int) envelope {
		t.Helper()
		var encoded []byte
		contentType := ""
		if body != nil {
			var err error
			encoded, err = json.Marshal(body)
			if err != nil {
				t.Fatal("encode proxy body")
			}
			contentType = "application/json"
		}
		request := map[string]any{"service": service, "method": method, "uri": uri, "content_type": contentType, "body": encoded}
		var response envelope
		if err := json.Unmarshal(public("POST", "/api/agent/v1/requests", issued.Secret, request, 200), &response); err != nil || response.Status != want {
			t.Fatalf("proxy %s %s: upstream status=%d want=%d", method, uri, response.Status, want)
		}
		if len(response.Trace) != 32 {
			t.Fatal("Agent proxy lost trace correlation")
		}
		return response
	}
	list := map[string]any{"service": "workflow", "method": "GET", "uri": "/api/v1/jobs?page_size=1"}
	public("POST", "/api/agent/v1/requests", "", list, 401)
	public("POST", "/api/agent/v1/requests", user, list, 401)
	for _, request := range []map[string]any{
		{"service": "auth", "method": "GET", "uri": "/api/auth/me"},
		{"service": "workflow", "method": "GET", "uri": "https://outside.invalid/api/v1/jobs"},
		{"service": "workflow", "method": "POST", "uri": "/api/v1/step-runs/claim"},
		{"service": "materials", "method": "POST", "uri": "/api/v1/asset-kinds"},
	} {
		public("POST", "/api/agent/v1/requests", issued.Secret, request, 400)
	}
	proxy("workflow", "GET", "/api/v1/jobs?page_size=1", nil, 200)
	proxy("materials", "GET", "/api/v1/materials?organization_id="+url.QueryEscape(w.Org)+"&page_size=1", nil, 200)
	proxy("materials", "GET", "/api/v1/materials?organization_id=not-a-member", nil, 404)
	created := proxy("materials", "POST", "/api/v1/materials?organization_id="+url.QueryEscape(w.Org)+"&scope=ai_clothing", map[string]any{"name": "Agent proxy temporary text fixture", "asset_kind": "movement", "content": "HTTP proxy test only", "visibility": "private"}, 201)
	var material struct {
		ID    string `json:"id"`
		Owner string `json:"owner_user_id"`
		Org   string `json:"organization_id"`
	}
	if err := json.Unmarshal(created.Body, &material); err != nil || material.ID == "" {
		t.Fatal("Agent created material not identifiable")
	}
	materialPath := "/api/v1/materials/" + material.ID + "?organization_id=" + url.QueryEscape(w.Org)
	// User cleanup remains available after Agent credential revocation.
	t.Cleanup(func() {
		status, _ := doJSONBearer(t, addr, "DELETE", materialPath, user, nil)
		if status != 200 && status != 404 {
			t.Errorf("temporary material cleanup status=%d", status)
		}
	})
	if material.Owner != w.Owner || material.Org != w.Org {
		t.Fatal("Agent write lost owner or organization attribution")
	}
	proxy("materials", "GET", materialPath, nil, 200)
	proxy("materials", "DELETE", materialPath, nil, 200)
	public("POST", "/api/api-tokens/"+issued.Record.ID+"/revoke", user, nil, 200)
	public("POST", "/api/agent/v1/requests", issued.Secret, list, 401)
}
