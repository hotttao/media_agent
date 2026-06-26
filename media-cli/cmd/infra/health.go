package infra

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"media_agent/media_cli/cmd"
)

// infraHealthCmd 测试 /health 端点。
var infraHealthCmd = &cobra.Command{
	Use:   "health",
	Short: "测试 /health 端点",
	RunE: func(c *cobra.Command, args []string) error {
		url := cmd.FlagHost + "/health"
		status, _, body, err := cmd.DoRequest("GET", url, nil, nil)
		if err != nil {
			return cmd.Fail("health", err.Error())
		}
		if status != 200 {
			return cmd.Fail("health", fmt.Sprintf("status=%d want 200, body=%s", status, body))
		}
		if !strings.Contains(body, `"status":"ok"`) {
			return cmd.Fail("health", fmt.Sprintf("body missing status:ok: %s", body))
		}
		return cmd.Pass("health", url)
	},
}

func init() {
	infraCmd.AddCommand(infraHealthCmd)
}
