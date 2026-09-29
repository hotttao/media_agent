# 变更记录

## 2026-09-29 · 公开管理接口

- 明确Create/List/Get/Disable/Delete HTTP与IDL契约，owner固定来自当前User；状态入口仅调用撤销Workflow。
- CreateWorkerApiToken由WorkerTokenProvisioning派生Actor/User/Organization并限制五类worker scope，Secret交付和持久恢复复用API Token生命周期。

## 2026-09-29

- 明确 Personal Worker Identity 的稳定身份、不可变 owner/Organization、active/disabled/deleted 状态归 media_auth；运行实例/Capability/任务不进入聚合。
- 新增0014迁移、domain/model/repository/service；User与Organization授权回调缺失或失败即拒绝。
- API Token签发、验证、轮换查询精确身份归属和状态；Worker与Agent权威隔离。
- 停用/删除由 WorkerAccessRevocation 同库事务协调 Token 清理意图，后台扫描晚到签发；公开IDL/CLI和测试Worker换发仍待实施。
