# 浏览器认证流程

## 登录与会话恢复

```mermaid
sequenceDiagram
    participant B as Browser
    participant KUI as ui_workflow Auth UI
    participant K as Kratos
    participant E as Traefik + Oathkeeper
    participant A as media_auth
    B->>KUI: GET /login 或 /registration
    KUI->>K: 创建 browser flow，按 ui.nodes 渲染并提交
    K-->>B: httpOnly Kratos Session cookie + return_to
    B->>E: GET /api/auth/me
    E->>K: sessions/whoami
    E->>A: Internal Identity JWT + original Cookie
    A->>K: sessions/whoami，校验 JWT subject 一致
    A->>A: 映射 Kratos Identity -> active User
    A-->>B: 业务 User Profile
```

未认证或业务 User 为 disabled/deleted 时，Shell 不渲染业务 App。401 发起新的 Kratos browser login flow，`return_to` 只能是当前同源的绝对 URL。

## 登录与注册页面归属

- `ui_workflow` 只拥有 `/login` 与 `/registration` 的视觉和交互呈现，样式与业务 Shell 解耦。
- Kratos 仍拥有 flow 生命周期、字段定义、CSRF、校验消息、分步注册和 session；前端不得复制身份规则或绕过 `ui.nodes`。
- 页面从同源 `/self-service/{login|registration}/flows` 读取 flow，并且只向同源 `/self-service/{login|registration}` action 提交。
- 登录与注册互相提供入口；登录保留密码恢复入口。窄屏只展示认证卡片，宽屏可展示简洁的产品说明，但不加载业务 Shell。
- 已登录用户完成或重新进入认证 flow 后回到同源业务首页；外部 `return_to` 必须降级为 `/`。

## 登出

Kratos browser logout 是两步协议：Shell 先请求 `/self-service/logout/browser` 获得一次性 `logout_url`，校验 URL 与当前 origin 相同且 path 为 `/self-service/logout`，再由浏览器导航执行。成功后回到 `/login`；仅清理旧 `mw_token` 或前端内存不算登出。

## 恢复与错误状态

- `/recovery`、`/settings`、`/verification` 暂由 Kratos Login UI 承载；验证码和恢复凭证不进入业务前端。
- 创建 flow、提交凭证或登出失败时保留明确错误，不假装本地状态已完成。
- loading 期间显示认证加载态；401 显示/跳转登录；403 的 disabled/deleted 业务状态不得无限重试。
