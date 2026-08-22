# Admin Workspace Query 实现规格

## 边界

Admin Workspace Query 组合 Workspace、Agent Configuration 与可选 Placement 的只读事实，不复制或修改这些 Domain 的命令状态。响应不得包含 Provider API Key、加密密文、Session Token 或其他凭据。

## 能力覆盖矩阵

| 设计能力 | 当前状态 |
|:---|:---|
| Admin/System 跨 owner 分页查询 Workspace | covered |
| owner、desired_state、agent_type 组合过滤 | covered |
| Agent Context version 与 Placement 摘要组合 | covered |
| 普通用户拒绝且响应排除任何密钥字段 | covered |

矩阵存在 `missing` 时不得声明 Admin Workspace Query 完成。

## HTTP API

`GET /api/v1/admin/workspaces?owner_user_id=...&desired_state=...&agent_type=...&page_size=...&page_offset=...`

`page_size` 默认 50、最大 200；`page_offset` 必须非负。`desired_state` 只接受 `running/stopped/deleted`，`agent_type` 只接受 `codex/claude-code`。

## Harness 映射

- IDL：`idl/media_control/platform_administration.proto`
- Domain：`media_control/biz/domain/platform_administration`
- Repository：`media_control/biz/dal/repo/platform_administration`
- Service/Handler：`media_control/biz/service/platform_administration`、`media_control/biz/handler/platform_administration`
- E2E：`media-cli/e2e/media_control_admin_workspace_e2e_test.go`
