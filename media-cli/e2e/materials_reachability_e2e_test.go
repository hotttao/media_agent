// materials_reachability_e2e_test.go - media_materials reachability + bindings_end_at
// + media_gen publish 跨服务集成 HTTP e2e 测试。
//
// Public requests use MEDIA_GATEWAY_ADDR / MEDIA_API_TOKEN /
// MEDIA_E2E_ORGANIZATION_ID; no simulated login or legacy credential CRUD.
// Private RPC tests additionally require MEDIA_MATERIALS_PRIVATE_ADDR.
// These retained feature gates deliberately fail if Binding/Reachability is
// disabled; current CRUD acceptance does not imply those features are covered.
// Publish integration additionally requires a prestarted MEDIA_GEN_ADDR.
package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// doJSONBearer 发 JSON 请求带 Bearer token，返回 status + body。
// 用于调 materials / workflow 的公开 /api/v1/* 接口（auth 中间件要求 Bearer 或 cookie）。
func doJSONBearer(t *testing.T, addr, method, path, token string, body any) (int, []byte) {
	t.Helper()
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reqBody = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, addr+path, reqBody)
	if err != nil {
		t.Fatalf("new req %s %s: %v", method, path, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	c := &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("do %s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, respBody
}

// materialsBearerToken borrows the provisioned API Token without mutating it.
func materialsBearerToken(t *testing.T) (uid, token string) {
	t.Helper()
	f := newMaterialsAPIFixture(t)
	return f.user, f.token
}

func materialsPrivateAddress(t *testing.T) string {
	t.Helper()
	addr := os.Getenv("MEDIA_MATERIALS_PRIVATE_ADDR")
	if addr == "" {
		t.Skip("configure private Materials RPC address")
	}
	return addr
}

// TestMaterialsReachability 驱动 materials reachability 全流程：
//
//	Setup: provisioned API Token + 创建 vip + material + binding
//	AssetReachability（MATERIAL，public material）-> reachable=true
//	AssetReachability（MATERIAL，不存在 id）-> blocked + NOT_FOUND
//	ResolveAssetByKind（virtual_ip, vip_id, scene）-> MATERIAL + material_id
//	AssetRead（MATERIAL, material_id）-> url
//	Cleanup
func TestMaterialsReachability(t *testing.T) {
	privateAddr := materialsPrivateAddress(t)
	uid, token := materialsBearerToken(t)
	matAddr := os.Getenv("MEDIA_GATEWAY_ADDR")
	scope := "ai_clothing"

	// Setup: 创建 virtual_ip（public，便于 reachability 不依赖 caller team）
	status, body := materialsGatewayRequest(t, matAddr, "POST", "/api/v1/virtual-ips?scope="+scope+"&owner_user_id="+uid, token, map[string]any{
		"name":               "e2e_reach_vip",
		"avatar_object_path": "ai_clothing/e2e/avatar.png",
		"visibility":         "public",
	})
	if status != 201 {
		t.Fatalf("create vip: status=%d body=%s", status, body)
	}
	var vip struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &vip); err != nil || vip.ID == "" {
		t.Fatalf("create vip unmarshal: err=%v body=%s", err, body)
	}
	t.Logf("created vip id=%s", vip.ID)
	defer materialsGatewayRequest(t, matAddr, "DELETE", "/api/v1/virtual-ips/"+vip.ID, token, nil)

	// 创建 material（public scene，owner=uid）
	status, body = materialsGatewayRequest(t, matAddr, "POST", "/api/v1/materials?scope="+scope+"&owner_user_id="+uid, token, map[string]any{
		"name":        "e2e_reach_scene",
		"asset_kind":  "scene",
		"object_path": "ai_clothing/e2e/scene.png",
		"visibility":  "public",
	})
	if status != 201 {
		t.Fatalf("create material: status=%d body=%s", status, body)
	}
	var mat struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &mat); err != nil || mat.ID == "" {
		t.Fatalf("create material unmarshal: err=%v body=%s", err, body)
	}
	t.Logf("created material id=%s", mat.ID)
	defer materialsGatewayRequest(t, matAddr, "DELETE", "/api/v1/materials/"+mat.ID, token, nil)

	// 绑定 material <-> virtual_ip
	status, _ = materialsGatewayRequest(t, matAddr, "POST", "/api/v1/materials/"+mat.ID+"/bindings?bound_by="+uid, token, map[string]any{
		"owner_kind": "virtual_ip",
		"owner_id":   vip.ID,
	})
	if status != 200 {
		t.Fatalf("bind material->vip: status=%d", status)
	}

	// 1. AssetReachability（MATERIAL，可达）
	// internal RPC 跳过 auth，可直接裸 doJSON 调用。
	internalClient := &http.Client{Timeout: 30 * time.Second}
	reachStatus, reachBody := doJSON(t, internalClient, "POST", privateAddr+"/api/internal/v1/reachability:asset", map[string]any{
		"user_id":        uid,
		"workflow_scope": scope,
		"refs": []map[string]any{
			{"resource_type": "MATERIAL", "id": mat.ID},
		},
	})
	if reachStatus != 200 {
		t.Fatalf("asset reachability: status=%d body=%s", reachStatus, reachBody)
	}
	var reachResp struct {
		Reachable map[string]bool `json:"reachable"`
		Blocked   []struct {
			Ref struct {
				ResourceType string `json:"resource_type"`
				ID           string `json:"id"`
			} `json:"ref"`
			Reason string `json:"reason"`
		} `json:"blocked"`
	}
	if err := json.Unmarshal(reachBody, &reachResp); err != nil {
		t.Fatalf("asset reachability unmarshal: %v body=%s", err, reachBody)
	}
	key := "MATERIAL:" + mat.ID
	if !reachResp.Reachable[key] {
		t.Errorf("asset reachability: %s not reachable, blocked=%+v", key, reachResp.Blocked)
	} else {
		t.Logf("asset reachability: %s reachable", key)
	}

	// 2. AssetReachability（MATERIAL，不存在 id）-> NOT_FOUND
	reachStatus, reachBody = doJSON(t, internalClient, "POST", privateAddr+"/api/internal/v1/reachability:asset", map[string]any{
		"user_id":        uid,
		"workflow_scope": scope,
		"refs": []map[string]any{
			{"resource_type": "MATERIAL", "id": "mat_nonexistent_xyz"},
		},
	})
	if reachStatus != 200 {
		t.Fatalf("asset reachability nonexistent: status=%d body=%s", reachStatus, reachBody)
	}
	json.Unmarshal(reachBody, &reachResp)
	if len(reachResp.Blocked) == 0 {
		t.Error("asset reachability: nonexistent material not blocked")
	} else {
		if reachResp.Blocked[0].Reason != "NOT_FOUND" {
			t.Errorf("asset reachability nonexistent: reason=%q, want NOT_FOUND", reachResp.Blocked[0].Reason)
		}
		t.Logf("asset reachability nonexistent: blocked reason=%s", reachResp.Blocked[0].Reason)
	}

	// 3. ResolveAssetByKind（virtual_ip, vip_id, scene）-> MATERIAL + material_id
	resolveStatus, resolveBody := doJSON(t, internalClient, "POST", privateAddr+"/api/internal/v1/reachability:resolve-asset", map[string]any{
		"owner_kind": "virtual_ip",
		"owner_id":   vip.ID,
		"asset_kind": "scene",
	})
	if resolveStatus != 200 {
		t.Fatalf("resolve asset by kind: status=%d body=%s", resolveStatus, resolveBody)
	}
	var resolveResp struct {
		ResourceType string `json:"resource_type"`
		AssetID      string `json:"asset_id"`
	}
	if err := json.Unmarshal(resolveBody, &resolveResp); err != nil {
		t.Fatalf("resolve asset unmarshal: %v body=%s", err, resolveBody)
	}
	if resolveResp.ResourceType != "MATERIAL" {
		t.Errorf("resolve asset: resource_type=%q, want MATERIAL", resolveResp.ResourceType)
	}
	if resolveResp.AssetID != mat.ID {
		t.Errorf("resolve asset: asset_id=%q, want %q", resolveResp.AssetID, mat.ID)
	}
	t.Logf("resolve asset by kind: %s/%s -> %s", "virtual_ip", vip.ID, resolveResp.AssetID)

	// 4. AssetRead（MATERIAL, material_id）-> url
	readStatus, readBody := doJSON(t, internalClient, "POST", privateAddr+"/api/internal/v1/reachability:read", map[string]any{
		"resource_type": "MATERIAL",
		"id":            mat.ID,
	})
	if readStatus != 200 {
		t.Fatalf("asset read: status=%d body=%s", readStatus, readBody)
	}
	var readResp struct {
		URL      string `json:"url"`
		Content  string `json:"content"`
		Metadata string `json:"metadata"`
	}
	if err := json.Unmarshal(readBody, &readResp); err != nil {
		t.Fatalf("asset read unmarshal: %v body=%s", err, readBody)
	}
	if readResp.URL != "ai_clothing/e2e/scene.png" {
		t.Errorf("asset read: url=%q, want ai_clothing/e2e/scene.png", readResp.URL)
	}
	t.Logf("asset read: url=%s", readResp.URL)

	// 5. CheckVirtualIPAccessible（vip_id, scope）-> accessible
	vipCheckStatus, vipCheckBody := doJSON(t, internalClient, "POST", privateAddr+"/api/internal/v1/reachability:virtual-ip", map[string]any{
		"user_id":        uid,
		"workflow_scope": scope,
		"virtual_ip_id":  vip.ID,
	})
	if vipCheckStatus != 200 {
		t.Fatalf("check vip accessible: status=%d body=%s", vipCheckStatus, vipCheckBody)
	}
	var vipAccResp struct {
		Accessible bool   `json:"accessible"`
		Reason     string `json:"reason"`
	}
	json.Unmarshal(vipCheckBody, &vipAccResp)
	if !vipAccResp.Accessible {
		t.Errorf("check vip accessible: accessible=false reason=%q", vipAccResp.Reason)
	}

	// 6. CheckProductAccessible（scope mismatch）
	// 先建一个 ai_clothing product，再用 scope=other_scope 检查
	prodStatus, prodBody := materialsGatewayRequest(t, matAddr, "POST", "/api/v1/products?scope="+scope+"&created_by="+uid, token, map[string]any{
		"name":            "e2e_reach_product",
		"target_audience": "test",
	})
	if prodStatus != 201 {
		t.Fatalf("create product: status=%d body=%s", prodStatus, prodBody)
	}
	var prod struct {
		ID string `json:"id"`
	}
	json.Unmarshal(prodBody, &prod)
	defer materialsGatewayRequest(t, matAddr, "DELETE", "/api/v1/products/"+prod.ID, token, nil)

	prodCheckStatus, prodCheckBody := doJSON(t, internalClient, "POST", privateAddr+"/api/internal/v1/reachability:product", map[string]any{
		"workflow_scope": scope,
		"product_id":     prod.ID,
	})
	if prodCheckStatus != 200 {
		t.Fatalf("check product accessible: status=%d body=%s", prodCheckStatus, prodCheckBody)
	}
	var prodAccResp struct {
		Accessible bool   `json:"accessible"`
		Reason     string `json:"reason"`
	}
	json.Unmarshal(prodCheckBody, &prodAccResp)
	if !prodAccResp.Accessible {
		t.Errorf("check product accessible: accessible=false reason=%q", prodAccResp.Reason)
	}

	// scope mismatch
	prodMismatchStatus, _ := doJSON(t, internalClient, "POST", privateAddr+"/api/internal/v1/reachability:product", map[string]any{
		"workflow_scope": "other_scope",
		"product_id":     prod.ID,
	})
	if prodMismatchStatus != 200 {
		t.Fatalf("check product mismatch: status=%d", prodMismatchStatus)
	}
	json.Unmarshal(prodCheckBody, &prodAccResp) // re-read
	// 重新读 mismatch 响应
	prodMismatchBody := doJSONRaw(t, internalClient, "POST", privateAddr+"/api/internal/v1/reachability:product", map[string]any{
		"workflow_scope": "other_scope",
		"product_id":     prod.ID,
	})
	var mismatchResp struct {
		Accessible bool   `json:"accessible"`
		Reason     string `json:"reason"`
	}
	json.Unmarshal(prodMismatchBody, &mismatchResp)
	if mismatchResp.Accessible {
		t.Error("check product mismatch: expected accessible=false for scope mismatch")
	} else {
		t.Logf("check product mismatch: reason=%s", mismatchResp.Reason)
	}

	t.Log("✅ materials reachability e2e passed")
}

// doJSONRaw 同 doJSON 但仅返回 body（用于二次解析）。
// 避免覆盖外层 reachResp（map 重用导致的数据错位）。
func doJSONRaw(t *testing.T, c *http.Client, method, url string, body any) []byte {
	t.Helper()
	_, b := doJSON(t, c, method, url, body)
	return b
}

// TestMaterialsBindingsEndAt 验证 owner（virtual_ip）软删时 material_bindings 置 bindings_end_at，
// ListByOwner 不再返回已失效的 binding。
//
//	Setup: provisioned API Token + 创建 vip + material + binding
//	ListByOwner（virtual_ip, vip_id）-> 含 material
//	Delete vip -> 触发 EndBindingsForOwner（material_bindings.bindings_end_at = now）
//	ListByOwner（virtual_ip, vip_id）-> 空（binding 已失效）
//	Cleanup material
func TestMaterialsBindingsEndAt(t *testing.T) {
	uid, token := materialsBearerToken(t)
	matAddr := os.Getenv("MEDIA_GATEWAY_ADDR")
	scope := "ai_clothing"

	// 创建 vip
	status, body := materialsGatewayRequest(t, matAddr, "POST", "/api/v1/virtual-ips?scope="+scope+"&owner_user_id="+uid, token, map[string]any{
		"name":               "e2e_bindend_vip",
		"avatar_object_path": "ai_clothing/e2e/avatar.png",
		"visibility":         "public",
	})
	if status != 201 {
		t.Fatalf("create vip: status=%d body=%s", status, body)
	}
	var vip struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &vip); err != nil || vip.ID == "" {
		t.Fatalf("create vip unmarshal: err=%v body=%s", err, body)
	}
	t.Logf("created vip id=%s", vip.ID)
	defer materialsGatewayRequest(t, matAddr, "DELETE", "/api/v1/virtual-ips/"+vip.ID, token, nil)

	// 创建 material
	status, body = materialsGatewayRequest(t, matAddr, "POST", "/api/v1/materials?scope="+scope+"&owner_user_id="+uid, token, map[string]any{
		"name":        "e2e_bindend_scene",
		"asset_kind":  "scene",
		"object_path": "ai_clothing/e2e/scene.png",
		"visibility":  "public",
	})
	if status != 201 {
		t.Fatalf("create material: status=%d body=%s", status, body)
	}
	var mat struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &mat); err != nil || mat.ID == "" {
		t.Fatalf("create material unmarshal: err=%v body=%s", err, body)
	}
	t.Logf("created material id=%s", mat.ID)
	defer materialsGatewayRequest(t, matAddr, "DELETE", "/api/v1/materials/"+mat.ID, token, nil)

	// 绑定 material <-> vip
	status, _ = materialsGatewayRequest(t, matAddr, "POST", "/api/v1/materials/"+mat.ID+"/bindings?bound_by="+uid, token, map[string]any{
		"owner_kind": "virtual_ip",
		"owner_id":   vip.ID,
	})
	if status != 200 {
		t.Fatalf("bind material->vip: status=%d", status)
	}

	// ListByOwner -> 含 material
	listStatus, listBody := materialsGatewayRequest(t, matAddr, "GET", "/api/v1/materials/by-owner?owner_kind=virtual_ip&owner_id="+vip.ID, token, nil)
	if listStatus != 200 {
		t.Fatalf("list by owner (before delete): status=%d body=%s", listStatus, listBody)
	}
	var byOwner struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if err := json.Unmarshal(listBody, &byOwner); err != nil {
		t.Fatalf("list by owner unmarshal: %v body=%s", err, listBody)
	}
	if len(byOwner.Items) == 0 {
		t.Fatal("list by owner (before delete): empty (binding not found)")
	}
	t.Logf("list by owner (before delete): %d items", len(byOwner.Items))

	// Delete vip -> 触发 EndBindingsForOwner
	delStatus, _ := materialsGatewayRequest(t, matAddr, "DELETE", "/api/v1/virtual-ips/"+vip.ID, token, nil)
	if delStatus != 200 {
		t.Fatalf("delete vip: status=%d", delStatus)
	}
	t.Logf("deleted vip id=%s (should have ended bindings)", vip.ID)

	// ListByOwner -> 空（binding 已失效）
	listStatus, listBody = materialsGatewayRequest(t, matAddr, "GET", "/api/v1/materials/by-owner?owner_kind=virtual_ip&owner_id="+vip.ID, token, nil)
	if listStatus != 200 {
		t.Fatalf("list by owner (after delete): status=%d body=%s", listStatus, listBody)
	}
	if err := json.Unmarshal(listBody, &byOwner); err != nil {
		t.Fatalf("list by owner (after delete) unmarshal: %v body=%s", err, listBody)
	}
	if len(byOwner.Items) != 0 {
		t.Errorf("list by owner (after delete): expected 0 items, got %d (bindings_end_at not applied?)", len(byOwner.Items))
	} else {
		t.Logf("list by owner (after delete): 0 items (bindings_end_at applied correctly)")
	}

	t.Log("✅ materials bindings_end_at e2e passed")
}

// TestMaterialsGenPublishIntegration 验证跨服务集成：
// materials 生成视频 generated_asset -> media_gen PublishVideo（mock）发布。
//
// 前提：media_auth + media_materials + media_gen server 全部已起。
//
//	Setup: provisioned API Token
//	UpsertGeneratedAsset（internal RPC，media_type=VIDEO, asset_kind=final_video）
//	GetGeneratedAsset（公开 API，验 asset 存在）
//	media_gen PublishVideo（asset_id, target=xhs）-> SUCCEEDED（mock）
//	Cleanup
func TestMaterialsGenPublishIntegration(t *testing.T) {
	privateAddr := materialsPrivateAddress(t)
	matAddr := os.Getenv("MEDIA_GATEWAY_ADDR")
	genAddr := skipIfNoGenServer(t) // 来自 media_gen_e2e_test.go
	uid, token := materialsBearerToken(t)
	scope := "ai_clothing"
	assetURL := os.Getenv("MEDIA_E2E_VIDEO_OUTPUT_URL")
	if assetURL == "" {
		t.Skip("configure a real native-storage video fixture URL in the test organization")
	}
	// The current contract derives organization from a real source container.
	productStatus, productBody := materialsGatewayRequest(t, matAddr, "POST", "/api/v1/products?scope="+scope, token, map[string]any{"name": "e2e-publish-source"})
	var source struct {
		ID string `json:"id"`
	}
	if productStatus != 201 || json.Unmarshal(productBody, &source) != nil || source.ID == "" {
		t.Fatalf("publish source fixture unavailable: status=%d", productStatus)
	}
	defer materialsGatewayRequest(t, matAddr, "DELETE", "/api/v1/products/"+source.ID, token, nil)
	runKey := fmt.Sprintf("publish-e2e-%d", time.Now().UnixNano())

	// 1. UpsertGeneratedAsset（internal RPC，skip auth）
	// Requires an existing native video object; no fabricated external URL.
	internalClient := &http.Client{Timeout: 30 * time.Second}
	upsertStatus, upsertBody := doJSON(t, internalClient, "POST", privateAddr+"/api/internal/v1/generated-assets:upsert", map[string]any{
		"scope":             scope,
		"asset_kind":        "final_video",
		"name":              "e2e_gen_video",
		"url":               assetURL,
		"owner_user_id":     uid,
		"visibility":        "public",
		"job_id":            "job_" + runKey,
		"step_run_id":       "sr_" + runKey,
		"template_id":       "tpl_" + runKey,
		"template_version":  1,
		"source_product_id": source.ID,
	})
	if upsertStatus != 200 {
		t.Fatalf("upsert generated asset: status=%d", upsertStatus)
	}
	var upsertResp struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(upsertBody, &upsertResp); err != nil || upsertResp.ID == "" {
		t.Fatal("upsert generated asset did not return identity")
	}
	assetID := upsertResp.ID
	defer materialsGatewayRequest(t, matAddr, "DELETE", "/api/v1/generated-assets/"+assetID, token, nil)

	// 2. GetGeneratedAsset（公开 API，带 Bearer）-> 验 asset 存在 + url
	getStatus, getBody := materialsGatewayRequest(t, matAddr, "GET", "/api/v1/generated-assets/"+assetID, token, nil)
	if getStatus != 200 {
		t.Fatalf("get generated asset: status=%d body=%s", getStatus, getBody)
	}
	var genAsset struct {
		ID         string `json:"id"`
		AssetKind  string `json:"asset_kind"`
		URL        string `json:"url"`
		ObjectPath string `json:"object_path"`
		Scope      string `json:"scope"`
	}
	if err := json.Unmarshal(getBody, &genAsset); err != nil {
		t.Fatalf("get generated asset unmarshal: %v body=%s", err, getBody)
	}
	if genAsset.ID != assetID {
		t.Errorf("get generated asset: id=%q, want %q", genAsset.ID, assetID)
	}
	if genAsset.AssetKind != "final_video" {
		t.Errorf("get generated asset: asset_kind=%q, want final_video", genAsset.AssetKind)
	}
	if genAsset.URL != "" || genAsset.ObjectPath == "" {
		t.Error("native video must persist its object path, not an external or signed URL")
	}
	t.Logf("got generated_asset id=%s asset_kind=%s", genAsset.ID, genAsset.AssetKind)

	// 3. media_gen PublishVideo（mock 返回 SUCCEEDED + 假 url）
	// media_gen 无 auth 中间件，可直接裸调
	genClient := &http.Client{Timeout: 30 * time.Second}
	pubStatus, pubBody := doJSON(t, genClient, "POST", genAddr+"/api/v1/publish:video", map[string]any{
		"asset_id": assetID,
		"target":   "xhs",
		"metadata": map[string]string{
			"title": "e2e cross-service video",
		},
	})
	if pubStatus != 200 {
		t.Fatalf("publish video: status=%d body=%s", pubStatus, pubBody)
	}
	var pubResult struct {
		AssetID           string `json:"asset_id"`
		Target            string `json:"target"`
		State             int32  `json:"state"`
		PublishURL        string `json:"publish_url"`
		PlatformPublishID string `json:"platform_publish_id"`
	}
	if err := json.Unmarshal(pubBody, &pubResult); err != nil {
		t.Fatalf("publish video unmarshal: %v body=%s", err, pubBody)
	}
	if pubResult.State != 3 { // PUBLISH_SUCCEEDED
		t.Errorf("publish video: state=%d, want 3 (SUCCEEDED)", pubResult.State)
	}
	if pubResult.AssetID != assetID {
		t.Errorf("publish video: asset_id=%q, want %q", pubResult.AssetID, assetID)
	}
	if !strings.Contains(pubResult.PublishURL, assetID) {
		t.Errorf("publish video: publish_url=%q should contain asset_id %q", pubResult.PublishURL, assetID)
	}
	t.Logf("cross-service publish: asset=%s -> media_gen publish_url=%s", assetID, pubResult.PublishURL)

	t.Log("✅ materials + media_gen cross-service publish e2e passed")
}
