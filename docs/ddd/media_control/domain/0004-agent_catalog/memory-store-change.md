# Memory Store 变更记录

## 2026-08-22：Memory Store 首个垂直切片

- 新增 Workspace-owned Memory Store 配置与 active/disabled 状态切换。
- `configuration` 以 JSONB 保存；可选 Credential Reference 必须属于同一 Workspace 和调用者。
- Memory Store 不读取凭据明文，写操作使用 `expected_version`。
