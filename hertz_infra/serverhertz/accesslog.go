// accesslog.go：访问日志。
//
// 走 hertz-contrib/logger/accesslog；format 走配置，默认提供常见模板。
package serverhertz

import (
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/hertz-contrib/logger/accesslog"

	configpb "media_agent/hertz_gen/config"
)

const defaultAccessLogFormat = "[${time}] ${status} - ${latency} ${method} ${path}"

func newAccessLogMiddleware(cfg *configpb.AccessLogConfig) app.HandlerFunc {
	if cfg == nil || !cfg.GetEnabled() {
		return nil
	}
	format := cfg.GetFormat()
	if format == "" {
		format = defaultAccessLogFormat
	}
	opts := []accesslog.Option{accesslog.WithFormat(format)}
	if tf := cfg.GetTimeFormat(); tf != "" {
		opts = append(opts, accesslog.WithTimeFormat(tf))
	}
	return accesslog.New(opts...)
}
