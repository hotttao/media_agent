# media_auth 认证鉴权上下文

media 平台统一的认证 / 鉴权微服务，承接原 agent-platform `control-plane` 中所有身份认证、会话、凭证签发与校验能力，对外以 Hertz + Protobuf IDL 暴露，自治数据库。

## 业务术语词典

### 身份与账号

**User（用户）**:
平台账号主体，一条 `users` 记录；由本地密码、LDAP 或企微派生创建。`role ∈ {user, admin, system}`，其中 `system` 仅作系统账号、禁止登录。
_禁用替代词_：Account（账户——指凭证而非人）、Member、Profile

**Auth Source（认证来源）**:
一个 User 的凭证来源，取值 `password | ldap | wecom`；由 `password_hash` 是否非空与身份绑定推导，前端据此判断能否改密。
_禁用替代词_：login type、auth method（method 留给 OAuth grant_type）

**Identity（外部身份绑定）**:
用户在第三方 provider（目前为企微）的身份记录 `(provider, external_id) → user_id`；一个用户对同一 provider 至多一条绑定。
_禁用替代词_：binding、external account、link

**Provider（身份提供方）**:
外部身份来源的标识字符串（如 `wecom`）；`identity` domain 按 provider 路由，新增 Google/GitHub 只需新增 provider 通道。
_禁用替代词_：vendor、channel（channel 语义留给登录通道）

### 会话与凭证

**Session（会话）**:
登录成功后下发的 httpOnly cookie `token`，承载 HS256 签名的 JWT（sub/username/name/email/role/exp）；24h 有效、半程滑动续期。归 `auth` domain。
_禁用替代词_：login state、JWT session（JWT 是载体不是会话本身）

**Service Token（服务令牌）**:
B2B API 调用凭证，`tos_` 前缀 + 32 随机字节；DB 仅存 SHA-256 `token_hash`，明文仅创建时返回一次。归 `service_token` domain。
_禁用替代词_：API key、access key、bearer token（bearer 是传输方式不是凭证类型）

**Platform Token（平台令牌）**:
特殊 Service Token，`is_platform=true`、`name='__platform__'`，登录后自动 ensure，每用户至多一个 active；为供 agent 回调 control_plane，**单独落明文 `token` 列**。归 `service_token` domain。
_禁用替代词_：agent token、internal token

**VerifyToken（令牌校验）**:
media_auth 暴露给 control_plane 的 mTLS 内部 RPC，把 opaque Service Token 解析为 Subject；control_plane 据此完成自有路由鉴权，不再直读 media_auth 库。
_禁用替代词_：introspect（introspect 是 OAuth2 RFC 7662 标准端点，语义不同）

**Subject（鉴权主体）**:
token 校验后得到的鉴权身份快照（sub/username/name/role 等），由 control_plane 中间件注入请求上下文。
_禁用替代词_：principal（保留给 OAuth2 RFC 8693 token-exchange 语义）、claim

### OAuth Provider（media_auth 作为 Authorization Server）

**OAuth Client（OAuth 客户端）**:
在 media_auth 注册的第三方应用，`oauth_clients` 记录；含 `redirect_uris` 白名单与可选 `secret_hash`（公开 client 无 secret）。归 `oauth` domain。
_禁用替代词_：consumer、app（app 留给 media_agent 微服务名）

**Authorization Code（授权码）**:
OAuth2 authorization_code grant 的一次性码，10 分钟 TTL，消费即删；可带 PKCE S256 `code_challenge`。归 `oauth` domain。
_禁用替代词_：auth code、grant code

**Refresh Token（OAuth 刷新令牌）**:
OAuth2 `refresh_token` grant 凭证，30 天 TTL，DB 存 SHA-256 `token_hash`，每次使用轮转（旧删新发）。归 `oauth` domain。
_禁用替代词_：renew token

**Access Token（OAuth 访问令牌）**:
OAuth2 签发的 JWT（HS256，1h TTL），供第三方 RS 访问 `/api/oauth/userinfo`；与 Session 的 cookie JWT 同算法同密钥、用途不同。归 `oauth` domain。
_禁用替代词_：bearer、JWT token

**Token Exchange（令牌交换）**:
RFC 8693 grant，第三方 RS 用 `subject_token` 换取 media_auth Access Token；需校验 `resource` 指向的 RS 的 `/introspect`。归 `oauth` domain。
_禁用替代词_：token swap、impersonation

### MCP OAuth（media_auth 作为 OAuth Client）

**MCP OAuth Client（MCP 客户端注册）**:
media_auth 反向作为 OAuth client 连接第三方 MCP 服务器时的注册记录，`mcp_oauth_clients`，以 `server_origin` 为主键，缓存该 server 的 OAuth metadata。归 `mcp_oauth` domain。
_禁用替代词_：mcp registration、mcp app

**MCP OAuth Token（MCP 连接令牌）**:
某用户对某 MCP server 的 access/refresh token，`mcp_oauth_tokens`，主键 `(user_id, server_origin)`，明文存（对 media_auth 不透明）。归 `mcp_oauth` domain。
_禁用替代词_：mcp credential、mcp session

**Refresh Failure Grace（刷新失败宽限）**:
MCP OAuth Token refresh 持续失败时的保留策略：`refresh_fail_count ≥ 3` 且 `refresh_fail_first_at` 距今 ≥ 600s 才删 token，之前视为暂态返回 null。归 `mcp_oauth` domain。

### 企微登录通道

**WeCom Login Channel（企微登录通道）**:
基于企微 corp OAuth 的登录/绑定流程；`wecom` domain 承载 corp 凭证获取、扫码 URL、callback，编排 `auth`(session) 与 `identity`(绑定)。归 `wecom` domain。
_禁用替代词_：wecom sso、wxwork auth
