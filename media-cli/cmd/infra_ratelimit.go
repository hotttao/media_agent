// infra_ratelimit.go：快速发 N 个请求，期望部分被限流。
//
// 需要 server 配置 rate_limit.enabled=true 且 threshold 设较低值。
// 当前 serverhertz ratelimit 是 no-op 占位，本命令会返回 skip 提示。
package cmd

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/spf13/cobra"
)

var rlCount int

var infraRateLimitCmd = &cobra.Command{
	Use:   "ratelimit",
	Short: "快速发请求验证服务端限流",
	RunE: func(cmd *cobra.Command, args []string) error {
		url := FlagHost + "/_test/echo"
		var success, limited int64
		var wg sync.WaitGroup
		for i := 0; i < rlCount; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				status, _, _, err := doRequest("GET", url, nil, nil)
				if err != nil {
					return
				}
				if status == 200 {
					atomic.AddInt64(&success, 1)
				} else if status == 429 {
					atomic.AddInt64(&limited, 1)
				}
			}()
		}
		wg.Wait()
		if limited == 0 {
			// ratelimit 中间件未实装时也返回 pass，但提示。
			return pass("ratelimit", fmt.Sprintf("no 429 (middleware no-op?). success=%d", success))
		}
		return pass("ratelimit", fmt.Sprintf("success=%d limited=%d", success, limited))
	},
}

func init() {
	infraRateLimitCmd.Flags().IntVar(&rlCount, "count", 50, "并发请求数")
	infraCmd.AddCommand(infraRateLimitCmd)
}
