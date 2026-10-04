# Agent API Proxy 变更记录

## 2026-10-04 · 公开代理与资源授权边界

- 落实单次代理包络、独立公钥入口、Content-Type绑定和权威Workspace；Job访问限定当前Workspace/企业/owner，并在Conductor同步前授权。
- Materials成员权限查询不使用缓存；User与Worker分支保持独立，旧StepRun Worker入口也不接受Agent断言。
- HTTP/CLI已验证现有任务读取、临时素材写入与撤销拒绝；自动创建、生命周期批量撤销和完整审计另列未完成项。

## 2026-10-03 · 独立委托签名信任链

- 明确 Gateway 的独立签名 issuer、单服务 audience 和60秒请求摘要绑定，业务服务保留 Agent Actor而不降级为User。
- 原始 Token只用于Auth验证；代理固定服务/路由，写请求不自动重试，不复用Oathkeeper私钥或Worker认证分支。
## 2026-10-04 · 委托调用审计

- 网关编排与目标服务请求边界记录可信身份、路由模板、阶段/真实业务状态，并按trace_id关联。
- 不记录原始URI/查询/载荷/凭据/错误正文；转发结果不明不误报回滚，不创建事务性审计表。
