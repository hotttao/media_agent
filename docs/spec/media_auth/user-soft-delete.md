# media_auth 用户软删除与访问撤销实现规格

## 范围

本切片实现管理员软删除用户、media_auth 内凭据最终一致撤销、旧会话即时失效，以及可操作的用户管理页面。旧 control-plane 的事件消费与路由迁移属于后续 rollout 切片。

## 一致性边界

- `users` 增加 `status/auth_version/disabled_at/deleted_at/deleted_by`；`deleted` 是不可逆终态。
- 管理员不能删除自己，也不能删除最后一个 active 管理员。
- 软删除事务原子更新用户并写入 `user_access_outbox`；重复删除返回 not found，不重复递增版本。
- worker 以 `user_id + auth_version` 幂等处理 Outbox，撤销该用户全部 Service/Platform Token、OAuth authorization code、OAuth refresh token和 MCP OAuth token，成功后确认；失败记录并重试。
- Cookie/OAuth JWT 携带 `auth_version`。受保护请求比较当前用户状态和版本；用户非 active 或版本不匹配时返回 401 并清 cookie。
- Service Token introspection 仅在用户仍为 active 时返回 active。
- 用户列表保留 deleted 用户，返回 status、auth_version 与删除审计字段。

## UI

- 新增管理员用户管理页面，作为现有平台 Shell 的内建管理入口，不创建第二套 Shell。
- 页面支持列表、创建、搜索、状态展示与删除确认；非管理员不展示入口且路由拒绝访问。
- 所有用户可见文案进入 i18n 资源。

## 能力覆盖矩阵

| 能力 | Domain/读模型 | Workflow | 状态 |
|:---|:---|:---|:---|
| 用户软删除且保留历史 | auth.User | User Soft Delete | covered |
| 防止删除自己与最后一个 active admin | auth policy/UserRepo | User Soft Delete | covered |
| 用户状态与 Outbox 原子提交 | auth.User + UserAccessOutbox | User Soft Delete | covered |
| 本地凭据撤销与失败重试 | service_token/oauth/mcp_oauth | User Soft Delete | covered |
| 旧 Cookie/OAuth JWT 即时失效 | auth.Session + middleware | User Soft Delete | covered |
| Service Token 随用户删除失效 | service_token + auth.User | User Soft Delete | covered |
| 管理员用户管理页面 | ui_platform user domain/page | User Soft Delete | covered |
| Go HTTP E2E | media-cli/e2e | User Soft Delete | covered |
| 真实 media-cli 进程 E2E | media-cli user commands | User Soft Delete | covered |
| 浏览器页面 E2E | ui_platform Playwright | User Soft Delete | covered |

## 验收结果

- `media_auth go test ./...` 与 `make arch-check` 通过。
- `media-cli go test ./...` 通过；强制非缓存 `TestUserE2E` 证明软删除后旧 Cookie 立即 401 且审计行保留。
- 独立真实 `media-cli` 进程完成 login/create/delete/list，列表返回 `deleted/auth_version=2/deleted_by`，数据库确认 Outbox delivered 且 active token 为 0。
- `ui_platform` 单元测试、依赖检查、生产构建和完整 11 条 Playwright E2E 通过。
