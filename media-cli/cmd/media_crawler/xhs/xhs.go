package xhs

import (
	"github.com/spf13/cobra"

	"media_agent/media_cli/cmd/media_crawler"
)

// XhsCmd 二级子命令 xhs（XhsQueryService，REST 小红书数据查询）。
var XhsCmd = &cobra.Command{
	Use:   "xhs",
	Short: "XhsQueryService（REST，小红书笔记/评论查询）",
}

func init() {
	media_crawler.MediaCrawlerCmd.AddCommand(XhsCmd)
}
