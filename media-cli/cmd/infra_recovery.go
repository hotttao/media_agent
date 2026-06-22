// infra_recovery.go：GET /_test/panic 期望 500（被 recovery 兜底），进程不崩。
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var infraRecoveryCmd = &cobra.Command{
	Use:   "recovery",
	Short: "测试 panic recovery 中间件",
	RunE: func(cmd *cobra.Command, args []string) error {
		url := FlagHost + "/_test/panic"
		status, _, _, err := doRequest("GET", url, nil, nil)
		if err != nil {
			return fail("recovery", err.Error())
		}
		// panic 被 recovery 兜底后，Hertz 默认返回 500。
		if status != 500 {
			return fail("recovery", fmt.Sprintf("status=%d want 500 (panic should be recovered)", status))
		}

		// 再发一个正常请求，确认进程没崩。
		status2, _, _, err := doRequest("GET", FlagHost+"/_test/echo", nil, nil)
		if err != nil || status2 != 200 {
			return fail("recovery", "server died after panic")
		}
		return pass("recovery", "panic recovered, server still alive")
	},
}

func init() {
	infraCmd.AddCommand(infraRecoveryCmd)
}
