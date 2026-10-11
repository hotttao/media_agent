# 状态所有权

| 状态 | 所有者 | 持久化 |
|:---|:---|:---|
| Workspace 名称、Agent 类型、是否配置 API Key | `media_control` | 服务端数据库 |
| Workspace Runtime Directory 与 mount identity | Runtime Environment / Runtime Controller | 服务端基础设施；UI 不读取路径、不持久化副本 |
| API Key 明文 | Workspace 设置表单到服务端的单向命令 | 不在浏览器持久化、不回显 |
| Agent Session Registration、固定 Agent/Context Version | `media_agent_gateway` | Gateway PostgreSQL；UI 只比较兼容性，不修改绑定 |
| Runtime Session、Message、Turn、Universal Event、Chat Status | `media_agents` | Agent PostgreSQL；UI 经 Gateway 授权代理读取和提交命令 |
| 当前 Organization/Team/Workspace ID | Shell Router | URL path `/o/:organizationId/t/:teamId/w/:workspaceId`，服务端层级和成员关系校验 |
| Shell Context 与 Navigation Port | Host Adapter | 内存注入；不得复制为 App 全局 store |
| 商品到 Workflow 的启动意图 | Host Navigation Port → 目标 App Instance | 仅实例内存，携带 requestId/Organization/Workflow/Product ID；不含 Token、输入快照或自动提交指令，用户清空后不重新填入 |
| Workflow 运行准备游标、选择与复核阶段 | Workflow App Instance | 企业/定义/启动意图隔离的内存；不是 StepRun 状态，不写 URL/localStorage，提交事实仍归后端 |
| 当前 Session ID | Workspace Page | URL query `?session=`，服务端列表校验 |
| Layout、Slot 打开实例和活动实例 | Slot Provider | Workspace 级 localStorage Profile |
| 移动端当前可见 App | Layout/Dock | 内存；从活动 Slot 推导 |
| Session Event sequence | Chat App | 当前 Session 的内存游标；只有带 SSE id 的 Universal Event 推进，断线时作为恢复参数 |
| 输入框、流式增量、错误提示 | 对应 App | 内存；由独立 Session Event Subscription 驱动 |
| Turn interrupting 请求态 | Chat App | 内存；仅用于防止重复提交，不替代服务端 Turn/Chat Status |
| Agent Request 请求、响应与状态 | `media_agents` | Agent PostgreSQL；Chat App 仅查询 pending 投影并提交 resolve |
| 用户状态与认证版本 | `media_auth` | User Management 页面只读取投影并提交 create/soft-delete 命令，不缓存授权事实 |
| Kratos 登录身份与 Web Session | Ory Kratos | UI 只消费 self-service flow/session，不复制凭证 |
| 企业/部门功能授权 | `media_auth` 事实源、Ory Keto 判定投影 | UI 可缓存展示结果，不据此替代服务端授权 |
| 独立站点语言偏好 | i18next / 浏览器本地存储 | `tos-language`；首次访问默认中文，插件模式跟随宿主 `lang.change` |
| Material 类型多选筛选、Product 搜索及页码 | 各列表组件 | 当前视图内存；服务端列表查询与总数是事实源；Material 空集合表示全部类型 |

localStorage key 必须带版本与 Workspace ID。Profile 包含 `layoutId`、`slots` 与预留的 `instances`；恢复数据必须迁移版本并校验已注册 App、允许的 Slot 和单实例约束，解析失败回退默认值。不得写入 API Key、消息正文或服务端授权事实。

P0 继续使用 localStorage，但访问必须通过 Profile Backend 接口，避免 UI 组件直接依赖存储。后续服务端 backend 加载时必须设置加载门，禁止默认 Layout 闪烁。
