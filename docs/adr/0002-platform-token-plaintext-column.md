# Platform Token 保留明文 token 列

`service_tokens` 表对 `is_platform=true` 的 Platform Token 保留明文 `token` 列（非 platform token 仅存 `token_hash`）。敲定保留该列，原因是 agent 进程需用明文 Platform Token 回调 control_plane，而 agent 无法访问 DB、也拿不到哈希反推的明文；若不存明文则每次 agent 启动都要重新签发并同步，破坏"每用户至多一个 active platform token"不变量。核心理由：这是凭证托管场景下"必须可读明文"的硬约束，哈希存储在此不适用。

## 状态元数据
accepted

## 备选方案考量
- **所有 token 一律仅存 hash**：被否决，agent 无法用 hash 回调。
- **拆独立 `platform_tokens` 表、不混入 `service_tokens`**：语义更干净，但本阶段迁移面大，留作远期重构。

## 衍生影响
- code review 强约束：新代码不得读取非 platform token 的 `token` 列（必为 NULL）。
- `idx_service_tokens_platform_user` 部分唯一索引保证每用户至多一个 active platform token。
- Platform Token 不可经 `DELETE /api/service-tokens/:id` 撤销（`WHERE is_platform = false`）；仅登录自动 ensure / 内部清理。
- 远期拆 `platform_tokens` 表时需同步迁移 agent 取 token 的内部接口。
