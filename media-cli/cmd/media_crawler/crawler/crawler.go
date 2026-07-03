package crawler

import (
	"github.com/spf13/cobra"

	"media_agent/media_cli/cmd/media_crawler"
)

// CrawlerCmd 二级子命令 crawler（CrawlerQueryService，REST job 查询）。
var CrawlerCmd = &cobra.Command{
	Use:   "crawler",
	Short: "CrawlerQueryService（REST，job 查询）",
}

func init() {
	media_crawler.MediaCrawlerCmd.AddCommand(CrawlerCmd)
}
