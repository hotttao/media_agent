package infra

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/spf13/cobra"

	"media_agent/media_cli/cmd"
)

// infraTimeoutCmd 测试 client 超时：服务端 /_test/slow?ms=5000，client 超时 1s。
//
// 这是 client 侧超时；server 侧超时通过 server.timeout 配置，需另测。
var infraTimeoutCmd = &cobra.Command{
	Use:   "timeout",
	Short: "测试 client 超时（slow endpoint + 1s client timeout）",
	RunE: func(c *cobra.Command, args []string) error {
		url := cmd.FlagHost + "/_test/slow?ms=5000"
		client := &http.Client{Timeout: 1 * time.Second}
		req, _ := http.NewRequestWithContext(context.Background(), "GET", url, nil)
		start := time.Now()
		_, err := client.Do(req)
		elapsed := time.Since(start)
		if err == nil {
			return fmt.Errorf("❌ timeout: request succeeded, expected timeout")
		}
		if elapsed > 2*time.Second {
			return fmt.Errorf("❌ timeout: timeout too late: %v", elapsed)
		}
		// 本命令直接用 fmt + 返回 error（不走 testharness 是因为没用 DoRequest）
		if cmd.FlagVerbose {
			fmt.Printf("✅ timeout: client timeout after %v (err=%v)\n", elapsed, err)
		} else {
			fmt.Printf("✅ timeout\n")
		}
		return nil
	},
}

func init() {
	infraCmd.AddCommand(infraTimeoutCmd)
}
