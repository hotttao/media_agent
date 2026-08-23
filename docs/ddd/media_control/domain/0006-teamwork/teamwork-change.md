# Teamwork 变更记录

## 2026-08-22：Topology 管理首个垂直切片

- 新增 Workspace-owned Topology、Member、Edge 三表及原子更新。
- 服务层校验成员唯一性、入口成员存在、边端点存在且不能自环；写入使用版本锁。
- Runtime Context 投影和 Gateway 任务限制留待运行时集成。

## 2026-08-22：内部 Runtime Context 只读投影

- Workspace 公开读模型补充 `agent_configuration_id`，使调用方能够构造可验证的 Teamwork Member 引用。
- 创建和更新 Topology 时拒绝不属于目标 Workspace 的 Agent Configuration。
- 新增仅内部 acting-user 可访问的 active Topology Runtime Context；投影不包含 Provider API Key。
- Gateway 的版本准入与拓扑变更 Inbox 保留为下一独立切片。

## 2026-08-23：Teamwork Topology 变更 Outbox

- Topology 创建、更新、删除与 `TeamworkTopologyChanged` Outbox 在同一本地事务提交。
- 后台 Relay 以至少一次语义向 Gateway 投递；失败不回滚已提交 Topology，并延迟重试。
- 事件只携带版本准入所需的非敏感事实。
