# Ory 承担身份基础设施，media_auth 持有企业业务事实

Ory Kratos 是终端用户登录身份、凭证恢复与浏览器会话的唯一事实源；Ory Oathkeeper 是外部 API 的策略执行点，认证 Kratos Session 后向内部服务签发短时 Internal Identity JWT；Ory Keto 保存用于请求路径判定的授权关系投影。media_auth 继续持有 User Profile、Organization、Team、Membership、Entitlement 及审计 Outbox，不保存用户密码，不自行签发新的 Web Session，也不把可变权限复制进长期 JWT。

业务数据库是企业关系与授权配置的事实源。变更在本地事务中连同 Outbox 提交，再以 `event_id` 幂等投影到 Keto；投影失败重试并告警，不回滚已提交的业务事实。资源 owner、visibility 与 grant 仍归资源服务，Keto 只接收其投影。Hydra 的 OAuth2/OIDC Provider 能力不属于本阶段，现有 B2B Service Token 迁移另行决策。

采用该边界可避免 media_auth 与 Kratos 双写密码/Session，也避免把组织权限冻结在 JWT 内；代价是登录链路增加 Oathkeeper，授权变更存在可观测的短暂投影延迟，因此高风险管理写入在投影未收敛时须由事实源二次校验。

## 状态元数据

accepted
