# Public Export 垂直切片

- [x] Public Export IDL、迁移与生成代码。
- [x] Workspace owner 下创建、查询和状态机更新。
- [x] storage-relative `object_path` 校验；控制面不保存导出二进制。
- [x] 仅 `ready` 可发布，支持 `pending → ready/failed/revoked`、`ready → revoked`。
- [x] 单测、`go test ./...`、`make arch-check`、media-cli HTTP E2E。
