package e2e

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"
)

// Read-only Control identity contract. The internal endpoint is deliberately not
// published through Traefik; no Agent/Workspace/Token is created by this test.
func TestAgentIdentityControl(t *testing.T) {
	addr, user, workspace := os.Getenv("MEDIA_GATEWAY_ADDR"), os.Getenv("MEDIA_API_TOKEN"), os.Getenv("MEDIA_E2E_WORKSPACE_ID")
	control, serviceToken := os.Getenv("MEDIA_CONTROL_ADDR"), os.Getenv("MEDIA_CONTROL_INTERNAL_TOKEN")
	if addr == "" || user == "" || workspace == "" || control == "" || serviceToken == "" {
		t.Skip("set gateway/user token/workspace and internal Control address/token")
	}
	status, body := doJSONBearer(t, addr, "GET", "/api/v1/workspaces/"+url.PathEscape(workspace), user, nil)
	if status != 200 {
		t.Fatalf("workspace status=%d", status)
	}
	var w struct {
		ConfigurationID string `json:"agent_configuration_id"`
		Org             string `json:"organization_id"`
		Team            string `json:"team_id"`
		CreatedBy       string `json:"created_by"`
	}
	if err := json.Unmarshal(body, &w); err != nil || w.ConfigurationID == "" || w.Org == "" || w.Team == "" || w.CreatedBy == "" {
		t.Fatal("workspace missing authoritative references")
	}
	client := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	request := func(id, token, role string, want int) []byte {
		t.Helper()
		req, err := http.NewRequest("GET", control+"/internal/v1/agent-identities/"+url.PathEscape(id), nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("X-Control-Service-Token", token)
		req.Header.Set("X-Control-Acting-User-ID", "media-auth")
		req.Header.Set("X-Control-Acting-Role", role)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != want {
			t.Fatalf("identity status=%d want=%d", resp.StatusCode, want)
		}
		result, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	var identity struct {
		ID         string `json:"agent_configuration_id"`
		Actor      string `json:"actor_id"`
		Workspace  string `json:"workspace_id"`
		Owner      string `json:"owner_user_id"`
		Org        string `json:"organization_id"`
		Team       string `json:"team_id"`
		Authorized bool   `json:"owner_authorized"`
	}
	result := request(w.ConfigurationID, serviceToken, "system", 200)
	if err := json.Unmarshal(result, &identity); err != nil || identity.ID != w.ConfigurationID || identity.Actor != "Agent:"+w.ConfigurationID || identity.Workspace != workspace || identity.Owner != w.CreatedBy || identity.Org != w.Org || identity.Team != w.Team || !identity.Authorized {
		t.Fatal("identity projection differs from Workspace authority")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(result, &fields); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"api_key", "encrypted_api_key", "configuration", "runtime_context"} {
		if _, exists := fields[key]; exists {
			t.Fatalf("identity response exposed %s", key)
		}
	}
	request(w.ConfigurationID, "", "system", 401)
	request(w.ConfigurationID, "invalid", "system", 401)
	request(w.ConfigurationID, serviceToken, "user", 403)
	request(fmt.Sprintf("missing-%d", time.Now().UnixNano()), serviceToken, "system", 404)
}
