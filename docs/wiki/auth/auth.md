# Neutree Agent Platform 认证鉴权全景

> 面向初级工程师的入门文档。读完这篇，你应该能回答：我用什么框架做认证？token 存哪？权限怎么判定？一个典型请求从浏览器到数据库经过哪些认证关卡？

---

## 1. 全局认知：整体架构

NAP 是多服务架构，认证鉴权分散在 4 个服务里，但共享同一套 JWT 密钥和用户体系。

```
┌─────────────────────────────────────────────────────────────────┐
│                         用户浏览器                                │
│  (cookie: token=JWT)                                            │
└────────────┬────────────────────────────────────┬───────────────┘
             │                                    │
             ▼                                    ▼
┌────────────────────────┐         ┌──────────────────────────────┐
│   Web 前端 (React)     │         │  Browser Service Web 控制台   │
│  cookie 认证 + 前端     │         │  cookie 认证 (browser_token)  │
│  权限路由               │         │                              │
└────────────┬───────────┘         └──────────────┬───────────────┘
             │ /api/*                            │ /api/* /cdp/* /live/*
             ▼                                    ▼
┌─────────────────────────────────────────────────────────────────┐
│                    Control Plane (CP)                            │
│  Hono + OpenAPIHono                                             │
│  ┌─────────────────────────────────────────────────────────┐    │
│  │ 认证中间件 (全局)                                         │    │
│  │  1. 白名单跳过                                            │    │
│  │  2. Plugin Admin Token (CI 专用)                         │    │
│  │  3. Bearer: Service Token 或 OAuth JWT                   │    │
│  │  4. Cookie: Session JWT (滑动续期)                       │    │
│  └─────────────────────────────────────────────────────────┘    │
│  ┌─────────────────────────────────────────────────────────┐    │
│  │ 权限层 (per-resource)                                     │    │
│  │  visibility: private | team | public                     │    │
│  │  my_permission: owner | editor | viewer | public         │    │
│  │  team grants: team_id + permission                        │    │
│  └─────────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────────┘
             ▲                                    ▲
             │ Bearer tos_xxx                     │ Bearer tos_xxx
             │                                    │
┌────────────┴───────────────┐    ┌──────────────┴───────────────┐
│  Channel Gateway            │    │  Browser Service (后端)       │
│  cookie JWT (管理 API)      │    │  双模认证:                    │
│  /webhook secret 签名验证   │    │  - Bearer (调 CP /auth/me)   │
│  /internal 无认证 (内部)    │    │  - Cookie (本地 JWT verify)  │
│  Slack: Socket Mode         │    │  - ?token= (共享 URL)         │
│  WeCom: WS + subscribe      │    │  所有权隔离: claim map        │
└─────────────────────────────┘    └──────────────────────────────┘
```

**核心设计原则**：
- **CP 是认证的 single source of truth**：所有服务最终都通过 CP 的 `/api/auth/me` 或共享 JWT_SECRET 验证身份
- **Session Token 给人，Service Token 给机器**：浏览器交互用 cookie，API 集成用 Bearer
- **权限在资源层判定**：不在中间件做粗粒度 RBAC，而是每个资源（prompt/skill/template/provider）自己查 visibility + grants

---

## 2. 技术栈一览

| 层 | 技术 | 用途 |
|----|------|------|
| **Web 框架（后端）** | Hono + `@hono/zod-openapi` | 所有服务都用 Hono，统一风格 |
| **JWT 库** | `hono/jwt` (HS256) | 签发/验证 session token 和 OAuth access token |
| **密码哈希** | `@node-rs/argon2` | 本地密码登录 |
| **LDAP** | `ldapts` | 企业 LDAP 登录 |
| **Service Token** | `node:crypto` (randomBytes + SHA-256) | 生成 `tos_` 前缀 token |
| **前端框架** | React 18 + react-router-dom v6 | SPA |
| **前端状态** | React Context (AuthContext) + zustand | 认证状态用 Context |
| **Token 存储** | HTTP Cookie (HttpOnly) | 前端不接触 token 字符串 |
| **数据库** | PostgreSQL (共享) | users / service_tokens / *_grants / browser.claims |

---

## 3. 三种 Token 体系

这是理解整个认证系统的核心。NAP 有三种 token，各司其职：

### 3.1 Session Token（给浏览器用户）

**是什么**：HS256 签名的 JWT，存在名为 `token` 的 HttpOnly cookie 里。

**签发位置**：`control-plane/src/services/auth.ts:121-138` `generateToken()`

```typescript
// 简化示意
const token = await sign(
  { sub: userId, username, name, role, exp: now + 86400 },
  JWT_SECRET,
  'HS256',
)
// 设为 cookie
setCookie(c, 'token', token, { httpOnly: true, maxAge: 86400, ... })
```

**JWT Payload 结构**（`auth.ts:32-40`）：

| 字段 | 类型 | 说明 |
|------|------|------|
| sub | string | 数据库 user id |
| username | string | 用户名 |
| name | string | 显示名 |
| role | 'user' \| 'admin' \| 'system' | 角色 |
| exp | number | 过期时间戳（24 小时） |

**滑动续期**（`auth.ts:156-174`）：每次请求时，如果 token 剩余有效期 < 12 小时（一半），自动签发新 token 写回 cookie。用户无感知续期。

**验证位置**：CP 认证中间件 `control-plane/src/index.ts:231-268`，用 `hono/jwt` 的 `verify(token, JWT_SECRET, 'HS256')`。

### 3.2 Service Token（给 API 集成）

**是什么**：`tos_` 前缀的随机字符串，永久有效（直到吊销）。

**生成位置**：`control-plane/src/lib/service-token.ts:4-6`

```typescript
export function generateToken(): string {
  return `tos_${randomBytes(32).toString('hex')}`  // tos_ + 64 位 hex
}
```

**存储方式**：数据库只存 SHA-256 hash，不存明文。创建时明文只返回一次。

**使用方式**：`Authorization: Bearer tos_xxxxxxxx`

**验证位置**：CP 认证中间件 `control-plane/src/index.ts:203-228`

```typescript
// 简化逻辑
const hash = sha256(bearerToken)
const row = await db.getServiceTokenByHash(hash)  // 查 service_tokens 表
if (row && !row.revoked_at) {
  c.set('user', { sub: row.created_by, username: `svc:${row.name}`, role: 'user' })
}
```

**平台 Token（特殊 Service Token）**：每个用户登录时自动创建一个 `is_platform=true`、名为 `__platform__` 的 token（`shares.ts:130-151` `ensurePlatformToken`）。这个 token 的明文存在数据库里（其他 token 只存 hash），供 Browser Service、Channel Gateway 等内部服务代用户调用 CP 时使用。

### 3.3 OAuth Access Token（给第三方应用 / 对接外部 MCP）

OAuth Token 在 NAP 里有**两个截然不同的角色**，对应两套实现。

#### 角色 A：NAP 作为 OAuth Provider（对外开放 API）

**代码**：`control-plane/src/services/oauth-provider.ts` + `routes/oauth-provider.ts`

**为什么需要**：NAP 要对外开放 API 给第三方应用调用，比如：
- 某个团队自建的应用想读 NAP 的工作区数据
- 某个 CI/CD 脚本想通过一个长期凭证调 NAP API
- 某个第三方 Agent 平台想接入 NAP

第三方应用**不应该拿到用户的密码**，也不能用 Session Token（那是浏览器 cookie，跨域用不了）。标准做法是 NAP 实现 RFC 6749 OAuth 2.0 授权码流程，发 access token 给第三方。

**支持的 Grant Types**：
- `authorization_code`：标准授权码流程（含 PKCE，强制 S256）
- `refresh_token`：刷新 access token
- `token_exchange`（RFC 8693）：可信后端服务交换 per-user token

**端点**：
- `POST /api/oauth/token` — 换 token（无需登录认证）
- `GET /api/oauth/userinfo` — 查用户信息（需 Bearer access token）
- `GET/POST /api/oauth/authorize` — 授权页

**授权码 + PKCE 流程**：

```
第三方应用           用户浏览器              NAP (CP)            第三方应用后端
   |                    |                      |                      |
   | 1. 跳转到          |                      |                      |
   | /api/oauth/authorize?client_id=xxx        |                      |
   |  &redirect_uri=...&code_challenge=yyy    |                      |
   |------------------->|                      |                      |
   |                    | 2. 用户已登录?        |                      |
   |                    |-------> GET /authorize 返回 client_name      |
   |                    |                      |                      |
   |                    | 3. 用户点"同意授权"    |                      |
   |                    |-------> POST /authorize 生成 10 分钟过期的    |
   |                    |         authorization code                   |
   |                    |<------- 重定向 redirect_uri?code=abc        |
   |                    |                      |                      |
   |                    |                      | 4. 后端拿 code 换 token|
   |                    |                      |<------- POST /api/oauth/token
   |                    |                      |         client_id+secret+code
   |                    |                      |         +code_verifier (PKCE)
   |                    |                      |-------- access_token (JWT 1h)  |
   |                    |                      |         + refresh_token (30d) |
   |                    |                      |                      |
   |                    |                      | 5. 后续调 API         |
   |                    |                      |<------- Authorization: Bearer <jwt>
```

**关键设计点**（`oauth-provider.ts:9-11`）：

```typescript
const ACCESS_TOKEN_EXPIRES_IN = 60 * 60           // access token 1 小时
const REFRESH_TOKEN_EXPIRES_DAYS = 30              // refresh token 30 天
const AUTH_CODE_EXPIRES_MIN = 10                   // 授权码 10 分钟
```

- **Access Token 是 JWT**，用 `@hono/jwt` 的 `sign` 生成，CP 本地验签，不需要查库 → 高并发友好
- **Refresh Token 是随机 hex**，SHA-256 哈希存库（`hashToken` 函数），换 access token 时查库校验
- **PKCE 强制 S256**（`oauth-provider.ts` 里 `code_challenge_method !== 'S256'` 直接 400），防止授权码被中间人截获后重放
- **Client Secret 也是 SHA-256 哈希存库**（`verifyClientSecret` 用常数时间比较，防侧信道）

**和 Session Token 的关系**：用**相同的 JWT_SECRET** 签名，所以 CP 的认证中间件在 Bearer 路径下，会先尝试 Service Token，失败再尝试 JWT verify（此时 OAuth access token 也能通过）。

#### 角色 B：NAP 作为 OAuth Client（对接外部 MCP Server）

**代码**：`control-plane/src/services/mcp-oauth.ts` + `routes/mcp-oauth.ts`

**为什么需要**：NAP 的 Agent 要连接**外部 MCP Server**（比如 GitHub MCP、Notion MCP），而这些 MCP Server 自己用 OAuth 2.1 保护。NAP 不能直接用用户的 NAP 凭证去访问它们，必须：

1. **Discovery**：`POST /api/mcp-oauth/discover` 检测外部 MCP 是否需要 OAuth
2. **走外部 OAuth 流程**：NAP 作为 OAuth Client 跳到外部服务的授权页，用户同意后拿回调
3. **存储外部 token**：`mcp-oauth.ts` 把外部 MCP 的 access_token / refresh_token 存到 NAP 数据库，key 是 `user_id + server_origin`
4. **Agent 调用时自动注入**：Agent 通过 NAP 调外部 MCP 时，NAP 把存储的外部 token 注入到 Authorization Header

`routes/mcp-oauth.ts` 中 `/status` 端点就是查询某用户已经连接了哪些外部 MCP：

```typescript
mcpOAuthRoutes.get('/status', async (c) => {
  const userId = c.get('user').sub
  const origins = serversParam.split(',').filter(Boolean)
  const tokens = await mcpOAuth.listTokensForUser(userId)
  // 返回每个 MCP server 的 connected/expires_at 状态
})
```

#### 两个角色的本质区别

| 维度 | 角色 A（Provider） | 角色 B（Client） |
|------|-------------------|-----------------|
| 谁签发 token | NAP 自己签 | 外部 MCP Server 签 |
| 谁校验 token | NAP 本地验签 | 外部 MCP Server 校验 |
| 谁存储 token | 第三方应用自己管 | NAP 数据库存 |
| 解决什么问题 | 让外部世界访问 NAP | 让 NAP 访问外部世界 |

#### 顶层视角

> NAP 提供 OAuth Token 的底层逻辑是"让外部世界安全地访问 NAP，同时让 NAP 安全地对接外部服务"，闭环两件事：作为 Provider 发 JWT 给第三方应用代表用户调 NAP API；作为 Client 替用户对接外部 OAuth 保护的 MCP Server，把外部 token 托管在 NAP，Agent 调用时自动注入。

### 三种 Token 对比

| 维度 | Session Token | Service Token | OAuth Access Token |
|------|---------------|---------------|-------------------|
| 载体 | cookie (`token`) | Bearer header | Bearer header |
| 格式 | HS256 JWT | `tos_` + hex | HS256 JWT |
| 过期 | 24h（滑动续期） | 永久（主动吊销） | 1h（refresh 30d） |
| 存储位置 | 客户端 cookie | DB 存 hash | 客户端自管 |
| 使用场景 | 浏览器交互 | API 集成/CI | 第三方 OAuth 应用 / 对接外部 MCP |
| 用户标识 | `sub: userId` | `sub: created_by, username: svc:<name>` | `sub: userId` |

> Session Token 解决的是"用户在 Web 上是谁"；Service Token 解决的是"NAP 内部服务之间怎么互信"；OAuth Token 解决的是"外部世界如何代表用户 / 对接外部服务"——三者各管一段，拉通对齐后才形成完整的认证体系。

---

## 4. 各服务认证实现

### 4.1 Control Plane（CP）— 认证中枢

**入口**：`control-plane/src/index.ts:83` 创建 `OpenAPIHono<AppEnv>`

**认证中间件**：`control-plane/src/index.ts:145-269`，全局注册 `app.use('/*', ...)`

**中间件执行顺序**（注册顺序 = 执行顺序）：

1. `compress()` — gzip 压缩，跳过 SSE/流式（`:92`）
2. Prometheus 指标中间件 — 记录请求数和耗时（`:96-109`）
3. 日志中间件 — 跳过 health check（`:118-131`）
4. Host-based 分发 — `files.*` 域名走无认证的文件导出（`:136-142`）
5. **认证中间件**（`:145-269`）
6. 全局错误处理（`:272-275`）

**认证中间件内部逻辑**：

```
请求进入
  │
  ▼
Step 1: 白名单检查 (:149-174)
  ├─ /login, /api/auth/login, /api/auth/wecom/*
  ├─ /api/shares/public/*, /s/* (公开分享)
  ├─ /api/docs*, /static/*, /assets/*
  ├─ /api/oauth/token, /api/oauth/userinfo
  └─ /mcp
  命中白名单 → 跳过认证，直接 next()
  │
  ▼
Step 2: Plugin Admin Token (:183-200)
  ├─ 检查 x-plugin-admin-token header
  ├─ 匹配 PLUGIN_ADMIN_TOKEN 环境变量
  └─ 匹配 → user.role = 'admin'，next()
  │
  ▼
Step 3: Bearer Token (:203-228)  [仅 /api/* 和 /_proxy/*]
  ├─ 先试 Service Token: SHA-256 hash → 查 service_tokens 表
  │   └─ 命中 → user = { sub: created_by, username: 'svc:<name>', role: 'user' }
  └─ 再试 OAuth JWT: hono/jwt verify
      └─ 验证通过 → user = JWT payload
  │
  ▼
Step 4: Cookie JWT (:231-268)
  ├─ 读 cookie 'token'
  ├─ 无 cookie → API 返回 401 JSON / 浏览器重定向 /login?next=...
  ├─ JWT verify 失败 → 删 cookie，同上
  └─ 滑动续期: 剩余 < 12h → 签发新 token 写回 cookie (:256-265)
```

**关键点**：Bearer 路径和 Cookie 路径是**互斥的**。有 Authorization header 就走 Bearer，没有才看 cookie。这保证了 API 调用不会被 cookie 干扰。

### 4.2 Web 前端 — 权限路由

**框架**：React 18 + react-router-dom v6 + React Context

**Token 存储**：**纯 cookie**，前端代码完全不接触 token 字符串。

```typescript
// web/src/lib/api/client.ts:114-121
const response = await fetch(url, {
  ...options,
  credentials: 'include',  // 浏览器自动带 cookie，前端无需手动处理
})
```

**路由守卫**：`web/src/App.tsx:29-44` `ProtectedRoute` 组件

```typescript
function ProtectedRoute({ children }) {
  const { user, isLoading } = useAuth()
  if (isLoading) return <Fallback />
  if (!user) {
    const next = location.pathname + location.search
    return <Navigate to={`/login?next=${encodeURIComponent(next)}`} replace />
  }
  return <>{children}</>
}
```

**受保护路由**：`/`、`/w/:workspaceId`、`/oauth/authorize`、`/invite/:token`
**公开路由**：`/login`、`/s/:shareId`（公开分享页）

**401 兜底**：`web/src/lib/api/client.ts:124-131`，API 返回 401 且不在 `/login` 页面时，硬跳转 `/login`。

**认证状态管理**：`web/src/contexts/AuthContext.tsx`

```
应用启动:
  AuthProvider 挂载 → GET /api/auth/me
    ├─ 成功 → setUser(user)
    └─ 失败 → setUser(null)
  setIsLoading(false)

登录:
  POST /api/auth/login (credentials: 'include')
    └─ 服务端 Set-Cookie: token=JWT
  setUser(user) → navigate(next)

登出:
  POST /api/auth/logout → 删 cookie
  setUser(null) → 自动跳 /login
```

### 4.3 Browser Service — 双模认证

**为什么需要双模**：Browser Service 既给浏览器用户用（Web 控制台看 live view），又给 Agent/CI 用（通过 MCP 工具调 API）。前者用 cookie 方便，后者用 Bearer 方便。

**认证中间件**：`browser-service/src/index.ts:52-97`

判定顺序（注意：前一个失败**不会** fallthrough 到后一个）：

```
/api/* 请求进入
  │
  ├─ 白名单? (/login, /callback, /logout, /docs)
  │   └─ 是 → 跳过
  │
  ▼
1. Bearer header? (:66-74)
   ├─ verifyServiceToken(bearer) → 调 CP /api/auth/me 验证
   ├─ 成功 → next()
   └─ 失败 → 直接 401（不 fallthrough）
   │
   ▼
2. ?token= query param? (:77-86)
   ├─ verifyServiceToken(queryToken)
   ├─ 成功 → next()
   └─ 失败 → 401
   │
   ▼
3. Cookie 'browser_token'? (:89-96)
   ├─ verifySessionToken(cookie) → 本地 JWT verify (JWT_SECRET)
   ├─ 成功 → next()
   └─ 失败 → 401
```

**verifyServiceToken**（`browser-service/src/lib/token.ts:15-50`）：调 CP 的 `/api/auth/me`，带 60 秒缓存。
**verifySessionToken**（`browser-service/src/lib/session.ts:1-16`）：本地 `hono/jwt` verify，不调 CP。

**WebSocket Upgrade 认证**（`browser-service/src/index.ts:189-220`）：HTTP upgrade 请求不走 Hono 中间件，单独处理。三种策略：

| 路径模式 | 策略 | 认证方式 |
|----------|------|----------|
| `/live/t/:token/:id/*` | live-token | 从 URL 路径取 token，verifyServiceToken |
| `/cdp/:id/devtools/*` | cdp | Bearer > ?token= > Cookie |
| `/live/:id/*` | live | Bearer(先session再service) > ?token= > Cookie |

**token 嵌入 URL 路径的设计**（`browser-service/src/routes/live.ts:6-7`）：neko 前端加载时会请求多个子资源（JS/CSS/WS 信令），token 放路径段后所有子请求自动继承，一次性解决认证。

**所有权隔离**（`browser-service/src/services/pool.ts:98-104`）：

```typescript
export function isOwnedBy(sbx, userId): boolean {
  if (claims.get(sbx.id)?.userId === userId) return true  // 暖池实例查 claim map
  return sbx.metadata?.['browser.user_id'] === userId      // 按需实例查 metadata
}
```

所有权不匹配返回 **404 而非 403**（避免泄露"这个浏览器存在但不属于你"的信息）。

### 4.4 Channel Gateway — 多渠道认证

**管理 API**（`/api/*`）：cookie JWT，和 CP 一样用 `hono/jwt` 验证（`channel-gateway/src/index.ts:49-62`）。

**内部 API**（`/internal/*`）：**无认证**，service-to-service 调用。

**Webhook**（`/webhook/*`）：在处理器内部验证 secret（`channel-gateway/src/connectors/webhook.ts:138-164`）：

| 模式 | header | 验证方式 |
|------|--------|----------|
| plain | `X-Webhook-Secret`（可自定义） | 直接字符串比较 |
| hmac-sha256 | `X-Hub-Signature-256`（可自定义） | HMAC-SHA256 + timingSafeEqual |

**Slack**：不用 HTTP 签名验证，用 **Socket Mode**（WebSocket 长连接）。认证 = `app_token` + `bot_token` 的机密性（`channel-gateway/src/connectors/slack.ts:98-103`）。

**WeCom**：同样用 WebSocket 长连接（`wss://openws.work.weixin.qq.com`），通过 `aibot_subscribe` 帧携带 `bot_id` + `secret` 认证（`channel-gateway/src/connectors/wecom.ts:122-134`）。

**调 CP 的方式**（`channel-gateway/src/services/db.ts:170-176`）：

```typescript
// 查 route 或 connector 所有者的 platform token
const token = await getPlatformToken(userId)  // 查 service_tokens 表 is_platform=true
const client = new TosClient({ baseUrl: NAP_URL, serviceToken: token })
await client.jobs.create(workspaceId, { ... })  // 自动带 Authorization: Bearer tos_xxx
```

---

## 5. 权限模型

NAP 不用传统的 RBAC（角色-权限矩阵），而是**资源级 visibility + team grants** 模型。四种资源共用同一套模式。

### 5.1 统一权限模型

```
                    资源 (Prompt / Skill / Template / Provider)
                    │
        ┌───────────┼───────────┐
        ▼           ▼           ▼
    visibility   grants     my_permission
        │           │           │
        │           │           ▼
        │           │     推导规则:
        │           │     1. isOwner → 'owner'
        │           │     2. team grant + editor → 'editor'
        │           │     3. team grant + viewer → 'viewer'
        │           │     4. visibility=public 可见 → 'public'
        │           │
        ▼           ▼
  private/team/  team_id +
  public         permission
                 (viewer/editor)
```

**visibility 三档**：

| 值 | 谁能看到 |
|----|----------|
| `private` | 仅 owner |
| `team` | owner + 通过 team grant 共享的团队成员 |
| `public` | 所有人 |

**my_permission 四档**（前端根据这个决定按钮显示/隐藏）：

| 值 | 能做什么 |
|----|----------|
| `owner` | 全部操作：编辑、删除、分享、改 visibility |
| `editor` | 编辑内容，不能删除/分享/改 visibility |
| `viewer` | 只读 |
| `public` | 只读（通过 public visibility 看到） |

**特例**：Provider 的 team grant 只允许 `viewer`（`control-plane/src/routes/providers.ts:62-73`），因为编辑 provider 会暴露上游凭证。

### 5.2 权限判定的核心代码位置

| 资源 | 可见性列表查询 | 单条权限查询 | Grants 管理 |
|------|---------------|-------------|------------|
| Prompt | `services/db/prompts.ts:listVisibleToUser` (:71) | `prompts.ts:getPromptForUser` (:112) | `prompts.ts:setPromptGrants` (:309) |
| Skill | `services/skills-composition.ts` → `skillsService.list` | `skillsService.getSkill` | `routes/skills.ts` → `skillsService.setGrants` |
| Template | `services/db/templates.ts:listVisibleToUser` (:48) | `templates.ts:getTemplateForUser` (:99) | `templates.ts:setTemplateGrants` |
| Provider | `services/db/model-providers.ts:listVisibleToUser` (:35) | `model-providers.ts:getProviderForUser` (:81) | `model-providers.ts:setProviderGrants` |
| Workspace | `services/db/workspaces.ts:listCallableWorkspaces` | `routes/workspaces/_shared.ts:canManage` (:8) | 无 grants，仅 owner |

**可见性查询的 SQL 模式**（以 prompt 为例，`prompts.ts:71-110`）：用 CTE 取三类可见资源的并集——

```sql
WITH visible AS (
  SELECT * FROM prompts WHERE user_id = $1           -- 自己的
  UNION
  SELECT * FROM prompts WHERE visibility = 'public'   -- 公开的
  UNION
  SELECT p.* FROM prompts p
  JOIN prompt_grants g ON g.prompt_id = p.id          -- team grant 共享的
  JOIN team_members m ON m.team_id = g.team_id
  WHERE m.user_id = $1
)
SELECT * FROM visible;
```

### 5.3 Template 链接可见性检查

模板可以引用 prompt/provider/skill。如果模板是 `public` 但引用的资源是 `private`，会导致通过模板间接暴露私有资源。

**检查位置**：`control-plane/src/services/db/template-acl.ts:52-82` `assertTemplateLinkVisible`

**规则**：模板的每个 grant team（或 public），被引用的资源必须对该受众可见——要么 public，要么对该 team 有 grant。

### 5.4 Workspace 权限

Workspace 的 visibility 略不同：`private | user | public`

| 值 | 含义 |
|----|------|
| `private` | 仅 owner，不能被其他 agent 调用 |
| `user` | owner 的其他 agent 可通过 `@agent/<slug>` 调用 |
| `public` | 任何人可通过 `@agent/<owner>/<slug>` 调用 |

**管理权限**（`routes/workspaces/_shared.ts:8-10`）：

```typescript
export function canManage(workspace, user): boolean {
  return workspace.user_id === user.sub || (workspace.is_system && user.role === 'admin')
}
```

### 5.5 前端如何根据权限展示不同 UI

前端**不做权限计算**，只消费后端返回的 `my_permission` / `is_own` / `is_owner` 字段。

**ScopeBadge 组件**（`web/src/components/resource/ScopeBadge.tsx`）：用图标可视化 visibility
- private → 🔒 Lock（灰色）
- team → 👥 Users（蓝色）
- public → 🌐 Globe（绿色）

**按钮显示规则**（以 Skills 为例，`web/src/components/library/SkillsSection.tsx`）：

| 操作 | 显示条件 | 代码位置 |
|------|----------|----------|
| 编辑/分享/删除入口 | `s.is_own \|\| s.my_permission === 'editor'` | `:649` |
| 分享按钮 | `s.is_own`（仅 owner） | `:654` |
| 删除按钮 | `s.is_own`（仅 owner） | `:676` |

**Admin 专属 UI**：通过 `user.role === 'admin'` 判断，不走路由级 RBAC。例如创建工作区时的"系统工作区"复选框（`web/src/components/dialogs/CreateWorkspaceDialog.tsx:217`）只有 admin 能看到。

---

## 6. 典型业务执行时间线

### 6.1 用户登录全流程

```
用户            Web 前端          CP 认证中间件        auth.ts          数据库
 │                │                  │                  │                │
 │ 1.输入用户名密码│                  │                  │                │
 ├───────────────>│                  │                  │                │
 │                │ 2.POST /api/auth/login              │                │
 │                │  (credentials:include)              │                │
 │                ├─────────────────>│                  │                │
 │                │                  │ 3.白名单? 是(/api/auth/login)      │
 │                │                  │   跳过认证                        │
 │                │                  ├─────────────────>│                │
 │                │                  │                  │ 4.查用户       │
 │                │                  │                  ├───────────────>│
 │                │                  │                  │  SELECT * FROM users
 │                │                  │                  │  WHERE username=$1
 │                │                  │                  │<───────────────┤
 │                │                  │                  │                │
 │                │                  │                  │ 5.验证密码      │
 │                │                  │                  │   argon2.verify│
 │                │                  │                  │   (hash, input)│
 │                │                  │                  │                │
 │                │                  │                  │ 6.generateToken│
 │                │                  │                  │   sign({sub,   │
 │                │                  │                  │    username,   │
 │                │                  │                  │    role, exp}, │
 │                │                  │                  │   JWT_SECRET)  │
 │                │                  │                  │                │
 │                │                  │                  │ 7.ensurePlatformToken
 │                │                  │                  │   (自动创建    │
 │                │                  │                  │    __platform__ │
 │                │                  │                  │    service token)│
 │                │                  │                  ├───────────────>│
 │                │                  │                  │  INSERT INTO   │
 │                │                  │                  │  service_tokens│
 │                │                  │                  │<───────────────┤
 │                │                  │                  │                │
 │                │                  │ 8.Set-Cookie: token=JWT            │
 │                │                  │   HttpOnly, Max-Age=86400          │
 │                │                  │<─────────────────┤                │
 │                │ 9.返回 User JSON │                  │                │
 │                │<─────────────────┤                  │                │
 │                │                  │                  │                │
 │                │ 10.setUser(user) │                  │                │
 │                │  navigate(next)  │                  │                │
 │ 11.跳转到首页  │                  │                  │                │
 │<───────────────┤                  │                  │                │
```

### 6.2 已登录用户访问受保护 API（如列出工作区）

```
用户浏览器        CP 认证中间件        workspaces 路由      数据库
     │                │                   │                 │
     │ GET /api/workspaces                 │                 │
     │ Cookie: token=eyJhb...              │                 │
     ├───────────────>│                    │                 │
     │                │                    │                 │
     │                │ Step 1: 白名单? 否  │                 │
     │                │ Step 2: Plugin Admin? 否              │
     │                │ Step 3: Bearer? 无 Authorization header│
     │                │ Step 4: Cookie JWT                    │
     │                │   verify(cookie, JWT_SECRET)          │
     │                │   → payload = { sub: userId, ... }    │
     │                │   滑动续期检查:                        │
     │                │     剩余 < 12h?                        │
     │                │     ├─ 是 → 重新签 token, Set-Cookie  │
     │                │     └─ 否 → 跳过                      │
     │                │   c.set('user', payload)              │
     │                ├───────────────────>│                 │
     │                │                    │ listWorkspaces   │
     │                │                    ├────────────────>│
     │                │                    │ SELECT * FROM   │
     │                │                    │ workspaces      │
     │                │                    │ WHERE user_id=$1│
     │                │                    │  OR visibility  │
     │                │                    │  IN ('user','public')│
     │                │                    │<────────────────┤
     │                │ 200 OK             │                 │
     │                │ { items: [...] }   │                 │
     │                │<───────────────────┤                 │
     │ JSON 响应      │                    │                 │
     │<───────────────┤                    │                 │
```

### 6.3 Agent 通过 Service Token 调 CP API

```
Agent (容器内)     CP 认证中间件        目标路由           数据库
     │                │                   │                 │
     │ POST /api/sessions                │                 │
     │ Authorization: Bearer tos_xxx     │                 │
     ├───────────────>│                   │                 │
     │                │                   │                 │
     │                │ Step 3: Bearer 路径                │
     │                │   1. 先试 Service Token:            │
     │                │      hash = sha256('tos_xxx')       │
     │                │      SELECT * FROM service_tokens   │
     │                │      WHERE token_hash=$1            │
     │                │        AND revoked_at IS NULL       │
     │                │      → 命中!                         │
     │                │      user = {                       │
     │                │        sub: created_by,             │
     │                │        username: 'svc:__platform__', │
     │                │        role: 'user'                 │
     │                │      }                              │
     │                │   2. (不试 JWT，service token 已命中)│
     │                │   c.set('user', user)               │
     │                ├──────────────────>│                 │
     │                │                   │ 创建 session    │
     │                │                   ├────────────────>│
     │                │                   │ INSERT INTO     │
     │                │                   │ sessions ...    │
     │                │                   │<────────────────┤
     │                │ 201 Created       │                 │
     │                │ { id, status, ... }│                │
     │                │<──────────────────┤                 │
     │ JSON 响应      │                   │                 │
     │<───────────────┤                   │                 │
```

### 6.4 Slack 消息触发 Agent 执行（跨服务认证链）

```
Slack 服务器      Channel Gateway        CP              Agent 容器
     │                │                   │                 │
     │ 1.Socket Mode 推送 app_mention     │                 │
     │   (WebSocket 长连接)               │                 │
     ├───────────────>│                   │                 │
     │                │                   │                 │
     │                │ 2.认证: Socket Mode 已建立          │
     │                │   (app_token + bot_token)           │
     │                │   无需 HTTP 签名验证                 │
     │                │                   │                 │
     │                │ 3.查 route 配置    │                 │
     │                │   找到 route.user_id                 │
     │                │                   │                 │
     │                │ 4.getPlatformToken(route.user_id)   │
     │                │   → tos_xxx (route owner 的平台 token)│
     │                │                   │                 │
     │                │ 5.TosClient 调 CP  │                 │
     │                │   POST /api/jobs   │                 │
     │                │   Authorization: Bearer tos_xxx     │
     │                ├──────────────────>│                 │
     │                │                   │ 6.CP 认证中间件  │
     │                │                   │   Bearer → Service Token
     │                │                   │   hash 查表命中  │
     │                │                   │   user.sub = route.user_id│
     │                │                   │                 │
     │                │                   │ 7.创建 job       │
     │                │                   │   → 触发 Agent   │
     │                │                   ├────────────────>│
     │                │                   │                 │ 8.Agent 执行
     │                │                   │                 │   (用的是 route owner 的
     │                │                   │                 │    workspace)
     │                │                   │                 │
     │                │                   │ 9.SSE 推送结果   │
     │                │                   │<────────────────┤
     │                │ 10.结果回传 Slack  │                 │
     │                │   WebClient.chat.postMessage        │
     │                │   (bot_token)      │                 │
     │ 11.消息出现在 Slack 频道            │                 │
     │<───────────────┤                   │                 │
```

**关键点**：Slack 消息用的是 **route owner** 的 platform token，不是 connector owner 的。因为 job 创建在 `route.workspace_id` 下，必须用 workspace owner 的身份才能通过 CP 权限检查。

### 6.5 用户观看 Agent 浏览器操作（Browser Service 双模认证）

```
Agent            CP/MCP Tools        Browser Service       沙箱容器        用户浏览器
 │                │                   │                     │              │
 │ 1.create_browser MCP               │                     │              │
 │  (Bearer tos_xxx)                  │                     │              │
 ├───────────────>│                   │                     │              │
 │                │ 2.POST /api/browsers                     │              │
 │                │ Authorization: Bearer tos_xxx           │              │
 │                ├──────────────────>│                     │              │
 │                │                   │ 3.认证: Bearer 路径  │              │
 │                │                   │   verifyServiceToken│              │
 │                │                   │   → 调 CP /api/auth/me             │
 │                │                   │   → user.sub = workspace owner     │
 │                │                   │                     │              │
 │                │                   │ 4.创建浏览器实例     │              │
 │                │                   ├────────────────────>│              │
 │                │                   │ 5.返回 live_view_url│              │
 │                │                   │   /live/t/{token}/{id}/            │
 │                │<──────────────────┤                     │              │
 │                │                   │                     │              │
 │ 6.Agent 通过 SSE 把 live_view_url 回传 Web UI             │              │
 │                │                   │                     │              │
 │                │                   │                     │   7.用户点击链接              │
 │                │                   │                     │   GET /live/t/{token}/{id}/  │
 │                │                   │                     │<─────────────┤
 │                │                   │ 8.live 路由认证:    │              │
 │                │                   │   token 从 URL 路径取│              │
 │                │                   │   verifyServiceToken│              │
 │                │                   │   → user            │              │
 │                │                   │   所有权检查:        │              │
 │                │                   │   pool.isOwnedBy()  │              │
 │                │                   │   httpProxy → 8080  │              │
 │                │                   ├────────────────────>│              │
 │                │                   │                     │ 9.neko 前端 HTML│
 │                │                   │                     │<─────────────┤
 │                │                   │                     │              │
 │                │                   │                     │ 10.WebRTC 建链 │
 │                │                   │                     │   WS upgrade: │
 │                │                   │                     │   /live/t/{token}/{id}/ws    │
 │                │                   │                     │<══════════════│
 │                │                   │                     │              │
 │                │                   │                     │ 11.实时画面流 │
 │                │                   │                     │   neko 采集 → WebRTC → 浏览器  │
 │                │                   │                     │═════════════>│
 │                │                   │                     │   用户看到 Agent 操作 │
```

---

## 7. 关键文件索引

### 认证核心

| 文件 | 职责 |
|------|------|
| `control-plane/src/index.ts:145-269` | CP 认证中间件（白名单/Bearer/Cookie） |
| `control-plane/src/services/auth.ts` | JWT 签发/验证/续期，密码/LDAP 认证 |
| `control-plane/src/lib/service-token.ts` | Service Token 生成 + SHA-256 哈希 |
| `control-plane/src/lib/session-token.ts` | Session Token（会话级代理 token） |
| `control-plane/src/routes/auth.ts` | 登录/登出/改密码路由 |
| `control-plane/src/routes/service-tokens.ts` | Service Token CRUD API |
| `control-plane/src/routes/oauth-provider.ts` | OAuth Provider 端点 |
| `control-plane/src/services/oauth-provider.ts` | OAuth Provider 实现 |
| `control-plane/src/routes/wecom-auth.ts` | 企业微信 OAuth 登录 |
| `internal/oauth-client/src/index.ts` | OAuth 客户端库（给其他服务用） |

### 权限模型

| 文件 | 职责 |
|------|------|
| `control-plane/src/services/db/prompts.ts` | Prompt 可见性查询 + grants + my_permission 推导 |
| `control-plane/src/services/db/templates.ts` | Template 可见性查询 + grants |
| `control-plane/src/services/db/template-acl.ts` | Template 链接可见性检查 |
| `control-plane/src/services/db/model-providers.ts` | Provider 可见性查询 + grants |
| `control-plane/src/routes/skills.ts` | Skill 路由 + grants API |
| `control-plane/src/routes/workspaces/_shared.ts:8` | Workspace canManage 函数 |
| `control-plane/src/services/db/teams.ts` | Team 管理（admin/member） |
| `control-plane/src/services/db/shares.ts:130-151` | ensurePlatformToken |

### 前端

| 文件 | 职责 |
|------|------|
| `web/src/App.tsx:29-44` | ProtectedRoute 路由守卫 |
| `web/src/contexts/AuthContext.tsx` | 认证状态管理 |
| `web/src/lib/api/client.ts:114-131` | API 客户端（credentials:include + 401 兜底） |
| `web/src/pages/LoginPage.tsx` | 登录页 |
| `web/src/components/resource/ScopeBadge.tsx` | visibility 图标 |
| `web/src/components/library/SkillsSection.tsx:649-676` | Skill 按钮权限门控 |
| `web/src/components/library/PromptsSection.tsx:122-534` | Prompt 按钮权限门控 |
| `web/src/components/library/TemplatesSection.tsx:316-744` | Template 按钮权限门控 |
| `web/src/components/management/ProvidersSection.tsx:215` | Provider 按钮权限门控 |

### Browser Service

| 文件 | 职责 |
|------|------|
| `browser-service/src/index.ts:52-97` | 双模认证中间件 |
| `browser-service/src/index.ts:189-220` | WebSocket upgrade 认证 |
| `browser-service/src/lib/token.ts:15-50` | verifyServiceToken（调 CP） |
| `browser-service/src/lib/session.ts:1-16` | verifySessionToken（本地 JWT） |
| `browser-service/src/routes/auth.ts` | OAuth redirect 路由 |
| `browser-service/src/services/pool.ts:98-104` | 所有权判定 isOwnedBy |

### Channel Gateway

| 文件 | 职责 |
|------|------|
| `channel-gateway/src/index.ts:49-62` | 管理 API cookie JWT 认证 |
| `channel-gateway/src/lib/auth.ts` | JWT 验证 |
| `channel-gateway/src/connectors/webhook.ts:138-164` | Webhook secret 签名验证 |
| `channel-gateway/src/connectors/slack.ts:98-103` | Slack Socket Mode |
| `channel-gateway/src/connectors/wecom.ts:122-134` | WeCom WS 认证 |
| `channel-gateway/src/services/db.ts:170-176` | getPlatformToken（查 service_tokens 表） |

---

## 8. 常见问题速查

**Q: 为什么我的 API 请求返回 401？**
A: 检查顺序：1) cookie 是否存在（浏览器 F12 → Application → Cookies）；2) token 是否过期（CP 会滑动续期，但如果超过 24h 没活动会过期）；3) 是否带了 Authorization header（Bearer 路径不 fallback 到 cookie）。

**Q: Service Token 和 OAuth Access Token 都是 Bearer，CP 怎么区分？**
A: CP 先试 Service Token（SHA-256 hash 查数据库），命中就用；没命中再用 `hono/jwt` verify。两者格式不同（`tos_xxx` vs `eyJhb...`），实际不会混淆。

**Q: 前端怎么判断该显示哪些按钮？**
A: 前端不计算权限，只看后端返回的 `my_permission` 和 `is_own`/`is_owner` 字段。`owner` 能看到所有按钮，`editor` 能看到编辑按钮，`viewer`/`public` 只能看不能改。

**Q: 为什么 Browser Service 的 live_view_url 要把 token 放进 URL 路径？**
A: neko 前端加载时会请求多个子资源（JS/CSS/WebSocket），token 放 query param 在 WebSocket upgrade 时拿不到；放路径段后所有子请求继承这段路径，一次性解决。

**Q: Slack 消息触发的 Agent 用的是谁的身份？**
A: 用的是 **route owner**（路由配置者）的 platform token，不是 connector owner 的。因为 job 创建在 route 的 workspace 下，必须用 workspace owner 的身份。

**Q: 为什么删除别人的浏览器返回 404 而不是 403？**
A: 防止信息泄露。返回 404 让攻击者无法确认"这个浏览器存在但不属于我"，返回 403 会暴露资源存在性。
