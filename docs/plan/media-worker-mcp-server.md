# 媒体 MCP 落地计划（实施中）

对应 [完整规格](../spec/media-worker-mcp-server.md)及 [设计审查](../review/media-worker-mcp-server.md)。2026-10-28 用户已确认，开始实施；按阶段记录真实完成项。

## P0 · 设计确认

- [x] 对照 Harness、Gateway/Plugin、Auth/Ory/Keto、Control、Workflow API 与资源权限实现复核旧稿。
- [x] 区分已确认需求与新增技术提案，重新编写 spec；撤掉回调代理和分级加载。
- [x] 明确统一授权事实、企业 Template 与 Workspace Workflow/Job 边界、写入失败和撤权窗口。
- [x] 用户要求所有媒体 MCP 工具统一批量接口，已同步 spec。
- [x] 用户确认 spec 第 10 节的具体提案并授权开始开发。
- [x] 确认后同步既有 MCP 调用、API Token、资源委托相关 DDD Workflow/change 文档；无需新建 MCP Domain。

验收：没有“实现时再决定”才能闭合的身份链路；先确认可信 Worker 的转换边界，不将其误称为最终 HTTP 请求的端到端签名。

## P1 · 公网身份与当前授权

依赖 P0。

- [x] Auth 支持 User mcp.access scope；页面签发入口可选，旧 Token 不自动扩权。
- [x] 配置公网 /mcp 的 Ory introspection、独立 audience、Header 清理和路由；保留托管内部入口。
- [x] Gateway 增加明确 User 身份分支与组织/可选 Workspace 选择，原 Agent/Session/epoch 校验保留。
- [x] 通过现有 Auth/Control 接口取当前模块授权；policy 仅作事实交集；不可用时拒绝。
- [x] User 不暴露 Session/Skill 平台工具；托管原平台能力回归。

验收：无效/过期/撤销 Token、Worker Token、缺 scope、组织不匹配均失败；mcp.access 不提升模块权限；不依赖异步 Keto 投影保证撤权。

## P2 · 直接委托与目标服务接入

依赖 P1 的 principal 合同。

- [x] hertz_infra 增加独立 mcp_delegation 类型和验签；保持旧 Agent 请求绑定协议不变。
- [x] Gateway 按 MCP 请求签发后端断言及按需目标 API 委托；无共享 Header/Token 缓存。
- [x] Worker 验证 method/工具/参数摘要、上下文一致性，按目标使用专用委托。
- [x] Workflow/Materials 新认证分支同时验证受众、期限、presenter 服务凭据和允许操作路由。
- [x] 将 User 和 Agent 委托的组织/Workspace 限制进入现有 handler Command、列表查询和最终对象 ACL。
- [ ] 配置私密签名密钥/JWKS、目标独立服务凭据、内网地址；完成受众隔离与撤权时效测试。

验收：Worker 直接调用业务 API；业务请求没有回 Gateway 的路径；无原始 User Token 转发；可信 Worker 转换范围有测试，不能越权改调用主体或调用 Worker/admin API。

## P3 · 来源与标准发现

依赖 P1；可在 P2 期间准备来源合同。

- [ ] 注册平台 media-workflow Definition 和内部业务来源映射；无需用户另建 Binding。
- [x] Gateway 连接前过滤模块权限，来源/运行限制取交集；媒体未授权时完全不连接。
- [x] 保留标准 tools/list 分页、稳定命名和排序；补齐 User 身份、范围和目录版本 cursor 绑定。
- [x] 每次调用重验，不把发现缓存作为权限；保留已有托管 Binding/OAuth 路径。
- [x] 保持无分级发现、元工具、Agent 动态注入和媒体 resources/prompts 的范围。

验收：普通客户端只靠标准 MCP 完成发现与调用；旧工具名和跨用户 cursor 不能绕过撤权；来源故障与权限为空有明确区别。

## P4 · 媒体工具实现

依赖 P2/P3。

- [x] 独立 media_worker_mcp 命令与配置；SDK 适配在 hertz_infra，handler 只作协议转换。
- [x] spec 第 7 节 13 个工具的统一 items/results 批量 Schema、逐项输出、只读提示与单项 HTTP 映射。
- [x] `jobs_submit` 批量异步提交：成功项返回 `job_id/status`；不等待生成；使用 `jobs_get` 批量查询。已覆盖部分失败保留 ID、回执缺失标记未知、重复幂等键预检和输出 Schema。
- [x] 通过已有业务 API 完成模板/工作流创建、明确发布及 Job 提交，不增加自有事实表。
- [x] 当前已发布版本锁、能力范围 policy、对象引用授权解析、结果链接部分失败语义。
- [x] 批次上限/列表总量、批内标识、写入串行/读取有界并发、结果顺序、逐项授权与部分失败。
- [x] 每项独立 Job 幂等键、规范化对象输入、所有写请求禁自动重试及 outcome_unknown 行为。
- [x] 整批期限/响应预留、未发送项回执和断线后的未知结果处理；禁止整批盲重试。
- [x] 停止 MCP 进程不影响独立生成 Worker；NEED_CONFIRM 保留现有页面处理。

验收：输入不能伪造身份、URL/本地路径或产品快照；发布异步结果如实返回；无 Workspace 可管理企业模板，创建 Workflow/提交 Job 需 Workspace。

## P5 · Review、集成与真实验收

依赖 P1–P4。

- [x] 对照 spec 验收矩阵完成协议、权限、委托、业务幂等测试。
- [ ] 相关模块 go test、必要 race/vet、archcheck；IDL/config 生成一致性检查。
- [ ] 验证既有 Agent API 逐请求签名、Session 固定版本、外部 OAuth/Binding、网页 ACL 无回归。
- [x] 检查跨企业/Workspace 列表过滤发生在分页前，total 没有泄露。
- [x] 普通 MCP 客户端以真实 User Token 完成 Template/Workflow/Job 全链路；另以真实页面签发 Token 验证 MCP 接入与撤销。
- [x] 在确认对应账号/Worker 在线后，GPT 和豆包各一个真实任务，记录 Job ID、产物、重试次数，确认没有重复生成。
- [x] 更新设计实现差异与验证证据；区分协议通过、真实 Job 成功和浏览器生成成功。

发布顺序：先部署目标服务新验签分支与内网 MCP，再登记来源，最后开放公网入口。回滚停用来源/公网路由即可阻止新 MCP 请求；不得删除已创建 Job、签发历史或重置生成检查点。

### 2026-10-28 Job 提交合同验证

`media_worker` 全量 `go test ./...` 与 archcheck 通过。批量提交测试证明仅调用提交 API，不轮询生成；查询通过独立批量 `jobs_get` 进行。成功项输出 Schema 强制非空 `job_id` 与 `status`，响应缺失不当作成功、不自动重试。此记录仅证明本地实现与测试，P1–P3 网关认证、来源登记和公网部署仍待集成验收。

### 2026-10-29 本地实现与 Review

本小节记录部署前状态：P1–P4 勾选项表示代码/配置实现及本地验证。后续已部署并继续 P5，最新进展见下方及 deployment 文档。

本地通过：Worker、Gateway、Workflow、Materials、Plugin 全量 Go tests；Auth API Token/service workflow tests；独立 hertz_infra 认证/协议/上游测试；Worker 批量/协议及 Gateway MCP 的 race；UI Platform 15 文件/45 tests、TypeScript、生产构建、依赖检查。Compose 配置校验通过。Worker/Materials/Auth archcheck 通过；Gateway/Workflow 的既有跨层依赖未在本次扩大，见实现审查记录。部署步骤见 [deployment](media-worker-mcp-deployment.md)。

### 2026-10-29 运行环境验收进展

专用密钥/服务凭据、平台来源、目标服务、Gateway、独立 MCP Worker、Edge 路由与 Token UI 已部署。使用指定 User 正常业务 API 签发 Token，公网完成 13 工具发现、模板/工作流创建发布、批量异步 Job 提交与查询。GPT 已真实生成并通过 MCP 下载校验；materials 经用户批准开通后实时生效。豆包首次首帧上传失败且未发送平台生成请求，通过原 Job 重试后生成成功，MCP 下载的 MP4 已验证 SHA256、720×1280、约 5 秒；StepRun 重试 1 次，平台发送 1 次。完整 Job ID 与产物证据见 deployment 文档。负向鉴权、Workspace 前置条件及真实 Token 撤销已通过。

已补做真实浏览器 Token 签发、关闭明文、刷新与撤销验收，并修复 HTTP 上 randomUUID 不可用导致的空白。页面临时 Token 撤销前后 MCP 分别 200/401。P5 未勾选项保留其完整要求；历史 archcheck 问题仍按 review 记录，不伪报全量通过。
