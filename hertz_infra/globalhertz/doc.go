// Package globalhertz 存放进程级全局唯一状态：OTel provider、hlog logger、sentinel。
//
// 这些是 serverhertz 与 clienthertz 共享的进程级单例（otel.SetTracerProvider 全局、
// hlog.SetLogger 全局、sentinel.InitDefault 全局），收口在本包：
//   - 避免 serverhertz ↔ clienthertz 相互依赖（二者都 import globalhertz，单向）；
//   - sync.Once 幂等，server / client 任一先初始化即可，重复调用安全。
//
// server/client 的具体治理实装仍在各自包（serverhertz / clienthertz）；
// 本包只管"全局唯一变量的初始化与生命周期"。
package globalhertz

import (
	"context"

	configpb "media_agent/hertz_gen/config"
)

// InitLogger 初始化结构化应用日志并替换 hertz 默认 hlog。
// log.enabled=false / nil → 不替换（enabled 语义）。幂等。
func InitLogger(app *configpb.AppConfig, log *configpb.LogConfig) error {
	return initLogger(app, log)
}

// InitOTel 初始化进程级 OTel tracing provider，返回 shutdown closure。
// otel.enabled=false / nil → no-op。幂等（server/client 共用，sync.Once 只构造一次）。
func InitOTel(app *configpb.AppConfig, otel *configpb.OtelConfig) func(context.Context) error {
	return initOTel(app, otel)
}

// InitSentinel 初始化 Sentinel 全局状态。幂等（sync.Once）。
// 限流（server）/ 熔断（client）共享同一套 sentinel 全局规则注册表。
func InitSentinel() error {
	return initSentinel()
}
