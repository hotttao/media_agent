// infra_pprof.go：GET /debug/pprof/ 返回 200 且内容含 pprof 标识。
package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var infraPprofCmd = &cobra.Command{
	Use:   "pprof",
	Short: "测试 /debug/pprof 端点",
	RunE: func(cmd *cobra.Command, args []string) error {
		url := FlagHost + "/debug/pprof/"
		status, _, body, err := doRequest("GET", url, nil, nil)
		if err != nil {
			return fail("pprof", err.Error())
		}
		if status != 200 {
			return fail("pprof", fmt.Sprintf("status=%d want 200", status))
		}
		// pprof index 页含 "Types of profiles available"
		if !strings.Contains(body, "profile") {
			return fail("pprof", "body missing 'profile' keyword")
		}
		return pass("pprof", url)
	},
}

func init() {
	infraCmd.AddCommand(infraPprofCmd)
}
