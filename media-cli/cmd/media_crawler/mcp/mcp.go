package mcp

import (
	"github.com/spf13/cobra"

	"media_agent/media_cli/cmd/media_crawler"
)

// McpCmd 二级子命令 mcp（调用 MCP tool，POST /mcp JSON-RPC）。
var McpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "调用 MCP tool（/mcp，JSON-RPC）",
	Long: "通过 MCP Streamable HTTP 协议调用 media_crawler 的 tool。\n" +
		"e2e 时 --host 指向被测服务：media-cli --host http://localhost:8004 media_crawler mcp start-crawler ...",
}

func init() {
	media_crawler.MediaCrawlerCmd.AddCommand(McpCmd)
}
