// observability.go — infra 包内的 OTel trace span 工具。
//
// 配置来源约定（与用户对齐后）：
//   - infra 包不导入 config（不"乱倒配置"）；
//   - main.go 是 cfg 的消费者与分发者：main 拿到 cfg 后，直接调 globalhertz
//     初始化 logger + OTel provider，并把 shutdown 闭包 defer 在 main；
//   - 本文件只提供 WithSpan 等"用全局 OTel provider"的工具，不参与初始化。
//
// 调用顺序：
//   1. main.go：config.NewLoader → loader.Current() → globalhertz.InitLogger + InitOTel
//   2. 各 subpackage RunE：infra.WithSpan(ctx, name) 在全局 tracer 上开 span
//   3. main.go defer：调用 InitOTel 返回的 shutdown flush trace
//
// 放进 infra 而非 cmd 根：infra 是"基础设施观测 / 治理"主题域；
// client/ e2e 测试也用 WithSpan 透传 trace_id，与 infra/trace.go（Jaeger 断言）同主题。

package infra

import (
	"context"

	"github.com/cloudwego/hertz/pkg/common/hlog"
	"go.opentelemetry.io/otel"
)

// WithSpan 在 ctx 上启动一个 OTel span，返回带 span 的 ctx 与 end closure。
//
// 用于 media-cli 发起下游调用前：把 trace 起点放在 client 侧，client middleware
// 会把该 span 的 trace_id 透传给下游服务，从而实现「从 client 起按 trace_id
// 关联全链路日志」（见 contributing/e2e.md）。OTEL_ENABLED=false / 全局 provider
// 未初始化时 span 为 no-op（otel.Tracer 返回 noop tracer）。
//
// 调用方应在调用前 hlog.CtxInfof(ctx, ...) 记录，使该条日志携带 trace_id。
func WithSpan(ctx context.Context, name string) (context.Context, func()) {
	ctx, span := otel.Tracer("media-cli").Start(ctx, name)
	hlog.CtxInfof(ctx, "media-cli: start %s", name)
	return ctx, func() { span.End() }
}
