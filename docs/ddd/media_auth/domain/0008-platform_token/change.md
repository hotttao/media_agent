# 2026-08-06

依据 ADR-0007 新建独立 Platform Token 聚合，替代原 `service_tokens.token` 明文列设计，并限制为 Gateway 与 Control Schedule Worker 的 purpose-bound 内部能力。
# 2026-10-04 · 独立加密存储约束

- 离线迁移Workflow锁定两侧写入、分页核对原事实及密文后同事务删除旧Platform行；默认演练回滚，不影响普通ServiceToken，不自动切换运行时。

- 明确AES-256-GCM、ID/User/用途/版本AAD、SHA-256复核与版本化外部密钥；独立密文表保留撤销历史。
- 新ID遵循仓库统一生成器，历史ID不改；有数据的降级拒绝删表。
- 旧表当前仍保存Platform明文；先实现和验证加密存储，再迁移及切换，不能提前删除共用认证代码。
