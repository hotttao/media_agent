# media_auth 服务设计文档

> 目标:把 `control_plane` 中所有认证 / 鉴权相关能力抽出,沉淀为独立的 `media_auth` 服务。
> 落地形态:Hertz + Protobuf IDL(遵循 `harness/contributing/idl.md`) + 自治数据库。
>
> 配套:领域模型与实现 spec 见 `docs/ddd/media_auth/`(CONTEXT / 6 domain / 4 workflow / spec);架构决策见 `docs/ddd/adr/`。

---

## 0. 范围

| 域 | 控制面现状 | 迁到 media_auth? |
|---|---|---|
| 本地账号 + LDAP 登录、`/api/auth/me*`、密码修改、登出 | `routes/auth.ts` + `services/auth.ts` + `services/db/users.ts` | 是 |
| 企微 OAuth(enabled / authorize / callback / identity list / unbind) | `routes/wecom-auth.ts` + `services/wecom.ts` + `services/db/identities.ts` | 是 |
| Service Token(create / list / revoke) | `routes/service-tokens.ts` + `lib/service-token.ts` + `services/db/shares.ts` | 是 |
| OAuth Provider(authorize / token / userinfo;含 PKCE、RFC 8693 token-exchange) | `routes/oauth-provider.ts` + `services/oauth-provider.ts` | 是 |
| MCP OAuth(discovery / status / authorize / callback / disconnect) | `routes/mcp-oauth.ts` + `services/mcp-oauth.ts` | 是 |
| 内部中间件鉴权(GET /health、metrics、files.* host 短路、`x-plugin-admin-token` 等) | `index.ts` 顶层 | 否(留在 control_plane) |

---

## 1. 服务目录与 IDL 切分

`media_auth` 是 `<app>` 名。按 IDL.md §1.1 约定,每个 proto 一个 service、一个 go_package 子段,生成产物按 service 隔离。

```
docs/prd/media_auth/
├── design.md                    # 本文档
├── README.md                    # 顶层说明
├── idl/
│   └── media_auth/              # 与 IDL.md <app>/<file>.proto 路径对齐
│       ├── auth.proto           # service: auth        (本地账号 / 会话)
│       ├── identity.proto       # service: identity    (外部身份绑定)
│       ├── wecom.proto          # service: wecom       (企微 OAuth 流程)
│       ├── service_token.proto  # service: service_token (B2B API token)
│       ├── oauth.proto          # service: oauth       (OAuth2 provider)
│       ├── mcp_oauth.proto      # service: mcp_oauth   (MCP 客户端 OAuth)
│       └── token.proto          # service: token       (internal VerifyToken,mTLS)
└── sql/
    └── 0001_init.sql            # media_auth 自管 schema
```

**为什么拆 6 个业务 proto(+1 internal token proto):**
- 一个域 = 一个 service = `biz/handler/<service>/` + `biz/router/<service>/`,IDL.md §1.2 强约定;
- `auth` / `identity` / `wecom` 是用户身份三层(账号 → 外部身份 → 登录通道),后续接 Google / GitHub 只需在 `identity.proto` 加 provider;
- `service_token` 与账号密码解耦,产品上是两类凭证;
- `oauth` 与 `mcp_oauth` 角色相反(AS vs client),不能合并。

---

## 2. IDL 规范遵循(逐条对应 IDL.md)

| 规范 | 落实方式 |
|---|---|
| §1.1 `idl/<app>/<file>.proto` + `go_package = "<app>/<service>"` | 6 业务 proto + `token.proto` `package media_auth.<service>` + `option go_package = "media_auth/<service>"` |
| §2 双重 HTTP 注解 | 业务 RPC 同时挂 `(google.api.http)` 和 `(api.get/post/...)`;路径参数 `api.*` 用 `:id`,`google.api.http` 用 `{id}`。`token.proto` 仅 `(api.post)`(internal,不挂 google.api.http、不出 OpenAPI) |
| §3 字段 snake_case | 全部 message 字段 snake_case;query 字段加 `[(api.query)="..."]` |
| §5 hz 工具链 | 注释里标 `hz model / hz client / hz update / swagger-gen` 触发点 |

`api.proto` 与 `google/api/annotations.proto` 在 `import` 时按 IDL.md §2 的相对路径处理。

---

## 3. 数据库初始化 SQL

`sql/0001_init.sql` 是 media_auth 的 schema 唯一源。从控制面 `migrations/001_init.sql` 抽取与认证相关的 8 张表:

| 表 | 用途 |
|---|---|
| `users` | 账号(本地密码 / LDAP 派生 / 企微派生) |
| `user_identities` | 外部身份绑定(provider, external_id) |
| `service_tokens` | B2B API token(`token_hash` + 可选明文 `token` 字段,仅 platform token 落明文) |
| `oauth_clients` | OAuth2 第三方客户端注册 |
| `oauth_authorization_codes` | 一次性授权码 |
| `oauth_refresh_tokens` | 刷新令牌(SHA-256 哈希) |
| `mcp_oauth_clients` | 第三方 MCP 服务器 OAuth client 注册 |
| `mcp_oauth_tokens` | 当前用户对每个 MCP 服务器的 access / refresh token |

**保留既有约束**(包括唯一索引、外键、CHECK 约束):
- `idx_service_tokens_platform_user` — 每用户最多一个 active platform token
- `users_ldap_username_key` — username 唯一
- `user_identities` 主键 `(provider, external_id)` + 唯一 `(user_id, provider)`
- `oauth_authorization_codes` 主键 `code`、`oauth_refresh_tokens` 主键 `token_hash`

**后续表新增约定**:`media_auth` 自治的 schema 演进单独维护 migration,不再耦合 control_plane 主迁移链。`mcp_oauth_tokens` 的刷新失败宽限对齐控制面 `migrations/117_*.sql`,采用 `refresh_fail_count`(连续失败次数)+ `refresh_fail_first_at`(首次失败时间)两列;宽限算法 = `refresh_fail_count >= 3` 且 `refresh_fail_first_at` 距今 `>= 600s` 才删 token,之前视为暂态;成功 refresh 重置两者。

---

## 4. 与 control_plane 的边界

媒体认证抽走后,控制面作为网关继续承接非认证路由,其自有路由仍需校验 `Authorization: Bearer <jwt|service_token>` 与 `Cookie: token=<jwt>`。校验路径(见 ADR-0001):

1. **JWT(本地验)** — cookie JWT 与 bearer OAuth access JWT 均为 HS256 自包含签名,control_plane 用共享 `JWT_SECRET` 本地验,零 RPC。
2. **Service Token(RPC 验)** — opaque `tos_xxx` 无法本地验,control_plane 经 mTLS 调 `media_auth` internal `VerifyToken` RPC 解析为 Subject,不再直读 media_auth 任何表。本批次 IDL 已暴露 `token.proto`。
3. **4 前缀反代** — `/api/auth` / `/api/oauth` / `/api/service-tokens` / `/api/mcp-oauth` 由 control_plane 网关反向代理到 media_auth,cookie/session 鉴权在 media_auth 自闭环。

> control_plane 中间件解析顺序:plugin-admin 旁路 → Bearer(service_token 经 VerifyToken)→ Bearer(JWT 本地验)→ Cookie(JWT 本地验 + 滑动续期)。
>
> 取消原 sidecar 双读过渡态:control_plane 不直读 `users`/`service_tokens` 表,收敛路径相应提前。

**不在 media_auth IDL 中的接口**(control_plane 自留):
- `GET /api/version` / `GET /metrics` / `GET /health`
- `GET /favicon.svg` 等静态资源
- `x-plugin-admin-token` 旁路(`/api/plugins/admin*`、`/api/admin/cluster/rebuild-stale`)
- `files.*` host 公共导出短路

---

## 5. 错误约定

`media_auth` 内部错误统一用 google.rpc.Status(可选),HTTP 层:
- 400 — 参数错 / invalid_request / invalid_grant
- 401 — Unauthorized / invalid_credentials / invalid_token / invalid_client
- 403 — system 账号禁止登录
- 404 — identity / token 不存在
- 500 — server_error / LDAP / 企微上游异常

**敏感字段脱敏:** Service Token 明文、`password_hash`、`client_secret_hash` 永远不出 service 边界(只在创建响应里返回一次)。

---

## 6. 收敛路径(后续 roadmap)

| 阶段 | 动作 |
|---|---|
| 本批次 | IDL(含 `token.proto`)+ SQL(修正)+ 领域模型 + spec 落盘;control_plane 不动 |
| P1 | media_auth 独立 Go service + Hertz 路由,`go_package = "media_auth/<service>"` 生成 6 service handler/router 骨架 + token service |
| P2 | control_plane 把 `/api/auth/*`、`/api/oauth/*`、`/api/service-tokens`、`/api/mcp-oauth/*` 反代到 media_auth;中间件接 `VerifyToken` RPC;前端适配 `identity` 路径变更(`/api/auth/identities/:provider`) |
| P3 | media_auth 接管上述表,control_plane 删除对应 migration,DB 物理独立或同库不同 schema |
| P4 | (远期)JWT 迁非对称签名,消除 `JWT_SECRET` 跨服务共享(见 ADR-0001 备选) |

---

## 7. HTTP 绑定源约定(Hertz IDL)

每个 RPC 的字段绑定方式由 IDL 字段 tag 决定,hz 工具链据此生成 Go struct tags(handler 端 `c.BindAndValidate` 自动分发):

| 字段 tag | HTTP 源 | 适用场景 | 本设计中的 RPC |
|---|---|---|---|
| `body: "*"` 或 `(api.body) = "key"` | JSON body(`application/json`) | 结构化请求 | Login / ChangePassword / UpdateMe / SetDefaultPrompt / CreateServiceToken / ApproveAuthorization / DiscoverOAuth |
| `(api.form) = "key"` | form body(`application/x-www-form-urlencoded` 或 `multipart/form-data`) | OAuth2 / 表单提交 | **ExchangeToken**(`/api/oauth/token`,RFC 6749 §3.2 强制) |
| `(api.query) = "key"` | URL query string | GET 参数 | GetAuthorizeInfo / GetAuthorizeUrl / HandleCallback / GetConnectionStatus / StartAuthorization / MCP HandleCallback / 各种 list |
| `(api.path) = "key"` | URL 路径参数(对应 `google.api.http` 的 `{key}`) | 资源定位 | DeleteIdentity / Disconnect / RevokeServiceToken |

> **关键边界**:`/api/oauth/token` 是本设计**唯一**走 form-urlencoded 的端点;其余 POST/PATCH/PUT 端点全部走 JSON body。
> 控制面 `/api/oauth/token` 现状兼容 form + JSON,media_auth IDL 只声明 form(JSON fallback 由 handler 层手动解析,IDL 不表达)。

---

## 8. 风险

| 风险 | 缓解 |
|---|---|
| `service_tokens.token` 明文列只为 platform token | init SQL 保留(见 ADR-0002),code review 要求新代码不读该列;远期拆 `platform_tokens` 表 |
| 企微 OAuth callback 在 auth skip list | 新服务继承同样短路;`identity.proto` 不暴露 callback(只在 `wecom.proto` 暴露) |
| MCP OAuth 浏览器 popup flow | callback RPC 返回 HTML 字符串;前端逻辑不变 |
| `JWT_SECRET` 跨服务共享 | media_auth 与 control_plane 共享,轮换需双方同步;远期迁非对称签名(ADR-0001 备选 / P4) |
| OAuth token-exchange 依赖外部 RS `/introspect` | 仅 RFC 8693 grant 用;`resource` 须合法 URL;上游不可达返回 invalid_grant |
| `mcp_oauth_tokens` 明文存第三方 access/refresh token | token 对 media_auth 不透明;DB 访问需受限;断开即删 |
| `identity` 路径泛化为带 provider | 原 `DELETE /api/auth/wecom/identity` 破坏性变更,P2 前端需适配 |
| `default_prompt_name` 不再由 media_auth 返回 | prompts 表留 control_plane,`default_prompt_id` 存 opaque text,name 由网关补 |
| mTLS `VerifyToken` 内网暴露 | 不挂网关路由、不出 OpenAPI;仅 control_plane 经 mTLS 调用 |