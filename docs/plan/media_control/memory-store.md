# Agent Catalog — Memory Store 垂直切片

- [x] Memory Store IDL、迁移与 Harness 生成代码。
- [x] Workspace 必填 owner 校验与 JSON configuration 校验。
- [x] 可选 Credential Reference 关联校验（同一 Workspace/Owner）。
- [x] Create/List/Get/Update/SetStatus service 与版本并发控制。
- [x] media_control 单测、`go test ./...`、`make arch-check`。
- [x] media-cli HTTP E2E 覆盖 Workspace 绑定、创建、禁用。
