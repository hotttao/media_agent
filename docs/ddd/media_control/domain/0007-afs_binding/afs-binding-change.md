# AFS Binding 变更记录

## 2026-08-22：绑定与成员投影

- 新增 Workspace-owned AFS Binding 和成员状态投影。
- Reconcile 只更新 observed state；真实 AFS API 调用由外部 adapter 负责，本切片不修改 AFS controller/sidecar。
