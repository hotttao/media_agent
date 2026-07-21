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
	"encoding/json"
	"strings"
	"testing"
)

// TestUserE2E 驱动 Register + 用户管理 HTTP 全流程。
func TestUserE2E(t *testing.T) {
	addr := skipIfNoServer(t)
	adminUser := envOr("MEDIA_AUTH_E2E_ADMIN_USER", "smoke")
	adminPass := envOr("MEDIA_AUTH_E2E_ADMIN_PASS", "password123")

	// ── Register（自服务注册 + 自动登录） ──────────────────────────────

	regClient := httpClient(t)
	regBody := `{"username":"e2e_reguser","password":"e2eregpass123","display_name":"E2E Reg"}`

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
		if resp.User.Username != "e2e_reguser" || resp.User.Role != "user" {
			t.Errorf("got user=%+v", resp.User)
		}
		// cookie 应已下发（自动登录）-> /me 返回新用户。
		status, body = doJSON(t, regClient, "GET", addr+"/api/auth/me", nil)
		if status != 200 || !strings.Contains(string(body), "e2e_reguser") {
			t.Errorf("me after register: status=%d body=%s", status, body)
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

	t.Run("CreateUser_admin", func(t *testing.T) {
		status, body := doJSON(t, adminClient, "POST", addr+"/api/users",
			json.RawMessage(`{"username":"e2e_adminuser","display_name":"E2E Admin","password":"e2eadmin1","role":"admin"}`))
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

	t.Run("ListUsers_admin_includesNew", func(t *testing.T) {
		status, body := doJSON(t, adminClient, "GET", addr+"/api/users?page=1&page_size=50", nil)
		if status != 200 {
			t.Fatalf("list users: status=%d body=%s", status, body)
		}
		if !strings.Contains(string(body), "e2e_adminuser") {
			t.Errorf("list users: missing e2e_adminuser, body=%s", body[:200])
		}
	})

	t.Run("ListUsers_search", func(t *testing.T) {
		status, body := doJSON(t, adminClient, "GET", addr+"/api/users?q=e2e_adminuser&page=1&page_size=10", nil)
		if status != 200 {
			t.Fatalf("search: status=%d", status)
		}
		if !strings.Contains(string(body), "e2e_adminuser") {
			t.Errorf("search: missing e2e_adminuser")
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
		// 删注册的 e2e_reguser（admin 权限）。
		status, body := doJSON(t, adminClient, "GET", addr+"/api/users?q=e2e_reguser&page=1&page_size=10", nil)
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
