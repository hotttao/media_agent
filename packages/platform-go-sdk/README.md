# platform-go-sdk

Go 服务端应用 SDK，复用 `idl/` 对应的 `hertz_gen` client。现有 JavaScript SDK 保留在 `packages/platform-server-sdk`。

```go
import sdk "media_agent/packages/platform-go-sdk"

platform, err := sdk.New(sdk.Config{
    BaseURL: "https://platform.example",
    ClientID: os.Getenv("PLATFORM_CLIENT_ID"),
    ClientSecret: os.Getenv("PLATFORM_CLIENT_SECRET"),
})
if err != nil { return err }
_, err = platform.UpsertUser(ctx, "business-user-123", sdk.UserProfileInput{
    DisplayName: "Example user",
})
if err != nil { return err }
user, err := platform.As("business-user-123", teamID,
    sdk.WorkspaceRead, sdk.WorkspaceEnsure, sdk.SessionChat)
if err != nil { return err }
workspace, err := user.EnsureAgent(ctx, "assistant", sdk.AgentConfiguration{
    AgentType: "codex", ModelConnectionID: connectionID,
})
if err != nil { return err }
session, err := user.CreateSession(ctx, workspace.ID, "New conversation")
if err != nil { return err }
turn, err := user.RunTurn(ctx, session.ID, "Hello", requestID)
```

应用必须预先获得 Team Grant，用户需具有对应组织、团队和资源权限。`As(externalID, "")` 仅支持用户资料；团队与 scopes 必须同时提供。`Me`、Agent、模型连接和会话接口使用应用用户凭证，管理接口不暴露在 SDK 内。

## 分层与生成代码

- 根目录：公共门面、配置、实例装配、自有契约的公开别名。
- `biz/workflow/application_access`：身份与授权上下文、凭证交换编排。
- `biz/workflow/agent_access`：Agent、个人模型连接调用。
- `biz/workflow/session_relay`：会话调用和 Workspace 归属检查。
- `biz/handler/session_relay`：标准 `net/http` 中继入口。
- `biz/shared/client`：唯一生成 client / DTO 适配层，消费 `hertz_infra/clienthertz`。SSE 在这里作为流式传输扩展实现。
- `biz/shared/credential`：实例内缓存、并发请求合并，提前 30 秒更新凭证。
- `biz/shared/caller`：复用平台 Caller claims 与 JWT 库验证签名。
- `biz/shared/contract`：SDK 自有输入和结果，无服务内部模型依赖。

变更协议时修改 `idl/`，运行仓库 `make hz-model` / `make hz-client`，再调整出站适配映射。不要手改生成文件或复制服务业务代码。SDK 不初始化宿主全局观测配置，不使用全局 client。

当前是仓库内独立 module，由根 `go.work` 关联 `hertz_gen` / `hertz_infra`。尚未发布为外部可 `go get` 的版本。

## 应用会话中继

```go
relay, err := sdk.NewRelay(sdk.RelayConfig{
    Prefix: "/api/agent",
    Origin: "https://business.example", // 部署配置中的固定 origin
    Authenticate: authenticateBusinessSession,
    Resolve: func(ctx context.Context, key string) (*sdk.UserClient, string, error) {
        // 根据已认证的应用身份与应用 key 查询映射；不能直接采用浏览器的 workspace ID。
        return resolveApplicationWorkspace(ctx, key)
    },
})
if err != nil { return err }
mux.Handle("/api/agent/", relay)
```

`Authenticate` 签名为 `func(*http.Request) (context.Context, error)`，应验证应用自己的登录态并把身份写入返回的 context。`Resolve` 接收该 context 和应用自定义 key；必须验证当前用户有权使用此映射。SDK 继续校验 Session 属于映射的 Workspace，平台再次鉴权。

入口为 `/api/agent/workspaces/:key/sessions`：

| 后缀 | 方法 | 输入 |
|---|---|---|
| 无 | GET / POST | POST: `title` |
| `/:session/messages` | GET | 无 |
| `/:session/requests` | GET | 无 |
| `/:session/events` | GET | `after_sequence` 或 `Last-Event-ID` |
| `/:session/turns` | POST | `message`, `idempotency_key` |
| `/:session/interrupt` | POST | `{}` |
| `/:session/turns/:turn/requests/:request/resolve` | POST | `response_json` |

写请求必须带与配置完全匹配的 `Origin`；请求 JSON 上限 1 MiB，拒绝未知字段和多个 JSON 值。浏览器 Authorization 不转发到平台。中继成功 JSON 响应为 200，列表保持 `sessions` / `messages` / `requests` 包装，SSE 保留事件及 ID，断开时取消上游流。

直接调用 `Events(ctx, sessionID, afterSequence)` 返回流式 HTTP response，调用方必须关闭 `Body`；以 context 控制流生命周期。普通请求超时为 30 秒；不会自动重试写操作。401/403 会清除对应缓存，后续显式调用重新交换凭证。并发交换独立于单个调用方取消，最长 30 秒。

## 个人 Codex 与 Caller

`CreateModelConnection` / `UpdateModelConnection` / `ListModelConnections` / `RevokeModelConnection` 使用平台个人连接协议。经用户授权的 Codex OAuth `auth.json` 作为 `AuthJSON` 上传后，将连接 ID 交给 `EnsureAgent`；不要求改用 API Key。认证材料仅在应用服务端读取，不记录日志或发送至浏览器。

业务 MCP 可用 `NewCallerVerifier(CallerConfig{Issuer, Audience, JWKS})`，再调用 `Verify(token, CallerBinding{WorkspaceID, SessionID})`。JWKS、issuer、audience 必须来自可信部署配置，不能从请求提供。验证固定 RS256、专用 typ、kid、有效期与身份字段；Caller 证明身份，业务资源授权仍由 MCP 自己执行。轮换公钥时用新 JWKS 构造 verifier。

## 验证

在仓库根目录：

```sh
go test ./packages/platform-go-sdk/...
(cd packages/platform-go-sdk && go run ../../harness/tools/archcheck)
```

`e2e/` 使用真实 HTTP 服务连接生成 client，验证凭证边界、错误状态、中继归属和 SSE，不调用模型。

`examples/runtime-smoke` 是显式启用的真实 Codex 验收程序，从 stdin JSON 接收 `base`, `app`, `secret`, `external`, `team`, `authFile`。需使用独立测试应用、用户和 Team Grant；程序创建连接、绑定 Workspace、执行真实推理、校验历史和断点续接、中断下一轮，最后撤销连接。不要在 shell 命令参数中放密钥。运行环境的测试容器与 Workspace 由部署侧按测试 ID 清理。
