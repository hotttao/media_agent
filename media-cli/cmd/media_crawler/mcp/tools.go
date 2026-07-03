package mcp

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/spf13/cobra"

	"media_agent/media_cli/cmd"
)

// callTool 完成 MCP 握手（Start + Initialize）后调 tools/call。
func callTool(tool string, args map[string]any) (*mcp.CallToolResult, error) {
	ctx := context.Background()
	c, err := client.NewStreamableHttpClient(cmd.FlagHost + "/mcp")
	if err != nil {
		return nil, err
	}
	defer c.Close()
	if err := c.Start(ctx); err != nil {
		return nil, fmt.Errorf("mcp start: %w", err)
	}
	if _, err := c.Initialize(ctx, mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			ClientInfo:      mcp.Implementation{Name: "media-cli", Version: "v1"},
		},
	}); err != nil {
		return nil, fmt.Errorf("mcp initialize: %w", err)
	}
	return c.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{Name: tool, Arguments: args},
	})
}

// printResult 打印 tool 返回的 text content；IsError 时返回 error。
func printResult(r *mcp.CallToolResult) error {
	for _, ct := range r.Content {
		if t, ok := ct.(mcp.TextContent); ok {
			fmt.Println(t.Text)
		}
	}
	if r.IsError {
		return fmt.Errorf("tool returned error")
	}
	return nil
}

func startCrawlerCmd() *cobra.Command {
	var platform, crawlerType, keywords, onBusy string
	var maxNotes int
	c := &cobra.Command{
		Use:   "start-crawler",
		Short: "MCP tool start_crawler（异步 job，返回 job_id）",
		RunE: func(c *cobra.Command, args []string) error {
			r, err := callTool("start_crawler", map[string]any{
				"platform":        platform,
				"crawler_type":    crawlerType,
				"keywords":        keywords,
				"on_busy":         onBusy,
				"max_notes_count": maxNotes,
			})
			if err != nil {
				return err
			}
			return printResult(r)
		},
	}
	c.Flags().StringVar(&platform, "platform", "xhs", "平台")
	c.Flags().StringVar(&crawlerType, "type", "search", "search|detail|creator|homefeed|hot_search")
	c.Flags().StringVar(&keywords, "keywords", "", "逗号分隔关键词（search 必填）")
	c.Flags().StringVar(&onBusy, "on-busy", "queue", "queue|reject")
	c.Flags().IntVar(&maxNotes, "max-notes", 0, "覆盖 CRAWLER_MAX_NOTES_COUNT（0=默认 200）")
	_ = c.MarkFlagRequired("keywords")
	return c
}

func getStatusCmd() *cobra.Command {
	var jobID string
	c := &cobra.Command{
		Use:   "get-status",
		Short: "MCP tool get_crawl_status",
		RunE: func(c *cobra.Command, args []string) error {
			r, err := callTool("get_crawl_status", map[string]any{"job_id": jobID})
			if err != nil {
				return err
			}
			return printResult(r)
		},
	}
	c.Flags().StringVar(&jobID, "job-id", "", "job id")
	_ = c.MarkFlagRequired("job-id")
	return c
}

func stopCmd() *cobra.Command {
	var jobID, signal string
	c := &cobra.Command{
		Use:   "stop",
		Short: "MCP tool stop_crawler",
		RunE: func(c *cobra.Command, args []string) error {
			r, err := callTool("stop_crawler", map[string]any{"job_id": jobID, "signal": signal})
			if err != nil {
				return err
			}
			return printResult(r)
		},
	}
	c.Flags().StringVar(&jobID, "job-id", "", "job id")
	c.Flags().StringVar(&signal, "signal", "term", "term|kill")
	_ = c.MarkFlagRequired("job-id")
	return c
}

func queryXhsNotesCmd() *cobra.Command {
	var keywords, sortField, sortOrder string
	var limit, offset int
	c := &cobra.Command{
		Use:   "query-xhs-notes",
		Short: "MCP tool query_xhs_notes",
		RunE: func(c *cobra.Command, args []string) error {
			toolArgs := map[string]any{
				"sort_field": sortField, "sort_order": sortOrder,
				"limit": limit, "offset": offset,
			}
			if keywords != "" {
				toolArgs["keywords"] = keywords
			}
			r, err := callTool("query_xhs_notes", toolArgs)
			if err != nil {
				return err
			}
			return printResult(r)
		},
	}
	c.Flags().StringVar(&keywords, "keywords", "", "逗号分隔关键词")
	c.Flags().StringVar(&sortField, "sort-field", "time", "time|liked_count|comment_count")
	c.Flags().StringVar(&sortOrder, "sort-order", "desc", "desc|asc")
	c.Flags().IntVar(&limit, "limit", 20, "分页大小")
	c.Flags().IntVar(&offset, "offset", 0, "偏移")
	return c
}

func init() {
	McpCmd.AddCommand(startCrawlerCmd())
	McpCmd.AddCommand(getStatusCmd())
	McpCmd.AddCommand(stopCmd())
	McpCmd.AddCommand(queryXhsNotesCmd())
}
