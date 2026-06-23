// observability.go：media-cli 自身可观测性初始化。
//
// media-cli 既是对外 CLI，也是 e2e 测试工具与链路发起方。接入可观测性后：
//   - 日志落 media_logs/media-cli/app.log（zap + zapotel，带 trace_id/span_id）；
//   - OTel provider 初始化后，调下游服务的 client middleware 会传播 trace_id，
//     实现「从 client 起的整条链路日志按 trace_id 关联」（见 contributing/e2e.md）。
//
// 配置不从 yaml 读（media-cli 是工具非服务），由环境变量驱动：
//
//	OTEL_ENABLED / OTEL_EXPORT_ENDPOINT   — tracing provider
//	LOG_ENABLED / LOG_DIR / LOG_LEVEL     — 结构化日志（LOG_DIR 默认 <repo>/media_logs/media-cli）
//	PROMETHEUS_ENABLED                    — metrics（CLI 通常不开）
package cmd

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/cloudwego/hertz/pkg/common/hlog"
	"go.opentelemetry.io/otel"

	configpb "media_agent/hertz_gen/config"
	"media_agent/hertz_infra/observability"
)

// withSpan 在 ctx 上启动一个 OTel span，返回带 span 的 ctx 与 end closure。
//
// 用于 media-cli 发起下游调用前：把 trace 起点放在 client 侧，client middleware
// 会把该 span 的 trace_id 透传给下游服务，从而实现「从 client 起按 trace_id
// 关联全链路日志」（见 contributing/e2e.md）。OTEL_ENABLED=false 时 span 为 no-op。
//
// 调用方应在调用前 hlog.CtxInfof(ctx, ...) 记录，使该条日志携带 trace_id。
func withSpan(ctx context.Context, name string) (context.Context, func()) {
	ctx, span := otel.Tracer("media-cli").Start(ctx, name)
	hlog.CtxInfof(ctx, "media-cli: start %s", name)
	return ctx, func() { span.End() }
}

var (
	cliCfgOnce sync.Once
	cliCfgVal  *configpb.Config
)

// initCLIObservability 初始化 media-cli 进程级可观测性，返回 shutdown closure。
// 由 Execute 调用并 defer。任一能力 enabled=false 时 no-op。
func initCLIObservability() func(context.Context) error {
	cfg := cliConfig()
	if err := observability.InitLogger(cfg.GetApp(), cfg.GetLog()); err != nil {
		log.Printf("media-cli: init logger: %v", err)
	}
	shutdown := observability.InitOTel(cfg.GetApp(), cfg.GetOtel())
	if err := observability.InitMetrics(cfg.GetPrometheus()); err != nil {
		log.Printf("media-cli: init metrics: %v", err)
	}
	return shutdown
}

// cliConfig 构造 media-cli 的可观测配置（环境变量驱动，幂等缓存）。
// app.name 固定 "media-cli"，便于在 media_logs / Jaeger / Loki 中区分。
func cliConfig() *configpb.Config {
	cliCfgOnce.Do(func() {
		cliCfgVal = &configpb.Config{
			App: &configpb.AppConfig{
				Name:    "media-cli",
				Version: "v1",
				Env:     envOr("APP_ENV", "local"),
			},
			Otel: &configpb.OtelConfig{
				Enabled:        envBool("OTEL_ENABLED", false),
				ExportEndpoint: envOr("OTEL_EXPORT_ENDPOINT", "localhost:4317"),
				Insecure:       true,
				SampleRatio:    1.0,
			},
			Log: &configpb.LogConfig{
				Enabled:    envBool("LOG_ENABLED", true),
				Level:      envOr("LOG_LEVEL", "info"),
				Dir:        envOr("LOG_DIR", defaultLogDir()),
				Filename:   "app.log",
				MaxSizeMb:  100,
				MaxBackups: 10,
				MaxAgeDays: 7,
				Compress:   true,
				Console:    true,
			},
			Prometheus: &configpb.PrometheusConfig{
				Enabled: envBool("PROMETHEUS_ENABLED", false),
			},
		}
	})
	return cliCfgVal
}

// defaultLogDir 定位仓库根（向上找 go.work），返回 <root>/media_logs/media-cli。
// 找不到时返回空串（observability.InitLogger 见空仅写 stdout）。
func defaultLogDir() string {
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.work")); err == nil {
			return filepath.Join(dir, "media_logs", "media-cli")
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "" // 到达根仍未找到
		}
		dir = parent
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envBool(key string, def bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}
