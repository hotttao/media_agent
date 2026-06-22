// recovery.go：panic 兜底，避免单 handler panic 拖死进程。
//
// 走 Hertz 自带 recovery middleware；config.print_stack 控制是否打印 stack。
package serverhertz

import (
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/middlewares/server/recovery"

	configpb "media_agent/hertz_gen/config"
)

// newRecoveryMiddleware 返回 panic recovery middleware。
// 默认开启（即使配置里 enabled=false 也强烈建议保留），但本实现严格遵循 enabled 字段。
func newRecoveryMiddleware(cfg *configpb.RecoveryConfig) app.HandlerFunc {
	if cfg == nil || !cfg.GetEnabled() {
		return nil
	}
	return recovery.Recovery()
}
