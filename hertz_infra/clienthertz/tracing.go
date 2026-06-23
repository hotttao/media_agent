// tracing.go：客户端 OpenTelemetry 链路追踪。
//
// 对齐官方文档（cloudwego.io Hertz OpenTelemetry）：
//
//	c, _ := client.NewClient()
//	c.Use(hertztracing.ClientMiddleware())
//
// 进程级 provider 由 ClientSuite.InitObservability 委托 hertz_infra/observability
// 初始化（与 serverhertz 共享同一全局 provider，内部 sync.Once 幂等）。
package clienthertz

import (
	hclient "github.com/cloudwego/hertz/pkg/app/client"
	hertztracing "github.com/hertz-contrib/obs-opentelemetry/tracing"

	configpb "media_agent/hertz_gen/config"
)

// applyTracingMiddleware 把 hertz tracing client middleware 挂到 client。
// otel 未启用时 no-op。
func applyTracingMiddleware(cli *hclient.Client, otel *configpb.OtelConfig) {
	if cli == nil || otel == nil || !otel.GetEnabled() {
		return
	}
	cli.Use(hertztracing.ClientMiddleware())
}
