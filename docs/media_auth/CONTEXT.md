# media_auth 认证鉴权上下文

media 平台统一的认证 / 鉴权微服务，承担所有身份认证、会话、凭证签发与校验能力，对外以 Hertz + Protobuf IDL 暴露，自治数据库。

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
特殊 Service Token，`is_platform=true`、`name='__platform__'`，登录后自动 ensure，每用户至多一个 active；**单独落明文 `token` 列**（ADR-0002；存留待复核）。归 `service_token` domain。
_禁用替代词_：agent token、internal token

**VerifyToken（令牌校验）**:
media_auth 暴露给下游服务的 mTLS 内部 RPC，把 opaque Service Token 解析为 Subject；下游服务据此完成自有路由鉴权，不直读 media_auth 库。
_禁用替代词_：introspect（introspect 是 OAuth2 RFC 7662 标准端点，语义不同）

**Subject（鉴权主体）**:
token 校验后得到的鉴权身份快照（sub/username/name/role 等），由调用方（下游服务中间件）注入请求上下文。
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

### 业务划分与归属

**Scope（业务范围）**:
标识一个业务线的字符串标签，例如 `ai_clothing` / `ai_guoxue`。
- 为什么需要：同一套服务要支撑多条互不相干的业务线。"AI 服装"的模板不能拿"AI 国学"的素材做合成——一旦允许，业务边界就糊了。scope 就是给每个业务线贴标签，运行期跨 scope 引用一律拒绝。
- **scope 是资源属性，不是 user 属性**：user **不直接持 scope**。user 通过拥有的 [Virtual IP](#virtual-ip虚拟形象) 间接关联 scope——virtual_ip 带 scope，一个 user 可拥有多个不同 scope 的 virtual_ip，从而跨业务线。
- **创建不限 scope**：当前版本不限制 user 能在哪些 scope 下创建 virtual_ip / template / workflow / asset（暂不限制）；scope 仅作为资源属性盖章，用于引用一致性校验。后续若要收口"谁能创建哪个 scope"，再加 user↔scope 授权。
- 规则：所有贴 scope 的实体（virtual_ip、product、template、workflow、asset_kind、asset）只能有 1 个 scope；scope 不可改；scope 字符串必须在 `config.SupportedScopes` 闭集合内（部署期固化，不开放用户/ops 写）。
- 跨 service 共享的术语；具体取值在本仓库 `media_materials/biz/policy/scope` 与 `media_workflow/biz/policy/scope` 同步引用同一份常量。
_禁用替代词_：tenant（保留给多租户隔离）、business（保留给 umbrella 业务名）、area。

**Product（业务产品）**:
一个可被视频带货的产品。`product` 归属于某个 scope，**没有 user 归属**；product 在 `media_materials` 服务落库。
_禁用替代词_：goods（保留给 SKU 层）、SKU、item。

**Virtual IP（虚拟形象）**:
一个 AI 形象（模特 / IP），归属于某个 scope，归属 user；一个 user 可拥有多个 VirtualIp，可见性规则遵循 [资源权限模型 §资源级 ACL](#资源级-acl-所有媒体资源通用)。在 `media_materials` 服务落库。
_禁用替代词_：character、avatar、figure。

### 资源级 ACL（所有媒体资源通用）

适用于所有"需要被引用 / 共享"的可写媒体资源：`Template`、`Workflow`、`VirtualIp`。Asset 因只读元数据，沿用 `visibility ∈ {private, team, public}` + 单值 `team_id`，不开放 grants 表。

**Resource Visibility（资源可见性）**:
资源自身的可见度档位 `private | team | public`：
- `private`：仅 owner（含 creator）可见。
- `team`：owner + 通过 [Resource Grant](#resource-grant资源共享授权) 显式授权的团队成员可见。
- `public`：所有人可见；scope 仍限制业务线归属，但 read 不被拒。
_禁用替代词_：ACL、access level。

**Resource Grant（资源共享授权）**:
`(resource_id, team_id) → permission ∈ {viewer, editor}` 的显式授权；不在 grant 内的团队对 `visibility=team` 资源不可见。owner 始终拥有全部权限，不需 grant。team 实体的 CRUD 与成员关系归 [team](domain/0007-team/team.md) domain。
_禁用替代词_：share、ACL。

**My Permission（资源访问权限）**:
当前 user 对某资源的有效权限，推导：
```
1. user == resource.owner_user_id                → 'owner'
2. user ∈ granted_teams(resource) 中 max(permission) → 'editor' or 'viewer'
3. resource.visibility == 'public'               → 'public' (read-only)
4. else                                            → no access
```
`owner` 含全部操作（编辑、归档、删除、改 visibility / grant、提交执行）；`editor` 含编辑、提交执行、改 visibility / grant 之外的子集操作；`viewer` 仅读 + 提交执行；`public` 仅读。
_禁用替代词_：role、claim。

**Team（团队）**:
一组 user 的命名集合，用于 [Resource Grant](#resource-grant资源共享授权) 范围内的资源共享；team 不是多租户隔离边界，也不是 billing 单位。归 `team` domain。
_禁用替代词_：group、org、tenant。

**Team Membership（团队成员关系）**:
`(team_id, user_id) → role` 的多对多关系记录；role 决定该 user 在该 team 内的管理能力。
_禁用替代词_：membership row、team user。

**Team Member Role（团队成员角色）**:
team 内的角色档位 `admin | member`：admin 可改 team / 增删成员 / 创建与撤销邀请；member 仅可查看与主动退团。归 `team` domain。
_禁用替代词_：permission、level、position。

**Team Invite（团队邀请）**:
由 team admin 签发的可分享链接（`tinv_` 前缀 + 随机串），持有者在有效期内可一键加入 team；链接与 team 绑定，到期或撤销后失效。归 `team` domain。
_禁用替代词_：invitation、share link。

**Last Admin Protection（末位管理员保护）**:
team 必须始终至少保留一名 admin 的不变量——降级 / 移除 / 退团最后一任 admin 一律拒绝（须删 team 或先提拔他人），由 `team` domain 在写前校验。
_禁用替代词_：single admin lock、final admin guard。
