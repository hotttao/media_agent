package infra

import (
	"fmt"

	"github.com/spf13/cobra"

	"media_agent/media_cli/cmd"
)

// infraCorsCmd 测试 CORS 预检：OPTIONS 带 Origin，期望响应含 Access-Control-Allow-Origin。
var infraCorsCmd = &cobra.Command{
	Use:   "cors",
	Short: "测试 CORS 预检",
	RunE: func(c *cobra.Command, args []string) error {
		url := cmd.FlagHost + "/_test/echo"
		status, h, _, err := cmd.DoRequest("OPTIONS", url, map[string]string{
			"Origin":                        "https://example.com",
			"Access-Control-Request-Method": "GET",
		}, nil)
		if err != nil {
			return cmd.Fail("cors", err.Error())
		}
		if status < 200 || status >= 300 {
			return cmd.Fail("cors", fmt.Sprintf("status=%d", status))
		}
		acao := h.Get("Access-Control-Allow-Origin")
		if acao == "" {
			return cmd.Fail("cors", "missing Access-Control-Allow-Origin header")
		}
		return cmd.Pass("cors", fmt.Sprintf("ACAO=%s", acao))
	},
}

func init() {
	infraCmd.AddCommand(infraCorsCmd)
}
