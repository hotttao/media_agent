# 应用认证与平台业务接入验收

日期：2026-11-08。分支：platform。范围：002 应用认证、托管用户映射、受限业务访问、ensureAgent、应用聊天入口、MCP Caller 与服务端 SDK。代码已实现并完成下列本地验证；未部署、未迁移任何现有业务数据库。

## 能力与事实所有者

| 能力 | 所有者 | 结果 |
|---|---|---|
| Application、secret、停用/轮换、凭证版本 | Auth/application | covered |
| 托管 User、不可变外部映射、禁止交互登录与身份旁路 | Auth/auth + identity，ApplicationUsers workflow | covered |
| Team Grant、限定 scopes、当前成员/权益检查 | Auth/application，既有组织/团队服务 | covered |
| 应用 Agent 幂等绑定、配置和可选初始 Metadata 原子创建 | Control/workspace，application_agent workflow | covered |
| Session/消息/Turn/中断/SSE/AgentRequest 专用应用入口 | Gateway/application_chat 与既有 Chat/Session/Turn | covered |
| MCP Target、可信外部ID解析 | Auth/application | covered |
| 每次出站 Caller、专用签名、公钥端点、保留业务权限检查 | Gateway，applicationcaller/mcpupstream 技术协议 | covered |
| SDK token缓存隔离、并发合并、失败不重放写请求、Caller验签 | packages/platform-server-sdk | covered |
| 组织成员开通、功能权益与Keto投影 | 既有Auth组织/团队领域 | external-owner：应用注册和upsert不隐式赋权 |
| 浏览器密码、恢复与Session | Kratos，Oathkeeper边缘，Auth业务身份适配 | external-owner：实际Kratos登录链已验证 |
| 实际模型推理、K8s/Docker工作负载创建、真实业务MCP运行 | 既有运行域及业务应用 | external-owner：本次没有调用真实模型或部署真实Agent |
| Hydra标准OAuth Provider、上游应用内容发布/system owner | 独立协议及资源发布规划 | 本次应用认证协议范围外，不宣称兼容上游全部平台功能 |

## 已执行验证

- Auth、Control、Gateway：各自 `umask 022; go test ./...` 全量通过。umask用于满足既有安全文件权限测试，不修改被测安全规则。
- Auth：`go test -tags=e2e ./e2e -run '^TestApplication' -count=1 -timeout=4m` 通过，实际启动隔离PostgreSQL并执行迁移0018/0019/0020。
- `TestApplicationBusinessGrantCurrentPermissions`：Grant不创建成员、缺权益拒绝、profile不能访问业务、角色/权益/组织成员撤销即时拒绝、Grant变化撤销旧凭证、scope不可提升；MCP origin、跨应用映射及目标抢占均拒绝。
- `TestApplicationAuthControlGatewayHTTPIntegration`：Auth和Control使用独立数据库，经实际Auth内部HTTP、Control生成路由和Gateway应用授权Workflow创建/查询绑定；profile凭证和外部Workspace拒绝，撤销Grant后Gateway拒绝。Session所有权在此测试使用确定性fixture，不把它称为真实Agent推理E2E。
- `TestApplicationRealKratosOathkeeperLogin`：真实Kratos迁移、创建登录身份、浏览器登录/CSRF/Session Cookie、真实Oathkeeper、Auth和PostgreSQL；管理员可注册应用，普通成员403；错误应用Bearer不回落合法Cookie，禁用后旧凭证拒绝。另保留可控Kratos fixture的边缘测试。
- Control：设置隔离 `MEDIA_CONTROL_APPLICATION_TEST_DSN` 后运行 `go test ./e2e -count=1` 通过；12并发ensure只有一个Workspace，配置冲突409语义、跨应用/用户/团队隔离，Metadata失败后Workspace与绑定全部回滚。
- Gateway：应用路由逐个检查专用认证，即使携带Cookie也不绕过；应用授权Workflow检查Session所有权及Control绑定，不提供默认Workspace回退。Caller每次请求在线解析，映射撤销或缺少签名器不放行。
- 基础设施：`go test ./authn/applicationcaller ./authn/appcredential ./mcpupstream` 通过；验证失败关闭、禁止重定向、过期/缺失字段/scope拒绝、Caller专用claims、GET/POST/DELETE均签发且删除伪造Caller。
- SDK：`node --test packages/platform-server-sdk/*.test.js` 4项通过；缓存隔离与并发合并、撤销后清缓存、不重放写操作，以及签名/typ/issuer/audience/期限/Workspace校验。
- IDL生成DTO、router、client及OpenAPI；生成client包编译测试通过。共享注解文件仅因protoc补丁版本产生的无关变化已清理。
- `git diff --check` 与本次Go文件gofmt检查通过。

## 架构检查

Auth archcheck通过。Control/Gateway全量archcheck仍失败：Control的15个既有service包、Gateway的agent_task/batch_run仍import具体DAL（Gateway含对应测试重复报告）。逐包与Git HEAD对照确认这些依赖已存在；本次新增包没有报告违规。此次涉及的Workspace service已将dal.TxRunner依赖改为本包事务接口。

未将其余已有service整体重构混入认证迁移，也未屏蔽检查或修改规则。因而不能将Control/Gateway的全量架构检查标为通过。业务新增SQL集中在DAL，handler仅处理协议和结果映射，跨领域事务由Workflow协调，事务内不调用远程Auth。

## 配置与运行边界

- Auth迁移0018、0019、0020；Control迁移000028。User managed_by是强约束，已有托管用户时0018回退拒绝；已有Agent绑定时Control回退拒绝。0019回退撤销业务凭证。
- Control/Gateway需配置正确的MEDIA_AUTH_URL及非空MEDIA_AUTH_INTERNAL_TOKEN。Gateway还需正确MEDIA_CONTROL_URL和既有Control服务凭证/运行配置。
- Caller需独立持久化RSA私钥APPLICATION_CALLER_SIGNING_KEY_FILE（0600）及APPLICATION_CALLER_ISSUER；MCP Gateway必须启用。登记目标使用精确HTTPS origin；第三方OAuth代理不发送Caller。业务方从可信配置的公钥地址取钥，仍校验本地用户与资源权限。
- Oathkeeper示例规则域名/上游地址需适配部署环境。应用路由noop只负责透传，认证在各专用handler执行；不暴露/internal/apps/*。外部部署使用HTTPS。
- 应用凭证每次新请求在线验证；不承诺主动断开已建立SSE。Caller离线声明最多60秒，撤销后停止签发，已签声明不保证即时失效。SDK不自动重放不确定写操作。
- 未运行race（当前环境缺CGO编译器），不能把数据库并发测试当作Go竞态检测。未验证新功能的真实Keto投影全链、真实模型推理或生产滚动升级；现有Keto授权与资源服务仍执行原有权限路径。

接入示例与Caller验签见 [SDK说明](../../packages/platform-server-sdk/README.md)；设计见 [规格](../spec/application-platform-access.md)。
