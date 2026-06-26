package infra

import (
	"fmt"

	"github.com/spf13/cobra"

	"media_agent/media_cli/cmd"
)

// infraRequestIDCmd 测试 X-Request-ID 中间件。
var infraRequestIDCmd = &cobra.Command{
	Use:   "requestid",
	Short: "测试 X-Request-ID 中间件",
	RunE: func(c *cobra.Command, args []string) error {
		// 1. 客户端传入 id，期望透传。
		url := cmd.FlagHost + "/_test/echo"
		status, h, _, err := cmd.DoRequest("GET", url, map[string]string{
			"X-Request-ID": "e2e-test-rid-123",
		}, nil)
		if err != nil {
			return cmd.Fail("requestid", err.Error())
		}
		if status != 200 {
			return cmd.Fail("requestid", fmt.Sprintf("status=%d", status))
		}
		got := h.Get("X-Request-ID")
		if got != "e2e-test-rid-123" {
			return cmd.Fail("requestid", fmt.Sprintf("expected echo rid, got %q", got))
		}

		// 2. 不传 id，期望 server 生成一个非空 id。
		_, h2, _, err := cmd.DoRequest("GET", url, nil, nil)
		if err != nil {
			return cmd.Fail("requestid", "no-rid: "+err.Error())
		}
		if h2.Get("X-Request-ID") == "" {
			return cmd.Fail("requestid", "server did not generate rid")
		}
		return cmd.Pass("requestid", fmt.Sprintf("echo=%s generated=%s", got, h2.Get("X-Request-ID")))
	},
}

func init() {
	infraCmd.AddCommand(infraRequestIDCmd)
}
