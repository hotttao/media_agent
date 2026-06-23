// client_timeout.go：用 clienthertz client 调 /_test/slow?ms=5000，期望超时返回。
package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

var clientTimeoutCmd = &cobra.Command{
	Use:   "timeout",
	Short: "clienthertz client 超时测试",
	RunE: func(cmd *cobra.Command, args []string) error {
		cli, _, err := buildClient(800, 1, false, false)
		if err != nil {
			return fail("client timeout", err.Error())
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		ctx, endSpan := withSpan(ctx, "client.timeout")
		defer endSpan()

		start := time.Now()
		status, body, err := cli.Get(ctx, nil, FlagHost+"/_test/slow?ms=5000")
		elapsed := time.Since(start)
		if FlagVerbose {
			fmt.Printf("status=%d body=%s err=%v elapsed=%v\n", status, string(body), err, elapsed)
		}
		// client 侧超时通常返回 err 非 nil 或 status != 200。
		if status == 200 && err == nil {
			return fail("client timeout", "request succeeded, expected timeout")
		}
		if elapsed > 2*time.Second {
			return fail("client timeout", fmt.Sprintf("timeout too late: %v", elapsed))
		}
		return pass("client timeout", fmt.Sprintf("timed out after %v", elapsed))
	},
}

func init() {
	clientCmd.AddCommand(clientTimeoutCmd)
}
