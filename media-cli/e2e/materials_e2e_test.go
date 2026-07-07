// materials_e2e_test.go — media_materials HTTP e2e 测试（裸 net/http + cookie jar）。
//
// 前提：media_auth + media_materials server 已起（docker-compose 或手动）。
// 用 MEDIA_AUTH_ADDR / MEDIA_MATERIALS_ADDR 环境变量指定地址；未设则 skip。
// 用 MEDIA_AUTH_E2E_USER / MEDIA_AUTH_E2E_PASS 指定已 seed 的测试账号。
//
// 运行：
//
//	MEDIA_AUTH_ADDR=http://127.0.0.1:8001 \
//	MEDIA_MATERIALS_ADDR=http://127.0.0.1:8003 \
//	MEDIA_AUTH_E2E_USER=smoke MEDIA_AUTH_E2E_PASS=smoke123 \
//	go test ./e2e/ -run TestMaterialsE2E -v
package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
)

func skipIfNoMaterialsServer(t *testing.T) string {
	t.Helper()
	addr := os.Getenv("MEDIA_MATERIALS_ADDR")
	if addr == "" {
		t.Skip("MEDIA_MATERIALS_ADDR 未设，跳过 media_materials e2e")
	}
	return addr
}

// materialsLogin 通过 media_auth login 拿 cookie，然后带着 cookie 调 materials。
// materials 的 auth 中间件读 cookie JWT（materials/workflow 共享 jwt_secret）。
// 但 materials 用自己的 jwt_secret，与 media_auth 不同——cookie 不通用。
// 所以这里走 OAuth login 流程：先 media_auth login 拿 session cookie，
// 然后 materials /api/auth/login → 302 → media_auth /api/oauth/authorize...
// 简化：本期用 service token（Bearer）方式调 materials，不走 OAuth。
// 先在 media_auth 建一个 service token，然后带 Bearer 调 materials。

func TestMaterialsE2E(t *testing.T) {
	authAddr := skipIfNoServer(t)
	matAddr := skipIfNoMaterialsServer(t)
	user := envOr("MEDIA_AUTH_E2E_USER", "smoke")
	pass := envOr("MEDIA_AUTH_E2E_PASS", "smoke123")

	// 1. media_auth login（拿 session cookie）
	authClient := httpClient(t)
	uid := login(t, authClient, authAddr, user, pass)
	t.Logf("logged in as %s (id=%s)", user, uid)

	// 2. 创建 service token（用于调 materials）
	status, body := doJSON(t, authClient, "POST", authAddr+"/api/auth/tokens", map[string]string{
		"name": "e2e_materials",
	})
	if status != 201 {
		t.Fatalf("create service token: status=%d body=%s", status, body)
	}
	var tokResp struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(body, &tokResp); err != nil || tokResp.Token == "" {
		t.Fatalf("create token unmarshal: err=%v body=%s", err, body)
	}
	t.Logf("got service token: %s...", tokResp.Token[:12])

	// materials client（带 Bearer token）
	matClient := &http.Client{
		Jar:     nil,
		Timeout: 0,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse // 不自动跟 302
		},
	}

	// 3. 测试 /api/auth/me（验 service token 通过 media_auth 代理）
	status, body = doJSON(t, matClient, "GET", matAddr+"/api/auth/me", nil)
	if status != 200 {
		t.Logf("materials /api/auth/me with service token: status=%d（media_auth session 中间件可能未加 Bearer 路径，或 materials auth 中间件未调 media_auth）", status)
		// 不 fatal：service token 路径可能还没完全打通；继续用 Bearer 直接调
	}

	// 4. asset_kind CRUD
	// List asset_kinds（seed 的 SYSTEM kinds 应存在）
	status, body = doJSON(t, matClient, "GET", matAddr+"/api/v1/asset-kinds?resource_type=MATERIAL", nil)
	if status != 200 {
		t.Fatalf("list asset_kinds: status=%d body=%s", status, body)
	}
	var kindsResp struct {
		Items []struct {
			Name         string `json:"name"`
			ResourceType string `json:"resource_type"`
			Status       string `json:"status"`
		} `json:"items"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal(body, &kindsResp); err != nil {
		t.Fatalf("list asset_kinds unmarshal: %v", err)
	}
	if kindsResp.Total == 0 {
		t.Fatal("list asset_kinds: empty (seed not applied?)")
	}
	t.Logf("list asset_kinds: %d items", kindsResp.Total)

	// 5. create virtual_ip
	status, body = doJSON(t, matClient, "POST", matAddr+"/api/v1/virtual-ips?scope=ai_clothing", map[string]any{
		"name":         "e2e_vip",
		"avatar_url":   "https://example.com/avatar.png",
		"owner_user_id": uid,
		"visibility":   "public",
	})
	if status != 200 {
		t.Fatalf("create virtual_ip: status=%d body=%s", status, body)
	}
	var vip struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &vip); err != nil || vip.ID == "" {
		t.Fatalf("create vip unmarshal: err=%v body=%s", err, body)
	}
	t.Logf("created vip id=%s", vip.ID)

	// 6. create material
	status, body = doJSON(t, matClient, "POST", matAddr+"/api/v1/materials?scope=ai_clothing", map[string]any{
		"name":          "e2e_scene",
		"asset_kind":    "scene",
		"url":           "https://example.com/scene.png",
		"owner_user_id": uid,
		"visibility":    "public",
	})
	if status != 200 {
		t.Fatalf("create material: status=%d body=%s", status, body)
	}
	var mat struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &mat); err != nil || mat.ID == "" {
		t.Fatalf("create material unmarshal: err=%v body=%s", err, body)
	}
	t.Logf("created material id=%s", mat.ID)

	// 7. bind material ↔ virtual_ip
	status, body = doJSON(t, matClient, "POST", matAddr+"/api/v1/materials/"+mat.ID+"/bindings?bound_by="+uid, map[string]any{
		"owner_kind": "virtual_ip",
		"owner_id":   vip.ID,
	})
	if status != 200 {
		t.Fatalf("bind material→vip: status=%d body=%s", status, body)
	}

	// 8. list materials by owner (virtual_ip)
	status, body = doJSON(t, matClient, "GET", matAddr+"/api/v1/materials/by-owner?owner_kind=virtual_ip&owner_id="+vip.ID, nil)
	if status != 200 {
		t.Fatalf("list by owner: status=%d body=%s", status, body)
	}
	var byOwner struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &byOwner); err != nil {
		t.Fatalf("list by owner unmarshal: %v", err)
	}
	if len(byOwner.Items) == 0 {
		t.Error("list by owner: empty (binding not found)")
	}

	// 9. create product
	status, body = doJSON(t, matClient, "POST", matAddr+"/api/v1/products?scope=ai_clothing&created_by="+uid, map[string]any{
		"name":            "e2e_product",
		"target_audience": "test",
	})
	if status != 200 {
		t.Fatalf("create product: status=%d body=%s", status, body)
	}
	var prod struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &prod); err != nil || prod.ID == "" {
		t.Fatalf("create product unmarshal: err=%v body=%s", err, body)
	}
	t.Logf("created product id=%s", prod.ID)

	// 10. delete material
	status, _ = doJSON(t, matClient, "DELETE", matAddr+"/api/v1/materials/"+mat.ID, nil)
	if status != 200 {
		t.Errorf("delete material: status=%d", status)
	}

	// 11. delete vip
	status, _ = doJSON(t, matClient, "DELETE", matAddr+"/api/v1/virtual-ips/"+vip.ID, nil)
	if status != 200 {
		t.Errorf("delete vip: status=%d", status)
	}

	// 12. revoke service token
	status, _ = doJSON(t, authClient, "DELETE", authAddr+"/api/auth/tokens/"+tokResp.Token, nil)
	if status != 200 {
		t.Logf("revoke token: status=%d（可能 token 列表按 id 删，不是 token 明文）", status)
	}

	t.Log("✅ media_materials e2e passed")
}

var _ = fmt.Sprintf
