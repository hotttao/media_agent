# 变更记录

## 2026-09-29

- 明确 Identity状态与API Token撤销意图同库事务提交，Talos调用只由已有恢复流程在事务外完成。
- 实现 WorkerAccessRevocation.ChangeStatus/ProcessBatch/Run；精确 ActorType/Actor/owner/Organization 批量写意图。
- 非active身份keyset扫描每批100条、每项15秒、每10秒恢复；失败项不阻塞后续页，下一轮重试，处理晚到签发。
- PG验证失败回滚、幂等、边界隔离与晚到激活CAS拒绝；真实Talos验证停用立即拒绝、重建撤销流程后Key终态revoked。
