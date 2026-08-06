# Control-plane 领域迁移覆盖审查

本表以 [008_migration.md](../refactor/008_migration.md) 为事实清单。`covered` 表示已存在明确 Domain/Workflow 设计，不表示业务代码已经实现。

| 原能力 | 目标限界上下文 | Domain / 外部所有者 | 关键 Workflow | 状态 |
|:---|:---|:---|:---|:---|
| Auth、WeCom、OAuth Provider/Application | `media_auth` | `auth`、`oauth_application` | `wecom-login-bind`、`oauth-authorize-code-flow` | covered |
| Service Token、Platform Token、`/me` 身份/Profile | `media_auth` | `service_token`、`platform_token`、`auth` | - | covered |
| Team、Invite、成员关系 | `media_auth` | `team` | `team-invite-accept` | covered |
| MCP OAuth 凭据 | `media_auth` | `mcp_oauth` | `mcp-oauth-connect` | covered |
| Admin User 与用户状态 | `media_auth` | `auth` | - | covered |
| Workspace 查询/写入/生命周期/Profile/Layout | `media_control` | `workspace`、`agent_configuration` | `workspace_lifecycle` | covered |
| Workspace Template、Command | `media_control` | `agent_configuration` | - | covered |
| Tag、Share、公共导出元数据 | `media_control` | `workspace_metadata` | - | covered |
| Environment、Environment Gateway、Placement | `media_control` | `runtime_environment` | `workspace_lifecycle` | covered |
| Prompt、Agent Template、Skill、Skill Registry | `media_control` | `agent_catalog` | `runtime_context_assembly` | covered |
| Provider、Credential Reference、Memory、内部配置查询 | `media_control` | `agent_catalog`、`agent_configuration` | `runtime_context_assembly` | covered |
| Schedule | `media_control` | `schedule` | 全局 `agent-schedule-execution` | covered |
| Workspace Agent 配置 | `media_control` | `agent_configuration` | `runtime_context_assembly` | covered |
| AFS 平台记录 | `media_control` | `afs_binding` | `workspace_lifecycle` | covered |
| System/Admin Workspace、Cluster/Setting/Stats | `media_control` | `workspace`、`platform_administration` | - | covered |
| Teamwork 定义、成员拓扑 | `media_control` | `teamwork` | 全局 `teamwork-runtime-change` | covered |
| Session、Chat、Message、Event、Agent Request | `media_agent_gateway` | `agent_session` | `run_turn` | covered |
| Agent Task、Job | `media_agent_gateway` | `agent_task` | `agent_task_dispatch` | covered |
| Batch Run | `media_agent_gateway` | `batch_run` | `batch_run_dispatch` | covered |
| `/me` Session、Activity、Usage | `media_agent_gateway` | `agent_session`、`session_analytics` | `run_turn`、`agent_task_dispatch` | covered |
| Teamwork 运行 Session | `media_agent_gateway` | `teamwork_runtime` | 全局 `teamwork-runtime-change` | covered |
| Platform `/mcp` | `media_agent_gateway` | `agent_session` | `platform_mcp_invocation` | covered |
| Plugin Catalog、Workspace Plugin Binding | `media_plugin` | `mcp_catalog`、`plugin_binding` | `plugin_binding_snapshot` | covered |
| 外部 OAuth MCP 注册、OAuth、Proxy | `media_plugin` | `external_mcp_connection` | `external_mcp_oauth_proxy` | covered |
| Notification、Preference、Delivery | `media_notification` | `notification`、`preference`、`delivery` | `notification_delivery` | covered |
| ASR、Transcription | `media_gen` | `transcription` | `job-execution-loop` | covered |
| Browser API 与实时代理 | Browser Service | Browser Service | 资源服务内分享链接流程 | external-owner |
| Sandbox API | Sandbox Service | Sandbox Service | Browser/Sandbox 内部调用 | external-owner |
| Channel/CG/SaaS 集成 | 独立集成服务 | 对应集成服务 | 各集成服务定义 | external-owner |
| Storage/Skill Content 二进制内容 | `media_storage` | `stored_object` | `object_retention_gc` | covered |

## 审查结论

- 迁移清单中没有 `missing` 项。
- 本次补齐 `workspace_metadata`、`platform_administration`、`session_analytics` 三个遗漏 Domain。
- 新服务的重要跨聚合或外部副作用流程均已建模；单聚合操作保留在 Domain 内。
- Edge 路由、mTLS 和 River 属于基础设施实现，不作为 Domain；它们只在相关 Workflow 中承担认证或投递职责。
