package infra

import (
	"fmt"

	"github.com/spf13/cobra"

	"media_agent/media_cli/cmd"
)

// infraRecoveryCmd 测试 panic recovery 中间件：GET /_test/panic 期望 500。
var infraRecoveryCmd = &cobra.Command{
	Use:   "recovery",
	Short: "测试 panic recovery 中间件",
	RunE: func(c *cobra.Command, args []string) error {
		url := cmd.FlagHost + "/_test/panic"
		status, _, _, err := cmd.DoRequest("GET", url, nil, nil)
		if err != nil {
			return cmd.Fail("recovery", err.Error())
		}
		// panic 被 recovery 兜底后，Hertz 默认返回 500。
		if status != 500 {
			return cmd.Fail("recovery", fmt.Sprintf("status=%d want 500 (panic should be recovered)", status))
		}

		// 再发一个正常请求，确认进程没崩。
		status2, _, _, err := cmd.DoRequest("GET", cmd.FlagHost+"/_test/echo", nil, nil)
		if err != nil || status2 != 200 {
			return cmd.Fail("recovery", "server died after panic")
		}
		return cmd.Pass("recovery", "panic recovered, server still alive")
	},
}

func init() {
	infraCmd.AddCommand(infraRecoveryCmd)
}
