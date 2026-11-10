# 平台应用接入实现规格

本阶段实现独立应用协议，不提供上游SDK完整兼容。服务边界见 application Domain；用户工作流必须在Auth本地事务完成。

## 契约与模型
正式接口与字段以 `idl/media_auth/application.proto` 为源，与 [Domain](../../ddd/media_auth/domain/0011-application/application.md) 能力清单一致。管理端使用现有Session且要求admin；/apps/v1独立认证，不回退Cookie。Basic ID由凭据解析，路径external_id只作为该应用的键。

applications、application_access_credentials以及users.managed_by通过0018增量迁移。托管User增加数据库CHECK：只能role=user，不允许password_hash/kratos_identity_id。username使用平台随机ID生成，不能用外部邮箱合并。identity命名空间 app: 仅通过受限创建写入。

Application Access Credential采用32随机字节 opaque 值，前缀mac_，有效期固定3600秒，DB保存SHA256与应用/用户版本；应用secret前缀mas_。这不是Talos长期API Token，也不是上游JWT。GET /apps/v1/me为唯一接受入口，普通Session/Internal JWT入口继续拒绝。每次读当前应用/User状态及版本，因此不用额外撤销投影或新签名配置；依赖故障503，凭据错误401。凭证响应使用Cache-Control:no-store与Pragma:no-cache；错误不输出secret、hash、SQL或请求原文。

## 实现映射
- domain/application：Application、Credential、仓库接口与不变量。
- service/application：应用CRUD/secret/hash/凭证用例，仅本领域，不依赖其他service。
- service/auth/managed_users：托管User读写能力；service/identity提供专用创建能力。
- workflow/application_users：锁Application→查Identity→锁/创建User→创建Identity→提交；签发同样校验关联。
- dal/repo/application、auth：原生GORM/SQL持久化，透传事务context；不在handler/workflow写SQL。
- handler/application：DTO/Command及Basic/Bearer协议转换；IDL生成路由，main装配独立runtime。

## 迁移与限制
无现有用户回填、无自动应用注册，无团队/Workspace/Agent隐含开通。生产部署先加表/字段再部署代码。存在托管用户时禁止down迁移（不能移除托管标记使其成为可登录用户）；须先显式处理数据，不自动删除。轮换/停用并发通过应用行锁串行；同一应用的provisioning暂采用串行锁保证简单可验证的一致性。

本阶段不实现组织授权配置、SDK、ensureAgent、MCP Caller、通用业务API交换；后续扩大入口必须另行定义audience/scope及资源授权契约，不能把本凭证视为通用User权限。

## Ory 接入
现有 ADR-0012 继续有效。应用管理走专用 Oathkeeper cookie_session 规则，使用 Kratos Session 认证并签发原有 internal-api 断言；Auth 解析 Kratos ID 为业务User并读取管理员身份，应用 service 再检查admin。注册应用、轮换和停用不接受应用Basic或mac_凭证。

/apps/v1 的三个明确路径族通过 Oathkeeper 的 noop authenticator/mutator保留原始凭证，由Auth协议入口验证。noop仅表示边缘不把应用协议误解释成浏览器用户，不表示后端免认证。普通业务规则与内部接口不增加这个例外；请求伪造内部身份头不影响应用协议的Basic/Bearer校验。当前部署示例固定host沿用已有mini-001.local:18080；生产需换成对应HTTPS域名并限制Auth内部监听可达性。

Keto不因创建应用用户自动得到成员授权；只有既有Organization/Team成员命令与Outbox产生关系投影。应用凭证扩展到业务API之前，必须新增受控introspection、明确scope/audience/application_id，以及下游当前权限检查，禁止复用当前普通用户入口直接放行mac_。
