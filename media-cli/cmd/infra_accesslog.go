// infra_accesslog.go：发一个请求，提示用户检查 server 日志输出。
//
// 该测试不能自动断言日志（日志在 server 进程 stdout），所以只验证请求成功；
// 详细断言由人工或 server side test 完成。
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var infraAccessLogCmd = &cobra.Command{
	Use:   "accesslog",
	Short: "触发一条 access log（验证中间件挂载）",
	RunE: func(cmd *cobra.Command, args []string) error {
		url := FlagHost + "/_test/echo?name=accesslog-test"
		status, _, _, err := doRequest("GET", url, nil, nil)
		if err != nil {
			return fail("accesslog", err.Error())
		}
		if status != 200 {
			return fail("accesslog", fmt.Sprintf("status=%d", status))
		}
		return pass("accesslog", "request sent; check server stdout for access log line")
	},
}

func init() {
	infraCmd.AddCommand(infraAccessLogCmd)
}
