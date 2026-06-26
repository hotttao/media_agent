package infra

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"media_agent/media_cli/cmd"
)

// infraPprofCmd 测试 /debug/pprof/ 端点。
var infraPprofCmd = &cobra.Command{
	Use:   "pprof",
	Short: "测试 /debug/pprof 端点",
	RunE: func(c *cobra.Command, args []string) error {
		url := cmd.FlagHost + "/debug/pprof/"
		status, _, body, err := cmd.DoRequest("GET", url, nil, nil)
		if err != nil {
			return cmd.Fail("pprof", err.Error())
		}
		if status != 200 {
			return cmd.Fail("pprof", fmt.Sprintf("status=%d want 200", status))
		}
		// pprof index 页含 "Types of profiles available"
		if !strings.Contains(body, "profile") {
			return cmd.Fail("pprof", "body missing 'profile' keyword")
		}
		return cmd.Pass("pprof", url)
	},
}

func init() {
	infraCmd.AddCommand(infraPprofCmd)
}
