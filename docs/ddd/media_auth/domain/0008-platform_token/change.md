# 2026-08-06

依据 ADR-0007 新建独立 Platform Token 聚合，替代原 `service_tokens.token` 明文列设计，并限制为 Gateway 与 Control Schedule Worker 的 purpose-bound 内部能力。
# 2026-10-04 · 独立加密存储约束

- 独立Platform IDL提供purpose-bound获取/内省/轮换，固定错误与no-store；专用mTLS监听器只注册该路由，默认关闭且缺失配置拒绝启动。确保/用户撤销不开放Secret接口。认证/DTO接入仍为单聚合用例，迁移与审计沿用既有Workflow。

- 明确真实mTLS CallerVerifier边界：独立Client CA、TLS1.3、精确单URI SAN和ClientAuth用途；连接事实绑定实例隔离context，当前时刻重新验证，不接受CN/Header/普通Internal Token替代。认证适配器本身不产生持久化业务状态，因此不新增空壳Workflow；既有迁移/审计Workflow不变。

- Outbox增加SKIP LOCKED领取、未过期lease确认/退避CAS与attempts饱和规则，delivered为不可重置终态；日志sink事务外Sync后确认，部分写入或确认未知保留重投。

- 补齐单聚合用例：User行锁协调当前状态/并发，可信CallerVerifier与固定purpose先于凭据操作；同事务审计Outbox仅含固定元数据，不保存Secret。mTLS接入、审计投递和运行时切换仍是独立门禁，不以用例测试冒充已完成。

- 离线迁移Workflow锁定两侧写入、分页核对原事实及密文后同事务删除旧Platform行；默认演练回滚，不影响普通ServiceToken，不自动切换运行时。

- 明确AES-256-GCM、ID/User/用途/版本AAD、SHA-256复核与版本化外部密钥；独立密文表保留撤销历史。
- 新ID遵循仓库统一生成器，历史ID不改；有数据的降级拒绝删表。
- 旧表当前仍保存Platform明文；先实现和验证加密存储，再迁移及切换，不能提前删除共用认证代码。
