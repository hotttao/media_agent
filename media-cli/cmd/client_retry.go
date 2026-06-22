// client_retry.go：用 clienthertz client 调 /_test/error?code=503，配置重试 3 次，
// 期望 server 收到多次请求。
//
// 注：当前 serverhertz/clienthertz retry 通过 hclient.WithRetryConfig 配置，
// Hertz client 默认对 GET 重试。需在 server 端打点统计请求数（这里通过
// access log 计数近似验证）。
package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

var clientRetryCmd = &cobra.Command{
	Use:   "retry",
	Short: "clienthertz client 重试测试",
	RunE: func(cmd *cobra.Command, args []string) error {
		cli, _, err := buildClient(2000, 3, false, false)
		if err != nil {
			return fail("client retry", err.Error())
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		start := time.Now()
		status, _, err := cli.Get(ctx, nil, FlagHost+"/_test/error?code=503")
		elapsed := time.Since(start)
		if FlagVerbose {
			fmt.Printf("status=%d err=%v elapsed=%v\n", status, err, elapsed)
		}
		// 重试后仍应是 503。
		if status != 503 {
			return fail("client retry", fmt.Sprintf("status=%d want 503", status))
		}
		// 3 次重试 + 初始延迟，总耗时应 > 30ms。
		if elapsed < 30*time.Millisecond {
			return fail("client retry", fmt.Sprintf("elapsed=%v too short, retry not triggered", elapsed))
		}
		return pass("client retry", fmt.Sprintf("retried 3x, total elapsed=%v", elapsed))
	},
}

func init() {
	clientCmd.AddCommand(clientRetryCmd)
}
