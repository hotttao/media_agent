# 2026-08-23

用户删除语义由物理删除改为不可逆软删除。`User` 增加 `status`、`auth_version`、`disabled_at`、`deleted_at`、`deleted_by`；删除事务同时递增 `auth_version` 并写入 `UserAccessRevoked` Outbox。已删除用户及旧版本会话不得继续访问，但身份、团队关系和历史资源保留。

旧 control-plane 接入 media_auth 的过渡契约落地：media_auth 可用 RS256 签发 Session 并发布 JWKS，同时提供受内部凭证保护的 Session introspection。当前即时撤销由远程 introspection 保证；`UserAccessRevoked` 的 Edge 事件消费者仍是后续能力，未实现前不得宣称本地验签缓存具备即时撤销语义。

# 2026-09-20

补齐 User 临时禁用与恢复：管理接口只接受 `active | disabled`，禁用与删除共用可靠撤销 Outbox，并增加 Kratos Web Session 撤销；恢复递增 `auth_version` 且不复活旧凭证。Ory browser session 请求仍在每次进入业务服务时回查业务 User 状态，从事实源即时拒绝 disabled/deleted User。

明确 Kratos `metadata_admin.role=platform_admin` 的受信任管理员引导映射；普通浏览器输入不得控制平台角色。
