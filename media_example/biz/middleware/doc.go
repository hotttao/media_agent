// Package middleware 存放手写全局 Hertz 中间件（auth token 解析、request id、日志、tracing、CORS、recovery、限流）。
// 复杂业务授权放 policy，不放 middleware。middleware 仅提取技术上下文（request id / trace id / auth subject / tenant id）。
package middleware
