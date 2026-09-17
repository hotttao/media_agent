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
