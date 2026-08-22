# Agent Catalog — Prompt 垂直切片

范围：先落地 `agent_catalog` 中的 Prompt 聚合；Agent Template、Skill、Provider、Credential Reference、Memory Store 后续分别实现。`media_materials` 与 `media_workflow` 不在本切片范围。

- [x] Prompt IDL、OpenAPI 路由与 PostgreSQL migration。
- [x] Prompt domain、owner scope、active/archived 状态机。
- [x] Create/List/Get/Update/Archive service 与持久化。
- [x] Update/Archive 使用 `expected_version` 做乐观并发控制。
- [x] media_control 单测、`go test ./...`、`make arch-check`。
- [x] media-cli HTTP E2E 覆盖创建、更新、冲突、归档。
