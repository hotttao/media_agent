package e2e

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// Explicit local fixture provisioning, never a synthetic internal JWT. Only
// newly created identities/departments/definitions are changed; no Job starts.
func TestResourceMultiuserGateway(t *testing.T) {
	if os.Getenv("MEDIA_E2E_RESOURCE_MULTIUSER") != "1" {
		t.Skip("explicit multi-user resource gate")
	}
	f := newMaterialsAPIFixture(t)
	admin, binary := os.Getenv("MEDIA_E2E_KRATOS_ADMIN"), os.Getenv("MEDIA_E2E_CLI_BINARY")
	u, err := url.Parse(admin)
	if err != nil || u.Scheme != "http" || (u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost") || !filepath.IsAbs(binary) {
		t.Fatal("local Kratos fixture admin and independent CLI required")
	}
	client := &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	call := func(c *http.Client, base, method, path, token string, body any) (int, []byte) {
		t.Helper()
		var input io.Reader
		if body != nil {
			data, err := json.Marshal(body)
			if err != nil {
				t.Fatal("invalid fixture JSON")
			}
			input = bytes.NewReader(data)
		}
		r, err := http.NewRequest(method, base+path, input)
		if err != nil {
			t.Fatal("invalid fixture request")
		}
		r.Header.Set("Accept", "application/json")
		r.Header.Set("Content-Type", "application/json")
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		response, err := c.Do(r)
		if err != nil {
			t.Fatal("fixture transport failed; details suppressed")
		}
		defer response.Body.Close()
		data, err := io.ReadAll(io.LimitReader(response.Body, 8<<20))
		if err != nil {
			t.Fatal("fixture response unreadable")
		}
		return response.StatusCode, data
	}
	decode := func(data []byte, out any) {
		t.Helper()
		if json.Unmarshal(data, out) != nil {
			t.Fatal("invalid fixture response")
		}
	}
	request := func(token, method, path string, body any, want int, out any) {
		t.Helper()
		status, data := call(client, f.gateway, method, path, token, body)
		if status != want {
			t.Fatalf("%s %s status=%d want=%d; response suppressed", method, path, status, want)
		}
		if out != nil {
			decode(data, out)
		}
	}
	prefix := "acl-multi-" + time.Now().UTC().Format("20060102T150405.000000000")
	type actor struct{ user, token, team string }
	actors := make([]actor, 3)
	for i, role := range []string{"editor", "viewer", "admin"} {
		secret := make([]byte, 24)
		if _, err := rand.Read(secret); err != nil {
			t.Fatal("fixture randomness unavailable")
		}
		password, email := hex.EncodeToString(secret), prefix+"-"+role+"@example.test"
		status, data := call(client, admin, "POST", "/admin/identities", "", map[string]any{
			"schema_id": "default", "traits": map[string]any{"email": email},
			"credentials": map[string]any{"password": map[string]any{"config": map[string]any{"password": password}}},
		})
		if status != 201 {
			t.Fatalf("own identity create status=%d", status)
		}
		var identity struct {
			ID string `json:"id"`
		}
		decode(data, &identity)
		if identity.ID == "" {
			t.Fatal("missing own identity ID")
		}
		t.Cleanup(func() {
			status, _ := call(client, admin, "DELETE", "/admin/identities/"+identity.ID, "", nil)
			if status != 204 {
				t.Errorf("own identity cleanup status=%d", status)
			}
		})
		jar, _ := cookiejar.New(nil)
		browser := &http.Client{Jar: jar, Timeout: 30 * time.Second, CheckRedirect: client.CheckRedirect}
		status, data = call(browser, f.gateway, "GET", "/self-service/login/browser", "", nil)
		if status != 200 {
			t.Fatalf("login flow status=%d", status)
		}
		var flow struct {
			ID string `json:"id"`
			UI struct {
				Nodes []struct {
					Attributes struct {
						Name  string `json:"name"`
						Value any    `json:"value"`
					} `json:"attributes"`
				} `json:"nodes"`
			} `json:"ui"`
		}
		decode(data, &flow)
		csrf := ""
		for _, n := range flow.UI.Nodes {
			if n.Attributes.Name == "csrf_token" {
				csrf, _ = n.Attributes.Value.(string)
			}
		}
		if flow.ID == "" || csrf == "" {
			t.Fatal("missing browser login CSRF")
		}
		status, _ = call(browser, f.gateway, "POST", "/self-service/login?flow="+url.QueryEscape(flow.ID), "", map[string]any{"method": "password", "identifier": email, "password": password, "csrf_token": csrf})
		if status != 200 && status != 303 {
			t.Fatalf("real browser login status=%d", status)
		}
		status, data = call(browser, f.gateway, "GET", "/api/auth/me", "", nil)
		if status != 200 {
			t.Fatalf("real actor profile status=%d", status)
		}
		var profile struct {
			ID string `json:"id"`
		}
		decode(data, &profile)
		if profile.ID == "" || profile.ID == f.user {
			t.Fatal("fixture actor is not distinct")
		}
		actors[i].user = profile.ID
		memberRole := "member"
		if role == "admin" {
			memberRole = "admin"
		}
		membership := "/api/organizations/" + f.organization + "/members"
		request(f.token, "POST", membership, map[string]any{"user_id": profile.ID, "role": memberRole}, 200, nil)
		t.Cleanup(func() {
			status, _ := call(client, f.gateway, "DELETE", membership+"/"+profile.ID, f.token, nil)
			if status != 200 {
				t.Errorf("own membership cleanup status=%d", status)
			}
		})
		status, data = call(browser, f.gateway, "POST", "/api/api-tokens", "", map[string]any{"name": prefix + "-" + role, "organization_id": f.organization, "scopes": []string{"api.access"}, "expires_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339), "idempotency_key": prefix + "-" + role})
		if status != 201 {
			t.Fatalf("own actor token status=%d", status)
		}
		var issued struct {
			Secret string `json:"secret"`
			Token  struct {
				ID string `json:"id"`
			} `json:"api_token"`
		}
		decode(data, &issued)
		if issued.Secret == "" || issued.Token.ID == "" {
			t.Fatal("missing own token")
		}
		actors[i].token = issued.Secret
		t.Cleanup(func() {
			// The short-lived bearer may expire during an interrupted run.
			// Use this fixture's browser identity to revoke its own Key.
			status, _ := call(browser, f.gateway, "POST", "/api/api-tokens/"+issued.Token.ID+"/revoke", "", map[string]any{})
			if status != 200 {
				t.Errorf("own token cleanup status=%d", status)
			}
		})
	}
	createTeam := func(suffix string, restricted bool) string {
		var team struct {
			ID string `json:"id"`
		}
		request(f.token, "POST", "/api/teams", map[string]any{"name": prefix + "-" + suffix, "organization_id": f.organization}, 201, &team)
		if team.ID == "" {
			t.Fatal("missing own department")
		}
		t.Cleanup(func() {
			status, _ := call(client, f.gateway, "DELETE", "/api/teams/"+team.ID, f.token, nil)
			if status != 200 {
				t.Errorf("own department cleanup status=%d", status)
			}
		})
		if restricted {
			request(f.token, "PUT", "/api/teams/"+team.ID+"/entitlements", map[string]any{"entitlement_mode": "restricted", "feature_keys": []string{}}, 200, nil)
		}
		return team.ID
	}
	wsTeam := createTeam("workspace", false)
	for i := range actors {
		actors[i].team = createTeam([]string{"editor", "viewer", "admin"}[i], true)
		for _, team := range []string{wsTeam, actors[i].team} {
			request(f.token, "POST", "/api/teams/"+team+"/members", map[string]any{"user_id": actors[i].user, "role": "member"}, 201, nil)
		}
	}
	var workspace struct {
		ID string `json:"id"`
	}
	request(f.token, "POST", "/api/v1/workspaces", map[string]any{"name": prefix, "organization_id": f.organization, "team_id": wsTeam, "agent_type": "codex"}, 201, &workspace)
	if workspace.ID == "" {
		t.Fatal("missing own Workspace")
	}
	t.Log("owned fixture Workspace=" + workspace.ID)
	var source struct {
		Scope string           `json:"scope"`
		Steps []map[string]any `json:"steps"`
	}
	request(f.token, "GET", "/api/v1/workflows/"+os.Getenv("MEDIA_E2E_PUBLICATION_SOURCE_ID"), nil, 200, &source)
	if len(source.Steps) == 0 || source.Scope == "" {
		t.Fatal("source has no root step")
	}
	step := source.Steps[0]
	var template map[string]any
	templateID, _ := step["template_id"].(string)
	version, _ := step["template_version"].(float64)
	request(f.token, "GET", "/api/v1/templates/"+templateID+"/versions/"+jsonNumber(int(version)), nil, 200, &template)
	body := map[string]any{"organization_id": f.organization, "name": prefix}
	for _, field := range []string{"prompt_body", "inputs", "output_asset_kind", "platform", "gen_way", "gen_type", "needs_confirm", "candidate_count", "capability_key", "capability_version", "invocation_mapping"} {
		if value, ok := template[field]; ok {
			body[field] = value
		}
	}
	createTemplate := func(token, suffix string) string {
		body["name"] = prefix + suffix
		var resource struct {
			ID string `json:"id"`
		}
		request(token, "POST", "/api/v1/templates?scope="+url.QueryEscape(source.Scope), body, 200, &resource)
		if resource.ID == "" {
			t.Fatal("missing own Template")
		}
		t.Cleanup(func() {
			status, _ := call(client, f.gateway, "POST", "/api/v1/templates/"+resource.ID+"/archive", token, map[string]any{})
			if status != 200 {
				t.Errorf("own Template archive status=%d", status)
			}
		})
		return "/api/v1/templates/" + resource.ID
	}
	tp := createTemplate(f.token, "-shared")
	ep := createTemplate(actors[0].token, "-editor-owned")
	request(f.token, "POST", tp+"/publish", map[string]any{}, 200, nil)
	step["template_id"], step["template_version"] = tp[len("/api/v1/templates/"):], 1
	var workflow struct {
		ID string `json:"id"`
	}
	request(f.token, "POST", "/api/v1/workflows?scope="+url.QueryEscape(source.Scope), map[string]any{"organization_id": f.organization, "workspace_id": workspace.ID, "name": prefix, "output_step_key": step["key"], "steps": []any{step}}, 200, &workflow)
	if workflow.ID == "" {
		t.Fatal("missing own Workflow")
	}
	wp := "/api/v1/workflows/" + workflow.ID
	t.Cleanup(func() {
		status, _ := call(client, f.gateway, "POST", wp+"/archive", f.token, map[string]any{})
		if status != 200 {
			t.Errorf("own Workflow archive status=%d", status)
		}
	})
	for _, path := range []string{tp, wp} {
		request(f.token, "POST", path+"/visibility", map[string]any{"visibility": "team"}, 200, nil)
		for i, permission := range []string{"editor", "viewer"} {
			request(f.token, "POST", path+"/grants", map[string]any{"team_id": actors[i].team, "permission": permission, "granted_by": actors[i].user}, 200, nil)
		}
		for _, a := range actors[:2] {
			request(a.token, "GET", path, nil, 200, nil)
		}
		for _, method := range []string{"GET", "PUT"} {
			request(actors[2].token, method, path, map[string]any{"name": "forbidden"}, 403, nil)
		}
		request(actors[1].token, "PUT", path, map[string]any{"name": "forbidden"}, 403, nil)
		for _, a := range actors[:2] {
			request(a.token, "POST", path+"/publish", map[string]any{}, 403, nil)
			request(a.token, "POST", path+"/grants", map[string]any{"team_id": actors[2].team, "permission": "editor", "granted_by": f.user}, 403, nil)
			request(a.token, "POST", path+"/visibility", map[string]any{"visibility": "public"}, 403, nil)
			request(a.token, "DELETE", path+"/grants/"+actors[1].team, nil, 403, nil)
		}
		update := map[string]any{"name": prefix + "-edited", "output_step_key": step["key"], "steps": []any{step}}
		if path == tp {
			update = make(map[string]any, len(body))
			for key, value := range body {
				update[key] = value
			}
			update["name"] = prefix + "-edited"
		}
		request(actors[0].token, "PUT", path, update, 200, nil)
	}
	cli := func(token, operation string, wantSuccess bool) {
		cmd := exec.Command(binary, "media_workflow", "workflow", operation, "--id", workflow.ID)
		cmd.Dir = ".."
		cmd.Env = append(os.Environ(), "APP_ENV=local", "MEDIA_WORKFLOW_ADDR="+f.gateway, "MEDIA_API_TOKEN="+token)
		_, err := cmd.Output()
		if (err == nil) != wantSuccess {
			t.Fatal("independent multi-user CLI outcome mismatch; output suppressed")
		}
	}
	cli(actors[1].token, "get", true)
	cli(actors[0].token, "publish", false)
	request(f.token, "PUT", "/api/teams/"+wsTeam+"/entitlements", map[string]any{"entitlement_mode": "restricted", "feature_keys": []string{"workspace"}}, 200, nil)
	for _, path := range []string{tp, wp} {
		request(actors[0].token, "PUT", path, map[string]any{"name": "forbidden"}, 403, nil)
		request(actors[1].token, "GET", path, nil, 200, nil)
	}
	request(actors[0].token, "POST", ep+"/publish", map[string]any{}, 403, nil)
	request(f.token, "PUT", wp, map[string]any{"name": "forbidden"}, 403, nil)
	request(f.token, "POST", wp+"/publish", map[string]any{}, 403, nil)
	request(f.token, "PUT", "/api/teams/"+wsTeam+"/entitlements", map[string]any{"entitlement_mode": "inherit", "feature_keys": []string{}}, 200, nil)
	request(actors[0].token, "POST", ep+"/publish", map[string]any{}, 200, nil)
	cli(f.token, "publish", true)
	for deadline := time.Now().Add(30 * time.Second); ; {
		var receipt struct {
			Status string `json:"publication_status"`
		}
		request(f.token, "GET", wp, nil, 200, &receipt)
		if receipt.Status == "registered" {
			break
		}
		if receipt.Status == "failed" || time.Now().After(deadline) {
			t.Fatal("owned publication did not converge")
		}
		time.Sleep(100 * time.Millisecond)
	}
	cli(actors[1].token, "get", true)
	for _, path := range []string{tp, wp} {
		request(f.token, "DELETE", path+"/grants/"+actors[0].team, nil, 200, nil)
		request(actors[0].token, "GET", path, nil, 403, nil)
		request(actors[1].token, "GET", path, nil, 200, nil)
	}
	t.Log("real viewer/editor/org-admin, precise entitlement revoke/restore, CLI and immediate Grant revoke passed; no Job submitted")
}

func jsonNumber(n int) string { data, _ := json.Marshal(n); return string(data) }
