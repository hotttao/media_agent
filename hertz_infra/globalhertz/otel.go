// otel.go：进程级 OTel tracing provider 全局单例初始化。
//
// 对齐官方文档（cloudwego.io Hertz OpenTelemetry）：
//
//	p := provider.NewOpenTelemetryProvider(
//	    provider.WithServiceName(serviceName),
//	    provider.WithExportEndpoint("localhost:4317"),
//	    provider.WithInsecure(),
//	)
//	defer p.Shutdown(context.Background())
//
// 本实现额外注入 app.version / app.env 资源属性与采样率；不开启 metrics
// （WithEnableMetrics）——metrics 走 monitor-prometheus pull，避免双计数。
package globalhertz

import (
	"context"
	"log"
	"sync"

	"github.com/hertz-contrib/obs-opentelemetry/provider"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/semconv/v1.21.0"

	configpb "media_agent/hertz_gen/config"
)

var (
	otelOnce     sync.Once
	otelProvider provider.OtelProvider
	otelShutdown sync.Once
)

func initOTel(app *configpb.AppConfig, otel *configpb.OtelConfig) func(context.Context) error {
	if otel == nil || !otel.GetEnabled() {
		return func(context.Context) error { return nil }
	}

	otelOnce.Do(func() {
		opts := []provider.Option{
			provider.WithServiceName(serviceName(app)),
			provider.WithExportEndpoint(otel.GetExportEndpoint()),
			provider.WithEnableTracing(true),
			provider.WithEnableMetrics(false), // metrics 走 monitor-prometheus pull
		}
		if otel.GetInsecure() {
			opts = append(opts, provider.WithInsecure())
		}
		// 用 WithResourceAttributes 而非 WithResource：provider 的 newResource 在
		// cfg.resource 非空时直接返回，会跳过 resourceAttributes（WithServiceName 写入处），
		// 导致 service.name 丢失。WithResourceAttributes 与 WithServiceName 同走
		// resource.New(WithAttributes(...)) 分支，正确合并。
		opts = append(opts, provider.WithResourceAttributes(otelResourceAttributes(app)))
		opts = append(opts, provider.WithSampler(sampler(otel.GetSampleRatio())))
		otelProvider = provider.NewOpenTelemetryProvider(opts...)
	})

	return func(ctx context.Context) error {
		var err error
		otelShutdown.Do(func() {
			if otelProvider != nil {
				if e := otelProvider.Shutdown(ctx); e != nil {
					err = e
					log.Printf("globalhertz: otel shutdown err: %v", e)
				}
			}
		})
		return err
	}
}

// serviceName 返回本服务逻辑名，缺失兜底 "unknown-service"。
func serviceName(app *configpb.AppConfig) string {
	if app != nil && app.GetName() != "" {
		return app.GetName()
	}
	return "unknown-service"
}

// otelResourceAttributes 返回 service.name / service.version / deployment.environment 资源属性。
//
// 直接通过 WithResourceAttributes 设置（与 WithServiceName 同走 resourceAttributes 合并分支）。
// 实测某些 obs-opentelemetry/provider 版本下 WithServiceName 不会落到最终 resource，
// 故在此显式带上 service.name，确保 Jaeger 中服务名非空。
func otelResourceAttributes(app *configpb.AppConfig) []attribute.KeyValue {
	attrs := []attribute.KeyValue{semconv.ServiceNameKey.String(serviceName(app))}
	if app != nil {
		if v := app.GetVersion(); v != "" {
			attrs = append(attrs, semconv.ServiceVersion(v))
		}
		if env := app.GetEnv(); env != "" {
			attrs = append(attrs, semconv.DeploymentEnvironment(env))
		}
	}
	return attrs
}

// sampler 把 [0,1] 采样率转为 ParentBased(TraceIDRatioBased) 采样器；
// ratio<=0 视为不采样（仍 ParentBased 以尊重上游），ratio>=1 全采样。
func sampler(ratio float64) sdktrace.Sampler {
	if ratio <= 0 {
		ratio = 0
	}
	if ratio >= 1 {
		ratio = 1
	}
	return sdktrace.ParentBased(sdktrace.TraceIDRatioBased(ratio))
}
