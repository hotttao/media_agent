# Media Agent 业务上下文总览

## 限界上下文

- [认证上下文](docs/media_auth/CONTEXT.md) — 用户身份、Web 会话、服务凭证、团队与 OAuth 凭据
- [控制上下文](docs/media_control/CONTEXT.md) — Workspace、运行环境、Agent 配置、定时计划与 Teamwork 拓扑
- [Agent 网关上下文](docs/media_agent_gateway/CONTEXT.md) — Agent Session、交互记录、执行任务与 Platform MCP
- [Plugin 上下文](docs/media_plugin/CONTEXT.md) — 受控 MCP 目录、Workspace 启用关系与外部 OAuth MCP 连接
- [存储上下文](docs/media_storage/CONTEXT.md) — 对象路径、对象读写、URL 解析与物理回收
- [通知上下文](docs/media_notification/CONTEXT.md) — 用户通知、投递偏好和渠道投递
- [共享资源访问语言](docs/resource_access/CONTEXT.md) — Scope、Visibility、Grant 与有效权限的跨服务统一语义
- [工作流上下文](docs/media_workflow/CONTEXT.md) — 媒体工作流定义、校验与执行
- [素材上下文](docs/media_materials/CONTEXT.md) — Material、Generated Asset、Product 与 Virtual IP
- [生成上下文](docs/media_gen/CONTEXT.md) — 媒体生成执行、转写与发布
- [抓取上下文](docs/media_crawler/CONTEXT.md) — 抓取任务、断点与平台数据
- `media_agents` — Agent Runtime、模型调用和工具执行；运行状态由 Runtime 管理，持久化交互归 Agent 网关

Browser、Sandbox、Channel Gateway 和 AFS 是外部上下文。平台仅保存其引用或状态投影，不复制其内部模型。

## 上下文关系

- `media_auth` 向 Edge 和内部服务提供身份验证、团队访问检查及受限凭据获取。
- 资源服务使用共享访问语言实现本地 ACL；共享词典不拥有资源或授权数据。
- `media_control` 是 Workspace 与 Agent 配置的权威来源；`media_agent_gateway` 消费其运行上下文，但不修改控制配置。
- `media_agent_gateway` 创建 Agent Session 并向 `media_agents` 下发运行配置与 MCP 快照。
- `media_plugin` 发布 Plugin Binding 变更；Gateway 据此重建快照，Agent 运行期直连平台内业务 MCP。
- `media_workflow` 编排 `media_crawler`、`media_gen` 和 `media_materials`，各服务保留自身任务事实。
- 所有需要二进制对象的业务服务依赖 `media_storage`，但业务归属、分享和删除决策仍归业务服务。
- 各业务服务发布通知请求，`media_notification` 负责投递，不反向拥有业务事件。

## 全局不变量

- Workspace 是平台租户边界；Workspace 内业务资源必须持有 `workspace_id`。
- 每项业务事实只有一个权威写入上下文；跨上下文只引用 ID，不共享表或建立跨库外键。
- 直接调用服务身份与被代表用户身份是两个独立维度。
- 资源所属上下文执行最终资源授权。
