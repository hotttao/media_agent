# Runtime Environment 垂直切片

- [x] Runtime Environment/Workspace Placement IDL、迁移与 Harness 生成代码。
- [x] 环境 Create/List/Get/Update/Delete 与版本并发控制。
- [x] Workspace Placement reconcile：选择 active 环境，无候选时记录 `unplaced`。
- [x] `EnsureWorkspaceRuntimeDirectory` 幂等返回 `/workspace/<workspace_id>` 稳定引用。
- [x] 单测、`go test ./...`、`make arch-check`、media-cli 容器 E2E。
- [ ] Environment Grant/Token 管理（独立切片）。
