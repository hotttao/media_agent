---
name: wecom
description: 企微 corp OAuth 登录/绑定通道，编排 identity 与 auth(session)
dependencies:
    - identity
    - auth
related_workflow:
    - wecom_login_bind
---

## 一句话职责
wecom 负责企微 corp OAuth 流程（corp 凭证获取、扫码 URL、callback），在 login 态下解析企微身份并签 session、在 bind 态下写身份绑定，是企微这个 provider 的登录通道实现。

## 边界

### 做什么
- 探测企微登录是否配置（`WECOM_CORP_ID/SECRET/AGENT_ID` 三者齐备）。
- 生成企微扫码授权 URL（区分 `mode=login` / `mode=bind`）。
- 处理企微 OAuth callback：`state=login` 解析 userid→查绑定→签 session；`state=bind` 解析 userid→查/建绑定。
- 获取并缓存企微 corp access_token（2h TTL，内存缓存 + 1min 安全余量）。
- 调用企微 API：`gettoken` / `auth/getuserinfo` / `user/get`。

### 不做什么
- 不直接写 users 档案改密/偏好（归 [auth](../0001-auth/auth.md)）。
- 不维护身份绑定表结构（归 [identity](../0002-identity/identity.md)），仅通过 identity 内部能力读写。
- 不签发 OAuth Access/Refresh Token（归 [oauth](../0005-oauth/oauth.md)）。
- 不承载其他 provider（Google/GitHub）通道；新增 provider 新建 domain。

## 能力清单

| 能力 | 说明 | 对外接口/函数 |
|:---|:---|:---|
| 探测启用 | 返回企微是否配置 | `GetEnabled` `GET /api/auth/wecom/enabled` |
| 授权 URL | 返回扫码登录/绑定 URL（带 state） | `GetAuthorizeUrl` `GET /api/auth/wecom/authorize` |
| OAuth 回调 | login 签 cookie / bind 写绑定，302 回前端 | `HandleCallback` `GET /api/auth/wecom/callback` |
| 获取 corp token（内部） | 取/缓存企微 corp access_token | `getCorpAccessToken()` |

## 数据模型

### Tables
无持久化表。运行态：corp access_token 内存缓存（进程级，TTL=2h−1min）。身份持久化委托 [identity](../0002-identity/identity.md)。

### UUID 生成规则
无。`state` 参数为 16 字节随机 base64url，区分 login/bind 并防 CSRF（control_plane 现状 state 即 mode 字符串，media_auth 保留同语义）。

## 能力详细说明

### GetEnabled
```
1. enabled = WECOM_CORP_ID && WECOM_CORP_SECRET && WECOM_AGENT_ID 均非空
2. 返回 { enabled }
```

### GetAuthorizeUrl
```
1. 入参：mode ∈ {login, bind}（默认 login）
2. 若未配置 → 400
3. url = 企微 oauth2 authorize URL?appid=CORP_ID&redirect_uri={origin}/api/auth/wecom/callback&state={mode}&...
4. 返回 { url }
```

### HandleCallback
```
1. 入参：code, state ∈ {login, bind}
2. 缺 code → redirect /login?error=missing_code
3. corp_token = getCorpAccessToken()
4. userid = 企微 auth/getuserinfo(code)
5. switch state:
   case login:
     - identity.findUserIdentity('wecom', userid) → 无 → /login?error=not_bound
     - user_id → 查 users；system 账号 → /login?error=system_account；无 user → /login?error=user_not_found
     - auth.issueSession(user) → ensurePlatformToken
     - redirect /
   case bind:
     - callback 在鉴权 skip list，需手动验 cookie JWT；无有效 token → /login?error=auth_required
     - userinfo = 企微 user/get(userid)
     - identity.bindIdentity(user.sub, 'wecom', userid, display_name, metadata)
       → already_bound → redirect /?wecom_bind=already_bound
       → success → redirect /?wecom_bind=success
6. 任意上游异常 → /login?error=oauth_failed
```
> 成功路径由 handler 直接 `c.Redirect` 302；proto 的 `HandleCallbackResponse.redirect_to` 仅在纯 JSON 回调场景消费。

### getCorpAccessToken（内部）
```
1. 缓存命中且未到 (exp - 60s) → 返回缓存
2. 否则 GET qyapi/cgi-bin/gettoken?corpid&corpsecret → 缓存 access_token (TTL=expires_in-60)
3. 返回 access_token
```

## 关联关系

### 依赖的 domain
- [identity](../0002-identity/identity.md) — 登录查绑定、绑定写入
- [auth](../0001-auth/auth.md) — 登录成功签 session + ensurePlatformToken

### 参与的 workflow
- [wecom_login_bind](../0001-wecom-login-bind/wecom_login_bind.md) — 作为登录通道主驱动

### 关联术语
[WeCom Login Channel](../CONTEXT.md)、[Provider](../CONTEXT.md)、[Session](../CONTEXT.md)
