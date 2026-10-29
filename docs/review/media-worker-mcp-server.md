# 媒体 MCP spec 设计审查

日期：2026-10-27。范围：设计与当前实现边界核对，不是实现验收。未运行业务测试，未启动/修改服务。产物为候选 spec 与 plan，需用户确认后实现。

## 审查结论

旧稿不能直接用于实现。它虽写出了服务列表，但身份信任链、授权事实来源与业务 API 合同没有闭合；后续局部改动未清理已失效的整体链路。本轮整体重写，不再沿用回调代理方案。

| 严重度 | 旧稿问题与证据 | 修订 |
|:---|:---|:---|
| 高 | Worker → Gateway → Workflow 回调只为复用 AgentSigner；`internaljwt/agent_delegation.go` 实际限定 Agent Actor 且绑定最终 HTTP 请求，不能凭入口 Token 直接复用 | 单向直连；独立后端/目标受众委托，保留旧签名严格性；明确 Worker 为可信转换服务 |
| 高 | 凭证 schema/有效期/接收方留待实现；没有说明目标服务为何信任 Worker | spec §5 定义签发者、受众、actor/presenter、有效期、Header、目标操作映射和验签失败策略 |
| 高 | 混用业务模块授权、Definition/Binding allowlist 与用户工具权限；只写 Ory 支持 ReBAC，没有核对落地语义 | 依据 Auth Team.Authorize 与 Control.CheckAccess 使用当前 use_workflow/use_materials；内部业务来源不要求第二次 Binding 授权 |
| 高 | 声称撤权即时影响所有请求，忽略异步 Keto 投影与已签内部 JWT | spec §4 区分当前权威事实、投影、在途检查与 60 秒委托窗口；不承诺撤销已提交 Job |
| 高 | User 无 Session 仍沿用 platform 工具、Plugin 固定快照和 Session-only cursor，未说明空身份碰撞 | User 独立认证上下文，隐藏 Session-only 能力，cursor 全身份范围绑定；第三方 User Binding 新接入移出范围 |
| 高 | 只提出 Gateway 新委托，没有列出 Workflow/Materials 的认证与资源范围改造 | spec §5/§9 明确目标服务新分支、真实 principal 注入、查询前范围限制；不能仅依赖旧 AgentPrincipal 检查 |
| 中 | Template 被统一 Workspace 授权绑住，违背 `TemplateAuthority()` 的企业资源边界 | 组织必选、Workspace 可选；Template 独立，Workflow 创建/Job 提交要求 Workspace |
| 中 | 草案暗示可提交任意已发布历史版本，实际 Job Submit 校验当前 Workflow.Version | 显式版本是当前发布版本乐观锁；禁止自动换版本 |
| 中 | 对象引用、链接刷新只有口号；当前 JobInput 是快照/URL 字段，不直接接收对象 ID | 明确 MCP object_path/product_id/virtual_ip_id → 授权 API → JobInput 的适配与部分产物访问失败 |
| 中 | 未定义首期入口 scope，api.access 是否足够、MCP 是否独立许可含混 | 提议新增 mcp.access；注明新增而非现有功能，列入用户确认与 UI/Auth 改动 |
| 中 | 持续扩展 resources/prompts、检索模块、User Binding API，与已缩减需求不一致 | 首期仅标准 tools；保持既有非媒体来源功能，删除不必要新增模块 |
| 中 | 缺少每工具 API 映射与具体重试边界 | 13 工具映射；发布与提交分开；Job 复用幂等，其他写入结果未知不盲重试 |

## 关键事实来源

- `harness/contributing/architecture.md`、`client.md`：业务层与协议/生成 DTO 隔离、policy 不做 I/O、非 HTTP Worker client 例外。
- `media_agent_gateway/biz/workflow/mcp_identity.go`、`mcp_gateway.go`：托管身份、platform 来源、固定 Binding、工具代理与 cursor。
- `media_plugin/biz/workflow/mcp_access.go`：固定 Session 绑定解析，不存在可直接给 User 复用的空 Session 模式。
- `media_auth/biz/service/team/service.go`、`workflow/authorization_projection.go`、`docker-compose/auth/keto/namespaces/media_agent.ts`：授权事实和异步投影的区别。
- `media_control/biz/service/workspace/service.go`：Workspace 功能授权委托给当前部门权限。
- `hertz_infra/authn/internaljwt/agent_delegation.go` 与 Workflow/Materials auth middleware：旧签名用途、目标服务身份边界。
- `media_workflow/biz/shared/client/resource_authority.go`：企业 Template 与 Workspace 资源范围。
- `idl/media_workflow/{template,workflow,job,capability}.proto` 与 `biz/service/job/{service,submission}.go`：接口字段、当前版本提交和幂等事实。
- `idl/media_materials/{library_object,generated_asset}.proto`：对象解析与资产结果字段。
- 本地 WeKnora `internal/agent/tools/{mcp_catalog,mcp_exposure,mcp_tool}.go`：运行时分级加载参考；按用户决定不纳入本期。

## 候选方案的明确取舍

1. 目标委托限定调用身份、组织/Workspace、操作和有效期，不宣称绑定 Worker 转换后的 HTTP body。信任 Worker 转换是必须由用户评审的边界。
2. 现有 Auth 的细粒度 feature key 会折叠成模块权限。网页/MCP 或只读/创建分离需后续单独扩展授权模型，本期不假装已经具备。
3. 组织必选、Workspace 可选是接入合同；跨 Workspace 查询仅返回原资源授权允许的对象。托管 Agent 固定 Workspace 不放宽。
4. 无 Workspace 的 User 只接入本期平台内部业务来源；没有顺带提供任意第三方 MCP 的 User 代理。
5. 没有新增业务数据库，不因 MCP 工具注册创建 Domain。未确认的身份委托设计没有写入正式 DDD 目标文档。

6. `library-objects:resolve` 当前是企业成员与企业路径授权，并非逐文件 ACL；spec 明确保留该现有语义。不能将路径解析宣传为对私有资产的 Resource Grant 检查。

## 批量接口调整（2026-10-28）

按用户要求，所有 13 个媒体工具统一 items 输入与逐项 results 输出，包含列表查询与能力查询。下游业务 API 保持单项合同；批量由应用 Workflow 编排，无跨项事务或自动回滚。审查重点新增：全批结构预检、单项 ACL、有限并发、结果稳定顺序、逐项幂等、部分成功、总期限和断线后的未知结果。单项调用也走数组，不引入两套工具。旧 platform/第三方来源接口保持兼容；标准 tools/list 协议不变。

## 自检与限制

已逐项检查 spec/plan 的范围、引用、旧方案残留及工具数量。尚未实现认证代码、部署配置或工具 Schema，因此本审查不能证明代码安全或真实链路已可用。实施必须按 spec §10 和 plan P5 验证，特别覆盖两个目标服务的当前资源授权、Token 撤销时效和 Job 不重复生成。

## 2026-10-29 · 实现 Review 与本地验证

本节为确认后的实现审查，前文保留为设计历史。本节记录部署前审查：独立 media_worker_mcp、13 个批量工具、User/托管身份、受众隔离的直接委托、业务 API 接入与个人 Token 设置入口均已实现。后续实际部署和真实验收见文末及 deployment 文档。

本轮修复：

- 未选 Workspace 的 Job 列表先读取当前可用 Workspace ID，再进入数据库过滤/计数/分页，空权限直接空列表，授权查询失败关闭访问。
- Materials 委托使用当前组织与实时团队关系进入原对象 ACL，避免团队共享失效或跨组织读取。
- Backend assertion 与目标服务 delegation 使用不同 audience；目标 API 同时检查专用服务凭据和操作路由；整体 admission 失败停止余项，不重试写请求。
- 固定配置停用的 Binding 通过 Plugin restricted_definition_ids 明确传递；自动业务来源不能绕过该停用，也不能静默切换固定 Definition 版本。
- 产品/虚拟形象按当前对象路径字段读取并解析图片，asset_ref 图片槽位禁止 CONST URL 或 content 替代，检查解析 MIME。
- jobs_submit 成功项强制返回 job_id/status，缺失回执标记未知；已成功项保留，生成状态由独立 jobs_get 查询。
- 产物/候选仅返回重新授权解析的链接；解析失败保留 Job/资产 ID，不透传旧签名 URL。
- MCP 公钥发布使用独立内网可访问的 `/.well-known/mcp-jwks.json`，避免误挂到要求服务凭据的目录接口组导致验签客户端无法读取。
- User 主体使用中立 BusinessPrincipal；旧 Agent 请求签名/验签合同没有放宽。

| 验证 | 结果与边界 |
|:---|:---|
| Worker、Gateway、Workflow、Materials、Plugin 全量 Go tests | 通过；不等于真实业务服务集成 |
| Auth Token service 与 workflow tests | 通过；mcp.access 必须显式选择且仅限 User |
| 独立 hertz_infra authn / mcpprotocol / mcpupstream tests | 通过；真实 RSA 验签及 SDK 协议适配 |
| Worker HTTP MCP 协议测试 | 真实 SDK 初始化、发现 13 个工具、批量返回两个 Job ID；业务 API 为测试替身，没有运行浏览器 |
| Gateway 权限与旧工具名测试 | 连接前过滤、撤权拒绝、来源故障区分、Binding 停用和工具限制通过；授权服务为测试替身 |
| Target admission 测试 | 服务凭据、错误受众、实时撤权与授权服务故障关闭访问通过 |
| Worker/Gateway MCP race | 通过 |
| UI Platform | 15 文件/45 tests、TypeScript、构建及依赖检查通过；未计作已登录页面真实签发 |
| Compose config 与 diff/gofmt | 通过 |

架构检查：Worker、Materials、Auth 通过。Gateway 仍有 agent_task/batch_run service 引用 DAL，Workflow 仍有 job/template/workflow service 引用 DAL/编译工作流；已用 `git grep HEAD` 核对这些 import 在本次修改前存在，本次没有扩大这些依赖。不能将整体 Gateway/Workflow archcheck 标成通过；相关旧架构问题留在原模块范围处理。

实现差异：能力列表通过三个精确版本查询获得结果，而非扫描全部能力分页；减少无关读取，缺失版本跳过、不可用报错，再对可用集合分页。技术来源校验继续放在既有 Gateway Workflow 编排中，没有为少量协议分支新建业务 Domain 或强造 policy 包。两项均已同步 spec。

本节结束时待做的部署验收已继续执行，最新结果见后续记录；不能将本节测试替身结果计作真实生成。

## 2026-10-29 · 公网部署与真实验收补充

实际部署与对象 ID、产物校验见 [deployment](../plan/media-worker-mcp-deployment.md)。标准 MCP 全链路已部署，使用指定真实 User 正常登录签发的 mcp.access Token，通过 Edge、Gateway 和独立 MCP Server 执行业务操作。13 工具发现、Template/Workflow 创建与发布、异步批量提交和查询通过。主 Token 保留；另签临时 Token 验证撤销后下一次公网请求 401。

materials 未授权时，豆包项在业务派发前明确拒绝，GPT 项仍保留成功 Job ID；GPT 生成完成后，查询保留 Job/资产 ID 并拒绝产物链接。按用户批准追加企业 materials 授权后，未改 User 角色、未重启服务，原 GPT Job 的 MCP 查询即返回有效产物链接；下载校验和与原产物一致。此项证明授权变化实际生效，不以测试替身代替。

验收修复：上游 RPC 参数/权限拒绝曾被合并为 outcome_unknown，导致缺少 Workspace 的未派发请求被错误提示为写入结果未知。基础设施仅保留 -32602/-32601/-32001 错误码，不传递远端 message/data；Gateway 映射为参数/权限错误，其他异常继续未知、禁止重试。真实 SDK 测试验证非法批次不调用业务 API，Gateway 回归验证确定拒绝分类；公网无 Workspace 提交已复验通过。

本轮修改后 Gateway workflow/client、Worker e2e 全量测试、独立协议/上游测试及定向 vet 通过。既有未知写请求不重试测试继续通过。随后完成真实隔离浏览器验收：HTTP 环境 randomUUID 导致空白的问题已修复；页面创建 201、显式 mcp.access、关闭与刷新丢弃明文、本地存储无 Secret、页面撤销 200，MCP 由 200 变为 401。主 Token 保留。新增组件回归通过，TypeScript、生产构建与依赖检查通过。

真实生成最终结果：GPT 与豆包 Job 均 SUCCEEDED；两者均从 MCP 新签发的链接下载并核对大小、SHA256。豆包 MP4 经 ffprobe 检查为 H.264/AAC、720×1280、5.056009 秒，已检查视频首帧。GPT 原生 StepRun retry_count=0；豆包 retry_count=1，首次尝试没有平台 submittedAt，重试仅一次平台提交并产出一个视频，不存在重复生成。临时诊断实例在任务完成后退出，恢复不开 CDP 的常规 Studio/Worker。原 Studio 仓库没有代码变更。

P5 边界：新功能真实公网与浏览器生成链路已验收；已有 Gateway/Workflow 架构检查失败仍在历史模块范围。既有托管 Agent/OAuth/Binding 的自动化回归通过，但本次没有另外执行第三方 OAuth 提供方的真实交互验收，不将其标为完成。
