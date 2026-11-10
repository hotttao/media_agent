# 平台服务端 SDK

只在受信业务后端使用。浏览器登录仍由业务应用负责；`externalId` 必须取已验证本地会话中的稳定用户 ID。SDK 不应被打包进浏览器。

```js
import {PlatformClient, verifyCaller} from '@media-agent/platform-server-sdk';

const platform = new PlatformClient({
  baseUrl: process.env.MEDIA_PLATFORM_URL,
  clientId: process.env.MEDIA_APPLICATION_ID,
  clientSecret: process.env.MEDIA_APPLICATION_SECRET,
});

const externalId = authenticatedLocalUser.id;
await platform.upsertUser(externalId, {display_name: authenticatedLocalUser.name});
const user = platform.as(externalId, {
  teamId: authorizedTeamId,
  scopes: ['workspace.read', 'workspace.ensure', 'session.chat'],
});
const agent = await user.ensureAgent('reviewer', {
  display_name: 'Reviewer', agent_type: 'goose',
});
const session = await user.createSession(agent.id, 'Review');
await user.runTurn(session.id, 'Review this change', stableRequestId);
```

平台管理员先注册应用并配置 Team Grant；用户须通过现有组织/团队成员管理加入对应组织与团队。`workspace.ensure` 还要求当前 Workspace 功能权益；`session.chat` 沿用当前运行接口的 manage 权限。upsert 不自动开通权限。

同一应用、用户、Team、Agent 名称的 ensure 请求幂等；不同配置返回 409。Session 创建不自动重试，结果不明时通过 listSessions 查询恢复。Turn 要求传入稳定幂等键，重试同一操作应复用该键。

短期凭证缓存按 externalId、Team 和 scopes 隔离，合并并发换取；在到期前刷新，401/403 清理缓存，不自动重放写请求。SSE 用 `events(sessionId, afterSequence)` 返回流式 Response；续连时重新鉴权，最长单次客户端请求一小时。服务端撤销检查在每次请求进入时执行，不承诺主动踢掉已经建立的 SSE 连接。

## 接收 Caller

平台管理员为应用登记 MCP Definition ID 与精确 HTTPS origin。Gateway 配置独立持久化 RSA 私钥 `APPLICATION_CALLER_SIGNING_KEY_FILE`（权限 0600）及 `APPLICATION_CALLER_ISSUER`，并启用现有 MCP Gateway/Agent 身份配置；未配置私钥不生成 Caller。Auth 服务间凭证 `MEDIA_AUTH_INTERNAL_TOKEN` 必须非空。

从可信部署地址 `/.well-known/application-caller-jwks.json` 获取公钥并由应用维护刷新；不要根据传入 JWT 的 issuer/jku/x5u 获取密钥。独立 Caller 密钥不与 Web Session 或内部委托密钥混用。轮换时业务侧可在 60 秒过渡期保留旧公钥；新 kid 刷新失败应拒绝。

```js
const caller = verifyCaller(request.headers.get('X-Platform-Caller'), {
  jwks: trustedPlatformCallerKeys,
  issuer: configuredPlatformCallerIssuer,
  audience: process.env.MEDIA_APPLICATION_ID,
  workspaceId: expectedLocalWorkspaceId,
});
const localUser = await users.findById(caller.externalId);
// 后续必须检查本地用户状态、会话/Workspace关联及该工具的业务资源权限。
```

Caller 只证明平台核验过的身份关联，最长 60 秒，不是平台访问 Token 或业务资源授权。禁用后停止新签发，已签发声明在到期前仍可能被离线验签接受；需要更强即时撤销的业务必须增加在线状态检查。当前只对直接登记的业务 MCP 签发，不向第三方 OAuth 代理端点发送。

## 验证

`node --test packages/platform-server-sdk/*.test.js`：缓存隔离与合并、失败不重试、撤销清缓存、Caller 签名/issuer/audience/类型/有效期/Workspace 约束。

## SessionRelay（第二阶段）

`createSessionRelay({basePath, authenticate, resolve, maxBodyBytes})` 提供Fetch Request/Response适配。完整示例见 `examples/agent-routes.js`。authenticate必须验证业务应用自己的登录Session；resolve用该用户查询或创建固定Workspace。传入的浏览器key不直接作为平台ID。

以 `/api/agent/workspaces/me/sessions` 为根：GET列表、POST创建；`/:sid/messages`读取历史，`/:sid/turns`提交Turn，`/:sid/interrupt`中断，`/:sid/events`订阅事件，`/:sid/requests`读取问答，`/:sid/turns/:turn/requests/:rid/resolve`提交回答。所有Session操作先确认其属于resolve得到的Workspace。创建body仅取title，Turn仅取message/idempotency_key；浏览器的workspace_id、user_id和凭证不转发。

POST要求Origin与Request URL同源；反向代理/框架适配必须正确构造外部Request URL。默认JSON上限1 MiB。错误响应只返回固定错误码，不泄漏上游正文。当前平台先创建Session，再提交Turn，再订阅events；不提供上游专有chat/pending/image接口。

`events`支持after_sequence或Last-Event-ID（前者优先）；SSE字节与event id保持不变。浏览器请求取消或响应流取消向上游传播，不自动中断正在运行的Turn。调用者取消等待共享凭证时不影响其他等待者。SDK会话方法均接受最后一个可选`{signal}`参数。

个人 Codex 登录使用同一套 Model Connection 能力。应用服务端在用户明确提供原生 `auth.json` 后导入到该应用托管用户名下，再在创建 Agent 时引用：

```js
const user = platform.as(externalId, {
  teamId, scopes: ['workspace.read', 'workspace.ensure', 'session.chat']
});
const connection = await user.createModelConnection({name: 'My Codex', auth_json: authJson});
const agent = await user.ensureAgent('assistant', {
  display_name: 'Assistant', agent_type: 'codex', model_connection_id: connection.id
});
// 替换认证需提供当前版本；不修改已有 Agent 的连接身份。
const updated = await user.updateModelConnection(connection.id, {
  name: 'My Codex', auth_json: replacementAuthJson, expected_version: connection.version
});
await user.revokeModelConnection(updated.id, updated.version);
```

列表只返回本人连接元数据；`auth_json` 只写不回显，不要将其交给 Session Relay 或记录日志。连接与 `api_key` 互斥。其它用户（包括平台交互登录用户）的连接不能直接绑定；导入到托管用户名下须由凭据所有者授权。列表要求 `workspace.read`，写操作要求 `workspace.ensure`，均须有效 Team Grant。更新和撤销使用版本 CAS，SDK 不自动重放。

已部署受管理 Runtime 的环境可运行 `examples/runtime-smoke.mjs` 做真实验收。通过 stdin JSON 提供 `base`、`app`、`secret`、`external`、`team`、`authFile`；不要把密钥写进命令行或仓库。用户映射、部门成员关系及 Team Grant 必须事先具备。脚本会导入指定的个人 Codex 认证，创建测试 Agent/Session，执行两轮推理、SSE 续接及准备阶段中断，并在 finally 撤销测试模型连接。测试 Workspace/Session 会保留；调用方负责撤销测试应用授权与停止测试工作负载。该脚本会实际调用模型，不属于默认单元测试。
