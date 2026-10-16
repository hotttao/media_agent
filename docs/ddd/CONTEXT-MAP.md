# Media Agent 业务上下文总览

## 限界上下文

- [认证上下文](media_auth/CONTEXT.md) — 用户身份、企业、部门、成员关系、API Token 与 OAuth 凭据
- [控制上下文](media_control/CONTEXT.md) — 部门所属 Workspace、个人模型连接、运行环境、Agent 配置、定时计划与 Teamwork 拓扑
- [Agent 网关上下文](media_agent_gateway/CONTEXT.md) — Agent Session 注册、访问控制、执行任务接入、SSE 协议适配、MCP 服务注册目录与统一 MCP 入口
- [Agent 执行上下文](media_agents/CONTEXT.md) — Runtime Session、Turn 队列、模型/工具执行、交互记录与可恢复事件
- [Plugin 上下文](media_plugin/CONTEXT.md) — 具体 Plugin 能力、Workspace 启用关系与外部 OAuth MCP 连接
- [对象存储上下文](media_storage/CONTEXT.md) — 由 S3 兼容服务提供对象读写、临时地址与物理生命周期
- [通知上下文](media_notification/CONTEXT.md) — 用户通知、投递偏好和渠道投递
- [共享资源访问语言](resource_access/CONTEXT.md) — Scope、Visibility、Grant 与有效权限的跨服务统一语义
- [工作流上下文](media_workflow/CONTEXT.md) — 媒体工作流定义、校验与执行
- [素材上下文](media_materials/CONTEXT.md) — Material、Generated Asset、Product 与 Virtual IP
- [生成上下文](media_gen/CONTEXT.md) — 媒体生成执行、转写与发布
- [抓取上下文](media_crawler/CONTEXT.md) — 抓取任务、断点与平台数据
Runtime Controller 是 Agent 资源创建的环境实现：Gateway 使用 Kubernetes 客户端提交自定义资源 manifest，由自定义控制器创建 StatefulSet 等资源；Docker 环境由 Gateway 调用有 Docker 权限的创建服务。Browser、Sandbox、Channel Gateway 和 AFS 是外部上下文。平台仅保存其引用或状态投影，不复制其内部模型。

## 上下文关系

- `media_auth` 是 Organization、Team 及成员关系的权威来源；Organization 表达企业及其可用功能上限，Team 表达企业内部门。
- `media_control` 为 Team 创建和管理 Workspace；每个 Workspace 只属于一个 Team，并由该 Team 的 Organization 确定企业归属。
- `media_auth` 是 API Token 管理与验证的业务入口，保存 Token 元数据和 Talos Key 引用；Talos 持有 Secret 生命周期，Edge/Gateway 将有效 API Token 换成短时 Internal Identity JWT。
- Agent 是 User 的委托 Actor：创建 Agent 时 `media_agent_gateway` 请求 `media_auth` 签发绑定该 Agent 的 API Token；运行时 Agent 只经 `media_agent_gateway` 代理访问内部服务，JWT 使用 `sub=User`、`act.sub=Agent` 表达委托。
- Personal Worker 是 User 的委托 Actor，只能经统一网关进入系统：Edge 将 opaque API Token 换成 `sub=User`、`act.sub=Worker` 的短时 Internal Identity JWT，`media_workflow` 再执行 Organization、Job 所有权与 Capability 授权；Conductor 不接受客户端连接或 API Token。
- `media_auth/worker_identity` 持有 Personal Worker 的稳定身份、不可变 owner/Organization 和停用事实；`media_workflow` 持有 Dispatch/lease/执行事实，客户端进程实例不成为授权主体。Agent 的配置和业务生命周期仍归 `media_control`，不迁入 Worker 身份领域。
- Managed Worker 属于 `media_workflow` 内部运行时，不使用 API Token 或 Talos；它只领取 Capability Version 指定的 Managed Execution Domain，并以内部运行身份写入 Dispatch 审计。
- 资源服务使用共享访问语言实现本地 ACL；共享词典不拥有资源或授权数据。
- `media_control` 持有 Workspace、Agent 期望配置与 Placement；Gateway 据此提交 Agent 资源 manifest 或请求 Docker 创建服务，并直接与 Agent 通信。Kubernetes StatefulSet 维护副本，kubelet 执行探针；自定义 Runtime Controller 不承担 Agent 探活、执行状态或调度。Docker 本期不考虑扩缩容。
- `media_control` 是 Workspace 与 Agent 配置的权威来源；`media_agent_gateway` 消费其运行上下文，但不修改控制配置。
- `media_agent_gateway` 创建 Agent Session Registration 并向 `media_agents` 注册同 ID 的 Runtime Session；两侧只共享稳定 ID，不共享数据库表。
- `media_agents` 是 Turn、Message、Universal Event、Chat Status 和 Session sequence 的权威写入上下文；`media_agent_gateway` 通过 Agent 命令/查询接口和 Session Event Bus 适配 Web API 与 SSE。
- 同一 Runtime Session 的 Turn 由 `media_agents` 持久化排队并串行执行；Gateway 不以进程内锁、SSE 连接或节点亲和性决定执行顺序。
- `media_agent_gateway` 是 MCP Definition 的权威来源，持有注册目录；`media_plugin` 通过 Gateway 接口查询定义，不共享目录表。
- `media_plugin` 发布 Plugin Binding 变更；Gateway 据此重建快照；Agent 运行期统一连接 Gateway，由 Gateway 代理平台内业务 MCP。
- `media_workflow` 编排 `media_crawler`、`media_gen` 和 `media_materials`，各服务保留自身任务事实。
- `media_workflow` 是客户端 Capability Worker 与内部 Conductor 之间的防腐层：客户端只见 Worker Gateway API 和不透明 Dispatch，Conductor Task Definition、Task Domain 与 Task ID 均留在内部。
- 所有需要二进制对象的业务服务通过 S3 Storage Port 依赖逻辑 `media_storage`；本地部署由 SeaweedFS 提供，生产可切换 MinIO、Aliyun OSS、AWS S3 等兼容实现。业务归属、分享、读取授权和删除决策仍归业务服务。
- 各业务服务发布通知请求，`media_notification` 负责投递，不反向拥有业务事件。

## 全局不变量

- Organization 是企业客户与功能授权边界；Team 是 Organization 内的部门，不能脱离 Organization 存在。
- 资源归属边界由权威上下文声明：Workspace 私有的运行数据持 `workspace_id`；Organization 内共享的 Material、Virtual IP、Product 与 Generated Asset 持 `organization_id`。Workspace 只属于一个 Team/Organization，跨边界引用必须同时校验 Organization 与业务 Scope。
- 每项业务事实只有一个权威写入上下文；跨上下文只引用 ID，不共享表或建立跨库外键。
- 直接调用服务身份与被代表用户身份是两个独立维度。
- Internal Identity JWT 的 `sub` 表示被代表主体；委托调用使用 RFC 8693 `act.sub` 表示当前 Agent/Worker Actor。API Token Credential、Actor、Worker Instance 与 Kratos Identity 是独立审计维度，不得混为授权主体。
- 资源所属上下文执行最终资源授权。
