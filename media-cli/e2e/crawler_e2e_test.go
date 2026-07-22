// crawler_e2e_test.go - media_crawler HTTP + MCP e2e 测试。
//
// 前提：media_crawler server 已起（docker-compose 或手动）。
//   - REST 接口（/api/crawler/jobs*）仅需 PG（crawler jobs 表）。
//   - MCP 工具（/mcp，JSON-RPC Streamable HTTP）：
//       list_platforms / get_crawl_status / stop_crawler 仅需 PG；
//       start_crawler 写 job 到 PG 后会启动 Python subprocess -- 若 Python/shim 未配置
//       则 subprocess 启动失败，job 落 failed，但 job_id 仍同步返回，可测。
//   - xhs 数据查询（/api/xhs/notes*）需 MySQL crawl_data -- 默认 skip。
//
// 用 MEDIA_CRAWLER_ADDR 环境变量指定地址；未设则 skip。
//
// 运行：
//
//	MEDIA_CRAWLER_ADDR=http://127.0.0.1:8004 \
//	go test ./e2e/ -run TestCrawler -v
package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

// skipIfNoCrawlerServer skips the test when MEDIA_CRAWLER_ADDR is unset.
func skipIfNoCrawlerServer(t *testing.T) string {
	t.Helper()
	addr := os.Getenv("MEDIA_CRAWLER_ADDR")
	if addr == "" {
		t.Skip("MEDIA_CRAWLER_ADDR 未设，跳过 media_crawler e2e（需先起 media_crawler server）")
	}
	return addr
}

// crawlerHTTPClient 是 media_crawler 用的 HTTP client（无 auth，无 cookie jar）。
func crawlerHTTPClient(t *testing.T) *http.Client {
	t.Helper()
	return &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// mcpCallTool 完成 MCP 握手（Start + Initialize）后调 tools/call。
// 返回 tool 输出的 text content（多条拼接）+ IsError 标志。
func mcpCallTool(t *testing.T, addr, tool string, args map[string]any) (text string, isError bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	c, err := client.NewStreamableHttpClient(addr + "/mcp")
	if err != nil {
		t.Fatalf("mcp new client: %v", err)
	}
	defer c.Close()

	if err := c.Start(ctx); err != nil {
		t.Fatalf("mcp start: %v", err)
	}
	if _, err := c.Initialize(ctx, mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			ClientInfo:      mcp.Implementation{Name: "media-cli-e2e", Version: "v1"},
		},
	}); err != nil {
		t.Fatalf("mcp initialize: %v", err)
	}

	result, err := c.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{Name: tool, Arguments: args},
	})
	if err != nil {
		t.Fatalf("mcp call %s: %v", tool, err)
	}

	var sb strings.Builder
	for _, ct := range result.Content {
		if tc, ok := ct.(mcp.TextContent); ok {
			sb.WriteString(tc.Text)
		}
	}
	return sb.String(), result.IsError
}

// TestCrawlerMCPListPlatforms 测 MCP list_platforms（仅需 server 起，无 DB 依赖）。
func TestCrawlerMCPListPlatforms(t *testing.T) {
	addr := skipIfNoCrawlerServer(t)

	text, isError := mcpCallTool(t, addr, "list_platforms", map[string]any{})
	if isError {
		t.Fatalf("list_platforms returned error: %s", text)
	}

	// 解析返回的 JSON：[{platform: xhs, ...}, ...]
	var platforms []map[string]any
	if err := json.Unmarshal([]byte(text), &platforms); err != nil {
		t.Fatalf("list_platforms unmarshal: %v body=%s", err, text)
	}
	if len(platforms) == 0 {
		t.Fatal("list_platforms: empty")
	}

	// xhs 平台应存在
	var xhsFound bool
	for _, p := range platforms {
		if p["platform"] == "xhs" {
			xhsFound = true
			label, _ := p["label"].(string)
			if label == "" {
				t.Error("list_platforms: xhs.label empty")
			}
			crawlerTypes, _ := p["crawler_types"].([]any)
			if len(crawlerTypes) == 0 {
				t.Error("list_platforms: xhs.crawler_types empty")
			}
			break
		}
	}
	if !xhsFound {
		t.Errorf("list_platforms: xhs not found, body=%s", text)
	}
	t.Logf("list_platforms: %d platforms (xhs found)", len(platforms))
}

// TestCrawlerJobLifecycle 驱动 crawler job 全生命周期：
//
//	REST ListJobs（初始）
//	MCP start_crawler（异步 job，返回 job_id；subprocess 可能失败，job 落 failed/completed）
//	REST GetJob(job_id) -> 存在
//	REST ListJobs -> 含新 job
//	MCP get_crawl_status -> 返回 job 视图
//	MCP stop_crawler -> 幂等（已终态直接返回 cancelled）
//	REST GetJob(unknown) -> 404
func TestCrawlerJobLifecycle(t *testing.T) {
	addr := skipIfNoCrawlerServer(t)
	c := crawlerHTTPClient(t)

	// 1. REST ListJobs（初始）
	status, body := doJSON(t, c, "GET", addr+"/api/crawler/jobs?limit=10", nil)
	if status != 200 {
		t.Fatalf("list jobs: status=%d body=%s", status, body)
	}
	var listResp struct {
		Total int32 `json:"total"`
		Rows  []struct {
			JobID    string `json:"job_id"`
			Status   string `json:"status"`
			Platform string `json:"platform"`
		} `json:"rows"`
	}
	if err := json.Unmarshal(body, &listResp); err != nil {
		t.Fatalf("list jobs unmarshal: %v body=%s", err, body)
	}
	t.Logf("initial list jobs: total=%d", listResp.Total)

	// 2. MCP start_crawler（search 类型 + 关键词）
	text, isError := mcpCallTool(t, addr, "start_crawler", map[string]any{
		"platform":     "xhs",
		"crawler_type": "search",
		"keywords":     "e2e_test_keyword_" + time.Now().Format("150405"),
	})
	// start_crawler 同步返回 job_id 即使 subprocess 启动失败；不应 isError。
	if isError {
		t.Fatalf("start_crawler returned error: %s", text)
	}

	var startResp struct {
		JobID  string `json:"job_id"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal([]byte(text), &startResp); err != nil {
		t.Fatalf("start_crawler unmarshal: %v body=%s", err, text)
	}
	if startResp.JobID == "" {
		t.Fatalf("start_crawler: empty job_id, body=%s", text)
	}
	t.Logf("start_crawler: job_id=%s status=%s", startResp.JobID, startResp.Status)

	// 3. REST GetJob(job_id) -> 存在
	status, body = doJSON(t, c, "GET", addr+"/api/crawler/jobs/"+startResp.JobID, nil)
	if status != 200 {
		t.Fatalf("get job: status=%d body=%s", status, body)
	}
	var jobView struct {
		JobID      string `json:"job_id"`
		Status     string `json:"status"`
		Platform   string `json:"platform"`
		CrawlerType string `json:"crawler_type"`
		QueuedAt   int64  `json:"queued_at"`
	}
	if err := json.Unmarshal(body, &jobView); err != nil {
		t.Fatalf("get job unmarshal: %v body=%s", err, body)
	}
	if jobView.JobID != startResp.JobID {
		t.Errorf("get job: job_id=%q, want %q", jobView.JobID, startResp.JobID)
	}
	if jobView.Platform != "xhs" {
		t.Errorf("get job: platform=%q, want xhs", jobView.Platform)
	}
	if jobView.CrawlerType != "search" {
		t.Errorf("get job: crawler_type=%q, want search", jobView.CrawlerType)
	}
	if jobView.QueuedAt == 0 {
		t.Error("get job: queued_at not set")
	}
	t.Logf("get job: job_id=%s status=%s platform=%s", jobView.JobID, jobView.Status, jobView.Platform)

	// 4. REST ListJobs -> 含新建的
	status, body = doJSON(t, c, "GET", addr+"/api/crawler/jobs?platform=xhs&limit=50", nil)
	if status != 200 {
		t.Fatalf("list jobs after start: status=%d body=%s", status, body)
	}
	if err := json.Unmarshal(body, &listResp); err != nil {
		t.Fatalf("list jobs unmarshal: %v body=%s", err, body)
	}
	found := false
	for _, r := range listResp.Rows {
		if r.JobID == startResp.JobID {
			found = true
			if r.Platform != "xhs" {
				t.Errorf("list jobs row: platform=%q, want xhs", r.Platform)
			}
			break
		}
	}
	if !found {
		t.Errorf("list jobs: missing %s in %d rows", startResp.JobID, len(listResp.Rows))
	}
	t.Logf("list jobs after start: total=%d rows=%d (new job found)", listResp.Total, len(listResp.Rows))

	// 5. MCP get_crawl_status -> 返回 job 视图
	text, isError = mcpCallTool(t, addr, "get_crawl_status", map[string]any{
		"job_id": startResp.JobID,
	})
	if isError {
		t.Errorf("get_crawl_status returned error: %s", text)
	}
	// 返回的 job 视图应含 job_id
	if !strings.Contains(text, startResp.JobID) {
		t.Errorf("get_crawl_status: body=%q should contain job_id %q", text, startResp.JobID)
	} else {
		t.Logf("get_crawl_status: returned job view")
	}

	// 6. MCP stop_crawler -> 幂等（已终态直接 cancelled，未终态则停止）
	text, isError = mcpCallTool(t, addr, "stop_crawler", map[string]any{
		"job_id":  startResp.JobID,
		"signal":  "term",
	})
	// stop_crawler 对已终态 job 返回 cancelled（幂等，不算 error）
	if isError {
		t.Logf("stop_crawler returned error (may be already terminal): %s", text)
	} else {
		t.Logf("stop_crawler: %s", text)
	}

	// 7. 等待 subprocess 结束（若启动了），最多 5s
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		status, body = doJSON(t, c, "GET", addr+"/api/crawler/jobs/"+startResp.JobID, nil)
		if status != 200 {
			break
		}
		json.Unmarshal(body, &jobView)
		if jobView.Status == "completed" || jobView.Status == "failed" || jobView.Status == "cancelled" {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Logf("final job status: %s", jobView.Status)

	// 8. REST GetJob(unknown) -> 404
	status, _ = doJSON(t, c, "GET", addr+"/api/crawler/jobs/nonexistent_job_xyz", nil)
	if status != 404 {
		t.Errorf("get unknown job: status=%d, want 404", status)
	}

	// 9. REST ListJobs with invalid platform filter（返回空，不报错）
	status, body = doJSON(t, c, "GET", addr+"/api/crawler/jobs?platform=invalid_platform&limit=10", nil)
	if status != 200 {
		t.Errorf("list jobs with invalid platform: status=%d, want 200", status)
	}

	t.Log("✅ media_crawler e2e passed")
}

// TestCrawlerMCPHealthCheck 测 MCP health_check 占位 tool（不需 DB）。
func TestCrawlerMCPHealthCheck(t *testing.T) {
	addr := skipIfNoCrawlerServer(t)

	text, isError := mcpCallTool(t, addr, "health_check", map[string]any{})
	if isError {
		t.Fatalf("health_check returned error: %s", text)
	}
	if !strings.Contains(text, "ok") {
		t.Errorf("health_check: body=%q should contain 'ok'", text)
	}
	t.Logf("health_check: %s", text)
}

// TestCrawlerStartCrawlerValidation 测 start_crawler 的入参校验（不需 subprocess 真正启动）。
func TestCrawlerStartCrawlerValidation(t *testing.T) {
	addr := skipIfNoCrawlerServer(t)

	// 1. 无效 platform -> error
	text, isError := mcpCallTool(t, addr, "start_crawler", map[string]any{
		"platform":     "invalid_platform",
		"crawler_type": "search",
		"keywords":     "test",
	})
	if !isError {
		t.Errorf("start_crawler invalid platform: expected error, got %s", text)
	} else {
		t.Logf("invalid platform rejected: %s", text)
	}

	// 2. search 无 keywords -> error
	text, isError = mcpCallTool(t, addr, "start_crawler", map[string]any{
		"platform":     "xhs",
		"crawler_type": "search",
	})
	if !isError {
		t.Errorf("start_crawler search without keywords: expected error, got %s", text)
	} else {
		t.Logf("search without keywords rejected: %s", text)
	}

	// 3. 无效 crawler_type -> error
	text, isError = mcpCallTool(t, addr, "start_crawler", map[string]any{
		"platform":     "xhs",
		"crawler_type": "invalid_type",
		"keywords":     "test",
	})
	if !isError {
		t.Errorf("start_crawler invalid crawler_type: expected error, got %s", text)
	} else {
		t.Logf("invalid crawler_type rejected: %s", text)
	}
}

// 保证未使用的 fmt import 不被编译器报错（测试里用 fmt.Sprintf 做 log 格式化）。
var _ = fmt.Sprintf
