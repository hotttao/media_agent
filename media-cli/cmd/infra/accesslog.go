package infra

import (
	"fmt"

	"github.com/spf13/cobra"

	"media_agent/media_cli/cmd"
)

// infraAccessLogCmd 触发一条 access log（验证中间件挂载）。
//
// 该测试不能自动断言日志（日志在 server 进程 stdout），所以只验证请求成功；
// 详细断言由人工或 server side test 完成。
var infraAccessLogCmd = &cobra.Command{
	Use:   "accesslog",
	Short: "触发一条 access log（验证中间件挂载）",
	RunE: func(c *cobra.Command, args []string) error {
		url := cmd.FlagHost + "/_test/echo?name=accesslog-test"
		status, _, _, err := cmd.DoRequest("GET", url, nil, nil)
		if err != nil {
			return cmd.Fail("accesslog", err.Error())
		}
		if status != 200 {
			return cmd.Fail("accesslog", fmt.Sprintf("status=%d", status))
		}
		return cmd.Pass("accesslog", "request sent; check server stdout for access log line")
	},
}

func init() {
	infraCmd.AddCommand(infraAccessLogCmd)
}
