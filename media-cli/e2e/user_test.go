// user_test.go - media_auth Register + 用户管理 HTTP e2e 测试（裸 net/http + cookie jar）。
//
// 遵循 e2e.md：被测 server 由 docker-compose 起；本测试只发 HTTP 请求。
// 用 MEDIA_AUTH_ADDR 指定 server；MEDIA_AUTH_E2E_ADMIN_USER / PASS 指定 admin 账号。
//
// 运行（需先起 server + seed admin 用户）：
//
//	MEDIA_AUTH_ADDR=http://127.0.0.1:8001 \
//	MEDIA_AUTH_E2E_ADMIN_USER=smoke MEDIA_AUTH_E2E_ADMIN_PASS=password123 \
//	go test ./e2e/ -run TestUserE2E -v
package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

// TestUserE2E 驱动 Register + 用户管理 HTTP 全流程。
func TestUserE2E(t *testing.T) {
	addr := skipIfNoServer(t)
	adminUser := envOr("MEDIA_AUTH_E2E_ADMIN_USER", "smoke")
	adminPass := envOr("MEDIA_AUTH_E2E_ADMIN_PASS", "password123")
	suffix := time.Now().UnixNano()
	regUsername := fmt.Sprintf("e2e_reg_%d", suffix)
	managedUsername := fmt.Sprintf("e2e_admin_%d", suffix)

	// ── Register（自服务注册 + 自动登录） ──────────────────────────────

	regClient := httpClient(t)
	regBody := fmt.Sprintf(`{"username":%q,"password":"e2eregpass123","display_name":"E2E Reg"}`, regUsername)
	var registeredID string

	t.Run("Register_success_autoLogin", func(t *testing.T) {
		status, body := doJSON(t, regClient, "POST", addr+"/api/auth/register", json.RawMessage(regBody))
		if status != 201 {
			t.Fatalf("register: status=%d body=%s", status, body)
		}
		var resp struct {
			User struct{ ID, Username, Role string } `json:"user"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if resp.User.Username != regUsername || resp.User.Role != "user" {
			t.Errorf("got user=%+v", resp.User)
		}
		registeredID = resp.User.ID
		// cookie 应已下发（自动登录）-> /me 返回新用户。
		status, body = doJSON(t, regClient, "GET", addr+"/api/auth/me", nil)
		if status != 200 || !strings.Contains(string(body), regUsername) {
			t.Errorf("me after register: status=%d body=%s", status, body)
		}
	})

	t.Run("Internal_token_API_is_protected_and_resolves_session_and_platform_token", func(t *testing.T) {
		internalToken := envOr("MEDIA_AUTH_INTERNAL_TOKEN", "dev-media-auth-internal-token")
		status, _ := doInternalJSON(t, addr+"/internal/token/platform", `{"user_id":"`+registeredID+`"}`, "")
		if status != 401 {
			t.Fatalf("internal endpoint without credential: want 401, got %d", status)
		}
		status, body := doInternalJSON(t, addr+"/internal/token/platform", `{"user_id":"`+registeredID+`"}`, internalToken)
		if status != 200 || !strings.Contains(string(body), "tos_") {
			t.Fatalf("platform token: status=%d body=%s", status, body)
		}
		u, _ := url.Parse(addr)
		var sessionToken string
		for _, cookie := range regClient.Jar.Cookies(u) {
			if cookie.Name == "token" {
				sessionToken = cookie.Value
			}
		}
		status, body = doInternalJSON(t, addr+"/internal/token/session", fmt.Sprintf(`{"token":%q}`, sessionToken), internalToken)
		if status != 200 || !strings.Contains(string(body), `"active":true`) {
			t.Fatalf("session introspection: status=%d body=%s", status, body)
		}
	})

	t.Run("Register_duplicate_409", func(t *testing.T) {
		status, _ := doJSON(t, httpClient(t), "POST", addr+"/api/auth/register", json.RawMessage(regBody))
		if status != 409 {
			t.Errorf("duplicate register: want 409, got %d", status)
		}
	})

	t.Run("Register_shortPassword_400", func(t *testing.T) {
		status, _ := doJSON(t, httpClient(t), "POST", addr+"/api/auth/register",
			json.RawMessage(`{"username":"e2e_short","password":"123"}`))
		if status != 400 {
			t.Errorf("short password: want 400, got %d", status)
		}
	})

	// ── 用户管理（admin） ─────────────────────────────────────────────

	adminClient := httpClient(t)
	login(t, adminClient, addr, adminUser, adminPass)

	var createdID string
	managedClient := httpClient(t)

	t.Run("CreateUser_admin", func(t *testing.T) {
		status, body := doJSON(t, adminClient, "POST", addr+"/api/users",
			json.RawMessage(fmt.Sprintf(`{"username":%q,"display_name":"E2E Admin","password":"e2eadmin1","role":"admin"}`, managedUsername)))
		if status != 201 {
			t.Fatalf("create user: status=%d body=%s", status, body)
		}
		var resp struct {
			ID string `json:"id"`
		}
		json.Unmarshal(body, &resp)
		createdID = resp.ID
		if createdID == "" {
			t.Fatal("create user: empty id")
		}
	})

	t.Run("DisableUser_revokesExistingSession", func(t *testing.T) {
		login(t, managedClient, addr, managedUsername, "e2eadmin1")
		status, body := doJSON(t, adminClient, "PATCH", addr+"/api/users/"+createdID+"/status",
			json.RawMessage(`{"status":"disabled"}`))
		if status != 200 {
			t.Fatalf("disable user: status=%d body=%s", status, body)
		}
		status, body = doJSON(t, managedClient, "GET", addr+"/api/auth/me", nil)
		if status != 401 {
			t.Fatalf("disabled session: want 401, got %d body=%s", status, body)
		}
	})

	t.Run("RestoreUser_requiresNewLogin", func(t *testing.T) {
		status, body := doJSON(t, adminClient, "PATCH", addr+"/api/users/"+createdID+"/status",
			json.RawMessage(`{"status":"active"}`))
		if status != 200 {
			t.Fatalf("restore user: status=%d body=%s", status, body)
		}
		status, _ = doJSON(t, managedClient, "GET", addr+"/api/auth/me", nil)
		if status != 401 {
			t.Fatalf("old session after restore: want 401, got %d", status)
		}
		login(t, managedClient, addr, managedUsername, "e2eadmin1")
		status, body = doJSON(t, managedClient, "GET", addr+"/api/auth/me", nil)
		if status != 200 {
			t.Fatalf("new session after restore: status=%d body=%s", status, body)
		}
	})

	t.Run("ListUsers_admin_includesNew", func(t *testing.T) {
		status, body := doJSON(t, adminClient, "GET", addr+"/api/users?page=1&page_size=50", nil)
		if status != 200 {
			t.Fatalf("list users: status=%d body=%s", status, body)
		}
		if !strings.Contains(string(body), managedUsername) {
			t.Errorf("list users: missing %s, body=%s", managedUsername, body[:200])
		}
	})

	t.Run("ListUsers_search", func(t *testing.T) {
		status, body := doJSON(t, adminClient, "GET", addr+"/api/users?q="+managedUsername+"&page=1&page_size=10", nil)
		if status != 200 {
			t.Fatalf("search: status=%d", status)
		}
		if !strings.Contains(string(body), managedUsername) {
			t.Errorf("search: missing %s", managedUsername)
		}
	})

	t.Run("ChangeUserPassword_admin", func(t *testing.T) {
		status, _ := doJSON(t, adminClient, "PUT", addr+"/api/users/"+createdID+"/password",
			json.RawMessage(`{"password":"newpass123"}`))
		if status != 200 {
			t.Errorf("change password: want 200, got %d", status)
		}
	})

	t.Run("DeleteUser_admin", func(t *testing.T) {
		status, _ := doJSON(t, adminClient, "DELETE", addr+"/api/users/"+createdID, nil)
		if status != 200 {
			t.Errorf("delete user: want 200, got %d", status)
		}
	})

	t.Run("DeleteUser_again_404", func(t *testing.T) {
		status, _ := doJSON(t, adminClient, "DELETE", addr+"/api/users/"+createdID, nil)
		if status != 404 {
			t.Errorf("re-delete: want 404, got %d", status)
		}
	})

	// ── 权限校验 ──────────────────────────────────────────────────────

	t.Run("NonAdmin_listUsers_403", func(t *testing.T) {
		// regClient 是注册的 user（role=user）。
		status, _ := doJSON(t, regClient, "GET", addr+"/api/users", nil)
		if status != 403 {
			t.Errorf("non-admin list: want 403, got %d", status)
		}
	})

	t.Run("SoftDelete_revokesExistingSession_andRetainsAuditRow", func(t *testing.T) {
		status, body := doJSON(t, adminClient, "GET", addr+"/api/users?q="+regUsername+"&page=1&page_size=10", nil)
		if status != 200 {
			t.Fatalf("find registered user: status=%d body=%s", status, body)
		}
		var page struct {
			Items []struct {
				ID          string `json:"id"`
				Status      string `json:"status"`
				AuthVersion int64  `json:"auth_version"`
			} `json:"items"`
		}
		if err := json.Unmarshal(body, &page); err != nil || len(page.Items) != 1 {
			t.Fatalf("find registered user: items=%+v err=%v", page.Items, err)
		}
		status, body = doJSON(t, adminClient, "DELETE", addr+"/api/users/"+page.Items[0].ID, nil)
		if status != 200 {
			t.Fatalf("soft delete: status=%d body=%s", status, body)
		}
		status, _ = doJSON(t, regClient, "GET", addr+"/api/auth/me", nil)
		if status != 401 {
			t.Fatalf("revoked session: want 401, got %d", status)
		}
		status, body = doJSON(t, adminClient, "GET", addr+"/api/users?q="+regUsername+"&page=1&page_size=10", nil)
		if status != 200 {
			t.Fatalf("list deleted user: status=%d body=%s", status, body)
		}
		if err := json.Unmarshal(body, &page); err != nil || len(page.Items) != 1 || page.Items[0].Status != "deleted" || page.Items[0].AuthVersion < 2 {
			t.Fatalf("deleted audit row not retained: items=%+v err=%v", page.Items, err)
		}
	})

	t.Run("DeleteSelf_400", func(t *testing.T) {
		// admin 删自己 -> 400。
		status, body := doJSON(t, adminClient, "GET", addr+"/api/auth/me", nil)
		var me struct {
			ID string `json:"id"`
		}
		json.Unmarshal(body, &me)
		status, _ = doJSON(t, adminClient, "DELETE", addr+"/api/users/"+me.ID, nil)
		if status != 400 {
			t.Errorf("delete self: want 400, got %d", status)
		}
	})

	// ── 清理 ─────────────────────────────────────────────────────────

	t.Cleanup(func() {
		// 测试用户已软删除；保留终态行作为审计记录。
		status, body := doJSON(t, adminClient, "GET", addr+"/api/users?q="+regUsername+"&page=1&page_size=10", nil)
		if status == 200 {
			var resp struct {
				Items []struct {
					ID string `json:"id"`
				} `json:"items"`
			}
			if json.Unmarshal(body, &resp) == nil {
				for _, it := range resp.Items {
					doJSON(t, adminClient, "DELETE", addr+"/api/users/"+it.ID, nil)
				}
			}
		}
	})
}

func doInternalJSON(t *testing.T, endpoint, body, token string) (int, []byte) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("X-Media-Auth-Internal-Token", token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	value, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, value
}
