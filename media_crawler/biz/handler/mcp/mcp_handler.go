// Package mcp 注册 MCP tool（mcp-go，Streamable HTTP，挂 /mcp）。
// 接入层定位：转 command → 调 service → marshal response → mcp.CallToolResult。
// handler 内手动组装输出（不走默认生成），proto message 仅作 struct 复用（见 mcp_crawler/mcp_xhs.proto）。
package mcp

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/adaptor"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	crawljobsvc "media_agent/media_crawler/biz/service/crawl_job"
	xhsdatasvc "media_agent/media_crawler/biz/service/xhs_data"
)

// Register 在 hertz 上注册 10 个 MCP tool 并挂 /mcp endpoint。
func Register(h *server.Hertz, crawlSvc *crawljobsvc.Service, xhsSvc *xhsdatasvc.Service) {
	s := mcpserver.NewMCPServer("media_crawler", "v0.1.0")

	// ---- 编排类（平台无关）----

	s.AddTool(mcp.NewTool("list_platforms",
		mcp.WithDescription("列出支持的平台及爬虫类型")),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return jsonResult(listPlatforms())
		})

	s.AddTool(mcp.NewTool("start_crawler",
		mcp.WithDescription("提交抓取任务（异步 job，返回 job_id）"),
		mcp.WithString("platform", mcp.Required(), mcp.Enum("xhs", "dy", "ks", "bili", "wb", "tieba", "zhihu")),
		mcp.WithString("crawler_type", mcp.Required(), mcp.Enum("search", "detail", "creator", "homefeed", "hot_search")),
		mcp.WithString("keywords", mcp.Description("search 必填，逗号分隔")),
		mcp.WithString("urls", mcp.Description("detail/creator 必填，逗号分隔")),
		mcp.WithBoolean("enable_comments"),
		mcp.WithBoolean("enable_sub_comments"),
		mcp.WithNumber("max_notes_count"),
		mcp.WithString("save_data_option", mcp.Enum("csv", "db", "json")),
		mcp.WithNumber("start_page"),
		mcp.WithBoolean("enable_checkpoint"),
		mcp.WithString("checkpoint_id"),
		mcp.WithString("on_busy", mcp.Enum("queue", "reject")),
	),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			a, _ := req.Params.Arguments.(map[string]any)
			cmd := crawljobsvc.StartCrawlerCommand{
				Platform:          asStr(a["platform"]),
				CrawlerType:       asStr(a["crawler_type"]),
				Keywords:          asStr(a["keywords"]),
				URLs:              splitCSV(asStr(a["urls"])),
				EnableComments:    asBool(a["enable_comments"]),
				EnableSubComments: asBool(a["enable_sub_comments"]),
				MaxNotesCount:     asInt(a["max_notes_count"]),
				SaveDataOption:    asStr(a["save_data_option"]),
				StartPage:         asInt(a["start_page"]),
				EnableCheckpoint:  asBool(a["enable_checkpoint"]),
				CheckpointID:      asStr(a["checkpoint_id"]),
				OnBusy:            asStr(a["on_busy"]),
			}
			jobID, err := crawlSvc.StartCrawler(ctx, cmd)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return jsonResult(map[string]any{"job_id": jobID, "status": "queued"})
		})

	s.AddTool(mcp.NewTool("get_crawl_status",
		mcp.WithDescription("查询任务状态（含进度/摘要）"),
		mcp.WithString("job_id", mcp.Required()),
		mcp.WithNumber("include_logs_tail")),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			job, err := crawlSvc.GetCrawlStatus(ctx, asStr(arg(req, "job_id")))
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return jsonResult(job)
		})

	s.AddTool(mcp.NewTool("stop_crawler",
		mcp.WithDescription("取消任务（SIGTERM/SIGKILL 子进程）"),
		mcp.WithString("job_id", mcp.Required()),
		mcp.WithString("signal", mcp.Enum("term", "kill"))),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			a, _ := req.Params.Arguments.(map[string]any)
			if err := crawlSvc.StopCrawler(ctx, asStr(a["job_id"]), asStr(a["signal"])); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return jsonResult(map[string]any{"status": "cancelled"})
		})

	s.AddTool(mcp.NewTool("list_jobs",
		mcp.WithDescription("列出抓取任务"),
		mcp.WithString("platform"), mcp.WithString("status"),
		mcp.WithNumber("limit"), mcp.WithNumber("offset")),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			a, _ := req.Params.Arguments.(map[string]any)
			limit := asInt(a["limit"])
			if limit == 0 {
				limit = 50
			}
			jobs, total, err := crawlSvc.ListJobs(ctx, crawljobsvc.ListJobsQuery{
				Platform: asStr(a["platform"]),
				Status:   asStr(a["status"]),
				Limit:    limit,
				Offset:   asInt(a["offset"]),
			})
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return jsonResult(map[string]any{"total": total, "rows": jobs})
		})

	// get_config / health_check / list_checkpoints / delete_checkpoint：v1 占位，后续实现
	s.AddTool(mcp.NewTool("get_config", mcp.WithDescription("查运行配置（v1 占位）")),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return jsonResult(map[string]any{"note": "TODO: db/redis/sign/accounts 配置"})
		})
	s.AddTool(mcp.NewTool("health_check", mcp.WithDescription("健康检查（v1 占位）")),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return jsonResult(map[string]any{"overall": "ok"})
		})
	s.AddTool(mcp.NewTool("list_checkpoints", mcp.WithDescription("列断点（v1 占位）"),
		mcp.WithString("platform"), mcp.WithString("mode")),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return jsonResult(map[string]any{"checkpoints": []any{}})
		})
	s.AddTool(mcp.NewTool("delete_checkpoint", mcp.WithDescription("删断点（v1 占位）"),
		mcp.WithString("checkpoint_id", mcp.Required())),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return jsonResult(map[string]any{"deleted": false})
		})

	// ---- xhs 数据查询 ----

	s.AddTool(mcp.NewTool("query_xhs_notes",
		mcp.WithDescription("查询小红书笔记"),
		mcp.WithString("keywords"), mcp.WithString("ids"),
		mcp.WithString("source_keyword"),
		mcp.WithNumber("time_start"), mcp.WithNumber("time_end"),
		mcp.WithString("sort_field", mcp.Enum("time", "liked_count", "comment_count")),
		mcp.WithString("sort_order", mcp.Enum("desc", "asc")),
		mcp.WithNumber("limit"), mcp.WithNumber("offset")),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			a, _ := req.Params.Arguments.(map[string]any)
			limit := asInt(a["limit"])
			if limit == 0 {
				limit = 50
			}
			notes, total, err := xhsSvc.QueryNotes(ctx, xhsdatasvc.QueryXhsNotesCommand{
				Keywords:      splitCSV(asStr(a["keywords"])),
				IDs:           splitCSV(asStr(a["ids"])),
				SourceKeyword: asStr(a["source_keyword"]),
				TimeStart:     int64(asInt(a["time_start"])),
				TimeEnd:       int64(asInt(a["time_end"])),
				SortField:     asStr(a["sort_field"]),
				SortOrder:     asStr(a["sort_order"]),
				Limit:         limit,
				Offset:        asInt(a["offset"]),
			})
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return jsonResult(map[string]any{"total": total, "rows": notes})
		})

	s.AddTool(mcp.NewTool("query_xhs_comments",
		mcp.WithDescription("查询小红书评论"),
		mcp.WithString("note_id", mcp.Required()),
		mcp.WithNumber("limit"), mcp.WithNumber("offset")),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			a, _ := req.Params.Arguments.(map[string]any)
			limit := asInt(a["limit"])
			if limit == 0 {
				limit = 50
			}
			comments, total, err := xhsSvc.QueryComments(ctx, xhsdatasvc.QueryXhsCommentsCommand{
				NoteID: asStr(a["note_id"]),
				Limit:  limit,
				Offset: asInt(a["offset"]),
			})
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return jsonResult(map[string]any{"total": total, "rows": comments})
		})

	// 挂 /mcp（Streamable HTTP，经 hertz adaptor 转 stdlib http.Handler）
	httpServer := mcpserver.NewStreamableHTTPServer(s)
	h.POST("/mcp", adaptor.HertzHandler(httpServer))
	h.GET("/mcp", adaptor.HertzHandler(httpServer))
	h.DELETE("/mcp", adaptor.HertzHandler(httpServer))
}

func jsonResult(v any) (*mcp.CallToolResult, error) {
	b, _ := json.Marshal(v)
	return mcp.NewToolResultText(string(b)), nil
}

// arg 从 mcp tool 入参（any，实际是 map[string]any）取值；nil map 索引返回零值不 panic。
func arg(req mcp.CallToolRequest, key string) any {
	a, _ := req.Params.Arguments.(map[string]any)
	return a[key]
}

func asStr(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
func asInt(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	}
	return 0
}
func asBool(v any) bool {
	b, _ := v.(bool)
	return b
}
func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// listPlatforms 内置平台元数据（与 xhs_data domain 对齐，v1 仅 xhs 完整）。
func listPlatforms() []map[string]any {
	return []map[string]any{{
		"platform":         "xhs",
		"label":            "小红书",
		"crawler_types":    []string{"search", "detail", "creator", "homefeed", "hot_search"},
		"data_tables":      map[string]string{"content": "xhs_note", "comment": "xhs_note_comment", "creator": "xhs_creator"},
		"content_id_field": "note_id",
		"time_field":       "time",
		"needs_login":      true,
	}}
}
