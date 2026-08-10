# 008 Control-plane 能力迁移规范

## 1. 迁移原则

- 不兼容旧 API URL；本表只用于证明能力有明确所有者。
- 迁移目标是业务语义覆盖，不是复制旧路由文件结构。
- Edge 只承接入口路由；没有所有者的能力不得继续留在新的 Control 中。

## 2. 路由映射

| 原能力或路由族 | 新所有者 |
|---|---|
| `auth`、`wecom`、`oauth-provider`、`oauth-applications` | `media_auth` |
| Service Token、Platform Token、`/me` 身份与 Profile | `media_auth` |
| Team、Invite、成员关系、MCP OAuth 凭据 | `media_auth` |
| Admin User、用户状态管理 | `media_auth` |
| Workspace 查询/写入/生命周期/Profile/Layout | `media_control` |
| Workspace Template、Command、Tag、公共导出元数据 | `media_control` |
| Environment、Environment Gateway、Placement | `media_control` |
| Prompt、Agent Template、Skill、Skill Registry | `media_control` |
| Provider、Credential Reference、Memory、内部配置查询 | `media_control` |
| Schedule、Workspace Agent 配置、AFS 平台记录 | `media_control` |
| System/Admin Workspace、Cluster/Setting/Stats | `media_control` |
| Teamwork 定义、成员拓扑 | `media_control` |
| Session、Chat、Message、Event | `media_agent_gateway` |
| Agent Request、Agent Task、Batch Run、Job | `media_agent_gateway` |
| `/me` 下的 Session 查询、Usage | `media_agent_gateway` |
| Teamwork 运行 Session、Platform `/mcp` | `media_agent_gateway` |
| Plugin Catalog、Workspace Plugin Binding | `media_plugin` |
| 外部认证 MCP 注册、OAuth、Proxy | `media_plugin` |
| Notification、Preference、Delivery | `media_notification` |
| ASR、Transcription | `media_gen` |
| Browser API 与实时代理 | Browser Service |
| Sandbox API | Sandbox Service |
| Channel/CG/SaaS 集成 | 对应独立集成服务 |
| Storage/Skill Content 二进制内容 | `media_storage` |

## 3. 原 Proxy 的处理

- Browser 实时流量直接进入 Browser Service，不经过 Control 或 Agent Gateway。
- 平台内 MCP 由 Agent 直连目标业务服务；Platform MCP 由 `media_agent_gateway` 提供。
- 普通业务 API 经 Edge 路由到目标服务，Control 不做代理。
- 只有外部 OAuth MCP 的凭据代理保留在 `media_plugin`。

## 4. 验收标准

- 每个旧路由族均能映射到一个权威服务。
- 新服务的 IDL 能表达旧能力的业务语义。
- 旧接口中混合多个数据所有者的操作必须拆成工作流或事件，不建立共享数据库。
- 完成实现前，本表表示设计覆盖，不表示代码已经迁移。

领域模型的逐项覆盖结果见 [Control-plane 领域迁移覆盖审查](../review/domain-migration-coverage.md)。
