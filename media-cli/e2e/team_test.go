// team_test.go — media_auth team 接口 HTTP e2e 测试（裸 net/http + cookie jar）。
//
// 遵循 e2e.md：被测 server 由 docker-compose 起；本测试只发 HTTP 请求。
// 用 MEDIA_AUTH_ADDR 环境变量指定 server 地址；未设则 skip（无 server 时 go test ./... 不失败）。
// 用 MEDIA_AUTH_E2E_USER / MEDIA_AUTH_E2E_PASS 指定已 seed 的测试账号（默认 smoke/smoke123）。
//
// 运行（需先起 server + seed 用户）：
//
//	MEDIA_AUTH_ADDR=http://127.0.0.1:8001 \
//	MEDIA_AUTH_E2E_USER=smoke MEDIA_AUTH_E2E_PASS=smoke123 \
//	go test ./e2e/ -run TestTeamE2E -v
package e2e

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"os"
	"strings"
	"testing"
	"time"
)

// envOr returns os.Getenv(key) or fallback.
func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// skipIfNoServer skips the test when MEDIA_AUTH_ADDR is unset.
func skipIfNoServer(t *testing.T) string {
	t.Helper()
	addr := os.Getenv("MEDIA_AUTH_ADDR")
	if addr == "" {
		t.Skip("MEDIA_AUTH_ADDR 未设，跳过 HTTP e2e（需先起 media_auth server）")
	}
	return addr
}

// httpClient builds a client with a cookie jar (session cookie 跨请求复用).
func httpClient(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar: %v", err)
	}
	return &http.Client{Jar: jar}
}

// doJSON sends a JSON request, returns status + body. Empty body for GET/DELETE.
func doJSON(t *testing.T, c *http.Client, method, url string, body any) (int, []byte) {
	t.Helper()
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reqBody = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		t.Fatalf("new req %s %s: %v", method, url, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("do %s %s: %v", method, url, err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, respBody
}

// login logs in and returns the ApiUser id. Uses the shared client (cookie jar persists session).
func login(t *testing.T, c *http.Client, addr, user, pass string) string {
	t.Helper()
	status, body := doJSON(t, c, "POST", addr+"/api/auth/login", map[string]string{
		"username": user, "password": pass,
	})
	if status != 200 {
		t.Fatalf("login: status=%d body=%s（需先 seed 用户 %s）", status, body, user)
	}
	var resp struct {
		User struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("login unmarshal: %v", err)
	}
	return resp.User.ID
}

// TestTeamE2E 驱动 organization/team HTTP 流程：login → create organization →
// configure organization entitlements → create team → configure team entitlements →
// list/get → create-invite → accept-invite（同 user 幂等 already_member=true）→ members → delete。
func TestTeamE2E(t *testing.T) {
	addr := skipIfNoServer(t)
	user := envOr("MEDIA_AUTH_E2E_USER", "smoke")
	pass := envOr("MEDIA_AUTH_E2E_PASS", "smoke123")

	c := httpClient(t)
	uid := login(t, c, addr, user, pass)
	t.Logf("logged in as %s (id=%s)", user, uid)

	// A team always belongs to an organization. Creating a fresh organization
	// keeps the authorization boundary explicit and makes this test repeatable.
	status, body := doJSON(t, c, "POST", addr+"/api/organizations", map[string]string{
		"name": "E2E Organization " + time.Now().Format("20060102-150405.000000000"),
	})
	if status != 201 {
		t.Fatalf("create organization: status=%d body=%s", status, body)
	}
	var organization struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &organization); err != nil || organization.ID == "" {
		t.Fatalf("create organization unmarshal: err=%v body=%s", err, body)
	}

	status, body = doJSON(t, c, "PUT", addr+"/api/organizations/"+organization.ID+"/entitlements", map[string]any{
		"feature_keys": []string{"materials.read", "workflow.run"},
	})
	if status != 200 {
		t.Fatalf("set organization entitlements: status=%d body=%s", status, body)
	}

	// create team
	status, body = doJSON(t, c, "POST", addr+"/api/teams", map[string]string{
		"name": "E2E Team", "description": "via media-cli e2e", "organization_id": organization.ID,
	})
	if status != 201 {
		t.Fatalf("create team: status=%d body=%s", status, body)
	}
	var team struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &team); err != nil {
		t.Fatalf("create team unmarshal: %v", err)
	}
	if team.ID == "" {
		t.Fatal("create team: empty id")
	}
	t.Logf("created team id=%s", team.ID)

	// Teams inherit their organization's effective feature set by default.
	status, body = doJSON(t, c, "GET", addr+"/api/teams/"+team.ID+"/entitlements", nil)
	if status != 200 || !strings.Contains(string(body), `"entitlement_mode":"inherit"`) || !strings.Contains(string(body), `"materials.read"`) {
		t.Fatalf("get inherited team entitlements: status=%d body=%s", status, body)
	}

	// Organization admins may restrict a team to a subset of the company's features.
	status, body = doJSON(t, c, "PUT", addr+"/api/teams/"+team.ID+"/entitlements", map[string]any{
		"entitlement_mode": "restricted",
		"feature_keys":     []string{"workflow.run"},
	})
	if status != 200 || !strings.Contains(string(body), `"entitlement_mode":"restricted"`) || strings.Contains(string(body), `"materials.read"`) {
		t.Fatalf("set restricted team entitlements: status=%d body=%s", status, body)
	}

	// list my teams → 应含刚建的
	status, body = doJSON(t, c, "GET", addr+"/api/teams", nil)
	if status != 200 {
		t.Fatalf("list teams: status=%d body=%s", status, body)
	}
	if !strings.Contains(string(body), team.ID) {
		t.Errorf("list teams: response missing team id, body=%s", body)
	}

	// get team
	status, body = doJSON(t, c, "GET", addr+"/api/teams/"+team.ID, nil)
	if status != 200 {
		t.Fatalf("get team: status=%d body=%s", status, body)
	}

	// create invite
	status, body = doJSON(t, c, "POST", addr+"/api/teams/"+team.ID+"/invites", map[string]string{})
	if status != 201 {
		t.Fatalf("create invite: status=%d body=%s", status, body)
	}
	var inv struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(body, &inv); err != nil || inv.Token == "" {
		t.Fatalf("create invite unmarshal: err=%v body=%s", err, body)
	}
	t.Logf("created invite token=%s", inv.Token)

	// accept invite (同 user → already_member=true，幂等)
	status, body = doJSON(t, c, "POST", addr+"/api/invites/"+inv.Token+"/accept", map[string]string{})
	if status != 200 {
		t.Fatalf("accept invite: status=%d body=%s", status, body)
	}
	if !strings.Contains(string(body), `"already_member":true`) {
		t.Errorf("accept invite (self): want already_member=true, body=%s", body)
	}

	// list members → ≥1（创建者）
	status, body = doJSON(t, c, "GET", addr+"/api/teams/"+team.ID+"/members", nil)
	if status != 200 {
		t.Fatalf("list members: status=%d body=%s", status, body)
	}
	if !strings.Contains(string(body), uid) {
		t.Errorf("list members: missing creator %s, body=%s", uid, body)
	}

	// delete team
	status, body = doJSON(t, c, "DELETE", addr+"/api/teams/"+team.ID, nil)
	if status != 200 {
		t.Fatalf("delete team: status=%d body=%s", status, body)
	}
	t.Logf("deleted team %s", team.ID)
}
