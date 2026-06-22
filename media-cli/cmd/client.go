// client.go：客户端治理能力 e2e 测试。
//
// 用 clienthertz.BuildBaseSuite + BuildAppSuite 构造一个 suite，
// 再用 hclient.NewClient(suite.Options()) 装配 client，
// 通过 ApplyMiddlewares / ApplyDiscovery 挂载治理 middleware，
// 然后调用 /_test/* 端点验证治理行为。
package cmd

import (
	"fmt"

	hclient "github.com/cloudwego/hertz/pkg/app/client"
	"github.com/spf13/cobra"

	configpb "media_agent/hertz_gen/config"
	"media_agent/hertz_infra/clienthertz"
)

var clientCmd = &cobra.Command{
	Use:   "client",
	Short: "客户端治理能力 e2e 测试",
}

// buildClient 构造一个调本机 media-example 的 hertz client，挂治理 middleware。
func buildClient(timeoutMs, maxAttempts int32, cbEnabled, rlEnabled bool) (*hclient.Client, *clienthertz.ClientSuite, error) {
	cfg := &configpb.Config{
		App: &configpb.AppConfig{Name: "media-cli", Version: "v1", Env: "local"},
		Otel: &configpb.OtelConfig{Enabled: false},
		Prometheus: &configpb.PrometheusConfig{Enabled: false},
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
					Enabled: cbEnabled,
					Strategy:           "error_ratio",
					Threshold:          0.5,
					StatIntervalMs:     10000,
					RetryTimeoutMs:     5000,
					MinRequestAmount:   5,
				},
				RateLimit: &configpb.RateLimitConfig{
					Enabled:  rlEnabled,
					Threshold: 5,
				},
			},
		},
	}
	base := clienthertz.BuildBaseSuite(cfg)
	suite := clienthertz.BuildAppSuite(base, "media_example", cfg.Client["media_example"])
	cli, err := hclient.NewClient(suite.Options()...)
	if err != nil {
		return nil, nil, err
	}
	clienthertz.ApplyMiddlewares(cli, suite)
	return cli, suite, nil
}

func init() {
	RegisterCmd(clientCmd)
	_ = fmt.Sprintf
}
