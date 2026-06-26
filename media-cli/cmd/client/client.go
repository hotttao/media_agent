// client.go — cmd/client 的 typed client（TestServiceClient）构造。
//
// 「suite 的初始化归 suite ,client 归 client」：本文件只管 typed client 的构造；
// ClientSuite（clienthertz）的初始化与构造见 suite.go。
//
// 共享工具（位于父包 cmd / infra）：
//   - HTTP 测试辅助：cmd.DoRequest / cmd.Pass / cmd.Fail / cmd.Skip / cmd.HTTPClient
//   - 可观测性 span：infra.WithSpan（infra 包，全局 OTel provider）
//   - 全局 flag（--host / -v）：cmd.FlagHost / cmd.FlagVerbose
package client

import (
	"fmt"

	"github.com/spf13/cobra"

	testclient "media_agent/hertz_gen/media_example/test_service"
	"media_agent/media_cli/cmd"
)

// clientCmd 是一级子命令 client，承载本分类下所有治理功能。
var clientCmd = &cobra.Command{
	Use:   "client",
	Short: "客户端治理能力 e2e 测试（生成 typed client + clienthertz 治理）",
}

// init 把自身挂到 rootCmd；治理功能子命令通过各自的 init() 把自己挂到本命令。
func init() {
	cmd.RegisterCmd(clientCmd)
}

// buildTestClient 构造调本机 media-example 的 typed TestServiceClient，挂 clienthertz 治理。
//
// suite 部分（ClientSuite）由 suite.go 的 buildClientSuite 构造；本函数拿到 suite 后
// 用其 Options() / Middlewares() 注入 typed client（option 给 timeout/retry，
// middleware 给 tracing/circuitbreaker）。
func buildTestClient(timeoutMs, maxAttempts int32, cbEnabled bool) (testclient.Client, error) {
	suite, err := buildClientSuite(timeoutMs, maxAttempts, cbEnabled)
	if err != nil {
		return nil, err
	}
	opts := []testclient.Option{
		testclient.WithHertzClientOption(suite.Options()...),
		testclient.WithHertzClientMiddleware(suite.Middlewares()...),
	}
	cli, err := testclient.NewTestServiceClient(cmd.FlagHost, opts...)
	if err != nil {
		return nil, fmt.Errorf("create test client: %w", err)
	}
	return cli, nil
}
