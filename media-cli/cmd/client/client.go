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
// 用 suite.NewClient() 产出一个共享 *hclient.Client（已烘焙 timeout/retry/discovery/
// tracing/metrics/circuitbreaker），再通过 WithHertzClient 注入 typed client。
//
// 同一下游 app 的多个 typed service stub 应共享同一个 NewClient() 产物：连接池、
// 中间件链、discovery resolver 都按 app 复用，避免每个 service 各起一个 client。
// 用 WithHertzClient 后不要再传 WithHertzClientOption / WithHertzClientMiddleware
// （前者会被静默丢弃，后者会在共享 client 上重复 Use）——治理已烘焙进 shared。
func buildTestClient(timeoutMs, maxAttempts int32, cbEnabled bool) (testclient.Client, error) {
	suite, err := buildClientSuite(timeoutMs, maxAttempts, cbEnabled)
	if err != nil {
		return nil, err
	}
	shared, err := suite.NewClient()
	if err != nil {
		return nil, fmt.Errorf("build shared client: %w", err)
	}
	cli, err := testclient.NewTestServiceClient(cmd.FlagHost, testclient.WithHertzClient(shared))
	if err != nil {
		return nil, fmt.Errorf("create test client: %w", err)
	}
	return cli, nil
}
