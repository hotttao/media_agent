# 变更记录

## 2026-10-11 · MCP 注册目录归 Gateway

- 将 MCP Catalog 与 MCP Definition 的权威归属从 media_plugin 移至 media_agent_gateway。
- Plugin 保留具体实现、Workspace 启用关系与外部连接，通过 Gateway 服务接口读取 Definition，不共享数据库表。
- 更新快照 Workflow，目录读取在 Plugin 本地事务外，版本冲突重新构建，失败保留上一有效快照。
- 此次为目标设计调整，接口、消息投递与发现刷新细节仍需在 spec 前定稿。

## 2026-10-11 · 统一 MCP 入口与 Prompt 职责

- Agent 统一连接 Gateway MCP 入口；Gateway 负责工具发现、授权过滤和调用代理，后端负责具体实现和资源授权。
- Gateway 组装 Platform Prompt；Agent 保留 Runtime 特有的转换、落盘与加载；Control 保留期望配置。
- 目录归属、发现刷新与协议兼容细节尚未定稿，本次不宣称实现完成。



### 2026-10-13 · P4 协议适配与执行身份

统一入口采用官方 Go MCP SDK；发现与调用均重新授权。Session Token 解析固定会话，稳定 Agent 委托验证身份，托管请求再核验实例凭证和实际 running Turn/epoch，防止旧实例继续调用。工具/Prompt 命名空间和资源 URI 来源映射、跨 Gateway 分页版本校验写入独立规格。

### 2026-10-13 · P4 请求作用域与兼容

官方当前协议入口采用 stateless；上游分页/调用在请求范围复用连接，结束关闭，不引入跨请求后台引用表。确认旧协议握手不继承入口协议元数据。调用前当前授权再校验，保留取消/deadline，不重放未知写；审计可靠采集归部署观测设施。
