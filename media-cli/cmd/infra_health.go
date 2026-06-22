// infra_health.go：GET /health 返回 200 + {"status":"ok"}。
package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var infraHealthCmd = &cobra.Command{
	Use:   "health",
	Short: "测试 /health 端点",
	RunE: func(cmd *cobra.Command, args []string) error {
		url := FlagHost + "/health"
		status, _, body, err := doRequest("GET", url, nil, nil)
		if err != nil {
			return fail("health", err.Error())
		}
		if status != 200 {
			return fail("health", fmt.Sprintf("status=%d want 200, body=%s", status, body))
		}
		if !strings.Contains(body, `"status":"ok"`) {
			return fail("health", fmt.Sprintf("body missing status:ok: %s", body))
		}
		return pass("health", url)
	},
}

func init() {
	infraCmd.AddCommand(infraHealthCmd)
}
