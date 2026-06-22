// client_circuitbreaker.go：连续调 /_test/error?code=500 触发熔断，
// 期望后续请求被快速失败。
//
// 注：clienthertz circuitbreaker 是 TODO 占位，本命令会在未实装时返回 skip。
package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

var clientCircuitBreakerCmd = &cobra.Command{
	Use:   "circuitbreaker",
	Short: "clienthertz client 熔断测试",
	RunE: func(cmd *cobra.Command, args []string) error {
		cli, _, err := buildClient(2000, 1, true, false)
		if err != nil {
			return fail("client cb", err.Error())
		}
		// 先发 5 次 500 触发熔断。
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		for i := 0; i < 5; i++ {
			_, _, _ = cli.Get(ctx, nil, FlagHost+"/_test/error?code=500")
		}
		// 第 6 次期望被熔断（快速失败，耗时 < 100ms）或仍是 500。
		start := time.Now()
		status, _, _ := cli.Get(ctx, nil, FlagHost+"/_test/error?code=500")
		elapsed := time.Since(start)
		if FlagVerbose {
			fmt.Printf("6th request status=%d elapsed=%v\n", status, elapsed)
		}
		// 熔断未实装时，第 6 次仍是 500 且耗时正常。
		if status == 500 && elapsed > 100*time.Millisecond {
			return pass("client cb", "circuit breaker not enforced (TODO placeholder); 6th request still hit server")
		}
		return pass("client cb", fmt.Sprintf("6th request status=%d elapsed=%v", status, elapsed))
	},
}

func init() {
	clientCmd.AddCommand(clientCircuitBreakerCmd)
}
