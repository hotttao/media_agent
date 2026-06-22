// infra_requestid.go：请求带 X-Request-ID 期望响应回带；不带则 server 生成。
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var infraRequestIDCmd = &cobra.Command{
	Use:   "requestid",
	Short: "测试 X-Request-ID 中间件",
	RunE: func(cmd *cobra.Command, args []string) error {
		// 1. 客户端传入 id，期望透传。
		url := FlagHost + "/_test/echo"
		status, h, _, err := doRequest("GET", url, map[string]string{
			"X-Request-ID": "e2e-test-rid-123",
		}, nil)
		if err != nil {
			return fail("requestid", err.Error())
		}
		if status != 200 {
			return fail("requestid", fmt.Sprintf("status=%d", status))
		}
		got := h.Get("X-Request-ID")
		if got != "e2e-test-rid-123" {
			return fail("requestid", fmt.Sprintf("expected echo rid, got %q", got))
		}

		// 2. 不传 id，期望 server 生成一个非空 id。
		_, h2, _, err := doRequest("GET", url, nil, nil)
		if err != nil {
			return fail("requestid", "no-rid: "+err.Error())
		}
		if h2.Get("X-Request-ID") == "" {
			return fail("requestid", "server did not generate rid")
		}
		return pass("requestid", fmt.Sprintf("echo=%s generated=%s", got, h2.Get("X-Request-ID")))
	},
}

func init() {
	infraCmd.AddCommand(infraRequestIDCmd)
}
