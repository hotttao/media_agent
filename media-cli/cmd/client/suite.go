// suite.go — cmd/client 的 ClientSuite 初始化与构造。
//
// 「suite 的初始化归 suite」：本文件只管 ClientSuite（clienthertz.ClientSuite）
// 的配置与构造；typed client（hertz_gen TestServiceClient）的构造见 client.go。
//
// 配置来源（避免"乱倒配置"）：
//   - main.go 加载 cfg 后调 client.Init(cfg)：本文件从 cfg.GetApp() / cfg.GetOtel()
//     捕获到包级 var appCfg / otelCfg，供 NewClientSuite 构造 ClientSuite 时使用；
//   - cfg 本身不持有；测试命令通过 buildClientSuite(per-test params) 复用。
package client

import (
	configpb "media_agent/hertz_gen/config"
	"media_agent/hertz_infra/clienthertz"
	config "media_agent/hertz_infra/config"
	"media_agent/media_cli/cmd"
)

// appCfg / otelCfg 由 Init 注入；NewClientSuite 用它们构造 ClientSuite 所需的 configpb.Config。
var (
	appCfg  *configpb.AppConfig
	otelCfg *configpb.OtelConfig
)

// Init 由 main.go 在加载 cfg 完成后调用。
//
// 仅捕获本包需要的 App / Otel 两段；cfg 本身不持有，避免"乱倒配置"。
func Init(cfg *config.Config) {
	appCfg = cfg.GetApp()
	otelCfg = cfg.GetOtel()
}

// buildClientSuite 构造 clienthertz.ClientSuite（用于装配 timeout / retry / circuit_breaker 治理）。
//
// per-test 参数（timeoutMs / maxAttempts / cbEnabled）由各测试命令提供，
// 与 cfg 的 App/Otel 段合并构造 configpb.Config。
func buildClientSuite(timeoutMs, maxAttempts int32, cbEnabled bool) (*clienthertz.ClientSuite, error) {
	cfg := &configpb.Config{
		App:             appCfg,
		Otel:            otelCfg, // 跟随 cfg.otel.enabled
		Prometheus:      &configpb.PrometheusConfig{Enabled: false},
		ConsulDiscovery: &configpb.ConsulDiscoveryConfig{Enabled: false},
		Client: map[string]*configpb.AppClientConfig{
			"media_example": {
				Enabled:     true,
				ServiceName: "media-example",
				BaseDomain:  cmd.FlagHost, // 测试目标 server 地址（运行中的 media_example）
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
	base := clienthertz.NewClientSuiteFromConfig(cfg)
	return base.NewAppClientSuite("media_example")
}
