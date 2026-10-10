# Workflow 查询补齐与 standalone 联调

目标：先补接口再联调，不接 media_platform。Backend 契约见 [spec](../spec/media_workflow/ui-query-support.md)，UI 沿用 v1.2 与 Slot。当前目标保持全部阶段，不以一个接口或只读测试通过替代完工。

## P1 查询接口（先于 UI 联调）

- [x] Capability 发布目录/指定版本及网关路由（源码，部署另验）。
- [x] Template/Workflow 当前权限动作投影与 owner-only Grants（源码与单测）。
- [x] Workflow 不可变版本存储/读取、当前记录回填、旧编译可证实读取。
- [x] IDL、Hertz model/router/client、OpenAPI 同步生成。
- [x] 单测、race、arch-check、真实隔离PG迁移/事务/权限门禁。
- [x] 部署服务与网关，新接口真实 HTTP 及独立 CLI 门禁。

## P2 UI 接入

- [x] Capability 目录/版本选择及契约展示，无内部路由字段。
- [x] owner/editor/viewer 动作按服务端投影，完整授权读取/撤销。
- [x] 执行拓扑、Resume/Rerun 输入从指定冻结版本读取，不用新头契约。
- [x] 中英、加载/失败/无权限、窄 Slot、实例草稿；保留旧宿主可选端口，不接实际平台。
- [x] typecheck/unit/dep-check/i18n、双构建、真实浏览器读写回归；失败继续/重跑与历史实际Resume仍分别按P3验收。

## P3 核心业务联调

- [x] 核对待执行队列、创建隔离契约/素材引用；显式启动独立 media_worker mock，内部 Renderer 仍归 media_workflow。
- [x] 一条完整 UI 运行：提交→真实调度→多候选选择/反馈再生成→接受→四节点完成/产物可读；固定原 Job、已接受命令不重复产生新意图。
- [x] 原生失败继续：独立失败Job，经真实UI/native retry进入候选并取消，保留成功/取消边界。
- [x] 输入覆盖整次重跑：独立failed Job，经真实UI/native rerun改资源、原Job运行后取消。
- [x] 历史补缺表单只读验证：冻结版本、只补缺、前缀/输入不变，未提交Resume。
- [x] 历史补缺 Resume 实际隔离执行：用户授权后临时导入一条新Workspace副本，视频后缀完成，入口恢复关闭，119条原Job不变。
- [x] 明确 mock、幂等回执、用户/企业/Workspace 与终态保护；清理仅测试资源，后端保持运行。
- [x] 更新范围/证据矩阵，接口与联调全部证明后才完成 goal。

## 风险

旧版本步骤曾被删除：只能从已保留事实恢复，不能假填历史；先核对实际样本再确定回填范围。权限依赖实时Auth/Control：错误失败关闭，不能用客户端角色兜底。E2E 与容器网络变化串行；不启动观测后端，必要日志关联只读检查 trace_id。审批跨窗口 generation 前置条件及全候选历史没有作为上述查询接口的隐式扩展，现有服务器判定仍为权威。

## 2026-10-09 阶段证据

已增加 migration 026、同事务 Save/Update 快照、当前 ACL 控制的版本查询以及已注册 schema v3 IR + TemplateVersion 的只读恢复路径。恢复结果 `definition_source=compiled`，缺失的历史名称/描述不由当前头记录伪造。隔离 PG `TestWorkflowCompiledVersionEvidence` 已通过：冻结槽位与旧拓扑、未注册排除、缺少模板版本、非法来源及丢失 draft 均有断言；查询不写回、不修改原编译。

`go -C media_workflow test ./...`、新增查询相关 policy/service/handler 的 race 测试、arch-check、`git diff --check` 通过。隔离 PG `TestWorkflowBusinessVersionPersistence` 通过：当前实际定义回填、旧/新定义保留、更新与快照共同回滚、不可覆盖、CAS 冲突、新建快照和非空降级保护。该测试不调用真实 Conductor/网关，不等同公开 HTTP/CLI 或 UI 联调。

media_workflow 已重建部署，migration 026 已应用；网关已重启加载 Capability 路由，不启动观测服务或外部 Worker。LAN 网关 `TestWorkflowUIQuerySupportGatewayAndCLI` 通过：目录/精确版本、两类当前权限、owner Grants、冻结定义以及非法查询门禁，独立 CLI 进程覆盖六个查询。仅使用既有可读测试定义，无 Job/授权/身份写入。127.0.0.1 入口不匹配既有 Oathkeeper LAN 规则，不作为此环境的认证入口。

UI 已开始消费冻结版本：Job 图、Resume/Rerun 表单查询键按版本隔离；缺失版本不会借用新版缓存，旧 IR 缺少 Scope 展示字段时使用原 Job 保留的 Scope。相关 3 文件/13 测试、typecheck、dep-check、i18n keys 和 standalone/plugin 双构建通过。Capability 选择器、权限按钮/完整共享列表、实际完整执行仍待实现与联调，P2/P3 不能据此宣称完成。未修改用户的 `ui_workflow/DESIGN.md`。

最新隔离 PG `TestBoundResourcePaginationPublicationAndRevokePersistence` 通过，新增查询断言覆盖真实 editor 投影、owner-only 两类 Grant 列表与撤权后的读取/列表变化。浏览器执行页只读回归（含真实登录）2 项通过：请求指定冻结版本、画布/节点 Inspector、窄屏及输入媒体预览；这不是创建/审批执行证明。共享工作站登录后 bootstrap 可显式设置 `E2E_APP_READY_TIMEOUT_MS=90000`，默认仍为15秒。全量 UI 并行测试有6项5秒超时；保持默认测试预算，`yarn test --maxWorkers=1` 复跑30文件/104测试全通过。CLI Query 单测及 race 也通过。

### Capability 与共享 UI 接入

模板创建/编辑使用真实目录和精确版本，契约未就绪时禁止保存；引用变化立即清除就绪态，不覆盖槽位/映射。能力输入、输出、生成方式与执行目标提供中文/英文展示。Template/Workflow 详情动作按 allowed_actions，编辑页保存和草稿变更均检查 update；owner 来源于 my_permission，不按 created_by 或管理员角色猜测。共享组件由页面注入查询端口，VIP 旧调用保留兼容；owner 能查看全部 Grants、撤销目录外部门记录，刷新失败不显示同步成功。Vite 增加 Capability 代理，不绕过后端网关。

新组件与纯规则相关门禁通过：Capability 精确版本/缺失/不自动选首项、owner-only 授权查询与目录外撤权、命令投影；Template 动作组件覆盖 viewer/editor/owner/功能撤销/缺失投影。最新相关5文件/17测试、typecheck、dep-check、i18n keys、standalone/plugin双构建通过。真实 UI `ui-v12-template-lifecycle` 通过：独立模板选择已发布能力→保存→发布→授部门权→完整列表核对→撤权→保存新版本→发布，最后只归档本次模板。首次失败在创建之前（Vite 缺代理），没有遗留写入；修复后新独立样本通过。运行表单另外按服务端 run 动作准入，未就绪文案不把权限不足误称为唯一注册问题；有实际历史注册事实但无旧 Publication 回执的定义可按 run 投影使用。以上不能替代 P3 完整执行证明；仍需完整UI回归与mock执行门禁。

为后续外部 Worker 联调只读审计 User 全部267个 Job：143 succeeded、5 cancelled、119 awaiting_resume，无 pending/running/need_confirm/paused；没有启动 media_worker 或恢复历史记录。实际启动前仍须再次审计，不能据此批量继续119个历史 Job。

### 2026-10-09 单条完整 UI + 外部 Worker 验收

`workflow-single-execution.spec.ts` 分 submit/review 两个显式门禁，私有 ledger 保存原 Workflow/Job 与审批回执，外部客户端在两门禁之间由操作员启动，测试不嵌入 Worker。旧 source 是未绑定企业的兼容定义，不能直接当作新企业资源复制；本次按指定冻结模板版本在授权测试企业/Workspace 创建四个隔离模板及定义，所有节点 force_new/always，不修改旧数据。

唯一 Job 为 `job_e8a850ec-20dd-445f-b7cd-e89fe1b35592`，Workflow `wf_4e7e7f21-efc2-4eba-a8de-d37c20cafb53`，企业 `8afeb26b9df643952f4f816576eb2504`，Workspace `2e6c5e63-b343-47c4-9f4e-64eb3c7855ce`。UI 真实提交成功并进入原执行图；submit 测试误读回执 id 导致断言失败，已修正为 IDL 的 job_id，通过唯一隔离 Workflow 的只读 Job 查询恢复原回执，没有再次 Submit。拒绝过的早期 Workflow 创建未产生 Job。不能把修复后未重新提交的 submit 脚本称作本轮重新全绿。

CLI `TestProviderCapabilityFixtureConfig` 校验本账号、企业、原四节点、Provider 契约和整个队列后，为原 Job 生成0600私有 mock配置。独立 media_worker provider_fixture 只注册两种 Provider 能力，经网关领取/回写；Renderer/Finalize 仍由服务端内部 Worker 执行。客户端五个实际 Dispatch 完成日志具有同一 trace_id `20edfebd1c7f96ad00466c9263640a06`，不打印提示词/Token/签名。

review UI 门禁通过：首节点两候选未选时禁确认→反馈重新生成→同原 Job 第二轮两候选选择第2个→后续三个单候选节点仍需明确审批→Job succeeded。原生 HUMAN 再生成回执 `review_3097089c-4469-40d8-abd8-085a1ac13f07` 已 applied；四个 StepRun succeeded，四个不同 Generated Asset ID，真实媒体 Range GET 全部206（3 PNG，1 MP4）。生成是已确定旧媒体的 mock，不代表厂商接入或质量验证。

仅归档本次四模板/定义，成功 Job 与产物保留供用户 review；已停止该外部 mock Worker，业务服务继续运行。119个历史 awaiting_resume未启动或迁移。下一步仍是独立的失败继续、输入覆盖重跑、历史补缺门禁与完整UI回归，不能以此样本代替它们或提前完成 goal。

### 后端隔离分支与 UI 回归补充

真实 Conductor `TestConductorNativeRetryCompiledParallelWorkflow` 通过：编译后的并行流程使用原生 retry，不对原成功业务样本重跑。`TestNativeInputRerunRecoversLostResponseWithoutSecondPost` 使用隔离 PG 和真实 Conductor 通过，覆盖输入覆盖重跑已执行但响应丢失的查询恢复，断言不再次 POST。`TestResumeOutboxTransactionAndReplay` 的隔离 PG 事务与重放门禁通过；此处是历史 Resume 的既有恢复命令机制，不是重新引入已移除的 Worker 结果 outbox。三者不等同真实用户 UI 重试/重跑/补缺的端到端验收，相关 P3 项仍未勾选。

本轮全量 UI 单测34文件/116测试通过；另增 `job/create.test.tsx` 的7项组件测试并通过，覆盖原 Job 冻结版本、Resume 仅补缺、Rerun 调用原 Job 的 input_override、不误 Submit 新 Job、冻结证据缺失/版本不符时阻止提交，以及响应丢失后 Resume 沿用原请求键、Rerun 禁止盲目再次 POST。未用新版定义补旧契约。编辑器/模板浏览器测试选样按服务器 update 动作，不从“可读”推断“可编辑”；不修改借用记录。完整浏览器矩阵仍在继续，最终结论以各项实际结果为准。

新增测试最终类型检查与 diff-check 通过。再次只读核对268条 Job：144 succeeded、5 cancelled、119 awaiting_resume、active为空；成功联调样本与历史任务状态未改变。

浏览器10项第一轮7通过，随后多语言及商品详情复跑通过。虚拟形象测试仍停在启动门禁；专门诊断记录多个动态模块同时 `net::ERR_NETWORK_CHANGED`，不属于后端查询错误，也未完成该详情页验收。不修改宿主网络/其他服务或自动重试业务写入来制造通过结果。新增测试及修改过的8个测试文件定向 lint 通过，日志不输出响应正文/URL查询。详细证据见 wiki；P2完整浏览器门禁与P3各分支真实UI验收仍保留未完成状态。

后续单独复跑虚拟形象详情通过（图片与窄屏、无业务写入），上述10项均已有通过证据，保留原网络失败记录，不宣称同一轮一次全绿。新增独立恢复样本门禁：沿用现有四节点合同复制机制，但固定另一个私有账本；只首节点 mock失败→UI原生失败继续→候选出现后取消；停止外部 mock客户端后，UI修改真实资源输入对同原Job rerun→取消。未知命令不盲目重放，不执行审批到下游，不触碰成功样本或119个历史Job。上述两项真实运行结果待后续记录。

### 恢复门禁结果与准入纠正

独立 Job `job_f0db5d4c-c8e4-48bb-98f9-be80cf16ec56` UI提交门禁通过；此前失败仅发生在输入候选加载的5秒断言，未提交，复用原隔离定义而非再次创建。私有Provider配置门禁通过。外部客户端首节点永久失败，真实UI以空override调用原生retry，首节点两候选need_confirm后UI取消，三个下游未执行；该浏览器门禁通过。仅归档这条部分样本的定义/四模板，Job保留cancelled，客户端停止。

尝试从cancelled输入覆盖rerun被后端400拒绝，暴露UI准入偏差：当前后端只允许FAILED/TIMED_OUT原生执行，而UI曾在succeeded/cancelled上也提供入口。按照既有原生语义将入口限于非历史failed（包括后端超时投影），未扩大后端能力；DDD交互同步，新增组件断言，相关2文件12测试通过。重跑门禁改为另一条独立failed样本，不复用取消或原成功记录；新的真实结果待记录。原失败记录保留，不能称该次400为重跑成功。

新独立重跑Job `job_b057f058-3cff-494a-beb4-9545c2f55892`：UI提交、Provider配置门禁、独立客户端永久失败均通过；停止mock后真实UI按冻结版本1更换至少一个真实资源，以input_override重跑原Job，进入running后UI取消，按原定义归档四模板/Workflow。浏览器门禁通过，绑定定义只存在一个Job；没有调用真实Provider、执行到下游或修改原成功样本。

只读历史表单门禁 `workflow-history-form.spec.ts` 通过：实际读取原历史Job、冻结版本与已完成前缀，只显示缺失槽位、选择兼容资源后准入提交；所有浏览器业务写请求被拦截，最终核对输入、状态和所有输出ID不变。不将表单验证称作实际Resume成功。现队列270条：144 succeeded、7 cancelled、119 awaiting_resume、active为空，两个测试mock客户端均停止。实际隔离Resume还需要新测试Workspace复制谱系，当前Token保护导入入口关闭；已向用户请求临时启用再关闭，不自动扩大迁移权限。

修正重跑准入后的最终门禁：类型检查、相关组件2文件12测试、定向lint、dep-check（212模块/708依赖）、i18n keys、standalone/plugin双构建与diff-check均通过。构建保留既有大chunk警告，未展开性能优化；用户DESIGN.md未改动。P1/P2已验证，P3仅实际历史Resume及其最终证据收尾未完成。

### 最终隔离Resume验收与收尾

用户明确授权临时启用导入入口后，经用户API创建新Workspace `91b0ddc9-1a86-45d5-a1fc-acdb296ec224`、冻结定义副本 `wf_cef4d150-3e05-4b06-85f4-604d072d0cb3`；以仅进程环境持有的临时迁移Token导入唯一Job `job_359bbae4-ab14-49f9-bed1-a2c77b4c9be8`。导入后EXIT清理立即恢复禁用，部署新代码后也再次确认入口关闭。原119条Job未改动。

实际联调发现Resume表单错误要求已完成图片阶段的pose/scene。修正为核对冻结绑定、模板版本及legacy_import成功前缀，只补未完成步骤的缺失入口；证据缺失禁止提交。候选缓存按企业/版本/显示契约隔离，防止恢复子集污染完整表单。新增组件与缓存断言通过，全量UI单测36文件127项通过；最终缺键保护定向9测试另行通过。

该历史谱系保留旧冻结 `media.video.generate@1`，独立客户端仅注册历史兼容视频能力，不修改为Provider新版本，不领取生图/内部Renderer。UI Resume一次202→三图片恢复→视频一次实际Dispatch→always单候选明确选中/确认→Job succeeded，浏览器门禁通过；补选radio之前的脚本失败未重发Resume。唯一新视频产物 `gen_7fbf8f33-d485-4aa0-ba29-e19f035b518c`，trace_id `6e1523a772b47616b271f1d895e5af1f`。前缀产物ID/来源摘要保持不变，逐条核对原119任务仍awaiting_resume。仅归档副本模板/定义，保留成功Job与标记清楚的新测试Workspace，客户端停止。

发现旧兼容候选签名过期时，新增响应只读刷新：仅已配置存储origin、同Organization/Scope路径可经内部解析器重签名，不写候选/任务/产物，外部URL不解析为内部对象，同origin越界拒绝。候选单测及race、完整backend Go测试通过并已部署；新候选链接HTTP206与真实浏览器videoWidth正值门禁通过，成功历史不再显示审批/重跑按钮。

最终队列271：145 succeeded、7 cancelled、119 awaiting_resume、active为空。类型检查、依赖边界（213模块/712依赖）、多语言键、定向lint、standalone/plugin双构建、diff-check通过；原有大chunk警告保留，不扩展性能工作。全部P1/P2/P3范围完成，不接media_platform、不调用真实厂商、不迁移全量新历史或扩展身份功能。早期失败证据保留，详细过程与样本见wiki。
