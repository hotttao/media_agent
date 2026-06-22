// timeout.go：服务端读 / 写 / 整体超时。
//
// Hertz server-level read/write timeout 已在 buildServerOptions 中通过 server.WithReadTimeout /
// server.WithWriteTimeout 注入；本文件保留 newTimeoutMiddleware 接口供细粒度（per-handler）超时扩展。
package serverhertz

// （当前 spec 未要求 per-handler 超时；server 级超时在 buildServerOptions 中处理。）
//
// 如未来引入 per-handler 超时，此处增加 newTimeoutMiddleware(cfg)；中间件调用
// context.WithTimeout 包裹 ctx 并在超时后返回 504。
