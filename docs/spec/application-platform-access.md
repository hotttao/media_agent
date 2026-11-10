# 应用代表用户访问平台

Auth持有应用部门Grant与短期凭证；Control持有Application Agent Binding；Gateway复用既有Session/Turn流程。没有新增服务。用户加入Organization/Team由既有管理员成员管理API完成，应用注册和upsert不自动授予成员或admin角色。

接口：Auth管理员PUT/DELETE /api/applications/:id/teams/:team_id；原token接口增加team_id和scopes（缺省保持profile）。POST /internal/apps/verify只给可信Control/Gateway验证，严格校验非空MEDIA_AUTH_INTERNAL_TOKEN；返回app/user/external/team/org/scopes/expiry，不包含secret。Oathkeeper只开放/app协议指定业务路径，原始短期Bearer由接收服务经Auth在线验证。普通Cookie、Basic、Agent身份不能代替此凭证。

Control PUT /apps/v1/agents/:name 幂等创建Workspace+配置+初始metadata+绑定，GET读取同一绑定；请求里的组织/用户/Team只能使用验证后主体。key=(app,user,team,name)，同键不同创建参数409；事务级key锁保证并发唯一，绑定/Workspace/metadata同库同事务提交，不嵌套新事务。运行上下文仍由Gateway通过既有内部协议读取，密钥不返回外部应用。

Gateway /apps/v1/chat/sessions 及消息/turn/interrupt/events/requests接口采用专门应用验证与绑定校验，再复用Chat用例。每次读取session都确认User所有权及Control当前绑定和部门权限；不提供默认会话回退。应用不能指定别的应用Workspace或通过同部门成员身份绕过绑定。Turn沿用业务idempotency_key，Session创建结果不明通过列表恢复，不自动重试创建。

每次Auth校验检查Grant与成员事实；Control及Gateway继续检查资源和runtime权限。流程网络失败503、身份错误401、权限不足403、资源不可见404；不在失败时回落Cookie或mock workspace。SDK自动缓存profile/业务token的键包括user、team及scopes，401清缓存而不自动重试写命令。

MCP Caller：管理员为应用登记唯一 MCP Definition ID 与精确 HTTPS origin；Gateway只在已验证 managed_agent 的Session/Agent身份上签发，使用Control返回的Team事实向Auth查询当前应用映射、授权与成员权限。第三方OAuth代理不携带Caller。每次上游请求在线解析，注册目标身份不满足时失败关闭；未注册目标不携带Caller。使用独立持久化RSA密钥、专用typ与issuer、应用aud、60秒有效期，声明sub/ext/workspace/session；SDK只验可信配置的JWKS，业务仍检查本地用户、workspace绑定及资源权限。该声明不是平台API令牌，撤销后不再签发，已签发声明最长60秒。
