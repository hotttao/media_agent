# 状态所有权

| 状态 | 所有者 | 持久化 |
|:---|:---|:---|
| Workspace 名称、Agent 类型、是否配置 API Key | `media_control` | 服务端数据库 |
| Workspace Runtime Directory 与 mount identity | Runtime Environment / Runtime Controller | 服务端基础设施；UI 不读取路径、不持久化副本 |
| API Key 明文 | Workspace 设置表单到服务端的单向命令 | 不在浏览器持久化、不回显 |
| Session、消息和 turn | `media_agent_gateway` | 服务端数据库/运行时 |
| Session Chat Status | `media_agent_gateway` | 服务端数据库；UI 只读取 human/agent 投影判断后台 Turn 是否仍在运行 |
| 当前 Workspace ID | Router | URL path `/w/:workspaceId`，服务端列表校验 |
| 当前 Session ID | Workspace Page | URL query `?session=`，服务端列表校验 |
| Layout、Slot 打开实例和活动实例 | Slot Provider | Workspace 级 localStorage Profile |
| 移动端当前可见 App | Layout/Dock | 内存；从活动 Slot 推导 |
| Session Event sequence | Chat App | 当前 Session 的内存游标；只有带 SSE id 的 Universal Event 推进，断线时作为恢复参数 |
| 输入框、流式增量、错误提示 | 对应 App | 内存；由独立 Session Event Subscription 驱动 |
| Turn interrupting 请求态 | Chat App | 内存；仅用于防止重复提交，不替代服务端 Turn/Chat Status |
| Agent Request 请求、响应与状态 | `media_agents` | Agent PostgreSQL；Chat App 仅查询 pending 投影并提交 resolve |
| 用户状态与认证版本 | `media_auth` | User Management 页面只读取投影并提交 create/soft-delete 命令，不缓存授权事实 |

localStorage key 必须带版本与 Workspace ID。Profile 包含 `layoutId`、`slots` 与预留的 `instances`；恢复数据必须迁移版本并校验已注册 App、允许的 Slot 和单实例约束，解析失败回退默认值。不得写入 API Key、消息正文或服务端授权事实。

P0 继续使用 localStorage，但访问必须通过 Profile Backend 接口，避免 UI 组件直接依赖存储。后续服务端 backend 加载时必须设置加载门，禁止默认 Layout 闪烁。
