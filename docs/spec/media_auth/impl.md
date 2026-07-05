# media_auth 实现规格 (spec)

> 本文是 media_auth 微服务的**自包含**实现规格：研发只看本文即可完成实现。
> 依据 `harness/contributing`（idl / architecture / server / config）+ DDD + agent-platform `control-plane` 认证实现编写。
> 详细领域语义见领域模型 [`docs/media_auth/`](../../media_auth/)（CONTEXT / 6 domain / 4 workflow）；架构决策见 [ADR-0001](../../adr/0001-media-auth-extraction-and-token-verification.md)、[ADR-0002](../../adr/0002-platform-token-plaintext-column.md)。
> 落地计划见 [plan/media_auth/rollout.md](../../plan/media_auth/rollout.md)。

---

## 1. 概述与边界

media_auth 接管 control-plane 全部认证鉴权能力，7 个业务 service / 7 个 domain（1:1）+ 1 个 internal token service。

| service (proto) | domain | 职责 | 来源 (control-plane) |
|---|---|---|---|
| `auth` | auth | 本地/LDAP 登录、session(JWT cookie)、me、改密、偏好 | routes/auth.ts + services/auth.ts + users |
| `identity` | identity | 外部身份绑定 list/unbind（generic，按 provider） | routes/wecom-auth.ts(identity 段) + user_identities |
| `wecom` | wecom | 企微 corp OAuth 登录/绑定通道 | routes/wecom-auth.ts + services/wecom.ts |
| `service_token` | service_token | tos_xxx 创建/列表/撤销 + platform token | routes/service-tokens.ts + lib/service-token.ts |
| `team` | team | team CRUD + 成员管理 + 邀请签发/预览/接受 | routes/teams.ts + routes/invites.ts + services/db/teams.ts + services/db/team-invites.ts |
| `oauth` | oauth | OAuth2 AS：code/refresh/token-exchange + userinfo | routes/oauth-provider.ts + services/oauth-provider.ts |
| `mcp_oauth` | mcp_oauth | 反向 OAuth client 连 MCP server | routes/mcp-oauth.ts + services/mcp-oauth.ts |
| `token` (internal) | (service_token) | VerifyToken RPC，供 control_plane 验 Bearer service token | 新增（control-plane 顶层中间件迁出）|

**留在 control_plane（不迁）**：HTTP 入口鉴权中间件（验 cookie/bearer JWT + 调 VerifyToken）、plugin-admin 旁路、`/health` `/metrics` `/api/version` 静态资源、`files.*` host 短路。见 ADR-0001。

**鉴权解析顺序（control_plane 中间件）**：plugin-admin 旁路 → Bearer(service_token 经 VerifyToken RPC) → Bearer(JWT 本地 HS256 验) → Cookie(JWT 本地验 + 滑动续期)。

---

## 2. 目录结构（遵循 architecture.md / server.md）

```text
media_agent/                                  # workspace 根 (go.work)
├── idl/
│   └── media_auth/                           # 本服务 IDL（§3 全文）
│       ├── auth.proto / identity.proto / wecom.proto
│       ├── service_token.proto / team.proto / oauth.proto / mcp_oauth.proto
│       └── token.proto                       # internal VerifyToken（mTLS，不挂网关）
├── hertz_gen/                                # 🔴 共享生成根（独立 module，禁改）
│   ├── model/media_auth/<service>/*.pb.go    # API DTO
│   └── media_auth/                           # 下游 client stub（供 control_plane 调 VerifyToken）
├── hertz_infra/                              # 共享治理 + 配置加载（独立 module）
└── media_auth/                               # 🆕 本微服务 module: media_agent/media_auth
    ├── main.go                               # 🟡 启动编排（轻薄）
    ├── router.go / router_gen.go             # 🟡 / 🔴
    ├── conf/{local,dev,prod}.yaml            # §7
    ├── migrations/
    │   └── 0001_init.sql                     # §4 全文（schema 唯一源）
    └── biz/
        ├── handler/<service>/                # 🟡 hz 生成骨架，填充函数体
        ├── router/<service>/                 # 🔴 hz 生成路由
        ├── middleware/                       # 🟢 全局中间件（session 解析注入等）
        ├── domain/<domain>/                  # 🟢 稳定核心：实体/值对象/仓库接口/不变量
        ├── policy/<domain>/                  # 🟢 易变决策
        ├── service/<domain>/                 # 🟢 用例编排（单 domain 单事务）
        ├── workflow/                         # 🟢 跨 domain 编排
        ├── shared/                           # 🟢 clock/id/pagination + client/（外部 client 装配）
        └── dal/
            ├── db.go / transaction.go
            ├── model/                        # 🟢 GORM 行模型
            ├── query/                        # 🔴 GORM Gen
            └── repo/<domain>/                # 🟢 仓库接口实现（唯一 import dal/query）
```

> 共享生成根 `hertz_gen/`、配置层 `hertz_infra/config`、治理层 `hertz_infra/serverhertz` 均为 workspace 级共享 module，通过 go.work 引用；本服务 `biz/` 下不保留 `config/infra/cache` 目录（architecture.md §2 约束）。

### 2.1 domain → 代码映射

| domain | handler | service | workflow | domain (聚合/不变量) | dal/repo | policy |
|---|---|---|---|---|---|---|
| auth | `handler/auth/` | `service/auth/` | — | `domain/auth/`(Session,User 聚合,JWT 签发/验签) | `dal/repo/auth/`(users) | `policy/auth/`(login 双路、改密规则) |
| identity | `handler/identity/` | `service/identity/` | — | `domain/identity/`(Identity 值对象,双唯一不变量) | `dal/repo/identity/` | — |
| wecom | `handler/wecom/` | `service/wecom/` | `workflow/wecom_login_bind.go` | `domain/wecom/`(corp token 缓存,authorize URL) | — | — |
| service_token | `handler/service_token/` | `service/service_token/` | — | `domain/service_token/`(Token 聚合,platform 唯一不变量) | `dal/repo/service_token/` | `policy/service_token/`(撤销权限) |
| team | `handler/team/` | `service/team/` | `workflow/team_invite_accept.go` | `domain/team/`(Team 聚合 + TeamMembership + TeamInvite,末位 admin 保护) | `dal/repo/team/` | `policy/team/`(末位 admin 校验,invite 过期判定) |
| oauth | `handler/oauth/` | `service/oauth/` | `workflow/oauth_authorize_code.go` | `domain/oauth/`(Client/AuthCode/RefreshToken,PKCE,轮转) | `dal/repo/oauth/` | `policy/oauth/`(redirect_uri/secret/PKCE/grant 校验) |
| mcp_oauth | `handler/mcp_oauth/` | `service/mcp_oauth/` | `workflow/mcp_oauth_connect.go` | `domain/mcp_oauth/`(McpClient/McpToken,宽限算法) | `dal/repo/mcp_oauth/` | `policy/mcp_oauth/`(宽限判定) |

### 2.2 同层隔离与跨 domain 编排（architecture.md 强约束）

- `domain/<a>` 不可 import `domain/<b>`；`service/<a>` 不可 import `service/<b>`；`policy` 同层隔离；`handler` 同层隔离。
- 跨 domain 协作的**两条合法路径**：
  - **workflow**：wecom 登录/绑定编排 identity+auth+service_token；oauth code flow 编排 auth(users 读取)+oauth；mcp connect 编排 auth(sub)+mcp_oauth。放 `biz/workflow/`。
  - **service 内读另一 domain 查询**（单事务内、不持对方聚合）：auth 只读 identity 推导 `auth_source`，经 identity 稳定查询接口调用。
- internal `token` service 的 `VerifyToken` 复用 `service_token` domain 的 `getServiceTokenByHash`，不新建 domain。

---

## 3. IDL 契约

IDL 落到 `idl/media_auth/<file>.proto`，`option go_package = "media_auth/<service>"`。每个业务 RPC 同时挂 `google.api.http`（`{id}`）与 `api.*`（`:id`）双重注解；`token.proto` 仅 `api.*`（internal，不挂 google.api.http、不出 OpenAPI）。字段全部 snake_case。

### 3.1 auth.proto

```proto
syntax = "proto3";
package media_auth.auth;
import "api.proto";
import "google/api/annotations.proto";
option go_package = "media_auth/auth";

// 本地账号 + LDAP 双路登录,以及当前用户/会话域。
// 登录成功下发 httpOnly cookie `token`(JWT),后续走 Cookie 鉴权。
service AuthService {
  rpc Login(LoginRequest) returns(LoginResponse) {
    option (google.api.http) = { post: "/api/auth/login" body: "*" };
    option (api.post) = "/api/auth/login";
  }
  rpc Logout(LogoutRequest) returns(LogoutResponse) {
    option (google.api.http) = { post: "/api/auth/logout" };
    option (api.post) = "/api/auth/logout";
  }
  rpc GetMe(GetMeRequest) returns(ApiUser) {
    option (google.api.http) = { get: "/api/auth/me" };
    option (api.get) = "/api/auth/me";
  }
  rpc UpdateMe(UpdateMeRequest) returns(UpdateMeResponse) {
    option (google.api.http) = { patch: "/api/auth/me" body: "*" };
    option (api.patch) = "/api/auth/me";
  }
  rpc ChangePassword(ChangePasswordRequest) returns(ChangePasswordResponse) {
    option (google.api.http) = { put: "/api/auth/me/password" body: "*" };
    option (api.put) = "/api/auth/me/password";
  }
  rpc SetDefaultPrompt(SetDefaultPromptRequest) returns(SetDefaultPromptResponse) {
    option (google.api.http) = { put: "/api/auth/me/default-prompt" body: "*" };
    option (api.put) = "/api/auth/me/default-prompt";
  }
}

message LoginRequest  { string username = 1; string password = 2; }
message LoginResponse { ApiUser user = 1; }
message LogoutRequest {}
message LogoutResponse { bool success = 1; }
message GetMeRequest {}
// UpdateMeRequest 用 optional 区分"未传"与"显式 false"
message UpdateMeRequest  { optional bool auto_evolution = 1; }
message UpdateMeResponse { bool success = 1; }
message ChangePasswordRequest  { string current_password = 1; string new_password = 2; }
message ChangePasswordResponse { bool success = 1; }
message SetDefaultPromptRequest  { optional string prompt_id = 1; }  // null=清除
message SetDefaultPromptResponse { bool success = 1; }

// ApiUser —— /me、登录响应统一用户档案。
// default_prompt_id 为 opaque id(prompts 表留 control_plane),不校验 FK;
// default_prompt_name 不返回,由网关层补。
message ApiUser {
  string id = 1;
  string username = 2;
  string role = 3;              // user | admin | system
  string auth_source = 4;       // password | ldap | wecom
  optional string default_prompt_id = 5;
  bool auto_evolution = 6;
}
```

### 3.2 identity.proto

```proto
syntax = "proto3";
package media_auth.identity;
import "api.proto";
import "google/api/annotations.proto";
option go_package = "media_auth/identity";

// 用户外部身份绑定的 generic CRUD,按 provider 路由。
service IdentityService {
  rpc ListIdentities(ListIdentitiesRequest) returns(ListIdentitiesResponse) {
    option (google.api.http) = { get: "/api/auth/identities" };
    option (api.get) = "/api/auth/identities";
  }
  rpc DeleteIdentity(DeleteIdentityRequest) returns(DeleteIdentityResponse) {
    option (google.api.http) = { delete: "/api/auth/identities/{provider}" };
    option (api.delete) = "/api/auth/identities/:provider";
  }
}
message ListIdentitiesRequest {}
message ListIdentitiesResponse { repeated UserIdentity identities = 1; }
message UserIdentity {
  string provider = 1;
  string external_id = 2;
  optional string display_name = 3;
  string created_at = 4;
}
message DeleteIdentityRequest  { string provider = 1 [(api.path) = "provider"]; }
message DeleteIdentityResponse { bool success = 1; }
```

### 3.3 wecom.proto

```proto
syntax = "proto3";
package media_auth.wecom;
import "api.proto";
import "google/api/annotations.proto";
option go_package = "media_auth/wecom";

// 企微 corp OAuth 流程。callback 在鉴权 skip list,handler 内不依赖 c.Get("user")。
// 成功路径由 handler 直接 c.Redirect(302);响应字段仅供纯 JSON 回调场景。
service WeComService {
  rpc GetEnabled(GetEnabledRequest) returns(GetEnabledResponse) {
    option (google.api.http) = { get: "/api/auth/wecom/enabled" };
    option (api.get) = "/api/auth/wecom/enabled";
  }
  rpc GetAuthorizeUrl(GetAuthorizeUrlRequest) returns(GetAuthorizeUrlResponse) {
    option (google.api.http) = { get: "/api/auth/wecom/authorize" };
    option (api.get) = "/api/auth/wecom/authorize";
  }
  rpc HandleCallback(HandleCallbackRequest) returns(HandleCallbackResponse) {
    option (google.api.http) = { get: "/api/auth/wecom/callback" };
    option (api.get) = "/api/auth/wecom/callback";
  }
}
message GetEnabledRequest {}
message GetEnabledResponse { bool enabled = 1; }
message GetAuthorizeUrlRequest  { string mode = 1 [(api.query) = "mode"]; }  // login|bind,默认 login
message GetAuthorizeUrlResponse { string url = 1; }
message HandleCallbackRequest {
  string code  = 1 [(api.query) = "code"];
  string state = 2 [(api.query) = "state"];   // login|bind
}
message HandleCallbackResponse { string redirect_to = 1; }   // 成功 /;失败 /login?error=xxx
```

### 3.4 service_token.proto

```proto
syntax = "proto3";
package media_auth.service_token;
import "api.proto";
import "google/api/annotations.proto";
option go_package = "media_auth/service_token";

// B2B API token(tos_xxx)。明文仅创建时返回一次;LIST 不含 hash/明文;
// DELETE 仅撤销本人非 platform token;每用户至多一个 active platform token。
service ServiceTokenService {
  rpc CreateServiceToken(CreateServiceTokenRequest) returns(CreateServiceTokenResponse) {
    option (google.api.http) = { post: "/api/service-tokens" body: "*" };
    option (api.post) = "/api/service-tokens";
  }
  rpc ListServiceTokens(ListServiceTokensRequest) returns(ListServiceTokensResponse) {
    option (google.api.http) = { get: "/api/service-tokens" };
    option (api.get) = "/api/service-tokens";
  }
  rpc RevokeServiceToken(RevokeServiceTokenRequest) returns(RevokeServiceTokenResponse) {
    option (google.api.http) = { delete: "/api/service-tokens/{id}" };
    option (api.delete) = "/api/service-tokens/:id";
  }
}
message CreateServiceTokenRequest  { string name = 1; }
message CreateServiceTokenResponse {
  string id = 1;
  string name = 2;
  string token = 3;          // tos_xxx 明文,仅本次响应可见
  string created_at = 4;
}
message ListServiceTokensRequest {}
message ListServiceTokensResponse { repeated ServiceTokenRecord tokens = 1; }
message ServiceTokenRecord {
  string id = 1;
  string name = 2;
  string created_by = 3;
  string created_at = 4;
  optional string revoked_at = 5;
  bool is_platform = 6;
}
message RevokeServiceTokenRequest  { string id = 1 [(api.path) = "id"]; }
message RevokeServiceTokenResponse { bool success = 1; }
```

### 3.5 team.proto

```proto
syntax = "proto3";
package media_auth.team;
import "api.proto";
import "google/api/annotations.proto";
option go_package = "media_auth/team";

// Team CRUD + 成员管理 + 邀请签发与撤销。创建者自动成为首任 admin。
// 末位 admin 保护:UpdateMemberRole / RemoveMember 拒绝把唯一 admin 降级或移除。
// 鉴权:每个 handler 内部 getMembership(teamID, session.sub) → 0/403/admin。
service TeamService {
  rpc ListMyTeams(ListMyTeamsRequest) returns(ListMyTeamsResponse) {
    option (google.api.http) = { get: "/api/teams" };
    option (api.get) = "/api/teams";
  }
  rpc CreateTeam(CreateTeamRequest) returns(ApiTeam) {
    option (google.api.http) = { post: "/api/teams" body: "*" };
    option (api.post) = "/api/teams";
  }
  rpc GetTeam(GetTeamRequest) returns(ApiTeam) {
    option (google.api.http) = { get: "/api/teams/{id}" };
    option (api.get) = "/api/teams/:id";
  }
  rpc UpdateTeam(UpdateTeamRequest) returns(ApiTeam) {
    option (google.api.http) = { patch: "/api/teams/{id}" body: "*" };
    option (api.patch) = "/api/teams/:id";
  }
  rpc DeleteTeam(DeleteTeamRequest) returns(DeleteTeamResponse) {
    option (google.api.http) = { delete: "/api/teams/{id}" };
    option (api.delete) = "/api/teams/:id";
  }
  rpc ListMembers(ListMembersRequest) returns(ListMembersResponse) {
    option (google.api.http) = { get: "/api/teams/{id}/members" };
    option (api.get) = "/api/teams/:id/members";
  }
  rpc AddMember(AddMemberRequest) returns(ApiTeamMember) {
    option (google.api.http) = { post: "/api/teams/{id}/members" body: "*" };
    option (api.post) = "/api/teams/:id/members";
  }
  rpc UpdateMemberRole(UpdateMemberRoleRequest) returns(UpdateMemberRoleResponse) {
    option (google.api.http) = { patch: "/api/teams/{id}/members/{userId}" body: "*" };
    option (api.patch) = "/api/teams/:id/members/:userId";
  }
  rpc RemoveMember(RemoveMemberRequest) returns(RemoveMemberResponse) {
    option (google.api.http) = { delete: "/api/teams/{id}/members/{userId}" };
    option (api.delete) = "/api/teams/:id/members/:userId";
  }
  rpc ListInvites(ListInvitesRequest) returns(ListInvitesResponse) {
    option (google.api.http) = { get: "/api/teams/{id}/invites" };
    option (api.get) = "/api/teams/:id/invites";
  }
  rpc CreateInvite(CreateInviteRequest) returns(ApiTeamInvite) {
    option (google.api.http) = { post: "/api/teams/{id}/invites" body: "*" };
    option (api.post) = "/api/teams/:id/invites";
  }
  rpc RevokeInvite(RevokeInviteRequest) returns(RevokeInviteResponse) {
    option (google.api.http) = { delete: "/api/teams/{id}/invites/{token}" };
    option (api.delete) = "/api/teams/:id/invites/:token";
  }
  // 邀请预览与接受 —— 走 /api/invites/* 前缀,鉴权主体是"持 token 的任意已登录 user",非 team admin。
  rpc PreviewInvite(PreviewInviteRequest) returns(ApiTeamInvitePreview) {
    option (google.api.http) = { get: "/api/invites/{token}" };
    option (api.get) = "/api/invites/:token";
  }
  rpc AcceptInvite(AcceptInviteRequest) returns(AcceptInviteResponse) {
    option (google.api.http) = { post: "/api/invites/{token}/accept" };
    option (api.post) = "/api/invites/:token/accept";
  }
}

message ListMyTeamsRequest  {}
message ListMyTeamsResponse { repeated ApiTeam teams = 1; }
message CreateTeamRequest  { string name = 1; optional string description = 2; }
message GetTeamRequest     { string id = 1 [(api.path) = "id"]; }
message UpdateTeamRequest  { string id = 1 [(api.path) = "id"]; optional string name = 2; optional string description = 3; }
message DeleteTeamRequest  { string id = 1 [(api.path) = "id"]; }
message DeleteTeamResponse { bool success = 1; }
message ListMembersRequest { string id = 1 [(api.path) = "id"]; }
message ListMembersResponse{ repeated ApiTeamMember members = 1; }
message AddMemberRequest   { string id = 1 [(api.path) = "id"]; string user_id = 2; optional string role = 3; } // 默认 member
message UpdateMemberRoleRequest  { string id = 1 [(api.path) = "id"]; string user_id = 2 [(api.path) = "userId"]; string role = 3; }
message UpdateMemberRoleResponse { bool success = 1; }
message RemoveMemberRequest  { string id = 1 [(api.path) = "id"]; string user_id = 2 [(api.path) = "userId"]; }
message RemoveMemberResponse { bool success = 1; }
message ListInvitesRequest   { string id = 1 [(api.path) = "id"]; }
message ListInvitesResponse  { repeated ApiTeamInvite invites = 1; }
message CreateInviteRequest  { string id = 1 [(api.path) = "id"]; optional int32 expires_in_days = 2; } // 默认 7
message RevokeInviteRequest  { string id = 1 [(api.path) = "id"]; string token = 2 [(api.path) = "token"]; }
message RevokeInviteResponse { bool success = 1; }
message PreviewInviteRequest { string token = 1 [(api.path) = "token"]; }
message AcceptInviteRequest  { string token = 1 [(api.path) = "token"]; }
message AcceptInviteResponse { string team_id = 1; bool already_member = 2; }

message ApiTeam {
  string id = 1;
  string name = 2;
  optional string description = 3;
  string created_by = 4;
  string created_at = 5;
  string updated_at = 6;
  string my_role = 7;       // admin | member
  int32 member_count = 8;
}
message ApiTeamMember {
  string user_id = 1;
  string user_name = 2;     // JOIN users.display_name
  string role = 3;          // admin | member
  string joined_at = 4;
}
message ApiTeamInvite {
  string token = 1;         // 明文,仅本次响应可见(CREATE)
  string team_id = 2;
  string created_by = 3;
  string created_by_name = 4;
  optional string expires_at = 5;
  string created_at = 6;
}
message ApiTeamInvitePreview {
  string team_id = 1;
  string team_name = 2;
  string inviter_name = 3;
  optional string expires_at = 4;
  bool already_member = 5;
}
```

### 3.6 oauth.proto

```proto
syntax = "proto3";
package media_auth.oauth;
import "api.proto";
import "google/api/annotations.proto";
option go_package = "media_auth/oauth";

// media_auth 作为 OAuth2 Authorization Server,三 grant 合一:
// authorization_code(带 PKCE S256) / refresh_token / token-exchange(RFC 8693)。
// /token 强制 application/x-www-form-urlencoded(RFC 6749 §3.2);JSON fallback 由 handler 手动解析。
service OAuthProviderService {
  rpc GetAuthorizeInfo(GetAuthorizeInfoRequest) returns(GetAuthorizeInfoResponse) {
    option (google.api.http) = { get: "/api/oauth/authorize" };
    option (api.get) = "/api/oauth/authorize";
  }
  rpc ApproveAuthorization(ApproveAuthorizationRequest) returns(ApproveAuthorizationResponse) {
    option (google.api.http) = { post: "/api/oauth/authorize" body: "*" };
    option (api.post) = "/api/oauth/authorize";
  }
  rpc ExchangeToken(ExchangeTokenRequest) returns(ExchangeTokenResponse) {
    option (google.api.http) = { post: "/api/oauth/token" };
    option (api.post) = "/api/oauth/token";
  }
  rpc GetUserInfo(GetUserInfoRequest) returns(OAuthUserInfo) {
    option (google.api.http) = { get: "/api/oauth/userinfo" };
    option (api.get) = "/api/oauth/userinfo";
  }
}
message GetAuthorizeInfoRequest {
  string client_id = 1 [(api.query) = "client_id"];
  string redirect_uri = 2 [(api.query) = "redirect_uri"];
  string response_type = 3 [(api.query) = "response_type"];
  string scope = 4 [(api.query) = "scope"];
}
message GetAuthorizeInfoResponse { string client_name = 1; string scope = 2; }

message ApproveAuthorizationRequest {
  string client_id = 1;
  string redirect_uri = 2;
  string response_type = 3;              // 必须 code
  optional string state = 4;
  optional string code_challenge = 5;
  optional string code_challenge_method = 6;   // 仅 S256
  string scope = 7;
}
message ApproveAuthorizationResponse { string redirect_uri = 1; }  // 已拼 ?code=&state=

// 三 grant 共用;handler 按 grant_type 分发。绑定源:form-urlencoded。
message ExchangeTokenRequest {
  string grant_type = 1 [(api.form) = "grant_type"];   // authorization_code|refresh_token|urn:ietf:params:oauth:grant-type:token-exchange
  optional string code = 2 [(api.form) = "code"];
  optional string redirect_uri = 3 [(api.form) = "redirect_uri"];
  optional string code_verifier = 4 [(api.form) = "code_verifier"];
  optional string refresh_token = 5 [(api.form) = "refresh_token"];
  optional string subject_token = 6 [(api.form) = "subject_token"];
  optional string subject_token_type = 7 [(api.form) = "subject_token_type"];
  optional string resource = 8 [(api.form) = "resource"];
  optional string client_secret = 9 [(api.form) = "client_secret"];
  string client_id = 10 [(api.form) = "client_id"];
}
message ExchangeTokenResponse {
  optional string access_token = 1;
  optional string refresh_token = 2;
  string token_type = 3;                 // Bearer
  int64 expires_in = 4;                  // access_token 剩余秒数
  optional string issued_token_type = 5; // token-exchange:urn:ietf:params:oauth:token-type:access_token
}
message GetUserInfoRequest { optional string access_token = 1; }  // handler 从 Authorization Header 读
message OAuthUserInfo {
  string sub = 1;
  string username = 2;
  string name = 3;
  optional string email = 4;
}
```

### 3.7 mcp_oauth.proto

```proto
syntax = "proto3";
package media_auth.mcp_oauth;
import "api.proto";
import "google/api/annotations.proto";
option go_package = "media_auth/mcp_oauth";

// media_auth 反向作为 OAuth client 去登第三方 MCP server。
// authorize/callback 走 popup + postMessage,handler 返回 HTML 字符串或 c.Redirect。
service McpOAuthService {
  rpc DiscoverOAuth(DiscoverOAuthRequest) returns(DiscoverOAuthResponse) {
    option (google.api.http) = { post: "/api/mcp-oauth/discover" body: "*" };
    option (api.post) = "/api/mcp-oauth/discover";
  }
  rpc GetConnectionStatus(GetConnectionStatusRequest) returns(GetConnectionStatusResponse) {
    option (google.api.http) = { get: "/api/mcp-oauth/status" };
    option (api.get) = "/api/mcp-oauth/status";
  }
  rpc StartAuthorization(StartAuthorizationRequest) returns(StartAuthorizationResponse) {
    option (google.api.http) = { get: "/api/mcp-oauth/authorize" };
    option (api.get) = "/api/mcp-oauth/authorize";
  }
  rpc HandleCallback(HandleCallbackRequest) returns(HandleCallbackResponse) {
    option (google.api.http) = { get: "/api/mcp-oauth/callback" };
    option (api.get) = "/api/mcp-oauth/callback";
  }
  rpc Disconnect(DisconnectRequest) returns(DisconnectResponse) {
    option (google.api.http) = { delete: "/api/mcp-oauth/{server_origin}" };
    option (api.delete) = "/api/mcp-oauth/:server_origin";
  }
}
message DiscoverOAuthRequest  { string url = 1; }
message DiscoverOAuthResponse { bool oauth_required = 1; optional string server_origin = 2; }
message GetConnectionStatusRequest  { string servers = 1 [(api.query) = "servers"]; }  // 逗号分隔
message GetConnectionStatusResponse { map<string, ConnectionStatus> statuses = 1; }
message ConnectionStatus { bool connected = 1; optional string expires_at = 2; }
message StartAuthorizationRequest {
  string server_origin = 1 [(api.query) = "server_origin"];
  string workspace_id = 2 [(api.query) = "workspace_id"];
}
message StartAuthorizationResponse { string redirect_url = 1; }   // handler 实际 c.Redirect
message HandleCallbackRequest {
  string code = 1 [(api.query) = "code"];
  string state = 2 [(api.query) = "state"];
  optional string error = 3 [(api.query) = "error"];
}
message HandleCallbackResponse { string html = 1; bool success = 2; }  // 完整 HTML(postMessage+close)
message DisconnectRequest  { string server_origin = 1 [(api.path) = "server_origin"]; }
message DisconnectResponse { bool success = 1; }
```

### 3.8 token.proto（internal，mTLS）

```proto
syntax = "proto3";
package media_auth.token;
import "api.proto";
option go_package = "media_auth/token";

// internal service —— 仅 mTLS / 内网暴露,不挂网关路由,不出 OpenAPI。
// 仅供 control_plane 验 opaque Bearer service token;cookie/bearer JWT 由 control_plane 本地 HS256 验。
service TokenService {
  rpc VerifyToken(VerifyTokenRequest) returns(VerifyTokenResponse) {
    option (api.post) = "/internal/token/verify";
  };
}
message VerifyTokenRequest {
  string token = 1;                          // tos_xxx 明文;内部 sha256 后查表,明文不出边界
  optional string token_type_hint = 2;       // service_token
}
message VerifyTokenResponse {
  bool active = 1;
  optional string sub = 2;                   // = service_tokens.created_by
  optional string username = 3;              // svc:{name}
  optional string name = 4;
  optional string role = 5;                  // user
  optional string token_type = 6;            // service_token
}
```

### 3.9 鉴权 skip list（media_auth 内部中间件）

media_auth 对 4 前缀做 cookie/session 鉴权，以下端点短路（无 JWT）：
- `POST /api/auth/login`、`GET /api/auth/wecom/enabled`、`GET /api/auth/wecom/authorize`、`GET /api/auth/wecom/callback`
- `GET /api/oauth/authorize`、`POST /api/oauth/token`、`GET /api/oauth/userinfo`
- wecom/mcp 的 callback 在 skip list 内由 handler 手动验 cookie（bind 态）。
- `VerifyToken` 走 mTLS 通道，不经 HTTP 鉴权中间件。

---

## 4. 数据库 schema

SQL 落到 `media_auth/migrations/0001_init.sql`（schema 唯一源；后续 `0002_xxx.sql` 追加，不再耦合 control_plane 主迁移链）。8 张表，全部在 `media_auth` schema 下。`users.id` / `service_tokens.id` 沿用应用层 `generateId()` 短随机串（兼容存量）。

```sql
BEGIN;

-- 1. users —— 账号主表
CREATE TABLE IF NOT EXISTS media_auth.users (
    id                  text PRIMARY KEY,
    username            text NOT NULL,
    display_name        text NOT NULL,
    email               text,
    password_hash       text,                 -- argon2;空=LDAP/企微派生,不可改密
    role                text NOT NULL DEFAULT 'user',   -- user|admin|system(system 禁登录)
    default_prompt_id   text,                 -- opaque,不校验 FK(prompts 表留 control_plane)
    auto_evolution      boolean NOT NULL DEFAULT false,
    created_at          timestamptz DEFAULT now(),
    last_login_at       timestamptz           -- 仅本地登录更新
);
CREATE UNIQUE INDEX IF NOT EXISTS users_username_key ON media_auth.users (username);

-- 2. user_identities —— 外部身份绑定
CREATE TABLE IF NOT EXISTS media_auth.user_identities (
    user_id       text NOT NULL REFERENCES media_auth.users(id) ON DELETE CASCADE,
    provider      text NOT NULL,
    external_id   text NOT NULL,
    display_name  text,
    metadata      jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at    timestamptz DEFAULT now(),
    PRIMARY KEY (provider, external_id)
);
CREATE UNIQUE INDEX IF NOT EXISTS user_identities_user_provider_key ON media_auth.user_identities (user_id, provider);
CREATE INDEX IF NOT EXISTS user_identities_user_idx ON media_auth.user_identities (user_id);

-- 3. service_tokens —— B2B API token(tos_xxx)
CREATE TABLE IF NOT EXISTS media_auth.service_tokens (
    id          text PRIMARY KEY,
    name        text NOT NULL,
    token_hash  text NOT NULL,                -- SHA-256(token)
    token       text,                         -- 仅 platform 落明文;非 platform 必为 NULL
    created_by  text REFERENCES media_auth.users(id),
    created_at  timestamptz DEFAULT now(),
    revoked_at  timestamptz,                  -- 非空=失效
    is_platform boolean NOT NULL DEFAULT false
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_service_tokens_platform_user
    ON media_auth.service_tokens (created_by)
    WHERE is_platform = true AND revoked_at IS NULL;

-- 4. oauth_clients —— OAuth2 客户端注册
CREATE TABLE IF NOT EXISTS media_auth.oauth_clients (
    id              text PRIMARY KEY,
    name            text NOT NULL,
    secret_hash     text,                     -- SHA-256(client_secret);空=公开 client
    redirect_uris   text[] NOT NULL,          -- 逐元素精确匹配
    homepage_url    text,
    description     text,
    created_by      text NOT NULL REFERENCES media_auth.users(id),
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);

-- 5. oauth_authorization_codes —— 一次性授权码(10min,消费即删)
CREATE TABLE IF NOT EXISTS media_auth.oauth_authorization_codes (
    code            text PRIMARY KEY,
    client_id       text NOT NULL REFERENCES media_auth.oauth_clients(id) ON DELETE CASCADE,
    user_id         text NOT NULL REFERENCES media_auth.users(id) ON DELETE CASCADE,
    redirect_uri    text NOT NULL,
    code_challenge  text,                     -- PKCE S256
    scope           text NOT NULL DEFAULT 'profile',
    expires_at      timestamptz NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS oauth_authorization_codes_expires_idx ON media_auth.oauth_authorization_codes (expires_at);

-- 6. oauth_refresh_tokens —— 刷新令牌(30d,轮转)
CREATE TABLE IF NOT EXISTS media_auth.oauth_refresh_tokens (
    token_hash  text PRIMARY KEY,             -- SHA-256(refresh_token)
    client_id   text NOT NULL REFERENCES media_auth.oauth_clients(id) ON DELETE CASCADE,
    user_id     text NOT NULL REFERENCES media_auth.users(id) ON DELETE CASCADE,
    expires_at  timestamptz NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS oauth_refresh_tokens_expires_idx ON media_auth.oauth_refresh_tokens (expires_at);

-- 7. mcp_oauth_clients —— 第三方 MCP server 的 OAuth client 注册
CREATE TABLE IF NOT EXISTS media_auth.mcp_oauth_clients (
    server_origin text PRIMARY KEY,
    metadata      jsonb NOT NULL DEFAULT '{}'::jsonb,   -- 缓存 /.well-known/oauth-authorization-server
    client_id     text NOT NULL,
    client_secret text,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);

-- 8. mcp_oauth_tokens —— 用户对每个 MCP server 的 token
--    refresh 失败宽限:refresh_fail_count>=3 且 refresh_fail_first_at 距今>=600s 才删;
--    之前视为暂态;成功 refresh 重置两者。
CREATE TABLE IF NOT EXISTS media_auth.mcp_oauth_tokens (
    user_id                   text NOT NULL REFERENCES media_auth.users(id) ON DELETE CASCADE,
    server_origin             text NOT NULL REFERENCES media_auth.mcp_oauth_clients(server_origin) ON DELETE CASCADE,
    access_token              text NOT NULL,
    refresh_token             text,
    token_type                text NOT NULL DEFAULT 'Bearer',
    scope                     text,
    expires_at                timestamptz,
    refresh_fail_count        integer NOT NULL DEFAULT 0,
    refresh_fail_first_at     timestamptz,
    created_at                timestamptz NOT NULL DEFAULT now(),
    updated_at                timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, server_origin)
);
CREATE INDEX IF NOT EXISTS mcp_oauth_tokens_user_idx ON media_auth.mcp_oauth_tokens (user_id);

COMMIT;
```

> 团队相关 3 张表（`teams` / `team_members` / `team_invites`）由 `team` domain 接管，从 control_plane 主迁移链迁出，下一批迁移 `0002_teams.sql` 追加，本节未列出。

GORM Gen：`biz/dal/query/` 🔴 禁改，schema 变更重新生成；`biz/dal/repo/` 把行模型转 domain 对象，禁止 GORM 行模型泄漏上层。

---

## 5. 业务规则与不变量

### 5.1 auth（session 归此 domain）
- **Login**：先本地（`argon2.Verify(password, password_hash)`，`password_hash` 非空时），失败再 LDAP（bind+search，命中则 upsert：已存在更新 `last_login_at`，不存在则创建 `password_hash` 留空）。两者皆失败 → 401。`role=='system'` → 403。成功签 session（HS256 JWT，`sub/username/name/email?/role/exp=now+86400`，下 httpOnly cookie `token`，`sameSite=Lax,maxAge=86400,path=/`），fire-and-forget `ensurePlatformToken`。返回 ApiUser。
- **auth_source 推导**：`password_hash` 非空 → `password`；否则有 wecom identity → `wecom`；否则 `ldap`。（只读 identity 查询。修正 control-plane 对企微账号误报 `ldap`。）
- **ChangePassword**：仅 `password_hash` 非空（否则 400）；`argon2.Verify(current)` 失败 → 401；`len(new)<6` → 400；`new==current` → 400；更新 argon2 hash。
- **GetMe**：按 `session.sub` 查 users，推导 auth_source，返回 ApiUser（仅 `default_prompt_id`，无 name）。
- **SetDefaultPrompt / UpdateMe**：用 `optional` 表达 tri-state / 显式 false；`prompt_id` opaque 不校验存在性。
- **session 续期**：`verifySession` 剩余 <12h 重签 24h 替换 cookie（滑动）。
- JWT 密钥 `JWT_SECRET` 与 control_plane 共享（ADR-0001）。

### 5.2 identity
- **ListIdentities**：按 `session.sub` 查全部绑定，返回 provider/external_id/display_name/created_at。
- **DeleteIdentity**：`DELETE WHERE user_id=sub AND provider=?`，0 行 → 404。（泛化自 control-plane 写死 wecom 的 `DELETE /api/auth/wecom/identity`，是 P2 前端破坏性变更。）
- **内部 findUserIdentity(provider, external_id)**：供 wecom login 解析 user_id；无 → not_bound。
- **内部 bindIdentity(...)**：供 wecom bind；先查 `(provider,external_id)` 是否已绑别人 → already_bound；`ON CONFLICT (user_id,provider) DO UPDATE`。
- 不变量：`(provider,external_id)` 全局唯一；`(user_id,provider)` 每用户每 provider 至多一条。

### 5.3 wecom（无持久化表；corp token 内存缓存 2h，1min 安全余量）
- **GetEnabled**：`WECOM_CORP_ID/SECRET/AGENT_ID` 三者齐备 → enabled。
- **GetAuthorizeUrl**：未配置 → 400；拼企微 oauth2 authorize URL，`state=mode`。
- **HandleCallback**：缺 code → redirect `/login?error=missing_code`；`corp_token=getCorpAccessToken()`；`userid=企微 auth/getuserinfo(code)`；
  - `state=login`：`identity.findUserIdentity('wecom',userid)` 无 → `/login?error=not_bound`；查 users，system → `system_account`，无 → `user_not_found`；`auth.issueSession` + `ensurePlatformToken`；redirect `/`。
  - `state=bind`：手动验 cookie JWT（callback 在 skip list），无效 → `/login?error=auth_required`；企微 `user/get` 取 display_name/metadata；`identity.bindIdentity`，已绑别人 → `/?wecom_bind=already_bound`，成功 → `/?wecom_bind=success`。
  - 上游异常 → `/login?error=oauth_failed`。
- handler 直接 `c.Redirect` 302。

### 5.4 service_token
- **CreateServiceToken**：缺 name → 400；`token="tos_"+hex(32)`；INSERT `{id,name,token_hash=sha256(token),created_by=sub,is_platform=false}`；明文仅本次返回。
- **ListServiceTokens**：查 `created_by=sub`，不含 hash/明文。
- **RevokeServiceToken**：`UPDATE revoked_at=now() WHERE id=? AND created_by=sub AND is_platform=false`，0 行 → 404。
- **内部 ensurePlatformToken(userID)**：已有 active platform token 则返回；否则 INSERT（`name='__platform__',is_platform=true,token=明文`），唯一索引并发冲突重读。fire-and-forget。
- **内部 getServiceTokenByHash(hash)**：`WHERE token_hash=? AND revoked_at IS NULL`，供 VerifyToken。
- **内部 getPlatformToken(userID)**：供 control_plane agent 回调取明文。
- 不变量：每用户至多一个 active platform token（部分唯一索引）；明文 `token` 列仅 platform 非 NULL（ADR-0002）。

### 5.5 team
- **CreateTeam**：事务内 INSERT `teams` + INSERT `team_members`(role='admin')，创建者自动升任首任 admin。
- **GetTeam / ListMyTeams / ListMembers**：鉴权点都是 `getMembership(teamID, session.sub)`，缺失统一返 404（不向非成员暴露 team 是否存在）。
- **UpdateTeam / DeleteTeam**：要求 `membership.role=='admin'`，否则 403；delete 级联清 team_members + team_invites。
- **AddMember**：admin only；目标 user 必须存在（按 users.id 查，由 auth 提供）；`INSERT ... ON CONFLICT (team_id,user_id) DO NOTHING` 幂等。
- **UpdateMemberRole**：admin only；目标 role 非 admin 且现存 admin 仅 1 名且为该 user → 409（"Cannot demote the last admin"）。
- **RemoveMember**：admin 可移任何人；非 admin 仅可 self-remove；目标 role=admin 且现存 admin 仅 1 名 → 409（"Cannot remove the last admin; delete the team instead"）。
- **CreateInvite**：admin only；`token="tinv_"+base64url(randomBytes(24))`；INSERT `(team_id, created_by, expires_at=now+7d)`；明文 token 仅本次响应可见。
- **ListInvites**：admin only；`WHERE expires_at IS NULL OR expires_at > now()`。
- **RevokeInvite**：admin only；DELETE by token。
- **PreviewInvite / AcceptInvite**：任意已登录 user；token 不存在 → 404，过期 → 410，team 已被删 → 404；AcceptInvite 已入团 → 幂等返 `{team_id, already_member:true}`；新加入固定 role='member'，不接受 invite 直升 admin。
- 不变量：team 必须始终至少一名 admin（Last Admin Protection）；invite token 全局唯一（PK）；team 删除级联清 team_members + team_invites。

### 5.6 oauth（Access Token 不落库，HS256 JWT 自包含）
- **GetAuthorizeInfo**：`response_type!=code` → 400；client 不存在 → 400 `invalid_client`；`redirect_uri` 不在白名单 → 400。返回 client_name/scope。
- **ApproveAuthorization**：同上校验 + `code_challenge_method` 仅 S256（否则 400）；`code=hex(32)` INSERT（`user_id=session.sub, expires_at=now+10min`）；返回 `{redirect_uri}?code=&state=`。
- **ExchangeToken**（form-urlencoded，按 `grant_type` 分发）：
  - `authorization_code`：`DELETE code RETURNING`（一次性），无/过期 → 400 `invalid_grant`；`client_id`/`redirect_uri` 不匹配 → 400；有 `code_challenge` 缺 `code_verifier` → 400；`base64url(sha256(verifier))!=challenge` → 400；签 access(1h)+refresh(hex(32) 存 hash,30d)。
  - `refresh_token`：hash 查表，无/过期 → 400，`client_id` 不匹配 → 400；旧删新发（轮转）。
  - `token-exchange`：`subject_token_type` 须 access_token 类型或缺省；缺 `subject_token`/`resource` → 400；confidential client 校验 `client_secret`（常量时间）；`POST {resource.origin}/introspect` 不 active → 400；签 access(1h)，**不发 refresh**，返回 `issued_token_type`。
  - 其它 → 400 `unsupported_grant_type`。
- **GetUserInfo**：Bearer access_token 本地 HS256 验签，失败 → 401；返回 sub/username/name/email。
- **清理**：每小时删过期 authorization_code / refresh_token。
- 不变量：auth_code 一次性；refresh_token 轮转；redirect_uri 精确匹配；PKCE 仅 S256。

### 5.7 mcp_oauth（access/refresh token 明文存，对 media_auth 不透明）
- **DiscoverOAuth**：缺 url → 400；`GET {origin}/.well-known/oauth-authorization-server`（header `MCP-Protocol-Version: 2025-03-26`，5s）；缺 authorize+token endpoint → `{oauth_required:false}`；有 → upsert `mcp_oauth_clients`，`{true,server_origin}`；不可达 → 502。
- **GetConnectionStatus**：`servers` 逗号分隔，逐个查 `(user_id=sub,server_origin)`；connected=存在且未过宽限；返回 map。
- **StartAuthorization**：缺参 → 400；未 discover → 400；`code_verifier=rand(32),code_challenge=S256,state=rand(16)`；`pendingAuths[state]={user_id,server_origin,code_verifier,workspace_id,created_at=now}`（内存 10min TTL，一次性）；`c.Redirect` 到 authorize_endpoint。
- **HandleCallback**：缺 code/state → 400；`pendingAuths[state]` 不存在/过期 → 400；`pending.user_id!=session.sub` → 403；`POST token_endpoint`(code,code_verifier,redirect_uri) 换 token；`upsertToken`（`ON CONFLICT DO UPDATE`，重置 `refresh_fail_count=0/refresh_fail_first_at=NULL`，`expires_at=now+expires_in`，无则 +1800s）；`notifyAgentReload(workspace_id,['config'])`（跨服务通知 control_plane）；返回 HTML（`postMessage+close`）。
- **getValidAccessToken(userID, serverOrigin, forceRefresh?)**（内部，供 control_plane mcp-proxy）：取 token 行，`expires_at` 空回退 `updated_at+1800s`；未过期且非 force → 返回 access；否则 refresh：成功 → upsert（重置计数）→ 返回新 access；失败（`invalid_grant/invalid_client/unauthorized_client`）→ `refresh_fail_count++`，首次记 `refresh_fail_first_at`；`count>=3 且 first>=600s` → 删 token + 抛 `McpOAuthReauthRequired`；否则返回 nil（暂态）。
- **Disconnect**：`DELETE WHERE user_id=sub AND server_origin=?`，返回 success。
- 不变量：pendingAuths 一次性；`(user_id,server_origin)` upsert 幂等；宽限算法见上。

---

## 6. 分层实现要点

- **handler**（轻薄）：请求绑定 → 基础校验 → DTO→Command → 调 service/workflow → result→response。禁止写 SQL/复杂规则/治理组件。
- **service**：单 domain 单事务。如 `service/auth.Login`、`service/service_token.Create`、`service/oauth.ExchangeToken`（按 grant 内部分发）。
- **workflow**：跨 domain。`wecom_login_bind`、`oauth_authorize_code`、`mcp_oauth_connect`。跨系统副作用（企微 API / 外部 RS introspect / MCP token_endpoint / notifyAgentReload）在 workflow 内，外部调用前尽量先 commit 本地状态。
- **policy**（纯决策）：`policy/auth`（system 禁登录、改密规则）、`policy/oauth`（redirect_uri/secret/PKCE/grant 校验）、`policy/service_token`（撤销权限）、`policy/mcp_oauth`（宽限判定）。不查 DB/Redis/外部 API。
- **domain**：不带 JSON/query/form/ORM tag，不依赖日志/配置/框架；持不变量。
- **main.go**（server.md §8 顺序）：`config.NewLoader` → `serverhertz.NewServerSuite(loader)` → `loader.Attach(KVSource)` → `loader.Watch` → `defer loader.Close` → `InitTracing/InitMetrics` → `NewServer` → `InitDI`（注入 service/workflow 到 handler）→ `register(h)` → `RegisterHealthRoutes` 等 → `RegisterService` → `h.Spin`。不直接拼 options/middleware、不直连 Consul/OTel。
- **外部依赖**：argon2、ldapts（LDAP）、企微 corp API（`qyapi.weixin.qq.com`）、外部 RS `/introspect`、MCP server `/.well-known/oauth-authorization-server`+token/registration endpoint。client 装配在 `biz/shared/client/`（消费 `hertz_infra/clienthertz`），service/domain 层不直接 import SDK。

---

## 7. 配置（conf/<env>.yaml，遵循 config.md / server.md）

启用字段统一 `enabled`，服务名 `app.name`。治理能力顶级块由 `hertz_infra/serverhertz` 消费（consul/otel/prometheus/access_log/recovery/request_id/cors/rate_limit/pprof/health）。media_auth 业务相关配置：

```yaml
app:
  name: media-auth
  version: v1
  env: ${APP_ENV:-local}

server:
  enabled: true
  address: ${SERVER_ADDR:-:8888}
  timeout: { enabled: true, read_timeout_ms: 10000, write_timeout_ms: 10000 }

database:
  enabled: true
  driver: postgres                # media_auth 用 postgres(control-plane 现状 pg)
  dsn: ${MEDIA_AUTH_DB_DSN:-postgres://...}
  max_open_conns: 50
  max_idle_conns: 10

# media_auth 业务配置(自定义顶级块,业务层 loader.Subscribe)
auth:
  jwt_secret: ${JWT_SECRET:-}             # 与 control_plane 共享(HS256)
  jwt_expires_in_seconds: 86400           # session 24h
  access_token_expires_in_seconds: 3600   # oauth access 1h
  refresh_token_expires_in_days: 30
  auth_code_expires_in_minutes: 10
ldap:
  enabled: ${LDAP_ENABLED:-false}
  url: ${LDAP_URL:-}
  bind_dn: ${LDAP_BIND_DN:-}
  bind_password: ${LDAP_BIND_PASSWORD:-}
  search_base: ${LDAP_SEARCH_BASE:-}
  search_filter: ${LDAP_SEARCH_FILTER:-}
  attr_name: ${LDAP_ATTR_NAME:-}
  attr_username: ${LDAP_ATTR_USERNAME:-}
  attr_email: ${LDAP_ATTR_EMAIL:-}
wecom:
  enabled: ${WECOM_ENABLED:-false}        # CORP_ID/SECRET/AGENT_ID 齐备
  corp_id: ${WECOM_CORP_ID:-}
  corp_secret: ${WECOM_CORP_SECRET:-}
  agent_id: ${WECOM_AGENT_ID:-}
mcp_oauth:
  pending_auth_ttl_seconds: 600
  default_token_ttl_seconds: 1800
  refresh_fail_threshold: 3
  refresh_fail_grace_seconds: 600
token_verify:                            # internal VerifyToken(mTLS) listener
  enabled: true
  address: ${INTERNAL_ADDR:-:8889}
```

> `JWT_SECRET` 跨服务共享，轮换需 media_auth 与 control_plane 同步（ADR-0001）。

---

## 8. 与 control_plane 的交互（跨服务，见 workflow）

| 交互 | 方向 | 机制 | 文档 |
|---|---|---|---|
| 4 前缀反代 | browser → media_auth | control_plane 网关反代 `/api/auth` `/api/oauth` `/api/service-tokens` `/api/mcp-oauth` | ADR-0001 |
| team/invites 反代 | browser → media_auth | control_plane 网关反代 `/api/teams` `/api/invites` | team domain（与 4 前缀同批次纳入 P2）|
| cookie/session 鉴权 | media_auth 自闭环 | media_auth 签+验 cookie JWT | wecom_login_bind |
| VerifyToken | control_plane → media_auth | mTLS internal RPC，opaque service_token→Subject | [service_token_verification](../../workflow/0001-service-token-verification.md) |
| JWT 本地验 | control_plane 自闭环 | HS256 共享密钥验 cookie/bearer JWT | ADR-0001 |
| notifyAgentReload | media_auth → control_plane | MCP OAuth 回调成功后通知 workspace reload config | mcp_oauth_connect |
| getPlatformToken | control_plane → media_auth | 内部接口取 agent 回调用 platform token 明文 | service_token domain |

**VerifyToken 实现**：`service_token.getServiceTokenByHash(sha256(token))` → 命中且 `revoked_at IS NULL` → `{active:true, sub=created_by, username=svc:{name}, name, role=user, token_type=service_token}`；否则 `{active:false}`。明文 token 永不出 media_auth 边界。

---

## 9. 落地路线

| 阶段 | 动作 | 状态 |
|---|---|---|
| 本批次 | IDL(含 token.proto) + SQL + 领域模型 + spec 落盘；control_plane 不动 | ✅ 完成 |
| P1 | media_auth 独立 Go service：`hz update` 生成 6 service handler/router 骨架 + token service；实现 domain/service/workflow/dal；e2e | ✅ 完成（7 domain + token internal 全部实现；build + arch-check + gofmt + go vet + 关键路径 e2e 冒烟通过；规范 e2e 待统一补）|
| P2 | control_plane 网关反代 4 前缀到 media_auth；前端适配 identity 路径变更；control_plane 中间件接 VerifyToken RPC | 待启动 |
| P3 | media_auth 接管 8 表，control_plane 删除认证相关 migration；DB 物理独立/同库不同 schema | 待启动 |
| P4 | （远期）JWT 迁非对称签名，消除 JWT_SECRET 共享（ADR-0001 备选）| 评估 |

---

## 10. 验收检查（architecture.md §工作流第 10 步）

- `gofmt -l .`
- `go test ./...`
- `make arch-check`（垂直层边界 + 同层 domain 隔离 + 框架 import 限制；`--strict-idl` 校验 `go_package` 末段==service）
- `make hz-model` / `make hz-client` / `make -C media_auth hz-update` / `make swagger-gen` 流程可跑通
- e2e（参考 control-plane `e2e/auth.test.ts` / `service-tokens.test.ts` / `oauth-provider` / `mcp-oauth` 用例）：6 service 关键路径 + VerifyToken RPC + 宽限算法
