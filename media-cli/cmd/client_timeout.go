// client_timeout.go：用生成的 typed client 调 Slow（/_test/slow?ms=5000），期望超时返回。
//
// 验证 clienthertz 注入的 timeout option（client.WithClientReadTimeout）生效：
// server 端 /_test/slow 延迟 5s，client 读超时 800ms → err 非 nil / 快速失败。
package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	testmodel "media_agent/hertz_gen/model/media_example/test"
)

var clientTimeoutCmd = &cobra.Command{
	Use:   "timeout",
	Short: "typed client 超时测试（clienthertz timeout option）",
	RunE: func(cmd *cobra.Command, args []string) error {
		cli, _, err := buildTestClient(800, 1, false)
		if err != nil {
			return fail("client timeout", err.Error())
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		ctx, endSpan := withSpan(ctx, "client.timeout")
		defer endSpan()

		start := time.Now()
		_, _, err = cli.Slow(ctx, &testmodel.TestSlowReq{Ms: 5000})
		elapsed := time.Since(start)
		if FlagVerbose {
			fmt.Printf("err=%v elapsed=%v\n", err, elapsed)
		}
		// client 读超时 800ms → err 非 nil 且耗时 < 2s。
		if err == nil {
			return fail("client timeout", "request succeeded, expected timeout error")
		}
		if elapsed > 2*time.Second {
			return fail("client timeout", fmt.Sprintf("timeout too late: %v", elapsed))
		}
		return pass("client timeout", fmt.Sprintf("timed out after %v (err=%v)", elapsed, err))
	},
}

func init() {
	clientCmd.AddCommand(clientTimeoutCmd)
}
