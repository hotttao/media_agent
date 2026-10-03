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
	"context"

	hclient "github.com/cloudwego/hertz/pkg/app/client"
	"github.com/cloudwego/hertz/pkg/protocol"
	hertztracing "github.com/hertz-contrib/obs-opentelemetry/tracing"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	configpb "media_agent/hertz_gen/config"
	"media_agent/hertz_infra/tracecontext"
)

// tracingMiddleware preserves W3C correlation even when span export is disabled.
func tracingMiddleware(otel *configpb.OtelConfig) hclient.Middleware {
	if otel == nil || !otel.GetEnabled() {
		return func(next hclient.Endpoint) hclient.Endpoint {
			return func(ctx context.Context, req *protocol.Request, resp *protocol.Response) error {
				ctx, span := tracecontext.Start(ctx, string(req.Method()), trace.SpanKindClient)
				defer span.End()
				carrier := propagation.MapCarrier{}
				tracecontext.Inject(ctx, carrier)
				// A reused request must not retain another request's trace state.
				req.Header.Del("traceparent")
				req.Header.Del("tracestate")
				for key, value := range carrier {
					req.Header.Set(key, value)
				}
				return next(ctx, req, resp)
			}
		}
	}
	return hertztracing.ClientMiddleware()
}
