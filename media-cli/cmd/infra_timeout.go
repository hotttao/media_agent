// infra_timeout.go：服务端 /_test/slow?ms=5000，client 超时 1s，期望 client 超时。
//
// 这是 client 侧超时；server 侧超时通过 server.timeout 配置，需另测。
package cmd

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/spf13/cobra"
)

var infraTimeoutCmd = &cobra.Command{
	Use:   "timeout",
	Short: "测试 client 超时（slow endpoint + 1s client timeout）",
	RunE: func(cmd *cobra.Command, args []string) error {
		url := FlagHost + "/_test/slow?ms=5000"
		client := &http.Client{Timeout: 1 * time.Second}
		req, _ := http.NewRequestWithContext(context.Background(), "GET", url, nil)
		start := time.Now()
		_, err := client.Do(req)
		elapsed := time.Since(start)
		if err == nil {
			return fail("timeout", "request succeeded, expected timeout")
		}
		if elapsed > 2*time.Second {
			return fail("timeout", fmt.Sprintf("timeout too late: %v", elapsed))
		}
		return pass("timeout", fmt.Sprintf("client timeout after %v", elapsed))
	},
}

func init() {
	infraCmd.AddCommand(infraTimeoutCmd)
}
