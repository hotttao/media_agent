# Agent Catalog 变更记录

## 2026-08-22：Prompt 聚合首个垂直切片

- 新增 Prompt 的 owner-scoped Create/List/Get/Update/Archive 能力。
- Prompt 状态仅允许 `active` 与 `archived`；归档不可逆。
- 更新和归档使用 `expected_version` 乐观并发控制，版本冲突拒绝写入。
- workspace-scoped Prompt 创建时校验 Workspace 所有权；平台级 Prompt 使用 `workspace_id = NULL`。
- API 响应不包含任何凭据字段；后续 Agent Template、Skill、Provider、Credential Reference、Memory Store 分别实现。
