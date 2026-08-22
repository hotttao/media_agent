# Runtime Environment Token 实现规格

## 边界

Environment Token 是远程 Runtime Runner 的长效、可撤销凭据，只解析为一个 `environment_id`。它不复用 Service Token，也不产生用户身份或继承创建者权限。

## 能力覆盖矩阵

| 设计能力 | Domain | 当前状态 |
|:---|:---|:---|
| 随机 Token 签发且明文只返回一次 | runtime_environment | covered |
| 数据库仅保存 SHA-256，不在列表响应泄露密文或摘要 | runtime_environment | covered |
| 同环境多 Token 并存以支持无中断轮换 | runtime_environment | covered |
| Token 撤销与 active Token 验证 | runtime_environment | covered |
| 验证结果仅包含受限 environment_id Principal | runtime_environment | covered |

矩阵仍有 `missing` 时不得声明 Environment Token 完成。

## HTTP API

- `POST /api/v1/runtime-environments/:environment_id/tokens`：签发 Token，明文仅在本次响应返回。
- `GET /api/v1/runtime-environments/:environment_id/tokens`：只返回 active Token 元数据。
- `POST /api/v1/runtime-environments/:environment_id/tokens/:token_id/revoke`：幂等边界内撤销指定 Token。

原始 Token 的验证是 Runtime Gateway 内部能力，不暴露为用户 HTTP API。调用者先签发新 Token、完成 Runner 切换，再撤销旧 Token，即完成无中断轮换。

## 安全不变量

- Token 格式为 `env_` 加 256-bit 随机值；日志、列表和错误不得包含原始 Token。
- 持久化和查询只使用完整 Token 的 SHA-256；撤销后验证立即失败。
- 删除 Runtime Environment 时数据库级联删除其 Token。
- `created_by` 仅用于管理审计，不参与验证授权。

## Harness 映射

- IDL：`idl/media_control/runtime_environment.proto`
- Domain：`media_control/biz/domain/runtime_environment`
- Repository：`media_control/biz/dal/repo/runtime_environment`
- Service/Handler：`media_control/biz/service/runtime_environment`、`media_control/biz/handler/runtime_environment`
- Migration：`media_control/migrations/000016_environment_tokens.*.sql`
- E2E：`media-cli/e2e/media_control_environment_token_e2e_test.go`
