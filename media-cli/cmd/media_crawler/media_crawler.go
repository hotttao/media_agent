// Package media_crawler 承载 media_crawler 微服务的 CLI 子命令（一级 app 父命令）。
//
// 子命令树：
//
//	media-cli media_crawler crawler list-jobs / get-job   （REST，CrawlerQueryService）
//	media-cli media_crawler xhs list-notes / get-note / list-comments （REST，XhsQueryService）
//	media-cli media_crawler mcp start-crawler / get-status / stop / query-xhs-notes （MCP /mcp）
//
// host 来源：rootCmd 的 --host persistent flag（e2e 时传 http://localhost:8004）。
package media_crawler

import (
	"github.com/spf13/cobra"

	"media_agent/media_cli/cmd"
)

// MediaCrawlerCmd 一级子命令 media_crawler。
var MediaCrawlerCmd = &cobra.Command{
	Use:   "media_crawler",
	Short: "调用 media_crawler 微服务接口（REST + MCP）",
	Long: "media_crawler 是社交数据抓取编排服务。\n" +
		"本命令树暴露 REST 与 MCP 接口，e2e 时用 --host 指向被测服务：\n" +
		"  media-cli --host http://localhost:8004 media_crawler crawler list-jobs\n" +
		"  media-cli --host http://localhost:8004 media_crawler xhs list-notes\n" +
		"  media-cli --host http://localhost:8004 media_crawler mcp start-crawler --platform xhs --type search --keywords deepseek",
}

func init() {
	cmd.RegisterCmd(MediaCrawlerCmd)
}
