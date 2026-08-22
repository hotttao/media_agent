# Agent Catalog — Agent Template 垂直切片

- [x] Agent Template IDL、OpenAPI 路由与 PostgreSQL migration。
- [x] Template domain、owner scope、JSON 配置校验与 active/archived 状态机。
- [x] Create/List/Get/Update/Archive service 与持久化。
- [x] Update/Archive 使用 `expected_version` 做乐观并发控制。
- [x] media_control 单测、`go test ./...`、`make arch-check`。
- [x] media-cli HTTP E2E 覆盖创建、更新、冲突、归档。
