// client.go：客户端治理能力 e2e 测试。
//
// 用 clienthertz.NewClientSuiteFromConfig + ForApp 构造 app 级 suite，
// 再用生成的 typed client（hertz_gen/media_example/test_service.TestServiceClient）
// + suite 的 clientOption / middlewares 装配，调用 /_test/* 端点验证治理行为。
//
// 真正的 e2e：media-cli → hertz_gen typed client → clienthertz 治理 → media_example server。
// media-cli 无 loader（内存 cfg 装配），进程级可观测性由 Execute → initCLIObservability 统一初始化。
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	hertzclient "github.com/cloudwego/hertz/pkg/app/client"
	configpb "media_agent/hertz_gen/config"
	testclient "media_agent/hertz_gen/media_example/test_service"
	"media_agent/hertz_infra/clienthertz"
)

var clientCmd = &cobra.Command{
	Use:   "client",
	Short: "客户端治理能力 e2e 测试（生成 typed client + clienthertz 治理）",
}

// buildTestClient 构造一个调本机 media-example 的 typed TestServiceClient，挂 clienthertz 治理。
//
// 通过生成的 NewTestServiceClient(host, WithHertzClientOption(opts), WithHertzClientMiddleware(mws))
// 注入 timeout/retry（option）+ tracing/circuitbreaker（middleware）。
// OTel 开关跟随 cliConfig().Otel（OTEL_ENABLED）。
func buildTestClient(timeoutMs, maxAttempts int32, cbEnabled bool) (testclient.Client, *clienthertz.ClientSuite, error) {
	oc := cliConfig()
	cfg := &configpb.Config{
		App:             oc.GetApp(),
		Otel:            oc.GetOtel(), // 跟随 OTEL_ENABLED
		Prometheus:      &configpb.PrometheusConfig{Enabled: false},
		ConsulDiscovery: &configpb.ConsulDiscoveryConfig{Enabled: false},
		Client: map[string]*configpb.AppClientConfig{
			"media_example": {
				Enabled:     true,
				ServiceName: "media-example",
				BaseDomain:  FlagHost,
				Timeout: &configpb.TimeoutConfig{
					Enabled:        true,
					ReadTimeoutMs:  timeoutMs,
					WriteTimeoutMs: timeoutMs,
				},
				Retry: &configpb.RetryConfig{
					Enabled:         maxAttempts > 1,
					MaxAttemptTimes: maxAttempts,
					InitialDelayMs:  10,
					MaxDelayMs:      100,
				},
				CircuitBreaker: &configpb.CircuitBreakerConfig{
					Enabled:          cbEnabled,
					Strategy:         "error_ratio",
					Threshold:        0.5,
					StatIntervalMs:   10000,
					RetryTimeoutMs:   5000,
					MinRequestAmount: 5,
				},
			},
		},
	}
	// media-cli 内存 cfg 装配（无 loader，不订阅）。
	base := clienthertz.NewClientSuiteFromConfig(cfg)
	suite, err := base.ForApp("media_example")
	if err != nil {
		return nil, nil, err
	}
	// 注入治理：clientOption（timeout/retry）+ middlewares（tracing/circuitbreaker）。
	opts := []testclient.Option{
		testclient.WithHertzClientOption(suite.Options()...),
		testclient.WithHertzClientMiddleware(suite.Middlewares()...),
	}
	cli, err := testclient.NewTestServiceClient(FlagHost, opts...)
	if err != nil {
		return nil, nil, fmt.Errorf("create test client: %w", err)
	}
	return cli, suite, nil
}

// hertzClientFromSuite 构造裸 hertz client（仅在需要直接拿 *hertzclient.Client 时用）。
// 当前 e2e 走 typed client，保留此辅助以备未来需要。
func hertzClientFromSuite(suite *clienthertz.ClientSuite) (*hertzclient.Client, error) {
	return suite.NewClient()
}

func init() {
	RegisterCmd(clientCmd)
}
