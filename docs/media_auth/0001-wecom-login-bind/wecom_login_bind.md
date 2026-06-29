---
name: wecom_login_bind
desc: 企微 corp OAuth 登录 / 绑定流程——wecom 通道驱动 identity 映射与 auth session 签发
---

## 一句话说明
用户经企微扫码完成登录（state=login：查绑定→签 session cookie）或绑定（state=bind：手动验 cookie→写 identity 绑定）；wecom domain 主导，identity 提供映射/写入，auth 提供 session 签发与 platform token ensure。

## 执行时间线（每个接口内嵌内部处理）

```text
browser            media_auth                                    企微 corp API
   |                  |                                              |
   |-- GET /wecom/enabled -->|                                       |
   |<-- {enabled} -----|                                              |
   |-- GET /wecom/authorize?mode=login/bind -->|                     |
   |   wecom.GetAuthorizeUrl                                          |
   |   ├─ 校验 WECOM_CORP_ID/SECRET/AGENT_ID 齐备                     |
   |   └─ 拼 authorize URL(state=mode)                                |
   |<-- {url} --------|                                              |
   |-- 跳转企微扫码 --->|                            <---- 企微扫码 ----|
   |                  |                              |-- gettoken -->|
   |                  |                              |<-- corp_token -|
   |<==== 企微重定向 callback?code=&state= ====|                     |
   |-- GET /wecom/callback?code=&state= -->|                        |
   |   wecom.HandleCallback                                           |
   |   ├─ corp_token = getCorpAccessToken()  ────── gettoken ──────>|
   |   ├─ userid = 企微 auth/getuserinfo(code) ─── getuserinfo ───>|
   |   ├─ switch state:                                              |
   |   │   login:                                                    |
   |   │     identity.findUserIdentity('wecom', userid)              |
   |   │       无 → redirect /login?error=not_bound                  |
   |   │     查 users；system→system_account；无→user_not_found      |
   |   │     auth.issueSession(user) → Set-Cookie token(JWT 24h)     |
   |   │     service_token.ensurePlatformToken(user.id) [fire&forget]|
   |   │     → redirect /                                            |
   |   │   bind:                                                     |
   |   │     手动验 cookie JWT（callback 在 skip list）              |
   |   │       无效 → /login?error=auth_required                     |
   |   │     企微 user/get(userid) → display_name/metadata           |
   |   │     identity.bindIdentity(sub,'wecom',userid,...)           |
   |   │       已绑别人 → /?wecom_bind=already_bound                 |
   |   │       成功 → /?wecom_bind=success                           |
   |   └─ 上游异常 → /login?error=oauth_failed                       |
   |<-- 302 ----------|                                              |
```

## 关键流程参数传递
- `state`（login|bind）：在 authorize URL 中下发给企微，callback 原样回传，media_auth 据此分支。control_plane 现状 state 即 mode 字符串，media_auth 沿用（未引入额外 CSRF 随机态，保持兼容；如需增强可在 P1 加随机 state + 映射表）。
- `code`：企微一次性授权码，仅用于 `auth/getuserinfo` 换 userid，不落库。
- bind 态 `sub`：因 callback 在鉴权 skip list，不能依赖 middleware 注入，须在 handler 内手动 `verifySession(cookie)` 取得 `sub`，再传给 `identity.bindIdentity`。
- 登录态签发的 cookie `token` 与本地/LDAP 登录同源同密钥，control_plane 网关可本地验。

## 状态流转
本流程无持久状态机；`user_identities` 的存在性决定 login 成败：
```mermaid
stateDiagram-v2
    [*] --> Scanning: GetAuthorizeUrl
    Scanning --> Bound: login + identity 命中 → issueSession
    Scanning --> NotBound: login + identity 缺失 → /login?error=not_bound
    Scanning --> Bound: bind + bindIdentity 成功
    Scanning --> AlreadyBound: bind + (provider,external_id) 已绑别人
    Bound --> [*]
    NotBound --> [*]
    AlreadyBound --> [*]
```

## 幂等设计
- `bindIdentity` 幂等：`ON CONFLICT (user_id, provider) DO UPDATE`，重复绑定同 provider 只更新 display_name/metadata，不报错。
- `ensurePlatformToken` 幂等：部分唯一索引保证每用户至多一个 active platform token，重复触发直接返回现有。
- 企微 `code` 一次性：callback 重复点击因 code 已被企微消费会失败，由企微侧保证，media_auth 不缓存 code。
