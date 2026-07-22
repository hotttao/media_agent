// media_gen_e2e_test.go - media_gen HTTP e2e 测试（裸 net/http）。
//
// 前提：media_gen server 已起（docker-compose 或手动）。
// 用 MEDIA_GEN_ADDR 环境变量指定地址；未设则 skip。
//
// media_gen 无 auth 中间件（worker / publish handler 均 nil middleware），可直接裸 HTTP 调用。
// worker loop 本期 stub（goroutine 启动后 select { <-ctx.Done() }，不实际 Claim），
//   故 e2e 测 RPC 生命周期（Start/Stop/Status/List）+ publish mock，不测 job 全链路。
//
// 运行：
//
//	MEDIA_GEN_ADDR=http://127.0.0.1:8005 \
//	go test ./e2e/ -run TestMediaGen -v
package e2e

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// skipIfNoGenServer skips the test when MEDIA_GEN_ADDR is unset.
func skipIfNoGenServer(t *testing.T) string {
	t.Helper()
	addr := os.Getenv("MEDIA_GEN_ADDR")
	if addr == "" {
		t.Skip("MEDIA_GEN_ADDR 未设，跳过 media_gen e2e（需先起 media_gen server）")
	}
	return addr
}

// genClient 是 media_gen 用的 HTTP client（无 cookie jar，无 redirect 跟随）。
// media_gen 无 auth，不需要 session。
func genClient(t *testing.T) *http.Client {
	t.Helper()
	return &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// TestMediaGenHealth 验证 media_gen server 已起 + /health 返回 ok。
func TestMediaGenHealth(t *testing.T) {
	addr := skipIfNoGenServer(t)
	c := genClient(t)

	status, body := doJSON(t, c, "GET", addr+"/health", nil)
	if status != 200 {
		t.Fatalf("health: status=%d body=%s", status, body)
	}
	var resp struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("health unmarshal: %v body=%s", err, body)
	}
	if resp.Status != "ok" {
		t.Errorf("health: status=%q, want ok", resp.Status)
	}
	t.Logf("media_gen health: %s", resp.Status)
}

// TestMediaGenWorkerLifecycle 驱动 worker 全生命周期：
//
//	Start -> GetStatus(RUNNING) -> List(includes) -> Start(again=conflict 409)
//	-> Stop -> GetStatus(STOPPED) -> Stop(again=idempotent) -> GetStatus(unknown)=404
//	-> List(state=RUNNING) 不含已 STOPPED
func TestMediaGenWorkerLifecycle(t *testing.T) {
	addr := skipIfNoGenServer(t)
	c := genClient(t)
	workerID := "e2e_worker_" + time.Now().Format("150405")

	// 1. Start worker
	status, body := doJSON(t, c, "POST", addr+"/api/v1/workers:start", map[string]any{
		"worker_id":       workerID,
		"platform":        "gpt",
		"gen_way":         "i2i",
		"gen_type":        "dall_e_3",
		"poll_interval_ms": 1000,
	})
	if status != 200 {
		t.Fatalf("start worker: status=%d body=%s", status, body)
	}
	var info struct {
		WorkerID       string `json:"worker_id"`
		Platform       string `json:"platform"`
		GenWay         string `json:"gen_way"`
		GenType        string `json:"gen_type"`
		State          int32  `json:"state"`
		PollIntervalMs int32  `json:"poll_interval_ms"`
		StartedAt      int64  `json:"started_at"`
		StoppedAt      int64  `json:"stopped_at"`
	}
	if err := json.Unmarshal(body, &info); err != nil {
		t.Fatalf("start worker unmarshal: %v body=%s", err, body)
	}
	if info.WorkerID != workerID || info.Platform != "gpt" || info.GenWay != "i2i" {
		t.Errorf("start worker: got %+v", info)
	}
	if info.State != 2 { // WORKER_RUNNING
		t.Errorf("start worker: state=%d, want 2 (RUNNING)", info.State)
	}
	if info.StartedAt == 0 {
		t.Error("start worker: started_at not set")
	}
	t.Logf("started worker id=%s state=%d started_at=%d", info.WorkerID, info.State, info.StartedAt)

	// 2. GetStatus -> RUNNING
	status, body = doJSON(t, c, "GET", addr+"/api/v1/workers/"+workerID, nil)
	if status != 200 {
		t.Fatalf("get worker status: status=%d body=%s", status, body)
	}
	if err := json.Unmarshal(body, &info); err != nil {
		t.Fatalf("get worker unmarshal: %v body=%s", err, body)
	}
	if info.State != 2 {
		t.Errorf("get worker: state=%d, want 2 (RUNNING)", info.State)
	}

	// 3. List -> 含新建的
	status, body = doJSON(t, c, "GET", addr+"/api/v1/workers", nil)
	if status != 200 {
		t.Fatalf("list workers: status=%d body=%s", status, body)
	}
	var listResp struct {
		Items []struct {
			WorkerID string `json:"worker_id"`
			State    int32  `json:"state"`
		} `json:"items"`
		Total int32 `json:"total"`
	}
	if err := json.Unmarshal(body, &listResp); err != nil {
		t.Fatalf("list workers unmarshal: %v body=%s", err, body)
	}
	if listResp.Total == 0 {
		t.Fatal("list workers: empty")
	}
	found := false
	for _, it := range listResp.Items {
		if it.WorkerID == workerID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("list workers: missing %s, items=%+v", workerID, listResp.Items)
	}
	t.Logf("list workers: total=%d (includes %s)", listResp.Total, workerID)

	// 4. Start again (same worker_id, RUNNING) -> 409 Conflict
	status, _ = doJSON(t, c, "POST", addr+"/api/v1/workers:start", map[string]any{
		"worker_id": workerID,
		"platform":  "gpt",
		"gen_way":   "i2i",
	})
	if status != 409 {
		t.Errorf("start worker again: status=%d, want 409 (already running)", status)
	}

	// 5. Stop worker -> STOPPED
	status, body = doJSON(t, c, "POST", addr+"/api/v1/workers/"+workerID+":stop", map[string]any{})
	if status != 200 {
		t.Fatalf("stop worker: status=%d body=%s", status, body)
	}
	if err := json.Unmarshal(body, &info); err != nil {
		t.Fatalf("stop worker unmarshal: %v body=%s", err, body)
	}
	if info.State != 1 { // WORKER_STOPPED
		t.Errorf("stop worker: state=%d, want 1 (STOPPED)", info.State)
	}
	if info.StoppedAt == 0 {
		t.Error("stop worker: stopped_at not set")
	}
	t.Logf("stopped worker id=%s stopped_at=%d", info.WorkerID, info.StoppedAt)

	// 6. Stop again (idempotent) -> 200 + STOPPED
	status, body = doJSON(t, c, "POST", addr+"/api/v1/workers/"+workerID+":stop", map[string]any{})
	if status != 200 {
		t.Errorf("stop worker again: status=%d, want 200 (idempotent)", status)
	}

	// 7. GetStatus -> STOPPED
	status, body = doJSON(t, c, "GET", addr+"/api/v1/workers/"+workerID, nil)
	if status != 200 {
		t.Fatalf("get stopped worker: status=%d body=%s", status, body)
	}
	if err := json.Unmarshal(body, &info); err != nil {
		t.Fatalf("get stopped worker unmarshal: %v body=%s", err, body)
	}
	if info.State != 1 {
		t.Errorf("get stopped worker: state=%d, want 1 (STOPPED)", info.State)
	}

	// 8. Get unknown worker -> 404
	status, _ = doJSON(t, c, "GET", addr+"/api/v1/workers/nonexistent_worker_xyz", nil)
	if status != 404 {
		t.Errorf("get unknown worker: status=%d, want 404", status)
	}

	// 9. Start without worker_id -> 400 (validation)
	status, _ = doJSON(t, c, "POST", addr+"/api/v1/workers:start", map[string]any{
		"platform": "gpt",
		"gen_way":  "i2i",
	})
	if status != 400 {
		t.Errorf("start worker without id: status=%d, want 400", status)
	}

	// 10. Start without platform -> 400 (validation)
	status, _ = doJSON(t, c, "POST", addr+"/api/v1/workers:start", map[string]any{
		"worker_id": "e2e_invalid",
		"gen_way":   "i2i",
	})
	if status != 400 {
		t.Errorf("start worker without platform: status=%d, want 400", status)
	}

	// 11. List with state=RUNNING filter
	status, body = doJSON(t, c, "GET", addr+"/api/v1/workers?state=RUNNING", nil)
	if status != 200 {
		t.Fatalf("list running workers: status=%d body=%s", status, body)
	}
	if err := json.Unmarshal(body, &listResp); err != nil {
		t.Fatalf("list running workers unmarshal: %v body=%s", err, body)
	}
	for _, it := range listResp.Items {
		if it.State != 2 {
			t.Errorf("list running workers: item %s state=%d, want 2", it.WorkerID, it.State)
		}
	}

	t.Log("✅ media_gen worker lifecycle e2e passed")
}

// TestMediaGenPublish 驱动 publish mock：
//
//	GetSupportedTargets -> [xhs douyin bilibili]
//	PublishVideo(valid) -> SUCCEEDED + mock publish_url
//	PublishVideo(empty asset_id) -> 400 + FAILED
//	PublishVideo(unsupported target) -> 400 + FAILED
func TestMediaGenPublish(t *testing.T) {
	addr := skipIfNoGenServer(t)
	c := genClient(t)

	// 1. GetSupportedTargets
	status, body := doJSON(t, c, "GET", addr+"/api/v1/publish/targets", nil)
	if status != 200 {
		t.Fatalf("get supported targets: status=%d body=%s", status, body)
	}
	var targetsResp struct {
		Targets []string `json:"targets"`
	}
	if err := json.Unmarshal(body, &targetsResp); err != nil {
		t.Fatalf("get supported targets unmarshal: %v body=%s", err, body)
	}
	wantTargets := map[string]bool{"xhs": true, "douyin": true, "bilibili": true}
	if len(targetsResp.Targets) != 3 {
		t.Errorf("get supported targets: got %v, want 3 items", targetsResp.Targets)
	}
	for _, tgt := range targetsResp.Targets {
		if !wantTargets[tgt] {
			t.Errorf("get supported targets: unexpected target %q", tgt)
		}
	}
	t.Logf("supported targets: %v", targetsResp.Targets)

	// 2. PublishVideo success (mock)
	assetID := "e2e_asset_" + time.Now().Format("150405")
	status, body = doJSON(t, c, "POST", addr+"/api/v1/publish:video", map[string]any{
		"asset_id": assetID,
		"target":   "xhs",
		"metadata": map[string]string{
			"title": "e2e test video",
			"tags":  "test,e2e",
		},
	})
	if status != 200 {
		t.Fatalf("publish video: status=%d body=%s", status, body)
	}
	var pubResult struct {
		AssetID           string `json:"asset_id"`
		Target            string `json:"target"`
		State             int32  `json:"state"`
		PublishURL        string `json:"publish_url"`
		PlatformPublishID string `json:"platform_publish_id"`
		PublishedAt       int64  `json:"published_at"`
		FailureReason     string `json:"failure_reason"`
	}
	if err := json.Unmarshal(body, &pubResult); err != nil {
		t.Fatalf("publish video unmarshal: %v body=%s", err, body)
	}
	if pubResult.State != 3 { // PUBLISH_SUCCEEDED
		t.Errorf("publish video: state=%d, want 3 (SUCCEEDED)", pubResult.State)
	}
	if pubResult.AssetID != assetID || pubResult.Target != "xhs" {
		t.Errorf("publish video: asset_id=%q target=%q, want %q/%q",
			pubResult.AssetID, pubResult.Target, assetID, "xhs")
	}
	if pubResult.PublishURL == "" {
		t.Error("publish video: empty publish_url")
	}
	if !strings.Contains(pubResult.PublishURL, assetID) {
		t.Errorf("publish video: publish_url=%q should contain asset_id %q", pubResult.PublishURL, assetID)
	}
	if pubResult.PlatformPublishID == "" {
		t.Error("publish video: empty platform_publish_id")
	}
	if pubResult.PublishedAt == 0 {
		t.Error("publish video: published_at not set")
	}
	t.Logf("publish video success: state=%d publish_url=%s", pubResult.State, pubResult.PublishURL)

	// 3. PublishVideo with empty asset_id -> 400 + FAILED
	status, body = doJSON(t, c, "POST", addr+"/api/v1/publish:video", map[string]any{
		"asset_id": "",
		"target":   "xhs",
	})
	if status != 400 {
		t.Errorf("publish video empty asset_id: status=%d, want 400", status)
	}
	if !strings.Contains(string(body), "asset_id is required") {
		t.Errorf("publish video empty asset_id: body=%q should mention 'asset_id is required'", body)
	}

	// 4. PublishVideo with unsupported target -> 400 + FAILED
	status, body = doJSON(t, c, "POST", addr+"/api/v1/publish:video", map[string]any{
		"asset_id": assetID,
		"target":   "wechat",
	})
	if status != 400 {
		t.Errorf("publish video unsupported target: status=%d, want 400", status)
	}
	if !strings.Contains(string(body), "unsupported target") {
		t.Errorf("publish video unsupported target: body=%q should mention 'unsupported target'", body)
	}

	// 5. PublishVideo to douyin (verify all 3 targets work)
	for _, tgt := range []string{"douyin", "bilibili"} {
		status, body = doJSON(t, c, "POST", addr+"/api/v1/publish:video", map[string]any{
			"asset_id": assetID,
			"target":   tgt,
		})
		if status != 200 {
			t.Errorf("publish video to %s: status=%d body=%s", tgt, status, body)
			continue
		}
		if err := json.Unmarshal(body, &pubResult); err != nil {
			t.Errorf("publish video to %s unmarshal: %v", tgt, err)
			continue
		}
		if pubResult.State != 3 {
			t.Errorf("publish video to %s: state=%d, want 3 (SUCCEEDED)", tgt, pubResult.State)
		}
		if !strings.Contains(pubResult.PublishURL, tgt) {
			t.Errorf("publish video to %s: publish_url=%q should contain target", tgt, pubResult.PublishURL)
		}
	}

	t.Log("✅ media_gen publish e2e passed")
}
