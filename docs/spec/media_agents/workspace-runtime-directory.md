# Workspace Runtime Directory 实现规格

## 边界

Agent 容器内工作目录固定为 `/workspace`，本地 Docker 部署把宿主
`${WORKSPACE_ROOT:-/workspace}/${RUNTIME_WORKSPACE_ID}` bind mount 到该路径。目录以
Workspace ID 独占，容器 stop/restart/recreate 后保留。该目录不替代 PostgreSQL 中的
Runtime Session、Turn、Message 或 Universal Event。

Agent 的 `HOME` 固定为 `/workspace/.home`。Codex 原生状态保存在
`/workspace/.home/.codex`，Claude Code 原生状态保存在 `/workspace/.home/.claude`。
同一 Workspace 切换 Agent 类型时继续使用同一目录；两个 Agent 不得对同一 Workspace
无约束并发写入。

## 配置与校验

- `RUNTIME_WORKSPACE_ID`：必填的安全路径段，只允许字母、数字、点、下划线和连字符，不能为
  `.`/`..`，最大 128 字符。
- `WORKSPACE_DIR`：容器内固定 `/workspace`；生产部署不得改到 bind mount 之外。
- `WORKSPACE_ROOT`：仅 Compose/Runtime Controller adapter 使用，默认宿主 `/workspace`，不传入
  页面或 Agent HTTP 请求。

Agent 启动时在 `/workspace/.media-agent/workspace.json` 创建或读取 mount identity，包含
`version`、`workspace_id`、`storage_id`、`created_at`。已存在 identity 的 workspace_id 不同则
启动失败；相同则复用 storage_id。写入使用同目录临时文件加无覆盖原子发布，不能暴露 API Key。

## API 与 media-cli

`GET /api/v1/runtime-workspace` 返回 workspace_id、storage_id、created_at 和容器内 mount path，
不返回宿主路径。`media-cli media_agents runtime-workspace info` 驱动该接口。E2E 在容器重启前后
读取两次并断言 storage_id 不变；另用 bind mount 文件检查证明宿主目录确为目标 Workspace。

## Harness 映射

| 职责 | 位置 |
|:---|:---|
| 配置验证 | `adapter/config/config.py` |
| identity 文件适配 | `adapter/infra/runtime_workspace.py` |
| HTTP DTO/路由 | `adapter/schemas/runtime_workspace/`、`adapter/biz/api/v1/routes/runtime_workspace/` |
| OpenAPI | `adapter/idl/runtime_workspace.yaml` |
| 部署映射 | `media_agent_gateway/docker-compose.yml` |
| E2E 驱动 | `media-cli/cmd/media_agents/`、`media-cli/e2e/` |

## 验收

- 非法 Workspace ID 在 Agent 启动前被拒绝。
- 同目录同 Workspace 重启复用 storage_id；不同 Workspace 拒绝启动。
- 宿主 `/workspace/<workspace_id>` 文件在容器重启后仍存在。
- API/日志不暴露宿主路径、API Key 或其他 Workspace identity。
