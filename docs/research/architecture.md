# Media Agent 平台重构架构研究

> 状态：服务划分待 Review
>
> 阶段约束：本文仅做架构设计、领域边界草案、IDL 规划和旧接口覆盖核对。不执行落地 plan，不实现业务，不生成 Hertz handler/router。
>
> 源系统：`D:\Code\koala\agent-platform`
>
> 目标仓库：`D:\Code\media\media_agent`
>
> 认证与 Token 的最终决议以第 25 节为准。前文中“平台统一签发短期 capability JWT”、“用 capability 取代 platform token”等表述为过程方案，不再是目标设计。

## 1. 目标与结论摘要

本次重构需要同时解决两个问题：

1. 将旧 `control-plane` 从“控制面业务 + 身份认证 + Agent 流量代理 + MCP OAuth 代理 + Plugin Registry”的集中式进程中拆解出来。
2. 让 `media_crawler`、`media_gen`、`media_materials`、`media_workflow` 作为可独立部署的业务扩展接入 Agent 平台，新增扩展时不再修改控制面业务代码。

当前建议采用“渐进式绞杀 + 热路径拆分”方案：

- `media_auth`：统一身份、令牌、团队目录和第三方 OAuth 凭据生命周期。
- `media_control`：低带宽、声明式平台控制面，管理 Workspace、运行环境和 Agent 配置资源。
- `media_agent_gateway`：Session、Chat、UniversalEvent、SSE/WebSocket 等 Agent 交互热路径。
- `media_plugin`：受控 Plugin/MCP Catalog、Workspace binding，以及外部需认证 MCP 的 OAuth Proxy；不代理平台内 MCP 的运行时调用。
- `+
+  -> X-Session-Token: session_xxx+
+media_workflow /mcp`：Claude Code、Codex、Goose 等实际 Agent Runtime Adapter。
- `media_crawler`、`media_gen`、`media_materials`、`media_workflow`：媒体业务扩展后端，同时通过 MCP Adapter 暴露适合 Agent 使用的工具。
- Browser、Sandbox、Channel Gateway 等高带宽或外部协议服务保持独立，不经 `media_control` 转发数据流。

本建议不是最终决定。需要重点 Review 第 6 节列出的三个边界问题后，才能初始化新服务和冻结 IDL。

## 2. 如何理解旧 control-plane

旧 CP 不是单纯的 API Gateway，而是多个角色的集合：

| 角色 | 当前职责 | 判断 |
|---|---|---|
| 控制面业务服务 | Workspace、Session、Agent、Prompt、Skill、Provider、Credential、Team 等元数据 | 合理，但边界过宽 |
| Runtime Orchestrator | 创建和回收 Agent Pod、解析动态地址、维护 Workspace 状态 | 合理的控制面职责 |
| Agent BFF / Gateway | 校验 Workspace ownership 后代理私有 Agent Pod | 有必要，但不应与全部控制面 CRUD 共进程 |
| Session Stream Processor | 拦截 SSE、持久化事件、更新 Session 状态、断线重连 | 属于数据热路径，应单独扩缩容 |
| Identity Provider | 登录、JWT、OAuth、Service Token | 应拆到 `media_auth` |
| MCP OAuth Proxy | 保存和刷新第三方 OAuth token、注入上游 Authorization | 凭据生命周期与流量代理应分离 |
| Plugin Registry | 保存和下发 UI bundle、插件 manifest | 应改为通用 Extension Registry |

Agent 流量经过 CP 并非完全错误。Agent Pod 地址动态且通常不可从集群外直达；Chat SSE 还需要维护 Session 状态和 UniversalEvent。因此 Agent 入口需要平台网关。问题在于该网关与低频控制面 CRUD、身份服务和扩展注册耦合在同一个进程中。

Browser 的 WebRTC、CDP、录屏和 Sandbox 文件流量不同。这些是高带宽长连接数据面，应该由对应资源服务直接承载。平台内部由 Platform MCP 使用 `platform_token` 代表用户调用资源服务；需要向可信边界外公开或分享具体资源时，由资源所属服务自行签发短时、受限的资源访问 Token。

## 3. 旧架构的主要问题

### 3.1 认证没有真正从 CP 热路径解耦

Browser 和 Sandbox 当前会回调 CP 的 `/api/auth/me` 校验 opaque Service Token，并用约 60 秒本地缓存减压。结果是：

- CP 仍是所有服务的同步认证依赖。
- CP 故障可能导致新 token 无法认证。
- token 撤销最多延迟一个缓存周期。
- 不同服务各自实现 Bearer、Cookie、query token 和 WebSocket upgrade 认证，行为容易漂移。

### 3.2 认证和鉴权的职责混淆

Gateway 可以完成签名、过期时间、issuer、audience 等粗粒度认证，但不能替代资源服务鉴权。只有资源所有者知道某个 `workspace_id`、`browser_id` 或 `workflow_id` 是否属于当前 Subject。

正确分工是：

- `media_auth` 回答“调用者是谁、凭证是否有效”。
- Edge Gateway 回答“该凭证是否允许进入这类路由”。
- 资源服务回答“该 Subject 能否执行这个资源动作”。

### 3.3 CP 同时承载冷路径和热路径

Workspace/Prompt/Provider CRUD 是低频控制面流量；Chat/SSE、MCP Streamable HTTP、Agent reconnect 是长连接热路径。二者共进程会导致：

- 扩容依据冲突。
- 长连接耗尽连接池或事件循环时影响管理 API。
- 发布控制面 CRUD 也会中断 Agent 会话。
- 故障域过大。

### 3.4 MCP Proxy 安全边界过弱

旧 MCP Proxy 从路径解析 `userId` 和上游 origin，再由 CP 查找 OAuth token 并发起请求。内部路由若被误暴露，会形成 SSRF、跨用户 token 选取和凭据泄露风险。目标架构必须同时具备：

- 固定 Extension Catalog，不接受任意上游 origin。
- `aud=media-plugin` 的短期 Agent Capability Token。
- token 中的 user/workspace/session 与路由参数一致性校验。
- 上游 egress allowlist。
- 内部路由 mTLS 或 workload identity。

### 3.5 共享数据库使服务成为分布式单体

旧服务直接读取同一个 PostgreSQL 中的其他域表。这样虽然开发快，但无法保证 schema 自治、独立发布和数据不变量。目标架构要求服务拥有自己的 schema/database；跨服务读取通过 IDL client，最终一致性投影通过 outbox event 完成。

## 4. 候选方案

### 4.1 方案 A：兼容优先，保留大 CP

结构：`media_auth + media_control(旧 CP 大部分能力) + 现有媒体服务`。

优点：

- 迁移量最小，旧接口几乎可以平移。
- 不需要立即引入新的 Session Gateway 和 Extension Gateway。
- 对现有 Web 改动较小。

缺点：

- Chat/SSE 热路径仍与控制面共进程。
- 每增加一种 Plugin，仍容易修改 `media_control`。
- `media_control` 会快速重新长成旧 CP。
- 无法独立扩缩容长连接入口。

适用：只要求短期完成语言迁移，不要求解决原架构问题。

### 4.2 方案 B：渐进式绞杀 + 热路径拆分（建议）

结构：`media_auth + media_control + media_agent_gateway + media_plugin + 现有媒体服务`。

优点：

- 将低频控制面、Agent 会话热路径和 Extension/MCP 流量拆成三个不同扩缩容单元。
- 新增业务扩展只注册 manifest，不修改 `media_control` 路由或数据库表。
- 可以按旧 API family 逐步迁移，不要求一次性切断旧 CP。
- 服务数量可控，适合当前团队和仓库成熟度。

缺点：

- `media_control` 内仍包含多个平台资源 domain，需要严格保持模块边界。
- `media_agent_gateway` 与 `media_control` 之间需要稳定的 Workspace Snapshot/Capability 契约。
- `media_auth` 和 `media_plugin` 必须明确 MCP OAuth token 的所有权边界。

适用：需要真正改善架构，同时要求迁移风险可控。

### 4.3 方案 C：细粒度平台微服务

进一步拆成 `workspace-service`、`agent-config-service`、`session-service`、`prompt-service`、`skill-service`、`memory-service`、`scheduler-service`、`extension-service`、`mcp-gateway` 等。

优点：

- 数据和发布边界最清晰。
- 每个域可以独立扩缩容和演进。
- 组织规模足够大时有利于团队自治。

缺点：

- 当前需要维护大量 IDL client、事务补偿、事件投影和部署单元。
- Workspace 创建等用例会跨多个服务，初期分布式事务复杂度很高。
- 在领域边界尚未稳定时容易拆错，最终形成分布式耦合。

适用：平台规模、团队数量和流量已经证明需要该复杂度。

### 4.4 对比结论

| 维度 | 方案 A | 方案 B | 方案 C |
|---|---:|---:|---:|
| 迁移成本 | 低 | 中 | 高 |
| 解决 CP 热路径压力 | 否 | 是 | 是 |
| Plugin 低侵入接入 | 部分 | 是 | 是 |
| 运维复杂度 | 低 | 中 | 高 |
| 分布式事务复杂度 | 低 | 中 | 高 |
| 当前仓库适配度 | 中 | 高 | 低 |
| 推荐 | 不推荐作为目标态 | 推荐 | 暂不采用 |

## 5. 建议目标架构

```mermaid
flowchart LR
    Client[Web / API / Channel] --> Edge[Traefik / Edge Gateway]

    Edge -->|/api/auth| Auth[media_auth]
    Edge -->|/api/control| Control[media_control]
    Edge -->|/api/sessions + SSE| Session[media_agent_gateway]
    Edge -->|/api/plugins + external OAuth MCP proxy| Extension[media_plugin]
    Edge -->|业务 API| Crawler[media_crawler]
    Edge -->|业务 API| Materials[media_materials]
    Edge -->|业务 API| Workflow[media_workflow]
    Edge -->|业务 API| Gen[media_gen]

    Auth -->|JWKS / token exchange| Edge
    Auth -->|MCP OAuth credential lifecycle| Extension

    Control -->|desired state| RuntimeController[Runtime Controller]
    RuntimeController --> Agent[+
+  -> X-Session-Token: session_xxx+
+media_workflow /mcp pods]

    Session <-->|UniversalEvent / ACP| Agent
    Session -->|Workspace snapshot| Control

    Extension -->|Catalog / binding changed| Session
    Session -->|MCP config snapshot| Agent
    Agent -->|session_token, direct MCP| Crawler
    Agent -->|session_token, direct MCP| Materials
    Agent -->|session_token, direct MCP| Workflow
    Agent -->|session_token, direct MCP| Gen
    Agent -->|session_token, OAuth proxy only| Extension
    Extension --> ExternalMCP[External authenticated MCP]

    Control --> ControlDB[(control DB)]
    Session --> SessionDB[(session DB)]
    Extension --> ExtensionDB[(extension DB)]
    Auth --> AuthDB[(auth DB)]
```

数据面直连关系：

```mermaid
flowchart LR
    Web -->|Cookie JWT / resource share token| Browser[Browser Service]
    Web -->|Cookie JWT / resource share token| Sandbox[Sandbox Service]
    Agent -->|session_token| PlatformMCP[media_agent_gateway Platform MCP]
    PlatformMCP -->|platform_token, internal only| Browser
    PlatformMCP -->|platform_token, internal only| Sandbox
    Browser --> BrowserRuntime[Chromium / WebRTC]
    Sandbox --> SandboxRuntime[OpenSandbox]
    Web -.不经过.-> Control[media_control]
```

## 6. 待 Review 的服务边界

### 6.1 `media_control` 是否需要立即再拆 `media_resource`

建议本阶段不新增独立进程，但在 `media_control/biz/domain` 内分为：

- `workspace`
- `runtime_environment`
- `agent_configuration`
- `prompt`
- `agent_template`
- `skill_catalog`
- `provider`
- `credential_reference`
- `memory_store`
- `schedule`

这些 domain 必须拥有各自 repository 接口，禁止同层互相 import。跨 domain 的 Workspace Provision、Template Sync、Skill Attach 由 workflow 编排。待 `prompt/skill/template/provider` 的流量、团队归属或发布节奏出现独立需求后，再按现有接口抽出 `media_resource`。

需要 Review：是否接受“先保持一个部署单元、内部严格模块化”的策略；如果目标是立即做到 database-per-bounded-context，则应现在增加 `media_resource`。

### 6.2 MCP OAuth 属于 `media_auth` 还是 `media_plugin`

建议分工：

- `media_auth` 拥有第三方 MCP OAuth client registration、authorization callback、access/refresh token 加密存储、刷新和撤销。
- `media_plugin` 拥有 MCP server catalog 和 Workspace binding；仅对外部需要 OAuth 的 MCP 承载代理流量、解析 Session 上下文并注入外部 OAuth 凭据。平台内自定义 MCP 由 Agent 直连。
- `media_plugin` 通过内部 IDL 请求 `media_auth.GetMcpAccessToken`，不读取 auth DB，不接触 refresh token。

理由：OAuth token 是用户凭据，属于身份安全域；MCP routing 是高频执行流量，属于 Extension Gateway。两者放在 `media_auth` 会让身份服务承载任意上游代理和长连接。

需要 Review：是否接受 `media_auth` 仅管理凭据，而不作为 MCP 数据面代理。

### 6.3 Agent Schedule 与 Media Workflow 是否合并

建议不合并：

- Agent Schedule 表示“在某个时间触发 Workspace Chat Turn”。
- Media Workflow 表示“按模板 DAG 执行媒体生成步骤并管理 StepRun”。

二者的聚合、不变量和失败恢复完全不同。Agent Schedule 暂归 `media_control.schedule`，由独立 scheduler worker 消费；媒体 DAG 继续归 `media_workflow`。

需要 Review：确认产品语义上没有把两者统一为同一种 Workflow Run 的计划。

## 7. 服务划分与职责

### 7.1 `media_auth`（已存在）

负责：

- User、Identity、Session、Service Token、OAuth Client、Team、Team Invite。
- JWT/OAuth access token 签发、刷新、撤销、token exchange。
- MCP OAuth credential 生命周期和内部 access-token 获取接口。
- 发布 JWKS；短期兼容旧 HS256，目标迁移到 RS256/ES256。

不负责：

- Workspace、媒体资源和 Session 的业务权限判断。
- 代理 MCP、Browser、Sandbox 或 Plugin Backend 流量。
- 保存 Plugin manifest。

### 7.2 `media_control`（建议新增，待 Review）

负责：

- Workspace 声明、配置、生命周期期望状态。
- Runtime Environment、Placement 和 Runtime Controller 对接。
- Agent Configuration：system prompt、commands、skills、provider reference、layout/profile。
- Prompt、Agent Template、Skill Catalog、Provider、Credential Reference、Memory Store 元数据。
- Workspace Schedule、Notification Preference、平台管理查询。
- 为 `media_agent_gateway`、Browser、Sandbox 和 Agent Runtime 签发资源级 capability 的授权依据。
- 旧 CP 非热路径 API 的兼容 facade，在迁移期转到内部 domain/use case。

不负责：

- 承载 Chat SSE、Agent reconnect、MCP streaming。
- 转发 Browser/Sandbox 数据。
- 保存用户 OAuth refresh token。
- 执行媒体生成 Workflow。

### 7.3 `media_agent_gateway`（建议新增，待 Review）

负责：

- Session、Turn、Message、UniversalEvent、AgentRequest。
- Chat dispatch、interrupt、respond、pending question。
- SSE/WebSocket broadcast、断线重连、幂等和背压。
- Session usage、activity、recent session、batch run 的交互执行部分。
- 连接动态 Agent Runtime；只通过 `media_control` 获取 Workspace Runtime Snapshot。

不负责：

- 修改 Workspace 配置。
- 管理 Prompt、Skill、Provider、Credential。
- 解释具体媒体业务 tool 的规则。

### 7.4 `media_plugin`（建议新增，待 Review）

负责：

- Extension Manifest 注册、版本、启停、权限声明。
- UI bundle URL、integrity、lazy surface 和 CDN/object-storage metadata。
- MCP Server Catalog、Workspace binding 和配置变更发布。
- 仅在外部需认证 MCP OAuth Proxy 中解析 `session_token`，用于选择对应用户的外部 OAuth 凭据。
- 外部需认证 MCP Proxy 的 endpoint allowlist、超时、限流、审计和 egress allowlist。
- 向 `media_auth` 获取短期第三方 MCP access token并注入上游请求。

不负责：

- 保存 Plugin 的业务数据。
- 实现 crawler/material/workflow/gen 的业务规则。
- 保存第三方 MCP refresh token。
- 动态执行不受信任的 UI JavaScript；UI bundle 由 CDN/Object Storage 分发。

### 7.5 `+
+  -> X-Session-Token: session_xxx+
+media_workflow /mcp`（已存在，Python Runtime）

负责：

- Claude Code、Codex、Goose 等 Agent Adapter。
- ACP/UniversalEvent 协议适配。
- 加载 Workspace Runtime Snapshot、skills 和 MCP endpoints。
- 使用下发的 MCP config snapshot 直连 Platform MCP 和平台内自定义 MCP；仅外部需 OAuth 的 MCP 经过 `media_plugin` Proxy。所有 Agent MCP 请求携带 `session_token`，不持有用户长期凭据。

### 7.6 媒体扩展服务（已存在）

`media_crawler`：抓取任务编排、平台数据只读查询；MCP tools 提供启动、取消、查询抓取及 XHS 数据检索。

`media_materials`：Material、GeneratedAsset、AssetKind、VirtualIP、Product、Reachability；MCP tools 提供资源查询、选择和绑定能力。

`media_workflow`：Template、Workflow、Job、StepRun、校验和执行状态机；MCP tools 提交和查询工作流，不直接执行生成器。

`media_gen`：Generation Worker 和 Publish Adapter；MCP 默认只暴露安全的生成/发布用例，不暴露内部 Claim/Complete/Fail worker 协议。

每个扩展服务同时提供：

- 面向 Web/BFF 的 Hertz REST API。
- 面向其他服务的 typed IDL client。
- 面向 Agent 的 MCP Adapter；Adapter 复用同一 application service，不复制业务逻辑。

### 7.7 保留的外部服务

| 服务 | 目标职责 | 与新平台关系 |
|---|---|---|
| Edge Gateway / Traefik | TLS、路由、CORS、粗粒度 JWT、限流 | 不承载资源鉴权 |
| Browser Service | Browser lifecycle、CDP、WebRTC、录屏 | 直接验 capability，不经 Control 转发视频 |
| Sandbox Service | Sandbox lifecycle、exec、files、preview | 直接验 capability，不使用全局 service key |
| Channel Gateway | Slack/企微/Webhook 入站验签和消息转换 | 调 `media_agent_gateway` 发起 Turn |
| Scheduler Worker | 消费 Agent Schedule 并触发 Session | 无状态 worker，Schedule 数据归 Control |
| media_storage | Skill Package、图片、视频、音频和导出文件 Blob/Object | 业务元数据归领域服务，字节归 Storage |
| Memory FUSE | 将 Memory Store 挂载到 Agent Pod | 通过内部 capability 访问 Control Memory API |

## 8. 认证与鉴权设计

### 8.1 凭证类型

| 场景 | 凭证 | 校验方式 |
|---|---|---|
| Web 登录 | httpOnly Session Cookie | Edge/BFF 换取内部 access token |
| 公共 API | 短期 user access JWT | `iss/aud/exp/signature/scope` 本地 JWKS 验签 |
| Service-to-Service | workload identity 或 client-credentials JWT | mTLS + audience/scope |
| 代表用户的下游调用 | RFC 8693 exchanged token | `sub + azp + aud + scope` |
| Browser/Sandbox/Export | 1 到 5 分钟 capability JWT | 绑定单资源、单动作、可选一次性 jti |
| Agent 调 MCP | session capability JWT | `aud=media-plugin`，绑定 user/workspace/session |

禁止将长期 Service Token 放在 query/path。WebSocket 无法设置 header 时，仅允许短期 capability，并避免在 access log 中记录完整 URL。

### 8.2 验证责任

1. Edge Gateway 校验外部 token 的基本有效性和 route audience。
2. 服务入口 middleware 必须再次离线验签，或只信任经过 mTLS 的认证代理头。
3. Handler 将 claims 转成稳定 `Subject`，不得将 Hertz context 传入 domain/service。
4. Application service 根据 action 调用本服务 Authorization Policy。
5. Policy 使用本服务 repository 读取 ownership/grant；跨服务团队成员关系通过 `media_auth` typed client 或本地只读投影获取。
6. 所有拒绝统一返回 401（未认证）或 403（已认证但无权限），不存在的私有资源可使用 404 防枚举。

### 8.3 JWT 算法迁移

仓库 ADR-0001 当前选择 HS256 共享密钥以兼容旧 CP。该方案可作为迁移阶段，但不应作为目标态：任何持有共享密钥的服务均可签发管理员 token。

建议分两阶段：

- 兼容期：`media_auth` 继续验证/签发旧 HS256；只有 `media_auth` 和旧兼容 facade 持有 secret，Plugin 服务通过 VerifyToken/JWKS，不共享 secret。
- 目标期：切换 RS256/ES256，`media_auth` 独占 private key，各服务只持有缓存的 public JWKS；通过 `kid` 支持平滑轮换。

## 9. Extension 与 MCP 接入模型

### 9.1 Manifest

```yaml
apiVersion: media.agent/v1
kind: Extension
metadata:
  id: media-crawler
  version: 1.0.0
spec:
  ui:
    bundleUrl: https://plugins.example/media-crawler/1.0.0/index.js
    integrity: sha256-...
    lazy: true
    owns:
      panels: [crawler.jobs]
      toolRenderers: [crawler.search_xhs]
  backend:
    service: media-crawler
    audience: media-crawler
    scopes: [crawler.read, crawler.run]
  mcp:
    endpoint: http://media-crawler/mcp
    transport: streamable-http
    tools:
      allow: [crawler_start, crawler_cancel, crawler_status, xhs_search]
  permissions:
    workspace: [read]
    outboundHosts: []
```

### 9.2 安装和调用流程

1. Plugin 后端和 MCP Adapter 独立部署。
2. CI/Operator 将 manifest 注册到 `media_plugin`。
3. Web 从 Registry 获取 manifest，再从 CDN 加载带 integrity 的 UI bundle。
4. `media_control` 只记录某 Workspace 启用了哪些 `extension_id/version`，不认识具体 Plugin 字段。
5. `media_agent_gateway` 根据 Catalog、Workspace binding 和 Agent 配置生成本次 Session 的 MCP config snapshot。
6. `media_agent_gateway` 将 MCP config snapshot 和 `session_token` 下发给 Agent Runtime。
7. Agent 使用 `session_token` 直连平台内自定义 MCP endpoint。
8. Plugin MCP Adapter 调用 `media_agent_gateway.ResolveSessionContext` 获得可信 user/workspace/session 上下文。
9. Plugin MCP Adapter 构造 Principal，并将 tool call 转成同进程 application service Command。

新增 Plugin 不需要修改 `media_control` route、handler、domain 或 schema。

## 10. 领域模型草案

### 10.1 `media_control`

| Domain | 聚合/值对象 | 核心不变量 |
|---|---|---|
| workspace | Workspace、WorkspaceProfile、WorkspaceLayout | owner/授权主体才能修改；desired state 与 observed state 分离 |
| runtime_environment | Environment、Placement、EnvironmentToken | Placement 只能属于一个 Environment；observed version 单调递增 |
| agent_configuration | AgentConfig、Command、ExtensionBinding | config version 不可变；更新产生新 revision |
| prompt | Prompt、PromptVersion、PromptGrant | published version 不可原地修改 |
| agent_template | AgentTemplate、TemplateVersion、TemplateGrant | Workspace sync 显式记录来源版本 |
| skill_catalog | Skill、SkillVersion、SkillSource | metadata 与 package blob 分离；版本内容不可变 |
| provider | Provider、ProviderGrant | API key 不出现在读模型中 |
| credential_reference | CredentialReference | 明文只写不读；实际 secret 进入 Vault/Secret Store |
| memory_store | MemoryStore、MemoryVersion、Attachment | Workspace 只能访问已 attachment 的 Store |
| schedule | AgentSchedule | 触发目标必须是可运行 Workspace；幂等 execution key |

关键 workflow：WorkspaceProvision、WorkspaceLifecycle、TemplateSync、SkillAttach、EnvironmentReconcile、AgentScheduleDispatch。

### 10.2 `media_agent_gateway`

| Domain | 聚合/值对象 | 核心不变量 |
|---|---|---|
| session | Session、Turn、Message | 同一 Session 的事件 sequence 单调递增 |
| stream | StreamLease、Subscriber、ResumeCursor | reconnect 只读取已授权 Session；cursor 幂等 |
| agent_request | AgentRequest、Resolution | pending request 只能被 resolve 一次 |
| usage | UsageRecord、UsageSummary | 原始 usage append-only，汇总可重建 |
| batch_run | BatchRun、BatchItem | item 幂等键唯一；终态不可回退 |

关键 workflow：RunTurn、ReconnectTurn、InterruptTurn、ResolveAgentRequest、PersistAndBroadcastEvent、BatchDispatch。

### 10.3 `media_plugin`

| Domain | 聚合/值对象 | 核心不变量 |
|---|---|---|
| extension_registry | Extension、ExtensionVersion、Manifest | version 不可变；启用前 manifest 必须验证 |
| workspace_binding | ExtensionBinding | 绑定版本必须存在且 enabled |
| mcp_catalog | McpServer、ToolPolicy | endpoint 来自可信 catalog，禁止 caller 提供任意 origin |
| mcp_session | McpInvocation、CapabilityContext | capability 中 subject/workspace/session 必须一致 |
| ui_bundle | BundleDescriptor | bundleUrl 必须 HTTPS，integrity 必填 |

关键 workflow：InstallExtension、EnableForWorkspace、ResolveWorkspaceMcpConfig、InvokeMcpTool、FetchMcpAccessToken、DisableCompromisedVersion。

## 11. IDL 设计草案

本节只冻结服务边界和 RPC family。Review 通过后才创建 proto 和生成代码。

### 11.1 `media_control`

```text
WorkspaceService:
  CreateWorkspace, GetWorkspace, ListWorkspaces, UpdateWorkspace, DeleteWorkspace
  StartWorkspace, StopWorkspace, RestartWorkspace, RebuildWorkspace
  GetWorkspaceStatus, GetWorkspaceConfig, UpdateWorkspaceConfig

EnvironmentService:
  CreateEnvironment, GetEnvironment, ListEnvironments, UpdateEnvironment, DeleteEnvironment
  ReplaceEnvironmentGrants, CreateEnvironmentToken, ListEnvironmentTokens, RevokeEnvironmentToken
  ListPlacements, ReportObservedPlacement, ReportHeartbeat

AgentConfigService:
  GetAgentConfig, UpdateAgentConfig
  ListCommands, CreateCommand, UpdateCommand, DeleteCommand, SetCommandsDisabled
  GetWorkspaceProfile, UpdateWorkspaceProfile

ResourceLibraryService:
  Prompt CRUD/version/grant/rollback
  AgentTemplate CRUD/version/grant/sync
  Skill CRUD/source/package-reference/grant
  Provider CRUD/model-discovery/test/grant
  CredentialReference list/upsert/delete

MemoryService:
  MemoryStore CRUD, version list/read/rollback
  Memory read/write/delete/move
  Workspace attachment CRUD

ScheduleService:
  CreateSchedule, ListSchedules, UpdateSchedule, DeleteSchedule, RunSchedule
```

### 11.2 `media_agent_gateway`

```text
SessionService:
  CreateSession, GetSession, ListSessions, UpdateSession, DeleteSession, StarSession
  ListMessages, ListSessionFacets, GetRecentSessions

ChatService:
  RunTurn(stream), ReconnectTurn(stream), InterruptTurn
  SubmitPendingResponse, DeletePendingResponse

AgentRequestService:
  GetAgentRequest, ResolveAgentRequest

BatchRunService:
  CreateBatchRun, ListBatchRuns, GetBatchRun, CancelBatchRun

UsageService:
  GetWorkspaceUsage, GetUserUsageSummary, GetActivitySummary
```

Hertz HTTP 可以定义创建/查询接口；真正的 SSE/ACP streaming contract 需要独立 `session_stream.proto` 事件 envelope，并由 transport adapter 实现，不能假装成普通 unary RPC。

### 11.3 `media_plugin`

```text
ExtensionRegistryService:
  RegisterExtension, GetExtension, ListExtensions, EnableExtension, DisableExtension
  BindWorkspaceExtension, UnbindWorkspaceExtension, ListWorkspaceExtensions

McpCatalogService:
  RegisterMcpServer, ListMcpServers, ResolveWorkspaceMcpConfig

McpGatewayInternalService:
  AuthorizeInvocation, ResolveInvocationContext, RecordInvocation

UiBundleService:
  GetBundleDescriptor
```

`POST /mcp/{extension_id}` 是 MCP Streamable HTTP transport，不建模成 Hertz unary RPC。其 tool input/output schema 归各 Extension MCP Adapter；Registry 只保存 schema hash、tool name 和 permission metadata。

### 11.4 现有媒体服务 IDL 补充方向

| 服务 | 当前 IDL | Review 后需补充 |
|---|---|---|
| media_auth | auth/identity/oauth/service-token/team/mcp-oauth 已存在 | JWKS、capability mint、内部 GetMcpAccessToken |
| media_crawler | crawler/xhs/MCP IDL 已存在 | Subject/audience 约束、统一 tool error |
| media_materials | material/generated-asset/product/virtual-ip/reachability 已存在 | MCP Adapter tool contract |
| media_workflow | template/workflow/job/step-run 已存在 | MCP Adapter tool contract、幂等键 |
| media_gen | worker/publish 已存在 | Agent 可调用的 generation/publish facade；内部 worker RPC 不暴露为 MCP |

## 12. Harness 架构规范 Review

`harness/contributing/architecture.md` 对 Hertz REST、generated DTO、handler、service、workflow、domain、DAL 的约束基本正确，本阶段不建议修改其核心依赖规则。

发现一个需要在实现前补充的空白：规范没有定义 MCP、SSE、WebSocket 等非 Hertz generated router 的 transport adapter 放置位置。建议 Review 后增加：

```text
<app>/biz/transport/
├── mcp/        # MCP Streamable HTTP / JSON-RPC adapter
├── sse/        # SSE encoder, resume cursor transport
└── websocket/  # upgrade and frame adapter
```

规则：

- transport 只做协议解析、Subject/Command 转换、流控和错误映射。
- transport 可调用 `biz/service` / `biz/workflow`，禁止直接访问 DAL。
- domain/service/policy 禁止 import MCP/Hertz/WebSocket SDK。
- MCP tool schema 可以由独立 IDL/schema 生成，但生成模型不能进入 domain。
- `router.go` 只挂载协议入口，不包含业务规则。

在该规则确认前，不应开始 MCP Adapter 业务开发。

## 13. Linux 初始化命令（Review 后执行）

现有 `media_auth`、`media_crawler`、`media_gen`、`media_materials`、`media_workflow` 已初始化，不得重复执行。

若确认方案 B 和当前命名/端口：

```bash
cd /path/to/media_agent

go run ./harness/tools/new_server media_control media-control 8006
go run ./harness/tools/new_server media_agent_gateway media-agent-gateway 8007
go run ./harness/tools/new_server media_plugin media-plugin 8008

make hz-model
make hz-client
make swagger-gen
make arch-check
go test ./...
```

注意：当前 Windows 尝试曾在 `make hz-model` 前中断，只留下未跟踪的 `idl/media_control/health.proto`，没有创建 `media_control` 服务目录。Linux 正式执行前应先检查该文件；若内容就是 `new_server` 默认 health IDL，可以保留后重新执行，但脚手架会因已存在的 IDL 目录如何处理而需先确认工具行为。不要在未确认 git 状态时直接删除用户文件。

## 14. 旧 control-plane 接口目标映射

本节验证的是“每个旧接口 family 都有目标服务归属和 IDL 设计位置”，不是“目标服务已经完成业务实现”。当前仍处于设计阶段。

| 旧 CP route / API family | 目标服务 | 目标 domain / 接口 | 状态 |
|---|---|---|---|
| `auth.ts`, `wecom-auth.ts` | media_auth | Auth/Identity/WeCom | 已有 IDL/实现，需兼容核对 |
| `oauth-provider.ts` | media_auth | OAuthService | 已有 IDL/实现，需兼容核对 |
| `service-tokens.ts` | media_auth | ServiceTokenService | 已有 IDL/实现 |
| `teams.ts`, `invites.ts` | media_auth | TeamService/InviteService | 已有 IDL/实现 |
| `applications.ts` | media_auth | OAuthClient/Application Client | 已有 OAuthClient IDL，字段需核对 |
| `mcp-oauth.ts` | media_auth + media_plugin | OAuth credential lifecycle + connection status facade | 需补内部 GetMcpAccessToken |
| `workspaces/read.ts`, `write.ts`, `workspaces.ts` | media_control | WorkspaceService | 待新增 IDL |
| `workspaces/lifecycle.ts` | media_control | WorkspaceLifecycle workflow | 待新增 IDL |
| `workspaces/profile.ts`, `workspace-layouts.ts` | media_control | AgentConfig/Profile/Layout | 待新增 IDL |
| `workspaces/commands.ts` | media_control | AgentConfigService | 待新增 IDL |
| `workspaces/templates.ts` | media_control | TemplateSync workflow | 待新增 IDL |
| `workspaces/schedules.ts` | media_control | ScheduleService | 待新增 IDL |
| `environments.ts`, `env/*`, `env-gateway.ts` | media_control | EnvironmentService/Placement | 待新增 IDL；gateway transport 单独实现 |
| `system-workspaces.ts`, `admin/workspaces.ts` | media_control | SystemWorkspace/Admin facade | 待新增 IDL |
| `admin/cluster.ts`, `admin/system-settings.ts` | media_control | PlatformAdminService | 待新增 IDL |
| `workspaces/chat.ts` | media_agent_gateway | ChatService.RunTurn | 待新增 streaming contract |
| `proxy.ts` Agent reconnect/respond | media_agent_gateway | ChatService/Reconnect transport | 待新增；不保留任意透传代理 |
| `workspaces/sessions.ts`, session reads in `read.ts` | media_agent_gateway | SessionService | 待新增 IDL |
| `workspaces/agent-requests.ts` | media_agent_gateway | AgentRequestService | 待新增 IDL |
| `batch-runs.ts` | media_agent_gateway | BatchRunService | 待新增 IDL |
| `jobs.ts` | media_agent_gateway | DispatchJob compatibility facade | 待确认是否与 BatchRun 合并 |
| `me/recent-sessions.ts`, `me/activity.ts`, `me/usage.ts` | media_agent_gateway | SessionQuery/UsageService | 待新增 IDL |
| `workspaces/usage.ts`, `admin/stats.ts` | media_agent_gateway + reporting projection | UsageService/AdminStats | 待新增 IDL/投影 |
| `prompts/*` | media_control | prompt domain | 待新增 IDL |
| `templates.ts` | media_control | agent_template domain | 待新增 IDL；不得与 media_workflow.Template 混名 |
| `skills.ts`, `skill-registry.ts` | media_control | skill_catalog domain | 待新增 IDL；blob 走对象存储 |
| `providers.ts` | media_control | provider domain | 待新增 IDL |
| `credentials.ts` | media_control | credential_reference + Secret Store | 待新增 IDL |
| `memory-stores.ts` | media_control | memory_store domain | 待新增 IDL |
| `internal.ts` memory/skills/config | media_control | Internal RuntimeConfig/Memory/Skill API | 待新增内部 IDL + workload identity |
| `workspaces/agent.ts` files/export | media_control + Agent File Service | AgentFile facade/capability export | 待确认是否独立 AFS 服务 |
| `workspaces/afs-shares.ts` | media_control | afs_share workflow | 待确认 AFS 数据所有者 |
| `public-exports.ts` | media_control + Object Storage/CDN | ExportCapabilityService | 待新增短期 capability |
| `shares.ts` | media_control | ShareService | 待新增 IDL |
| `tags.ts` | media_control | TagService | 待新增 IDL |
| `notifications.ts` | media_control | NotificationPreference/Outbox | 待新增 IDL；发送 worker 外置 |
| `plugins.ts` | media_plugin | ExtensionRegistryService/UiBundleService | 待新增 IDL |
| `mcp-catalog.ts` | media_plugin | McpCatalogService | 待新增 IDL |
| `mcp-proxy.ts` | media_plugin | 仅外部需认证 MCP 的 OAuth Proxy | 待新增受控 transport；固定 Catalog endpoint，删除任意 origin 模式 |
| `workspace-browsers.ts` | Browser Service | Browser lifecycle client facade | 保留独立数据面；Control 只授权 |
| `workspace-sandboxes.ts` | Sandbox Service | Sandbox lifecycle client facade | 保留独立数据面；Control 只授权 |
| `cg-proxy.ts` | Channel Gateway | Channel connector API | 保留独立 adapter，不经 Control 通用代理 |
| `saas-proxy.ts` | Edge Gateway / explicit extension | 明确 upstream route | 禁止通用匿名 proxy |
| `asr.ts` | 独立 ASR Extension 或 media_gen adapter | TranscribeService/MCP tool | 待确定业务所有者 |
| `teamwork.ts` | media_agent_gateway + media_control | TeamworkSession + Workspace topology | 待补跨服务 workflow |
| `admin/users.ts` | media_auth | AdminUserService | 已有部分能力，需兼容核对 |
| `me/profile.ts` | media_auth | User/ProfileService | 已有能力，需字段核对 |

### 14.1 尚未闭合的映射

以下项目不能在 Review 前假定完成：

1. `workspaces/agent.ts` 和 `afs-shares.ts`：文件元数据、文件内容和共享关系究竟归 `media_control` 还是独立 AFS Service。
2. `jobs.ts` 与 `batch-runs.ts`：是否都是 Agent Turn 批处理，能否统一为 BatchRun 聚合。
3. `teamwork.ts`：Workspace topology 归 Control，协作 Session 归 Session，需要定义 Saga/补偿边界。
4. `asr.ts`：应作为独立 Extension，还是归入 `media_gen` 的媒体处理能力。
5. `notifications.ts`：偏好属于 Control，但投递执行应由 Channel/Notification Worker 完成。

这些不是遗漏，而是服务划分 Review 必须做出的决定。

## 15. Review 检查清单

- [ ] 接受方案 B，还是改选方案 A/C。
- [ ] 接受 `media_control` 先作为一个部署单元、内部多 domain，还是立即拆 `media_resource`。
- [ ] 接受 `media_auth` 管 MCP OAuth 凭据、`media_plugin` 管 MCP 流量。
- [ ] 接受 Agent Schedule 与 `media_workflow` 分离。
- [ ] 确定 AFS/File/Export 的数据所有者。
- [ ] 确定 Teamwork 跨 Control/Session 的一致性要求。
- [ ] 确定 ASR 所属 Extension。
- [ ] 确认 `media_agent_gateway` 是否包含 BatchRun/Job。
- [ ] 确认新服务命名和端口 `8006/8007/8008`。
- [ ] Review 通过后再运行 `new_server`、更新正式 CONTEXT/domain/workflow/ADR、定义 IDL。



## 16. 2026-08-04 Review 决议（优先于前文待 Review 标记）

本节记录本轮已经确认的架构决议。前文章节若仍保留“待 Review”字样，以本节为准，后续定稿时再统一清理文案。

### 16.1 已确认决策

- `media_control` 本阶段不再拆分 `media_resource`，采用单部署单元、内部多 domain 的结构。
- MCP OAuth credential 生命周期归 `media_auth`；MCP catalog、tool policy 和数据面转发归 Plugin Gateway。
- Agent Schedule 与 `media_workflow` 不合并。
- 不保留旧 HS256 兼容期，认证体系直接迁移到 RS256/ES256 + JWKS；结合 Go、Python、Traefik 和第三方 OIDC 兼容性，默认选择 RS256。
- AFS 平台业务数据所有者为 `media_control`。

### 16.2 服务命名建议

#### Extension 服务

推荐名称：`media_plugin`。

原因：该服务管理完整 Plugin 生命周期，包括 manifest、版本、启停、Workspace binding、UI bundle descriptor、MCP server catalog 和 MCP gateway。`extension` 容易与浏览器扩展、文件扩展名或一般扩展点混淆；`media_mcp_gateway` 又无法覆盖 UI Plugin Registry。

备选名称：

- `media_plugin_gateway`：强调入口职责，但会弱化 Registry 和版本管理。
- `media_integration`：覆盖面广，但过于抽象，不建议。
- `media_mcp_gateway`：只有在 UI Plugin Registry 被拆到其他服务时才准确。

#### Agent 交互服务

推荐名称：`media_agent_gateway`。

原因：该服务不是透明 proxy。它拥有 Session、Turn、Message、UniversalEvent、断线重连、事件持久化、背压和 AgentRequest。`media_agent_proxy` 会让维护者误以为该服务只做无状态转发；`media_session` 又容易与登录 Session、MCP Session 混淆。

备选名称：

- `media_agent_session`：突出聚合所有权，但弱化 Agent 入口和流量治理。
- `media_agent_stream`：突出 SSE 热路径，但无法覆盖 Session CRUD 和 BatchRun。
- `media_agent_proxy`：仅当服务退化成完全无状态透传时使用，不符合当前设计。

### 16.3 `media_storage` 设计

`media_storage` 合并原 Skills Content Storage，并为 Skill Package、图片、视频、音频、导出文件和其他不可变 Blob/Object 提供统一存储控制能力。

负责：

- Blob/Object metadata：object_id、namespace、content_type、size、checksum、version、owner_service、owner_resource、retention。
- Multipart upload session、complete/abort、幂等键和 checksum 校验。
- 签发短期 pre-signed upload/download URL；大文件字节流默认由客户端直连 S3/MinIO，不经过服务进程。
- 对象版本、临时对象晋升、软删除、保留期和垃圾回收。
- Skill Package、图片、视频等统一 `BlobRef` 契约。
- 存储 provider 适配，首选 S3-compatible API，开发环境使用 MinIO。

不负责：

- 不拥有 Material、GeneratedAsset、Skill 等业务元数据；对应服务只保存 `BlobRef`。
- 不解释图片、视频、Skill Package 的业务状态。
- 不提供 POSIX/FUSE 语义，不与 AFS 合并。
- 不代理 Browser WebRTC、Sandbox filesystem 或 Agent Workspace 文件操作。

建议 IDL：

```text
ObjectService:
  CreateUpload, CompleteUpload, AbortUpload
  HeadObject, CreateDownload, DeleteObject
  CopyObject, PromoteTemporaryObject, BatchHeadObjects

RetentionService:
  SetRetention, ReleaseObject, RunGarbageCollection
```

建议 `BlobRef = {object_id, version, checksum, content_type, size}`。逻辑资源删除时由资源所有者调用 `ReleaseObject`；初期不做跨租户内容去重，避免通过 checksum 形成侧信道。

### 16.4 AFS 数据边界

`D:\Code\github\afs` 当前实现中，`afs-controller` 使用 SQLite 持有 filesystem、directory、access_key 和 mount session，因此现状与“全部数据归 media_control”并不一致。目标边界定义为：

- `media_control` 是 AFS Share、Workspace membership、读写权限、创建/删除期望状态的业务事实源。
- `afs-controller` 保留 storage backend、mount session、heartbeat、ForceUnmount 等可重建运行态。
- `afs-fuse` 负责 FUSE enforcement 和实际文件操作。
- 旧长期 `access_key` 不再作为平台授权源，改为 `media_control` 签发的短期、单 directory、单 permission capability。
- `media_control` 通过 outbox/IDL 驱动 AFS directory 创建和撤销；AFS 运行态故障不得反向篡改业务授权。

### 16.5 更新后的 Linux 初始化命令

```bash
go run ./harness/tools/new_server media_control media-control 8006
go run ./harness/tools/new_server media_agent_gateway media-agent-gateway 8007
go run ./harness/tools/new_server media_plugin media-plugin 8008
go run ./harness/tools/new_server media_storage media-storage 8009
```

以上命令仍须等服务名称最终确认后执行。

### 16.6 剩余未决事项

按阻塞程度排序：

1. **服务名称最终确认**：是否正式采用 `media_plugin`、`media_agent_gateway`。
2. **AFS 运行态所有权**：是否接受上述“Control 持业务真相、AFS Controller 持可重建运行态”，还是要求 AFS Controller 完全无状态。后者需要较大改造。
3. **Storage 生命周期**：业务资源删除后立即删除 Blob，还是保留回收窗口；是否允许多个业务资源共享同一个 BlobRef。
4. **`jobs.ts` 与 `batch-runs.ts`**：统一为一个 BatchRun 聚合，还是保留两种不同的 Agent 执行模型。
5. **Teamwork**：Workspace topology 归 `media_control`、协作 Session 归 `media_agent_gateway` 后，创建/取消协作任务需要怎样的一致性和补偿。
6. **ASR 归属**：独立 `media_asr` Plugin，还是作为 `media_gen` 的音频处理能力。
7. **Notification 边界**：Preference/Outbox 归 `media_control` 已较清楚，但实际发送由 Channel Gateway 还是独立 Notification Worker 承担。
8. **Plugin 信任模型**：manifest 是否必须签名；谁可以安装、升级和禁用 Plugin；UI bundle 是否允许加载第三方 origin。
9. **服务间授权查询**：团队 membership 每次 RPC 查询 `media_auth`，还是由各服务消费事件维护授权投影。
10. **事件基础设施选型**：迁移初期使用 PostgreSQL outbox + worker，还是立即引入 NATS JetStream/Kafka。
11. **旧 API 兼容要求**：只要求能力覆盖，还是必须保持旧 URL、请求字段和错误码兼容。没有历史包袱时建议只做能力覆盖，不建设长期兼容 facade。
12. **Browser/Sandbox/Channel 的迁移范围**：本轮只定义接入契约并复用旧服务，还是同步迁移到新仓库。

其中 1-3 会影响新服务初始化和 IDL，应优先确定；4-12 可以在对应领域建模时继续收敛。



## 17. 2026-08-04 Review 第二轮决议

本节覆盖第 16 节中关于 AFS 改造和旧 API 兼容的建议。

### 17.1 已确认

- 服务正式命名为 `media_plugin` 和 `media_agent_gateway`。
- `afs-controller` 是不修改源码的第三方依赖。目标系统沿用旧 control-plane 的调用方式，不要求其无状态，也不替换其 SQLite、access_key 或 session 机制。
- `media_control` 迁移旧 CP 的 `afs_shares`、`afs_share_members` 业务数据和 create/mount/revoke/unmount 编排；通过既有 AFS API 调用第三方 `afs-controller`/`afs-fuse`。
- `media_storage` 的 Blob 删除采用回收期，不立即物理删除。回收期具体时长和恢复 API 后续建模时确定。
- 不兼容旧 API URL、请求字段和错误码；旧接口核对只用于保证能力没有遗漏，不建设兼容 facade。

### 17.2 AFS 中“无状态”的澄清

前文所谓“AFS Controller 完全无状态”，是指让它不再持有 SQLite 中的 filesystem/directory/session 数据，而全部改由 `media_control` 保存。该建议现已否决。目标设计按第三方 AFS 当前实现使用：

```text
media_control
  ├─ 保存平台侧 AfsShare / AfsShareMember
  ├─ 保存 afs_dir_id / access_key 引用
  └─ 编排 create / mount / revoke / unmount

afs-controller（第三方，原样使用）
  ├─ SQLite 保存 filesystem / directory / access_key / session
  ├─ 校验 access_key
  └─ 向 afs-fuse 推送 ForceUnmount

afs-fuse（第三方，原样使用）
  └─ 执行 mount / unmount 和 FUSE 文件操作
```

两边保存的是不同层的数据：`media_control` 保存平台业务共享关系；AFS 保存其内部执行和认证数据。这种重复引用是集成契约，不要求合并数据库。

### 17.3 下一项待讨论：Job 与 BatchRun

旧系统中的 `Job` 是 pg-boss 队列记录，包含 workspace、prompt、trigger 和 service token；`BatchRun` 是用户可见批次，包含多个 BatchTask。每个 BatchTask 最终投递一个 Job。两者不是同一个聚合。

建议目标模型：

- `AgentTask`：一次异步 Agent Turn 投递，状态、workspace、prompt、trigger、session_id、error；可以独立存在，也可以属于一个 BatchRun。
- `BatchRun`：多个 AgentTask 的分组和控制聚合，负责 concurrency、批量取消和统计。
- Queue Job：pg-boss/NATS 中的内部投递记录，不进入公共 IDL，不把 queue id 暴露为业务 id。

关系：

```text
BatchRun 1 ── N AgentTask
AgentTask 0..1 ── 1 Session
AgentTask 1 ── 1 internal queue delivery
```

候选方案：

| 方案 | 说明 | 评价 |
|---|---|---|
| A | AgentTask + BatchRun，Queue Job 内部化 | 推荐，领域与基础设施分离 |
| B | 保留旧 Job API + BatchRun API | 暴露 pg-boss 细节，不推荐 |
| C | 所有单任务也包装成只有一项的 BatchRun | 模型简单但语义别扭，不推荐 |

待确认：采用方案 A 后，`media_agent_gateway` 同时拥有 AgentTask 和 BatchRun；Scheduler 只是消费内部队列的 worker，不拥有这些业务数据。



## 18. 2026-08-04 Review 第三轮：AgentTask 与 Teamwork

### 18.1 AgentTask / BatchRun 已确认

采用第 17.3 节方案 A：

- `media_agent_gateway` 拥有 `AgentTask` 与 `BatchRun`。
- `AgentTask` 表示一次异步 Agent Turn 投递，可以独立存在，也可以归属一个 `BatchRun`。
- `BatchRun` 负责批次成员、并发上限、批量取消和统计。
- pg-boss、NATS 等中间件中的 Queue Job 只作为内部投递记录，不进入领域模型和公共 IDL。
- Scheduler 是消费内部投递的 worker，不拥有 AgentTask 或 BatchRun 业务数据。

### 18.2 Teamwork 现状与关键约束

旧 control-plane 的 Teamwork 同时涉及四类能力：

1. TeamworkTask、参与者名单和 coordinator workspace。
2. AFS share 的创建、挂载、撤销和销毁。
3. coordinator/member Session 与 parent session 关系。
4. MCP `call_agent` 根据 task/session 上下文解析可调用成员。

迁移时不能依赖旧共享数据库中的 `session_token -> session -> teamwork_session -> task` 反向查询链；否则 `media_control`、`media_agent_gateway` 和 `media_plugin` 仍然通过数据库结构隐式耦合。

### 18.3 Teamwork 服务边界候选

| 方案 | 数据归属 | 优点 | 主要问题 |
|---|---|---|---|
| A | `media_control` 拥有 Teamwork 拓扑与 AFS 生命周期；`media_agent_gateway` 拥有 Session/Turn 及 TeamworkSessionLink | 业务边界清晰，Session 与控制资源各归其主服务 | 需要显式运行上下文和事件同步 |
| B | Teamwork 全部归 `media_agent_gateway` | Agent 执行链路最短 | Workspace、成员关系和 AFS 编排落入数据面网关，职责过重 |
| C | Teamwork 全部归 `media_control`，包括 Session 关联 | 控制操作集中 | Session 生命周期跨服务，形成跨库外键和双写问题 |

建议采用方案 A。

### 18.4 方案 A 的数据所有权

`media_control`：

- `Teamwork`：名称、brief、owner、coordinator_workspace_id、状态、版本。
- `TeamworkMember`：成员 workspace、角色、加入/移除状态。
- `TeamworkAfsBinding`：afs_share_id、afs_dir_id、access_key 引用及挂载状态。
- 创建、增删成员、删除时对第三方 AFS 的既有编排和补偿。

`media_agent_gateway`：

- `Session`、`Turn`、`Message`、`AgentTask`。
- `TeamworkSessionLink`：teamwork_id、session_id、role、parent_session_id、teamwork_version。
- Teamwork Turn 的启动、执行中状态和历史保留。

`media_plugin`：

- 不保存 Teamwork 业务事实。
- MCP 工具调用时校验 gateway 签发的短期执行 capability，并按 capability 限制 task、session、workspace 和工具范围。

第三方 AFS：

- 代码与现有使用方式保持不变。

### 18.5 创建与运行链路

创建 Teamwork：

```text
Client -> media_control: CreateTeamwork
media_control: Teamwork(PROVISIONING)
media_control -> afs-controller: create share / mount coordinator
media_control: Teamwork(READY)
media_control -> outbox: TeamworkReady
```

AFS 调用失败时，`media_control` 执行既有补偿并把 Teamwork 标记为 `ERROR` 或删除未完成记录。状态机建议为：

```text
PROVISIONING -> READY -> DELETING -> DELETED
       |          |           |
       +--------> ERROR <------+
```

运行 Teamwork Turn：

```text
Client -> media_agent_gateway: RunTeamworkTurn(teamwork_id, ...)
media_agent_gateway -> media_control: GetTeamworkRuntimeContext
media_control: 校验主体、READY 状态、coordinator 和成员名单
media_agent_gateway: 创建 Session/AgentTask/TeamworkSessionLink
media_agent_gateway: 签发短期 execution capability
media_agent_gateway -> media_plugin/MCP: 携带 capability 执行 call_agent
```

`GetTeamworkRuntimeContext` 返回带 `teamwork_version` 的只读快照。capability 至少绑定：

- subject/user/team
- teamwork_id 和 teamwork_version
- root_session_id / current_session_id
- coordinator_workspace_id
- 允许调用的 member_workspace_ids
- tool scopes
- expires_at

服务之间不建立数据库外键，也不使用用户 JWT 直接驱动内部 MCP 调用。

### 18.6 删除和成员变更

- 移除成员时，`media_control` 先更新 roster 版本并禁止新的调用，再按旧方式撤销 AFS mount。
- 删除 Teamwork 时进入 `DELETING`，停止创建新 Turn，完成 AFS teardown 后进入 `DELETED`。
- `TeamworkDeleted` 事件通知 `media_agent_gateway` 失效对应执行权限。
- Session、Turn 和消息作为审计与历史记录保留，不随 Teamwork 删除。
- 不使用分布式事务；采用状态机、幂等命令和 outbox 事件保证最终一致性。

仍需确认的运行策略：成员被移除或 Teamwork 被删除时，已经在执行的 Turn/子 Agent 调用是允许自然结束，还是主动中断。


### 18.7 Teamwork 运行策略已确认

成员被移除或 Teamwork 被删除时采用自然结束策略：

- `media_control` 立即递增 roster/teamwork version，禁止创建新的 Turn 和新的 `call_agent`。
- `media_agent_gateway` 取消尚未开始的 AgentTask。
- 已经开始的 Turn 和子 Agent 调用允许自然结束，不主动中断。
- 已开始的调用在结束前不得基于旧 capability 再派生新的子调用；每次新派生调用必须重新校验当前 Teamwork version。
- Session、Turn、Message 和执行结果继续保留。
- Teamwork 删除流程不等待运行中 Turn 结束；AFS 按现有 control-plane 流程撤销，因此运行中任务必须把 AFS 撤销视为可预期的资源不可用错误。

## 19. ASR 服务归属 Review

### 19.1 现状

旧 control-plane 的 ASR 是无持久状态的同步适配能力：

```text
multipart audio
  -> 25 MiB 限制
  -> FFmpeg 转为 WAV 16kHz mono
  -> 读取 active provider/config
  -> OpenAI-compatible transcription
  -> text/language/duration
```

它至少有两类调用者：

- Channel Gateway：Slack/企微语音消息的低延迟同步转写。
- 媒体生产流程：对较长音频或视频提取字幕、文案和可检索文本，适合异步执行。

音频和视频字节应统一进入 `media_storage`；ASR 接口消费 `BlobRef` 或短期下载 capability，不在服务间传输大文件。

### 19.2 候选方案

| 方案 | 说明 | 优点 | 主要问题 |
|---|---|---|---|
| A | 在 `media_gen` 内增加独立 `transcription` domain/provider adapter | 不增加部署单元；复用生成侧 provider、限流、worker 和 FFmpeg 依赖 | `media_gen` 名称比实际职责窄，需要严格保持内部 domain 边界 |
| B | 新建 `media_transcription` 服务，同时提供同步 Transcribe 和异步 worker | 职责、扩缩容和故障隔离最好 | 增加服务、IDL、部署和运维成本；当前能力规模不足以证明独立服务必要 |
| C | ASR 仅实现为 `media_plugin` 中的 MCP tool | 插件接入简单 | Channel Gateway 被迫走 MCP；长任务、重试、限流和内部 typed RPC 边界不清晰，不推荐 |

当前建议方案 A：ASR 归 `media_gen.transcription`，但不归 generation 聚合。它是同一部署单元中的并列 domain：

```text
media_gen
  ├─ generation      # 图像/视频/文本等生成执行 adapter
  ├─ transcription   # 音频/视频 -> 文本
  └─ publish          # 平台发布 adapter
```

### 19.3 建议接口与执行模式

同步短音频：

```text
TranscriptionService.Transcribe
  input:  source BlobRef, language?, hint?, idempotency_key
  output: text, detected_language?, duration_ms?, provider
```

异步长音视频：

```text
media_workflow StepRun
  -> media_gen transcription worker
  -> 读取 media_storage BlobRef
  -> 转码/转写
  -> 将 transcript 结果写回 StepRun candidate/output
```

同步接口设置明确的大小和时长上限；超过上限返回“必须异步处理”，不能让 Channel Gateway 长时间持有连接。MCP Adapter 可以把安全的 `transcribe_media` tool 暴露给 Agent，但复用同一个 application service，不复制 ASR 逻辑。

### 19.4 配置与鉴权边界

- Provider catalog、默认 provider 和非敏感模型参数归 `media_control` 的 provider/runtime config domain。
- API key 等密钥保存到 Secret Store，只在 workload identity 授权后由 `media_gen` 获取。
- Channel Gateway 使用 workload token 调同步 TranscriptionService，不使用用户长期 token，也不通过 `media_control` 代理音频。
- Agent/MCP 调用使用 `media_agent_gateway` 签发、audience 为 `media_gen` 或 `media_plugin` 的短期 capability。
- `media_gen` 做最终资源鉴权：校验主体是否对输入 BlobRef 具有读取权限，以及输出写入范围。
- 原始音视频和可选 transcript 文件归 `media_storage`；业务上的 transcript 元数据由发起领域持有。

待确认：采用方案 A，将 ASR 作为 `media_gen` 内部并列的 `transcription` domain，而不是新增服务。


### 19.5 ASR 归属已确认

采用方案 A：

- 不新增 ASR 服务。
- ASR 作为 `media_gen` 部署单元内独立的 `transcription` domain。
- 同步短音频使用 `TranscriptionService.Transcribe`；长音视频通过 `media_workflow` 的 StepRun 异步执行。
- 输入输出大文件统一使用 `media_storage.BlobRef`，不经 `media_control` 转发。
- MCP tool、Channel Gateway 和 workflow worker 复用同一 application service。

## 20. Notification 服务边界 Review

### 20.1 现状和问题

旧系统中：

- control-plane 保存 `notifications`、`notification_preferences`、`notification_deliveries`、`notification_reads`。
- control-plane 根据 event/scope preference 解析发送 channel，并向 pg-boss 投递 `notification-deliver`。
- Scheduler 消费任务，直接读取和更新 control-plane 表。
- Scheduler 内置 Inbox、Apprise、WeCom Adapter，并直接查询认证域的 `user_identities`。
- Channel Gateway 已经拥有 Slack/WeCom 等渠道的连接器、凭据和发送实现，但未作为通用通知发送边界。

主要问题不是是否异步，而是 Scheduler 通过共享数据库同时耦合 Notification 和 Identity 两个领域。

### 20.2 候选方案

| 方案 | 说明 | 优点 | 主要问题 |
|---|---|---|---|
| A | Notification domain 归 `media_control`；同代码库提供独立 Notification Worker 进程；外部渠道经 Channel Gateway 发送 | 不增加数据服务；消除跨库访问；Worker 可单独扩缩容 | `media_control` 同一代码库包含 API 和 worker 两种进程角色 |
| B | 新建 `media_notification` 服务，拥有 preference/inbox/delivery | 边界和扩缩容最彻底 | 当前规模下增加服务、IDL、数据库和运维成本 |
| C | 全部归 Channel Gateway | 渠道发送集中 | Inbox、用户偏好、已读状态和业务通知事实不属于渠道接入层，不推荐 |

建议采用方案 A。这里的 Notification Worker 是独立部署进程，不是新的业务服务或数据所有者：

```text
media_control codebase/image
  ├─ api process
  │    └─ NotificationPreference / Notification / Inbox APIs
  └─ notification-worker process
       └─ claim delivery -> dispatch -> update result
```

API 和 Worker 共享 Notification domain/repository 代码与同一数据库 schema。部署时可以使用同一镜像的不同启动参数，例如 `--role=api` 和 `--role=notification-worker`。

### 20.3 数据所有权

`media_control.notification`：

- `Notification`：不可变通知内容和业务事件类型。
- `NotificationPreference`：user + event_type + scope + channel 的覆盖规则。
- `NotificationDelivery`：每个目标用户和 channel 的投递状态、attempt、next_retry_at、error。
- `NotificationRead`：Inbox 已读状态。
- channel resolution、inbox 查询、已读操作、重试和死信策略。

Notification Worker：

- 没有独立业务数据库。
- 只通过 Notification domain repository claim/update Delivery。
- 不查询 `media_auth` 数据库，不持有 Workspace/Session 等业务事实。

Channel Gateway：

- 拥有 Slack、WeCom 等渠道协议适配、connector credentials、外部 user/channel destination 解析。
- 提供内部 `SendNotification` typed API。
- 不保存 NotificationPreference、Inbox 或业务 Delivery 状态。

### 20.4 处理链路

```text
业务服务
  -> media_control.NotificationService.CreateNotification
     (idempotency_key, event_type, recipients, payload, scope)
  -> 同一事务：
       Notification
       + resolved NotificationDelivery
       + outbox/queue signal

notification-worker
  -> claim pending deliveries
  -> inbox: 直接标记 sent
  -> slack/wecom:
       ChannelGateway.SendNotification(
         delivery_id,
         user_id,
         channel,
         payload
       )
  -> media_control repository: sent / retry / dead-letter
```

业务服务也可以发布领域事件，由 `media_control.notification` 的事件消费者转换为 Notification；事件本身不能直接等同于用户通知，因为通知还需要模板、收件人解析和 preference 判断。

### 20.5 一致性与幂等

- CreateNotification 必须接收业务 `idempotency_key`，避免领域事件重放生成重复通知。
- 每个 Delivery 使用 `delivery_id` 作为 Channel Gateway 的幂等键。
- 投递语义为 at-least-once；外部渠道不支持幂等时，极端情况下仍可能重复。
- Worker 使用 lease/claim 防止并发重复消费，失败按 `attempt/next_retry_at` 重试，超过上限进入 dead letter。
- Inbox delivery 不需要调用 Channel Gateway，创建后即可视为 sent。
- 外部渠道发送成功不影响 Inbox 查询和业务事务提交。

### 20.6 鉴权

- 用户调用 Preference、Inbox、MarkRead 时，`media_control` 校验 JWT subject，只能访问自己的通知；管理员代查需要显式 scope。
- 业务服务创建通知时使用 workload identity，并校验允许发布的 `event_type` namespace，避免任意服务伪造安全类通知。
- Notification Worker 调 Channel Gateway 使用 audience 限定的 workload token。
- Channel Gateway 根据 connector ownership、tenant 和 channel policy 做最终发送授权，不信任调用方传入任意 webhook URL 或外部账号。

待确认：采用方案 A，Notification 业务数据继续归 `media_control`；使用同代码库、独立进程的 Notification Worker；Slack/WeCom 等实际发送统一调用 Channel Gateway 内部接口。


### 20.7 方案重新评估：推荐改为 B

结合本项目没有历史兼容负担、正在重新划分数据所有权，方案 B 比方案 A 更合适。第 20.2 节“建议采用方案 A”由本节覆盖，推荐新建 `media_notification`。

理由：

1. Notification 已经是完整 bounded context，具有独立的 Preference、Notification、Delivery、Read、模板解析、重试和 dead-letter 状态机。
2. 它被 `media_control`、`media_agent_gateway`、`media_workflow`、`media_crawler` 等多个服务共同使用，不是 Control 专属子域。
3. Notification 的容量、保留周期和扩缩容维度与 Workspace/Provider/Teamwork 等控制面资源不同。
4. 独立服务可以彻底消除 Scheduler 直接读取 control-plane 与 auth 表的问题，而不是只把同库访问封装到另一个进程。
5. 当前处于重构设计期，现在拆分成本最低；如果先落在 `media_control`，后续迁移数据和 API 的成本更高。

目标边界调整为：

```text
业务服务
  -> media_notification.NotificationCommandService
       ├─ Notification
       ├─ Preference
       ├─ Delivery
       ├─ Read
       └─ Outbox
             |
             v
       notification worker（同 media_notification 代码库）
             |
             v
       Channel Gateway.SendNotification
             |
             v
       Slack / WeCom / other channel
```

`media_notification` 是业务服务和数据所有者；其 API process 与 worker process 可以使用同一代码库、镜像和数据库，但分别部署和扩缩容。

`media_control` 不再保存 Notification 业务表，仅作为普通调用方发布控制面事件或调用通知命令。

建议服务职责：

- `media_notification`：通知模板、收件人、偏好覆盖、Inbox、已读、Delivery、重试、dead letter 和审计。
- Channel Gateway：Connector、外部目标解析、渠道协议、发送凭据和 provider response；不拥有通知偏好或 Inbox。
- `media_auth`：用户及外部身份绑定事实源；通过内部 typed API 提供必要映射，不共享数据库。
- 各业务服务：产生领域事件或显式 Notification command，不实现渠道发送。

建议核心 IDL：

```text
NotificationCommandService:
  CreateNotification
  CreateNotificationFromTemplate

NotificationInboxService:
  ListInbox
  GetUnreadCount
  MarkRead
  MarkAllRead

NotificationPreferenceService:
  ListPreferences
  SetPreference
  DeletePreference

NotificationDeliveryInternalService:
  ClaimDeliveries
  CompleteDelivery
  RetryDelivery
  DeadLetterDelivery
```

初始化命令候选：

```bash
go run ./harness/tools/new_server media_notification media-notification 8010
```

仍不在设计阶段执行。待确认：正式采用方案 B，并新增 `media_notification` 服务。


### 20.8 Notification 方案已确认

正式采用方案 B：

- 新增 `media_notification` 服务，独立拥有 Notification、Preference、Delivery、Read、Template、Retry 和 Dead Letter。
- `media_notification` API 与 Worker 使用同一代码库/镜像/数据库，可以作为两个进程分别部署。
- Channel Gateway 只拥有渠道协议、Connector 凭据、外部目标解析和最终发送。
- `media_control` 不保存通知业务表，只是通知生产者之一。
- 后续初始化命令为 `go run ./harness/tools/new_server media_notification media-notification 8010`，设计阶段不执行。

## 21. Plugin 信任模型 Review

### 21.1 当前风险

旧实现允许管理员上传 bundle 和任意 JSON metadata，Web 再用普通 `<script src=...>` 注入主页面。当前机制存在以下边界：

- 管理员身份只能说明谁上传，不能证明 bundle 来自哪个发布者或 CI。
- metadata 只验证为合法 JSON，不验证 schema、权限、endpoint 或 surface ownership。
- bundle 没有内容 digest/integrity 校验。
- 同页面 script 与 Host 共享 DOM、JavaScript global、local storage 和可发起的请求；签名或 SRI 只能防篡改，不能隔离恶意代码。
- MCP catalog URL 若允许任意 origin，会把 Plugin Gateway 变成 SSRF/凭据转发入口。
- 原地 upsert 覆盖版本，缺少不可变版本、审批、回滚和 compromised-version 禁用。

### 21.2 候选方案

| 方案 | 说明 | 评价 |
|---|---|---|
| A | 仅管理员上传，Registry 信任上传者，不要求签名 | 实现简单，但供应链、版本不可变性和审计不足，不推荐 |
| B | Publisher 签名 + 不可变 PluginVersion + 权限审批；可信 UI 可同页面运行，非可信 UI 必须隔离 | 推荐，能覆盖内部 Plugin 并保留第三方扩展路径 |
| C | 所有 Plugin UI 都运行在 sandbox iframe，所有发布使用 Sigstore/透明日志 | 隔离最强，但 Host SDK、登录态、部署和运维复杂度较高，适合作为后续增强 |

建议采用方案 B，并把“可信”定义为平台明确登记和审批的发布者，而不是“签名有效即可信”。

### 21.3 信任等级

```text
PLATFORM_TRUSTED
  平台自有 CI 签名和审批
  UI 可以加载到 Host realm
  仍必须校验 digest/SRI、CSP 和声明权限

PARTNER_SANDBOXED
  已登记合作方签名
  UI 只能运行在独立 origin 的 sandbox iframe
  通过窄化 postMessage Host API 交互

BACKEND_ONLY
  不加载 UI bundle
  只注册后端服务和 MCP tools
  Agent 根据下发配置直连该服务的 MCP endpoint
```

初期可以只实现 `PLATFORM_TRUSTED` 和 `BACKEND_ONLY`。在 sandbox Host SDK 完成前，不允许第三方 UI 以普通 script 进入 Host realm。

### 21.4 签名和制品规则

推荐初期采用登记 Publisher Ed25519 公钥的离线签名模型：

1. CI 构建 immutable bundle/MCP descriptor。
2. 对 bundle 计算 SHA-256。
3. 对 RFC 8785/JCS canonical manifest（其中包含全部 artifact digest）生成 detached Ed25519 signature。
4. `media_plugin` 根据 `publisher_id + key_id` 验签，校验 manifest schema、权限和 endpoint policy。
5. 验证后的 bundle 写入 `media_storage`，`ExtensionVersion` 保存 immutable `BlobRef`、digest、signature 和 verified_at。
6. Web 只接收 Registry 下发的受控 URL和 SRI；不接受 Plugin 自报任意 URL。
7. 私钥只存在于 CI Secret/KMS，平台数据库只保存公钥；支持 key rotation 和 revoke。

签名对象必须覆盖：

- extension id/version/publisher。
- backend service identity、audience。
- MCP endpoint descriptor、tool schema digest。
- UI artifact digest、surfaces。
- requested permissions、outbound hosts。
- build/provenance metadata。

签名验证通过不代表自动启用；它只证明来源和内容未被修改。

### 21.5 角色和生命周期

权限建议：

| 操作 | 角色 |
|---|---|
| 注册 Publisher、公钥、吊销 Publisher | Platform Security Admin |
| 注册新 PluginVersion | Release CI 或 Platform Plugin Operator |
| 审批版本及其权限 | Platform Plugin Approver |
| Tenant/Workspace 安装已审批版本 | Tenant Admin / Workspace Owner（受组织策略限制） |
| 全局禁用 compromised version | Platform Plugin Operator |
| 普通用户直接上传或注册 endpoint | 禁止 |

状态分离：

```text
ExtensionVersion:
  UPLOADED -> VERIFIED -> APPROVED -> REVOKED

ExtensionBinding:
  DISABLED <-> ENABLED
  pinned_version 必须指向 APPROVED version
```

版本不可覆盖；升级创建新的 `ExtensionVersion`。如果 requested permissions、outbound hosts、MCP tools 或 UI surfaces 增加，Binding 必须重新审批，不能静默自动升级。Registry 保留旧版本用于回滚，但被 `REVOKED` 的版本不能重新启用。

### 21.6 Backend/MCP 约束

- Manifest 的 backend 使用已登记的 `service_id`，不允许安装者填写任意 URL。
- `service_id -> endpoint/workload audience` 由平台 Service Catalog 或部署配置解析。
- 第三方 MCP endpoint 必须经过 scheme、DNS/IP、egress allowlist 和 redirect 再校验，禁止访问 loopback、link-local、metadata endpoint 和内部管理网段。
- `media_plugin` 按声明的 tool allowlist、JSON schema digest、timeout、rate limit 和 capability scope 执行。
- MCP OAuth refresh token 仍归 `media_auth`；`media_plugin` 只按调用上下文获取短期 access token。
- Plugin 后端只接收短期 audience-bound workload/session capability，不接收浏览器原始 cookie 或长期用户 JWT。

### 21.7 对新增 Plugin 的影响

平台内置的 `media_crawler`、`media_gen`、`media_materials`、`media_workflow` 发布流程：

```text
Plugin repo/CI
  -> build backend + MCP adapter + optional UI
  -> generate manifest
  -> hash artifacts
  -> sign manifest
  -> media_plugin.RegisterExtensionVersion
  -> verify / approve
  -> bind tenant/workspace
```

新增 Plugin 不修改 `media_control`。需要新增的仅是：

- Plugin 自身后端和 domain/IDL。
- Plugin 自身 MCP Adapter/tool schema。
- 可选 UI bundle。
- 一个签名 manifest 和部署/Service Catalog 条目。

待确认：采用方案 B；初期只开放 `PLATFORM_TRUSTED` 同页面 UI 和 `BACKEND_ONLY`，第三方 UI 在 sandbox iframe Host SDK 完成前不开放。


### 21.8 Plugin 模型简化决议（覆盖 21.2-21.7 的推荐）

第 21.2-21.7 节按“第三方插件市场”设计，超出了本项目需求。本项目的 Plugin 是平台团队开发和部署的内置模块，不允许普通用户上传或注册前端代码，因此取消 Publisher 签名、UI Bundle Registry、SRI、sandbox iframe、前端版本审批等设计。

#### 前端边界

以 `media_workflow` 为例：

```text
ui_workflow
  -> 在开发/发布阶段集成到 ui_platform 的受控 plugin/module 目录
  -> 与 ui_platform 一起编译
  -> 作为同一个 Web artifact 发布
```

规则：

- `media_plugin` 不接收、保存或分发 UI bundle。
- 不存在用户提交的 `bundleUrl`、任意前端 origin 或运行时 script 注入。
- UI 模块由代码仓库、code review 和统一 CI/CD 信任链管理。
- UI 路由、面板、tool renderer 在构建期静态注册。
- Workspace 是否显示/可用某模块，可以查询后端 capability/catalog，但这只是功能开关，不是动态加载代码。
- `ui_workflow`、后续 `ui_crawler`、`ui_materials` 等遵守 `ui_platform` 的前端模块契约和依赖规则。

因此 `ExtensionVersion.ui_bundle`、`UiBundleService`、Publisher key、artifact signature 和前端安装 API 全部从目标模型删除。

#### 后端和 MCP 边界

运行期 Plugin 只包含：

```text
Plugin
  ├─ backend service     # media_workflow 等业务服务
  └─ MCP adapter/server # 将 MCP tool 转换为同一 application service 调用
```

`media_plugin` 负责：

- 平台内置 Plugin/MCP Catalog。
- Plugin backend 的 `service_id`、workload audience、MCP endpoint 和 tool policy。
- Workspace/Team 对已部署 Plugin 的启用关系。
- 向 `media_agent_gateway` 提供当前 Workspace 可用的 MCP servers/tools 配置数据，并在 binding 变更时发布配置变更。
- 仅为外部需要 OAuth 的 MCP 提供 Streamable HTTP Proxy、Session context 解析、OAuth 凭据注入、timeout、rate limit 和审计。
- 向 `media_auth` 获取第三方 MCP 短期 access token。

`media_plugin` 不负责：

- Plugin 的业务数据和业务规则。
- 前端代码、前端制品或前端版本。
- 构建和部署 Plugin backend。
- 接受普通用户提供的 MCP URL 或 backend URL。
- MCP OAuth refresh token。

#### 简化的 Catalog 来源

初期不建设用户可调用的 Plugin 上传/注册市场。Catalog 使用平台部署仓库中的声明式配置，由 Platform Operator/CI 随服务部署：

```yaml
id: media-workflow
backend:
  service_id: media-workflow
  audience: media-workflow
mcp:
  service_id: media-workflow-mcp
  transport: streamable-http
  tools:
    - workflow.create
    - workflow.run
    - workflow.status
permissions:
  - workflow.read
  - workflow.run
```

推荐初期由 `media_plugin` 启动时加载受控配置/ConfigMap，并校验：

- plugin id、service id 和 tool name 唯一。
- service id 必须存在于平台 Service Catalog。
- endpoint 由 service id 解析，配置中不允许任意 URL。
- audience、required scopes 和 tool allowlist 必填。
- 配置错误时该 Plugin 不进入 active catalog，并产生明确告警。

如果未来确实需要不重启地注册服务，再增加仅供 CI/Operator 使用的内部 Catalog Admin API；当前不提前建设。

#### 新增 Plugin 的实际步骤

新增 `media_workflow` 类型的 Plugin：

1. 开发或修改 `media_workflow` 后端，定义业务 IDL。
2. 在同一服务或独立进程中实现 MCP Adapter；Adapter 只转换协议并调用 application service。
3. 开发 `ui_workflow`，按 `ui_platform` 的模块契约静态集成并统一构建。
4. 在部署配置中增加 backend/MCP Service 和 Catalog 条目。
5. 部署后 `media_agent_gateway` 根据 Catalog、Workspace binding 和 Agent 配置生成 MCP config snapshot 并下发；Agent 直接调用 `media_workflow /mcp`。

不需要修改 `media_control` 的 route、handler、domain 或 schema，也不需要上传 URL、bundle 或签名 manifest。

#### 保留的安全要求

取消前端签名系统不等于取消后端安全边界：

- 只有 Platform Operator/CI 可以修改 Catalog 配置。
- 普通用户不能注册 backend/MCP endpoint。
- backend、MCP 和内部 typed IDL 使用 mTLS 认证调用服务；Agent MCP 请求同时携带 `session_token` 解析用户上下文。
- Agent 直连平台内 MCP 时携带 `session_token`；MCP 服务通过 `media_agent_gateway.ResolveSessionContext` 获取可信 user/workspace/session 上下文。
- endpoint 只从 Service Catalog 解析。
- 平台在 Agent 启动和配置变更时校验并下发 tool 配置；平台内 MCP 对本服务 tool 和业务资源执行最终校验。外部需 OAuth MCP 的代理策略仍由 `media_plugin` 执行。
- Plugin 业务服务对资源所有权执行最终鉴权，不能只依赖 Gateway。

待确认的只剩一个实现选择：MCP Adapter 与业务服务默认同进程部署，还是作为独立进程部署；两者都复用同一 application service 和 IDL，不改变上述服务边界。


### 21.9 MCP Adapter 部署方式已确认

- MCP Adapter 与 Plugin 业务服务默认同代码库、同进程、同部署单元。
- 例如 `media_workflow` 同时暴露 Hertz/IDL 业务 API 和 `/mcp` Streamable HTTP transport。
- MCP transport 只负责 JSON-RPC/MCP 协议、`session_token` 解析、Principal 构造和错误映射，直接调用同进程 application service。
- 不新增 `media-workflow-mcp` Service、Deployment 或独立数据存储。
- 未来只有在 MCP 流量、长连接或故障隔离形成明确瓶颈时，才允许保持同代码库并拆为独立进程；业务边界和 IDL 不变。
- Catalog 中 backend 与 MCP 使用同一个 `service_id`，MCP endpoint 是该服务的受控 `/mcp` 路径。

## 22. 跨服务认证与鉴权 Review

> 本节保留早期讨论过程，其中通用 capability JWT、workload identity 和 Agent/MCP 短期 capability 的结论已被第 25 节覆盖，不得作为实现依据。

### 22.1 必须区分的三层判断

```text
Authentication:
  请求者是谁？
  -> 每个服务本地验证 media_auth JWKS 签发的 JWT/capability

Platform authorization:
  是否具有 platform admin、workload audience、scope？
  -> token claims + 本地 policy

Resource authorization:
  是否能读写这个具体 Workspace/Workflow/Material/Blob？
  -> 资源所有者服务根据本地资源数据做最终判断
```

`media_auth` 拥有 User、Team 和 TeamMembership，但不拥有其他服务的业务资源 ACL。它只能回答“用户在 Team 中是什么角色”，不能替 `media_workflow` 判断某个 Workflow 是否可修改。

### 22.2 候选方案

| 方案 | 说明 | 优点 | 主要问题 |
|---|---|---|---|
| A | 需要 Team 关系时同步调用 `media_auth.CheckTeamAccess`；资源规则本地判断 | 简单、成员移除立即生效、无需事件投影 | media_auth 成为部分请求的同步依赖 |
| B | 每个服务消费 Team 事件并维护本地 Membership projection | 查询快、Auth 短暂故障不影响读取 | 最终一致性、撤权延迟、投影与对账复杂 |
| C | 把全部 Team membership/roles 写进用户 JWT | 服务完全本地判断 | Token 变大，成员变化无法及时生效，不适合长期使用 |

当前建议先采用方案 A。原因是系统仍处于重构初期，Team 关系规模和请求量尚未证明需要投影，而且事件基础设施尚未定型。先建立一个正确、统一的授权契约，比让每个服务复制 Membership 表更稳妥。

### 22.3 方案 A 的调用规则

每个服务：

1. 本地通过缓存的 JWKS 验证 JWT，不为 Authentication 每次调用 `media_auth`。
2. 从 JWT/capability 构造统一 `Subject`：`subject_id`、`subject_type`、`platform_role`、`scopes`、`audience`。
3. 读取本地资源，确定 `owner_type=user|team` 和 `owner_id`。
4. user-owned 资源直接比较 `subject_id == owner_id`，不调用 Auth。
5. team-owned 资源调用 `media_auth.CheckTeamAccess(team_id, subject_id, required_role)`。
6. 结合本地资源状态、visibility、share/grant 等规则给出最终 Allow/Deny。

禁止：

- Edge Gateway 鉴权通过后，业务服务跳过资源鉴权。
- 将客户端传入的 `owner_id/team_id` 直接视为已授权。
- 让 `media_auth` 查询其他服务数据库决定 Workflow/Material/Workspace 权限。
- 列表中对每一行分别调用 Auth，形成 N+1 RPC。

建议内部 IDL：

```text
AuthorizationInternalService:
  CheckTeamAccess(team_id, subject_id, required_role)
  BatchCheckTeamAccess(team_ids, subject_id, required_role)
  ListSubjectTeamGrants(subject_id)
```

响应包含 `allowed`、`actual_role`、`membership_version`，但不返回业务资源权限。

### 22.4 性能和可用性

- 单请求内相同 `subject_id + team_id` 的判断去重。
- List API 先调用一次 `ListSubjectTeamGrants` 或 `BatchCheckTeamAccess`，再在本地 SQL 中按允许的 team ids 过滤。
- RPC 设置短超时、有限重试和熔断；Auth 不可达时对需要 Team 判断的操作 fail closed。
- user-owned 资源、公开资源以及已签发的短期 capability 不依赖实时 Auth RPC。
- `media_auth` 作为核心基础服务需要至少多副本部署，但这不改变数据所有权。
- 不建议跨请求长时间缓存 Allow 结果；否则成员移除无法立即生效。若以后增加缓存，Deny 可以短缓存，Allow 必须受 membership version/短 TTL 约束。

### 22.5 Agent/MCP 热路径

Agent 的每个 tool call 不应同步查询 `media_auth`：

```text
创建 Session/AgentTask
  -> media_agent_gateway/media_control 做一次实时资源和 Team 校验
  -> 签发短期 capability
       subject + workspace + session + tools + scopes + exp
  -> media_plugin 和 Plugin backend 本地验 capability
```

Capability 有效期内代表已完成的一次授权委托。对成员移除等需要立即阻止新操作的场景：

- 禁止基于旧 capability 派生新 capability。
- 每次新 Turn、续期或子 Agent 派生都重新查询当前授权。
- 已开始的 Teamwork Turn 按已确认策略允许自然结束。

### 22.6 后续演进条件

只有满足以下任一条件，才把特定高频服务演进到方案 B：

- Auth RPC 的 p95/p99 明确成为主要延迟。
- Team-owned 资源列表规模导致 BatchCheck 仍不可接受。
- 业务要求在 Auth 短暂不可用时继续提供 Team 资源读取。
- 已确定可靠的 outbox/event bus、projection version 和全量对账机制。

投影是逐服务选择，不要求所有服务同时维护。即使采用投影，高风险写操作、授权变更和 capability 签发仍可同步查询 `media_auth`。

待确认：初期采用方案 A；认证本地验 JWT，Team membership 按需同步 RPC，资源级权限始终由资源所有者服务最终判断；暂不建设跨服务 Membership 投影。


### 22.7 跨服务鉴权方案已确认

采用方案 A：

- 各服务通过 JWKS 本地验证 JWT/capability。
- User-owned 资源由资源所有者服务本地鉴权。
- Team-owned 资源按需调用 `media_auth.AuthorizationInternalService`。
- 列表查询使用 BatchCheck/ListSubjectTeamGrants，禁止 N+1 Auth RPC。
- Agent 直连平台内 MCP 并携带 `session_token`；MCP 通过 `media_agent_gateway.ResolveSessionContext` 获取可信上下文，不逐 tool call 查询 `media_auth`。
- 暂不建设跨服务 TeamMembership projection。

## 23. 事件基础设施 Review

### 23.1 先区分四种机制

| 需求 | 推荐机制 | 示例 |
|---|---|---|
| 调用方必须立即得到成功/失败 | typed synchronous RPC | CheckTeamAccess、GetRuntimeContext |
| 本地事务提交后触发跨服务副作用 | transactional outbox | TeamworkDeleted、NotificationRequested |
| 某领域内部的长任务领取和重试 | 领域 Job/Delivery 表 + lease/SKIP LOCKED | StepRun、AgentTask、NotificationDelivery |
| 浏览器实时展示执行事件 | SSE/WebSocket event log | Turn UniversalEvent |

不能因为都叫“异步”就统一塞进 Kafka/NATS。尤其 `media_workflow.StepRun` 已经有自己的状态机和 Claim/Complete 协议，不应再把 broker message 当作业务事实源。

### 23.2 候选方案

| 方案 | 说明 | 优点 | 主要问题 |
|---|---|---|---|
| A | 每个服务使用 PostgreSQL transactional outbox，由本服务 dispatcher 调用目标服务的 typed internal API | 基础设施最少；与本地事务原子；容易调试和补偿 | Producer 需要知道少量 destination；不适合大规模广播 |
| B | PostgreSQL outbox relay 到 NATS JetStream，消费者订阅 durable stream | 发布订阅、扩展消费者和解耦更好 | 新增 broker、subject/retention/consumer 运维，仍然不能省略 outbox |
| C | PostgreSQL outbox relay 到 Kafka | 强 replay、partition ordering 和数据平台生态 | 当前吞吐和消费者数量不支持其运维成本，不推荐 |

初期建议方案 A。目标仓库当前只有 PostgreSQL，跨服务事件数量有限，而且大部分关系是明确的一对一副作用。先把 outbox/inbox 语义做正确；未来增加 NATS 时只替换 dispatcher 的发布 adapter，不修改业务事务。

### 23.3 Outbox 模型

每个拥有本地数据库的服务维护自己的 `integration_outbox`：

```text
event_id
event_type
event_version
aggregate_type
aggregate_id
aggregate_version
destination
payload
occurred_at
available_at
attempts
status              # pending / delivering / delivered / dead
locked_until
last_error
trace_id
```

在修改聚合的同一个本地事务中写入 outbox。事务提交后 dispatcher 使用 lease + `FOR UPDATE SKIP LOCKED` 领取，携带 workload token 调目标服务内部接口。

对于一个事件有多个明确消费者的情况，事务内为每个 destination 创建独立 delivery 行，使每个消费者具有独立的 attempts/status。不要因为一个消费者成功就把所有消费者标记为 delivered。

### 23.4 Consumer 幂等

会改变状态的消费者维护 `integration_inbox` 或等价的业务幂等键：

```text
consumer
event_id
processed_at
result
```

处理流程：

1. 接收 `event_id/event_type/version/payload`。
2. 在本地事务中检查 inbox。
3. 已处理则直接返回成功。
4. 执行业务状态变更并写 inbox。
5. 提交后返回成功。

语义为 at-least-once，不承诺 exactly-once。全局顺序不保证；需要顺序的聚合携带单调 `aggregate_version`，消费者忽略旧版本，并对版本缺口告警或触发重新读取事实源。

### 23.5 Dispatcher 运行方式

- Dispatcher 与所属服务同代码库、同镜像，可以用 `--role=outbox-dispatcher` 独立部署，也可以初期嵌入 API 进程。
- Dispatcher 只读写本服务 outbox，不直接访问其他服务数据库。
- 调用目标内部 API 时携带 audience-bound workload token、`event_id` 和 trace context。
- 指数退避、最大 attempts、dead-letter、人工 replay 和积压指标必须统一实现。
- 外部调用不能发生在业务数据库事务内。
- 目标服务返回不可重试业务错误时进入 dead-letter；超时、网络错误和 5xx 重试。
- 服务关闭前释放 lease；超时 lease 可被其他实例重新领取。

建议在 `harness` 提供共享的 outbox/inbox 基础设施包和 migration 模板，但 domain event 类型、payload 和目标仍由各服务定义。

### 23.6 与领域任务的边界

以下仍使用本地业务表，不通过 integration outbox 代替：

- `media_workflow`：Job/StepRun + Claim/Complete/Fail。
- `media_agent_gateway`：AgentTask/BatchRun + internal queue delivery。
- `media_notification`：NotificationDelivery + worker。
- `media_storage`：multipart session、retention/GC task。

Outbox 只负责这些状态变化之后的跨服务通知，例如：

```text
TeamworkDeleted       media_control -> media_agent_gateway
AgentTaskCompleted    media_agent_gateway -> media_notification
WorkflowCompleted     media_workflow -> media_notification
BlobReleased          business service -> media_storage
PluginBindingChanged  media_plugin -> media_agent_gateway
```

### 23.7 演进到 NATS JetStream 的条件

出现以下情况再采用方案 B：

- 同一事件需要被多个独立消费者动态订阅，destination 配置开始频繁修改 Producer。
- outbox HTTP dispatcher 的连接、重试或吞吐成为可观测瓶颈。
- 需要消费者按自身进度 replay 较长事件历史。
- 需要跨语言 durable pub/sub，并愿意承担 broker 运维。
- 已定义稳定的 event envelope、schema evolution 和 consumer ownership。

迁移路径：

```text
当前：
local transaction -> outbox -> HTTP dispatcher -> consumer inbox

未来：
local transaction -> outbox -> NATS publisher -> JetStream -> consumer inbox
```

业务事务仍写 outbox，因此引入 NATS 不需要改聚合和 application service。

Kafka 只在后续出现大规模顺序事件流、长期 replay、数据平台/分析消费等明确需求时重新评估。

待确认：初期采用方案 A，即 PostgreSQL transactional outbox + typed internal API dispatcher；不立即引入 NATS/Kafka，并在 harness 中统一 outbox/inbox、lease、retry、dead-letter 和 telemetry。


### 23.8 Outbox/Inbox 与旧 pg-boss 的关系

Outbox/Inbox 是跨事务边界的一致性模式；pg-boss 是 PostgreSQL 上的 Node.js Job Queue 实现。两者可以组合，但不能相互等同。

旧源码中的典型 Notification 流程：

```text
INSERT notifications
INSERT notification_deliveries
boss.send("notification-deliver")
```

当前这些步骤不在同一个显式数据库事务中。可能出现：

- 业务记录已提交，进程在 `boss.send` 前退出：数据存在但永远没有 Job。
- pg-boss Job 已创建，但业务数据写入只完成一部分：Worker 读取到不完整状态。
- Worker 成功调用外部系统，但更新 Delivery 状态前退出：Job 重试并可能重复发送。

因此“使用 pg-boss”本身没有解决 transactional outbox 和 consumer idempotency。

三种组合方式：

| 方式 | 说明 | 评价 |
|---|---|---|
| 业务提交后直接 `boss.send` | 两次独立写入 | 存在双写窗口，不接受 |
| 同一 DB transaction 同时写业务表和 pg-boss Job | 对单数据库、单消费者 command 可实现原子性 | 强耦合 pg-boss 内部 API/schema，且目标服务是 Go，不适合作为统一方案 |
| 同一事务写业务表 + outbox，pg-boss 只用于唤醒/调度 dispatcher | Outbox 是事实源；Job 丢失仍可由 scanner 补发 | 可用于旧 Node 服务过渡，但新 Go 服务没有必要保留 pg-boss |

目标架构建议：

- 旧 control-plane/scheduler 迁移期可以继续运行 pg-boss，不要求立即改写旧项目。
- 新 Go 服务不引入 Node pg-boss，也不共享旧 pg-boss schema。
- 新服务在业务事务内写 `integration_outbox`。
- Go dispatcher 直接使用 PostgreSQL lease + `FOR UPDATE SKIP LOCKED` 领取和重试。
- 消费方通过 `integration_inbox` 或业务唯一键幂等。
- `AgentTask`、`NotificationDelivery`、`StepRun` 等领域任务表是业务事实源，不把 pg-boss Job ID 暴露为业务 ID。

如果希望迁移期间复用 pg-boss，可将它作为优化性的 wake-up signal：

```text
business transaction:
  business row + outbox row COMMIT

after commit:
  boss.send(outbox_event_id)   # 允许失败

dispatcher:
  被 pg-boss 唤醒，按 event_id 处理
  + 周期扫描 pending outbox，补偿丢失的 wake-up
```

这里即使 `boss.send` 失败，pending outbox 仍能被扫描处理；即使 Job 重复，consumer inbox 仍保证幂等。pg-boss 不再是真相来源。

最终建议仍为第 23 节方案 A，但补充明确：新项目的 PostgreSQL outbox dispatcher 不依赖 pg-boss；pg-boss 仅属于旧 Node 服务的迁移实现或可选唤醒机制。


### 23.9 最终决议：River 管任务，Outbox/Inbox 管集成事件

本节覆盖第 23.2 节中“全部使用自研 PostgreSQL lease worker”和第 23.8 节中“新 Go 服务不依赖任务框架”的建议。

正式采用混合方案：

```text
本地后台任务
  -> TaskScheduler Port
  -> River Adapter
  -> PostgreSQL

跨服务集成事件
  -> Integration Outbox
  -> Integration Publisher Port
  -> 初期 Typed HTTP
  -> 后续可替换 NATS JetStream

跨服务消费
  -> Consumer Inbox / 业务幂等键
  -> Application Service
```

#### River 的职责

River 作为 Go 服务的本地后台任务基础设施，负责：

- 自动重试和退避。
- 延迟执行和 Scheduled Job。
- 固定系统 Cron/Recurring Job。
- 多队列、优先级、并发 Worker。
- Job timeout、cancel、unique 和失败记录。
- 通过事务入队保证业务数据与本地后台任务同时提交。
- 可选 River UI 运维入口。

使用场景：

- `media_agent_gateway`：AgentTask 执行投递。
- `media_notification`：NotificationDelivery 发送重试。
- `media_storage`：Retention/GC。
- 各服务：超时扫描、补偿任务、Outbox Dispatcher 唤醒。
- `media_control`：到期 AgentSchedule 的执行投递。

River Job 只是基础设施记录，不是业务事实源。AgentTask、StepRun、NotificationDelivery、AgentSchedule 等领域实体继续使用自身业务表和业务 ID。

#### Outbox/Inbox 的职责

显式 `integration_outbox` 保留用于跨服务集成事件：

- 一条事件可能有多个消费者。
- 需要稳定 event envelope、schema version 和 aggregate version。
- 需要独立投递状态、审计或 replay。
- 未来可能发布到 NATS JetStream。
- 事件生命周期不能退化为某个 Worker 的内部 Job。

典型事件：

```text
TeamworkDeleted       media_control -> media_agent_gateway
AgentTaskCompleted    media_agent_gateway -> media_notification
WorkflowCompleted     media_workflow -> media_notification
BlobReleased          business service -> media_storage
PluginBindingChanged  media_plugin -> media_agent_gateway
```

消费者使用 `integration_inbox` 或业务唯一键保证幂等，语义为 at-least-once。

#### 初期与未来传输

初期：

```text
local transaction
  -> integration_outbox
  -> HTTP Dispatcher
  -> typed internal API
  -> consumer inbox
```

未来：

```text
local transaction
  -> integration_outbox
  -> NATS Publisher
  -> JetStream
  -> consumer inbox
```

引入 NATS 时只替换 `IntegrationPublisher` 和 consumer transport adapter，不修改聚合、application service、outbox 写入和事件 schema。引入消息队列不要求替换 River；River 与 NATS 分别承担本地任务和跨服务事件传输，可以长期共存。

#### 端口隔离

业务代码禁止直接依赖 River 类型：

```go
type TaskScheduler interface {
    Enqueue(ctx context.Context, tx Transaction, task Task) error
    Schedule(ctx context.Context, tx Transaction, task Task, runAt time.Time) error
    Cancel(ctx context.Context, taskID string) error
}

type IntegrationPublisher interface {
    Publish(ctx context.Context, event IntegrationEvent) error
}
```

建议位置：

```text
biz/
├── domain/                    # 不 import River/NATS
├── service/                   # 不 import River/NATS
├── workflow/                  # 依赖 application port
└── shared/
    └── taskqueue/             # TaskScheduler port/types

biz/dal 或 infrastructure adapter:
├── taskqueue/river/           # River 实现
└── integration/
    ├── http/                  # 初期 Publisher/Consumer
    └── nats/                  # 未来实现
```

具体目录需在正式实现前与 `harness/contributing/architecture.md` 的依赖规则对齐；Infrastructure Adapter 不得反向进入 domain。

River Job payload 只携带业务 ID 和必要的幂等/trace 元数据：

```go
type RunAgentTaskArgs struct {
    AgentTaskID string `json:"agent_task_id"`
}
```

禁止：

- 在 domain/service/IDL 暴露 River Job ID 或 River 状态。
- 把完整业务实体复制进 Job payload 作为事实源。
- 使用 River Job 替代多消费者 Integration Event。
- 使用 Integration Outbox 替代 AgentTask/StepRun 等领域状态机。

#### 定时任务边界

固定系统任务可以使用 River Cron/Recurring Job，例如 Storage GC、统计刷新和补偿扫描。

用户动态配置的 Agent Schedule 仍归 `media_control`：

```text
AgentSchedule:
  cron_expression
  timezone
  next_run_at
  enabled
  version
```

Schedule 业务表是事实源；调度器只把到期 Schedule 转换为带唯一键的 River Job。修改、暂停、删除和错过执行的策略由 AgentSchedule 领域定义，不由 River Cron 配置代替。

#### GORM 事务集成要求

目标项目使用 GORM。实现前必须做一个小型技术验证：

1. 固定 River 版本。
2. 使用 River `database/sql` driver。
3. 验证现有 `TxRunner` 能向 River Adapter 提供同一个 `*sql.Tx`。
4. 验证业务写入与 River `InsertTx` 同事务提交/回滚。
5. 验证 migration、schema、PgBouncer 模式和测试事务兼容性。

如果无法可靠共享同一事务，则不得退化为“业务提交后再 Insert River Job”；应改为同事务写显式 outbox，再由 River/dispatcher 领取。

#### 最终选型

- 本地任务框架：River。
- 业务数据库：PostgreSQL。
- ORM：GORM。
- 跨服务一致性：Integration Outbox/Inbox。
- 初期事件传输：typed internal HTTP。
- 未来事件传输：按需要增加 NATS JetStream。
- Kafka 和 Temporal 当前不引入。
- 旧 pg-boss 不迁移，仅作为旧行为核对来源。


## 24. media_storage 最终设计决议

本节覆盖第 16.3 节及其他章节中关于通用 `BlobRef`、Storage 持有对象生命周期、预签名上传 URL、S3/MinIO 优先和 Storage 处理派生资源的旧建议。

### 24.1 边界

`media_storage` 合并原 Skills Content Storage，为 Skill Package、图片、视频、音频和导出文件提供统一的字节存取能力。它是薄存储门面，不是业务资源目录或通用资源权限中心。

负责：

- 提供固定的流式上传、读取、查询和删除 API；首期不使用预签名上传 URL。
- 通过 mTLS 客户端证书 SAN 校验 `CallerService` 以及允许访问的 namespace；不得使用服务 JWT、HTTP `Origin`/`Referer` 或生产环境请求 Header 识别服务。
- 将稳定的逻辑 `storage_path` 映射到当前存储 Driver 的实际位置。
- 封装本地文件系统、阿里云 OSS 等实现，不向业务服务暴露 bucket、endpoint、本地根目录或 provider object key。
- 本地文件 Driver 必须规范化路径、防止目录越界，并使用临时文件加原子替换避免暴露未完成文件。

不负责：

- 不拥有 Material、GeneratedAsset、Skill 等业务元数据。
- 不决定资源能否删除、共享或进入回收期；这些规则归上传资源的业务服务。
- 不维护通用 `ObjectReference`、引用计数或跨业务共享关系。其他服务需要独立生命周期时，应复制对象到自己的 namespace。
- 暂不处理图片缩略图、视频转码等派生资源。
- 不提供 POSIX/FUSE 语义，也不与 AFS 合并。

### 24.2 数据模型与路径

资源所属业务服务自行定义 namespace 之后的相对路径规则，`media_storage` 根据 mTLS `CallerService` 强制调用方只能访问被授权的 namespace。例如：

```text
media-materials/team-1001/materials/mat-001/original/asset-001.jpg
```

业务数据库保存逻辑路径，不保存完整 URL、域名、bucket、Provider 类型、本地根目录或临时签名 URL。Material 记录示例：

```text
id             = mat-001
team_id        = team-1001
storage_path   = media-materials/team-1001/materials/mat-001/original/asset-001.jpg
visibility     = public
content_type   = image/jpeg
size_bytes     = 2384921
width          = 1920
height         = 1080
deleted_at     = NULL
purge_after    = NULL
```

上传、删除和回收均由资源所属业务服务发起。需要回收期时，业务服务保存自身的 `deleted_at/purge_after`，使用 River 定时调用 `DeleteObject`；`media_storage` 不拥有通用 Retention 领域模型。

### 24.3 读取可见性

采用 `public/private` 双模式：

- `public`：业务 API 返回数据时，通过配置的 `public_base_url + storage_path` 派生稳定 URL；数据库不持久化该 URL。得到 URL 的访问者可以直接读取。
- `private`：资源所属业务服务先校验终端用户权限，再以 workload identity 调用 `media_storage` 读取，并向客户端流式返回。

`media_storage` 只判断调用服务能否读写对应 namespace，不解释 Material、Skill 或 Workflow 的终端用户权限。公开 URL 使用平台控制的稳定域名，可按部署指向 `media_storage`、OSS 自定义域名或 CDN，以便更换底层存储而不修改业务数据。

### 24.4 存储 Driver

核心端口使用通用字节流和逻辑路径，不依赖 S3 API：

```go
type BlobStore interface {
    Put(ctx context.Context, path string, body io.Reader, size int64, meta Metadata) error
    Open(ctx context.Context, path string) (io.ReadCloser, ObjectInfo, error)
    Stat(ctx context.Context, path string) (ObjectInfo, error)
    Delete(ctx context.Context, path string) error
}
```

首期 Driver：

- `LocalFileStore`：开发、测试和单机部署。
- `AliyunOSSStore`：阿里云 OSS 部署。

后续可以增加 S3/MinIO Driver，但 S3-compatible API 不是领域契约。首期每个部署只配置一个主 Driver：

```yaml
storage:
  driver: aliyun_oss
  public_base_url: https://media.example.com
```

当前不按对象动态选择 Provider，因此业务表不增加 `provider_type`。存储迁移属于 `media_storage` 和运维职责，对业务服务保持同一个 `storage_path`。

建议 IDL：

```text
ObjectService:
  PutObject
  GetObject
  HeadObject
  DeleteObject
  CopyObject

InternalObjectService:
  BatchHeadObjects
```

IDL 中使用 `storage_path`、流式字节、content type、size 和可选 checksum，不暴露具体 Driver 类型。是否需要 multipart/chunk upload 在出现实际大文件需求后作为传输能力扩展，不改变业务数据模型。

## 25. 认证、Platform Token 与资源分享最终决议

本节覆盖前文中以下过程方案：平台统一 Capability Issuer、用短期 capability 取代 `platform_token`、Agent/MCP 热路径统一使用 capability JWT，以及 Browser/Sandbox 必须直接验证平台 capability 的结论。这些均不作为后续实现依据。

### 25.1 凭证语义

每个面向用户的业务服务支持两种通用用户认证方式：

1. OAuth 认证获取用户信息，然后签发 Cookie JWT，用于 Web 访问。
2. `service_token` 认证，用于 CLI、自动化和非浏览器访问。

系统中的凭证分工为：

| 凭证 | 语义 | 主要用途 |
|---|---|---|
| Cookie JWT | 当前登录用户 | Web 调用业务 API |
| `service_token` | 代表创建该 Token 的用户 | CLI、自动化、非浏览器 API |
| `session_token` | 当前 Agent Session 代表的用户、Workspace 和 Session | Agent 调用 Platform MCP 及平台内第三方 MCP |
| `platform_token` | 特殊的长期 `service_token`，本质上仍代表用户 | 非用户直接发起的任务，以及平台代理调用不支持 `session_token` 的服务 |

`platform_token` 不删除，也不被平台统一 capability 取代。它与普通 `service_token` 使用同一 Bearer Token 认证通道，业务服务验证后均得到统一的 User Principal。除非某个业务规则明确要求，资源服务不需要区分普通 `service_token` 和 `platform_token`。

### 25.2 Agent 代表用户调用 Browser

Agent 只持有 `session_token`，不持有也不能读取用户的 `platform_token`。Platform MCP 归 `media_agent_gateway`，由 Gateway 解析 `session_token`并在可信后端内取得对应用户的 `platform_token`，再调用 Browser Service：

```text
Agent
  -> X-Session-Token: session_xxx
media_agent_gateway / Platform MCP
  -> 解析 user + workspace + session
  -> 在服务端取得用户 platform_token
  -> Authorization: Bearer tos_alice_platform_token
Browser Service
  -> 验证 Token，得到 user-alice
  -> 根据本地资源数据执行最终鉴权
```

创建 Browser 的具体请求：

```http
POST /api/browsers
Authorization: Bearer tos_alice_platform_token
```

Browser Service 通过认证服务验证 opaque Service Token，可对验证结果做短 TTL 缓存，然后创建：

```text
browser_id = browser-123
owner_id   = user-alice
```

这条链路中 `platform_token` 会出现在 Gateway 和目标业务服务的可信后端请求中，但不得进入 Agent Runtime、UI、第三方 MCP、对外 URL、任务 Payload、事件消息、日志或 Trace。

### 25.3 定时任务

`platform_token` 继续用于用户不在线时执行的定时任务。River Job Payload 只保存 `schedule_id`、`task_id`、`user_id` 等业务引用，不保存 `platform_token`。可信执行端领取任务后，再根据用户和任务上下文从可信凭证存储中取得 Token 并调用目标服务。

`platform_token` 需要可恢复原文，因此不能只存 Hash；应当加密存储，并支持撤销、轮换和使用审计。

### 25.4 资源服务自主分享 Token

当某个服务需要将具体资源向可信边界外公开或分享时，由该资源的所属服务自行创建、验证和撤销短时、受限的资源访问 Token。平台不建设通用 Capability Issuer，`media_auth` 不拥有具体资源的分享规则。

Browser 分享链接示例：

```http
POST /api/browsers/browser-123/share-links
Authorization: Bearer tos_alice_platform_token
Content-Type: application/json

{
  "permission": "live:access",
  "expires_in": 3600
}
```

Browser Service 必须同时检查：

1. Service Token 有效，并解析得到 `user-alice`。
2. `browser-123.owner_id == user-alice`。

检查通过后，Browser Service 生成至少 256 位强度的随机 Token，原文仅在创建响应中返回一次，数据库只保存 Hash：

```text
browser_access_tokens
---------------------------------------------------
token_hash       SHA-256(blv_K7pQeR...)
browser_id       browser-123
owner_id         user-alice
permission       live:access
expires_at       2026-08-04 18:00:00
revoked_at       NULL
created_at       ...
```

响应：

```json
{
  "url": "https://browser.example.com/live/t/blv_K7pQeR.../browser-123/",
  "expires_at": "2026-08-04T18:00:00Z"
}
```

持有分享链接的访问者不需要 Cookie、Service Token、`platform_token` 或 NAP 账号。该 Token 表达“持有者可以在有效期内以 `live:access` 访问 `browser-123`”，不表达访问者的用户身份。如果业务要求只能分享给指定用户，必须要求对方 OAuth 登录并校验 Browser ACL，不使用匿名分享 Token。

Browser Service 的 HTML 请求和 WebSocket Upgrade 必须使用同一套资源 Token 校验，避免 HTML 通过而 WebSocket 无法连接或绕过校验。

### 25.5 Browser Service 的认证与资源授权边界

Browser Service 在实现上有三条凭证校验路径，但只有前两条是通用用户认证：

| 凭证 | 允许路由 | 校验结果 | 性质 |
|---|---|---|---|
| Cookie JWT | `/api/browsers/*` | User Principal | 用户认证 |
| 普通/Platform Service Token | `/api/browsers/*` | User Principal | 用户认证 |
| Browser Access Token | `/live/t/:token/:browser_id/*` | Browser Grant | 局部资源授权 |

Browser Access Token 不是第三套通用用户认证，不产生 User Principal，不能用于 `/api/browsers/*`。Cookie JWT 或 Service Token 也不得作为 `/live/t/*` 的隐式降级方式。两组中间件和路由必须显式隔离：

```text
/api/browsers/*
  -> AuthenticateUser(Cookie JWT | Service Token)
  -> AuthorizeBrowserResource(UserPrincipal)

/live/t/*
  -> ValidateBrowserGrant(raw token, browser_id, permission, expiry, revoked_at)
  -> ProxyToBrowser(browser)
```

### 25.6 安全底线

- `platform_token` 只能在可信平台后端和目标业务服务之间通过 TLS 传递。
- `platform_token` 不得下发给 Agent Runtime、UI、第三方 Plugin/MCP 或匿名访问者。
- 所有服务必须在 access log、application log、error report 和 distributed trace 中对 `Authorization`、Cookie 和分享 Token 脱敏。
- 资源分享 Token 必须使用 CSPRNG，数据库只保存 Hash，并同时绑定资源、权限、有效期和撤销状态。
- 含 Token 的分享 URL 不得记录完整 path；应配置访问日志脱敏，并使用严格的 Referrer Policy。
- 资源服务始终根据本地 owner/ACL 做最终鉴权，Gateway 或 Token 验证通过不能替代资源鉴权。

### 25.7 平台内自定义 MCP 的直连认证与上下文

平台内自定义 MCP（例如 `media_workflow`、`media_gen`、`media_materials`、`media_crawler`）在 Agent 运行期不经过 `media_plugin` 转发。`media_plugin` 仅管理受控 MCP Catalog、Workspace binding 和配置变更，不是平台内 MCP 的数据面 Gateway。

Agent 启动时，平台根据 Agent、Workspace、Catalog 和 binding 生成 MCP 配置快照并下发给 `+
+  -> X-Session-Token: session_xxx+
+media_workflow /mcp`。当 Catalog、Workspace binding 或 Agent 配置变更时，平台重新生成并下发配置，行为与旧 `agent-platform/agents` 保持一致。

```text
media_plugin
  -> 管理 Catalog / Workspace binding
  -> 配置变更通知 media_agent_gateway

media_agent_gateway
  -> 在 Agent 启动或配置变更时生成 MCP config snapshot
  -> 下发给 +
+  -> X-Session-Token: session_xxx+
+media_workflow /mcp

+
+  -> X-Session-Token: session_xxx+
+media_workflow /mcp
  -> 按 config snapshot 直接连接平台内 MCP endpoint
```

示例配置：

```json
{
  "mcpServers": {
    "media-workflow": {
      "url": "https://media-workflow.internal/mcp",
      "headers": {
        "X-Session-Token": "session_xxx"
      }
    }
  }
}
```

运行期调用链路：

```text
media_agents
  -> mTLS caller: media-agents
  -> X-Session-Token: session_xxx
media_workflow /mcp
  -> media_agent_gateway.ResolveSessionContext(session_xxx)
  -> user_id + workspace_id + session_id + agent_id
  -> MCP Adapter 构造 Principal
  -> 直接调用同进程 Application Service
```

`session_token` 是这条链路中唯一的 Agent Session 用户上下文凭证，mTLS 另外证明调用服务是 `media-agents`。MCP 服务必须从 `media_agent_gateway` 拥有的 Session 记录解析 `user_id`、`workspace_id`、`session_id` 和 `agent_id`，不得信任 Agent 自行传入的 `X-User-ID`、`X-Workspace-ID` 或同名 body 字段。

建议内部 IDL：

```text
AgentSessionInternalService:
  ResolveSessionContext(ResolveSessionContextRequest)

ResolveSessionContextRequest:
  session_token

ResolveSessionContextResponse:
  active
  user_id
  workspace_id
  session_id
  agent_id
  session_status
  context_version
```

该接口只返回构造业务 Principal 所需的最小 Session 上下文，不返回 `platform_token`、Cookie JWT、OAuth access/refresh token 或其他用户长期凭证。解析结果可以使用短 TTL 缓存，但新 MCP 连接、缓存过期和 Session 状态变更后必须重新校验。

MCP Adapter 与业务服务同进程，因此不需要用 `platform_token` 调用自己的 HTTP API。Web 入口和 Agent MCP 入口分别构造同一业务 Principal：

```text
Web -> Cookie JWT / service_token -> UserPrincipal -> Application Service
Agent -> session_token -> SessionContext -> UserPrincipal -> Application Service
```

只有 Platform MCP 代表用户调用 Browser 等仅支持 Service Token 的独立服务时，才由 `media_agent_gateway` 在可信后端内使用 `platform_token`。平台内自定义 MCP 不得为了获取用户或 Workspace 信息而读取 `platform_token`。

四类 MCP 的运行路径最终确定为：

| MCP 类型 | Agent 运行期路径 | 用户上下文来源 | `platform_token` |
|---|---|---|---|
| Platform MCP | Agent -> `media_agent_gateway /mcp` | Gateway 本地解析 `session_token` | 必要时由 Gateway 内部使用 |
| 平台内自定义 MCP | Agent -> 业务服务 `/mcp` | 业务 MCP 调 Gateway `ResolveSessionContext` | 不获取 |
| 外部无认证 MCP | Agent -> 外部 MCP | 不注入平台用户上下文 | 不获取 |
| 外部需 OAuth MCP | Agent -> `media_plugin` OAuth Proxy -> 外部 MCP | Proxy 解析 `session_token` 后选择用户 OAuth 凭据 | 不获取 |

### 25.8 内部服务身份：mTLS 最终决议

平台内部服务调用使用 mTLS 认证调用服务，不新增服务身份 JWT，也不复用代表用户的 `service_token`。

凭证语义最终分工为：

| 安全对象 | 机制 |
|---|---|
| Web 用户身份 | Cookie JWT |
| CLI/自动化用户身份 | `service_token` |
| Agent Session 用户上下文 | `session_token` |
| 平台代理用户执行 | `platform_token` |
| 平台内部调用服务身份 | mTLS 客户端证书 |

mTLS 不是第三种用户认证方式。它只证明“哪个服务发起了内部请求”，用户或 Agent Session 上下文仍由 Cookie JWT、`service_token`、`session_token` 或 `platform_token` 提供。

每个服务使用独立证书，证书 SAN 携带稳定服务身份，示例：

```text
spiffe://media-platform/service/media-materials
spiffe://media-platform/service/media-workflow
spiffe://media-platform/service/media-agent-gateway
spiffe://media-platform/service/media-agents
```

初期不强制引入完整 SPIFFE 系统，但 URI SAN 格式和服务名映射必须稳定。服务端验证平台内部 CA、证书有效期和 SAN，再将 SAN 映射为 `CallerService`。请求 Header 中的 `X-Service-ID` 不得作为生产身份依据。

例如 `media_storage` 的 namespace 授权：

```text
mTLS SAN = media-materials
storage_path = media-materials/team-1001/...
  -> allow

mTLS SAN = media-workflow
storage_path = media-materials/team-1001/...
  -> deny
```

对外 Web 流量继续使用普通 HTTPS：

```text
Browser -> Edge Gateway: HTTPS + Cookie JWT
Edge Gateway -> media_workflow: internal mTLS
```

终端浏览器不需要客户端证书。若不使用透明 Service Mesh，同一服务可以区分公共 Listener 和内部 mTLS Listener；不得在公网 Listener 上强制终端用户提供客户端证书。

#### 统一配置开关

mTLS 由共享基础设施配置控制，配置顶级命名与 harness 的现有服务端治理配置保持一致：

```yaml
security:
  internal_mtls:
    enabled: ${INTERNAL_MTLS_ENABLED:-false}
    ca_file: ${INTERNAL_MTLS_CA_FILE:-}
    cert_file: ${INTERNAL_MTLS_CERT_FILE:-}
    key_file: ${INTERNAL_MTLS_KEY_FILE:-}
```

开关语义：

- `enabled=true`：内部 Listener 必须使用 `RequireAndVerifyClientCert`；内部 Client 必须提供客户端证书并验证服务端 CA。
- `enabled=false`：仅允许 `app.env=local|test|dev` 使用，便于单元测试、本地调试和 Docker Compose 联调。
- `app.env=staging|prod` 且 `enabled=false`：服务必须在启动期拒绝运行，不允许静默降级。
- 证书文件缺失、CA 解析失败、SAN 不合法时 fail closed。

测试模式关闭 mTLS 时，可通过 `X-Internal-Service` 注入模拟 `CallerService`，但中间件必须同时确认 `app.env=local|test|dev` 且 `internal_mtls.enabled=false`。生产模式必须忽略并删除该 Header，不得将其视为真实身份。

证书来源可按部署环境替换，但协议和身份语义不变：Kubernetes 可使用 cert-manager、Service Mesh 或后续 SPIFFE；Docker Compose 使用开发 CA 和挂载证书。实现前需验证 Agent 使用的 MCP HTTP Client 是否支持客户端证书；若不支持，由 Agent Pod 的透明代理/Sidecar 承载 mTLS，不改变 MCP 协议。

### 25.9 `ResolveSessionContext` 安全边界最终决议

`ResolveSessionContext` 是 `media_agent_gateway` 的内部接口，使用 mTLS 认证调用服务，使用 `session_token` 定位 Agent Session：

```text
media_workflow /mcp
  -> mTLS caller: media-workflow
  -> session_token: session_xxx
media_agent_gateway.ResolveSessionContext
  -> caller_service = media-workflow
  -> user_id + workspace_id + session_id + agent_id
```

建议 IDL：

```text
AgentSessionInternalService:
  ResolveSessionContext(ResolveSessionContextRequest)

ResolveSessionContextRequest:
  session_token

ResolveSessionContextResponse:
  active
  user_id
  workspace_id
  session_id
  agent_id
  session_status
  context_version
  expires_at
```

Request 不携带 `caller_service_id`、`user_id`、`workspace_id` 或 `agent_id`。`caller_service_id` 必须由 mTLS SAN 获取，其他字段必须由 Gateway 的 Session 记录获取。

Gateway 对接口执行服务级白名单鉴权，示例：

```yaml
security:
  session_context:
    allowed_callers:
      - media-workflow
      - media-gen
      - media-materials
      - media-crawler
      - media-plugin
```

`media_storage` 等不需要用户 Session 上下文的服务默认不允许调用。测试模式关闭 mTLS 时，可使用第 25.8 节限定的 `X-Internal-Service` 模拟调用方。

`ResolveSessionContext` 只负责验证 Session 状态并返回构造 Principal 所需的最小上下文；不查询 Plugin Catalog、Workspace MCP binding 或 tool allowlist。MCP 是否下发由 Agent 配置生成阶段判断，具体资源权限由业务服务最终判断。

返回内容禁止包含 `platform_token`、Cookie JWT、用户 `service_token`、OAuth access/refresh token、完整 Team/Workspace 权限列表和其他非必要个人信息。

`session_token` 使用 Header 或 RPC metadata 传递，不得出现在 URL、query string、日志、Trace attribute、River Job Payload 或 MCP tool arguments 中。Gateway 数据库仅保存 Token Hash，不保存可恢复原文。

解析结果允许短 TTL 缓存：

- 有效结果最长缓存 60 秒，且不得超过 Session 自身 `expires_at`。
- 无效结果最长缓存 5 秒，用于抑制重复无效查询。
- 新 MCP 连接必须至少解析一次。
- 缓存未命中且 Gateway 不可达时 fail closed。
- 已开始的 Tool Call 按已确认策略允许自然结束，新调用必须在缓存过期后重新验证。
