// tracing.go：客户端 OpenTelemetry middleware。
//
// 走 hertz-contrib/obs-opentelemetry/tracing.ClientMiddleware()。
package clienthertz

import (
	hclient "github.com/cloudwego/hertz/pkg/app/client"

	configpb "media_agent/hertz_gen/config"
)

func applyTracingMiddleware(cli *hclient.Client, otel *configpb.OtelConfig) {
	if cli == nil || otel == nil || !otel.GetEnabled() {
		return
	}
	// TODO(infra): cli.Use(hertztracing.ClientMiddleware())
}
