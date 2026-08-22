# Credential Reference 变更记录

## 2026-08-22：Credential Reference 首个垂直切片

- 新增 workspace-scoped 外部凭据引用的创建、更新、查询和撤销。
- Control 只保存 `secret_ref` 标识，不接收或返回 Provider API Key/Secret 明文。
- 引用必须指向已存在 Provider；撤销不可逆，写操作使用 `expected_version`。
