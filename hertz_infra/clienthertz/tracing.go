// tracing.go：客户端 OpenTelemetry 链路追踪。
//
// 对齐官方文档（cloudwego.io Hertz OpenTelemetry）：
//
//	c, _ := client.NewClient()
//	c.Use(hertztracing.ClientMiddleware())
//
// 进程级 provider 由 ClientSuite.InitObservability 委托 hertz_infra/globalhertz
// 初始化（与 serverhertz 共享同一全局 provider，内部 sync.Once 幂等）。
package clienthertz

import (
	hclient "github.com/cloudwego/hertz/pkg/app/client"
	hertztracing "github.com/hertz-contrib/obs-opentelemetry/tracing"

	configpb "media_agent/hertz_gen/config"
)

// tracingMiddleware 返回 hertz tracing client middleware；otel 未启用时返回 nil。
func tracingMiddleware(otel *configpb.OtelConfig) hclient.Middleware {
	if otel == nil || !otel.GetEnabled() {
		return nil
	}
	return hertztracing.ClientMiddleware()
}
