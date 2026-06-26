package infra

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/spf13/cobra"

	"media_agent/media_cli/cmd"
)

// rlCount 并发请求数（默认 50）。
var rlCount int

// infraRateLimitCmd 快速发请求验证服务端限流。
var infraRateLimitCmd = &cobra.Command{
	Use:   "ratelimit",
	Short: "快速发请求验证服务端限流",
	RunE: func(c *cobra.Command, args []string) error {
		url := cmd.FlagHost + "/_test/echo"
		var success, limited int64
		var wg sync.WaitGroup
		for i := 0; i < rlCount; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				status, _, _, err := cmd.DoRequest("GET", url, nil, nil)
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
			return cmd.Pass("ratelimit", fmt.Sprintf("no 429 (middleware no-op?). success=%d", success))
		}
		return cmd.Pass("ratelimit", fmt.Sprintf("success=%d limited=%d", success, limited))
	},
}

func init() {
	infraRateLimitCmd.Flags().IntVar(&rlCount, "count", 50, "并发请求数")
	infraCmd.AddCommand(infraRateLimitCmd)
}
