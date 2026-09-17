---
name: authorization_projection
description: 将企业、部门、成员和资源授权事实可靠投影到 Ory Keto
participants: [organization, team, resource_service, outbox_relay, keto]
domains: [organization, team]
---

## 一句话说明

业务服务在本地事务提交授权事实与 Outbox 后，Projection Worker 以事件 ID 幂等更新 Keto 关系元组，使请求路径授权最终收敛而不引入跨库事务。

## 规则

- Organization/Team/Membership/Entitlement 的事实源是 media_auth；owner/visibility/grant 的事实源是对应资源服务。
- Keto tuple 包含稳定 namespace、object、relation、subject；不得把展示名称当 ID。
- 撤权事件优先消费并持续重试；投影延迟、失败次数和最后成功游标必须可观测。
- 管理写入在需要立即生效的安全边界上同时校验业务事实源；普通读执行可使用 Keto 投影。
- 重建投影从各事实源快照与 Outbox 游标恢复，不要求停机。

## 事务边界与事件顺序

1. 管理命令先锁定聚合根并校验当前版本、操作者角色及 entitlement 上限。
2. 同一个数据库事务提交业务事实与一条 `authorization_outbox`；事件携带聚合类型、稳定 ID、单调 `sequence` 和完整目标状态，不携带展示名。
3. Relay 以 `created_at, id` 领取未投递事件。同一聚合只能按 `sequence` 串行投影，不同聚合可以并行。
4. Worker 把目标状态转换为 Keto relation tuples：先幂等写入当前应存在的 tuple，再删除已不属于目标状态的 tuple。
5. 全部写入成功后才标记 Outbox delivered，并推进该聚合 checkpoint。进程在步骤 4/5 之间崩溃时，重放得到相同结果。

撤权和停用事件不能被后续普通事件越过。管理写与撤权敏感请求在投影完成前回查 media_auth 事实源，因此投影延迟不会扩大权限。

## 身份映射前置条件

- Keto 的 User subject 使用 `media_auth.users.id`，不能直接使用 Kratos Identity ID。
- 首次 Kratos 登录由内部 Identity Bridge 按 `kratos_identity_id` 解析业务 User；启用 Kratos 前已有账号可在 identity 尚未绑定时按规范化 email 一次性关联。
- `kratos_identity_id` 全局唯一，绑定后不可由登录请求改绑。Identity traits 只更新 profile 字段，不改变 User ID、Organization Membership 或 Team Membership。

## 幂等、失败与恢复

- 事件唯一键为 `(aggregate_type, aggregate_id, sequence)`；重复写事件由数据库拒绝，重复消费由目标状态写法吸收。
- Keto 暂时不可用时增加 `attempts`、记录 `last_error` 并指数退避；事件保持 pending，不跳过同聚合后续 sequence。
- 非法 tuple 或连续失败进入告警，但不删除 Outbox。修复 mapper 后从 checkpoint 重放。
- 全量重建从 Organization/Team/Resource 快照生成同一套目标 tuple；构建到新 namespace/version，校验 shadow check 后切换，避免清空在线授权面。

## 可观测与验收

- 指标至少包括 pending 数、最老 pending age、成功/失败计数、每聚合最后 checkpoint 和 Keto check shadow mismatch。
- 日志必须带 `trace_id`、event_id、aggregate_id、sequence；禁止记录 Session Cookie、Internal JWT 或邀请明文 token。
- 验收覆盖重复事件、乱序事件、Keto 超时、写成功但 checkpoint 失败、撤权延迟和全量重建。
