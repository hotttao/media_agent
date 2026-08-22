# Provider 变更记录

## 2026-08-22：Provider 首个垂直切片

- 新增平台 Provider 定义的 CRUD 与 active/disabled 状态切换。
- `capabilities` 以 JSONB 保存并在写入前校验；更新和状态切换必须携带 `expected_version`。
- Provider 元数据不包含 Provider Secret，凭据由后续 Credential Reference 聚合管理。
