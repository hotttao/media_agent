// infra_cors.go：OPTIONS 预检带 Origin，期望响应含 Access-Control-Allow-Origin。
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var infraCorsCmd = &cobra.Command{
	Use:   "cors",
	Short: "测试 CORS 预检",
	RunE: func(cmd *cobra.Command, args []string) error {
		url := FlagHost + "/_test/echo"
		status, h, _, err := doRequest("OPTIONS", url, map[string]string{
			"Origin":                        "https://example.com",
			"Access-Control-Request-Method": "GET",
		}, nil)
		if err != nil {
			return fail("cors", err.Error())
		}
		if status < 200 || status >= 300 {
			return fail("cors", fmt.Sprintf("status=%d", status))
		}
		acao := h.Get("Access-Control-Allow-Origin")
		if acao == "" {
			return fail("cors", "missing Access-Control-Allow-Origin header")
		}
		return pass("cors", fmt.Sprintf("ACAO=%s", acao))
	},
}

func init() {
	infraCmd.AddCommand(infraCorsCmd)
}
