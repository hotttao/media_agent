# Penpot v1.2 交互对齐审查

2026-10-08 用户已确认原型，进入前端开发，明确要求保留 Slot 以接入 media_platform。以下候选稿和待确认措辞记录设计阶段的历史状态；实施进度以 [实施审查](workflow-v1.2-implementation.md) 和 [plan](../../plan/ui_design/workflow-v1.2.md) 为准。

## 边界与设计来源

只修改交互、文案和状态表达，保留 v1.1 的布局、样式、字体、配色、尺寸与组件层次。不实现前端，不为迁就画板而扩展后端，不恢复旧本地调度器、Binding 或发布剪辑流程。确认后的设计与实施规格另行固化。

2026-10-08 经官方 Penpot MCP 读取文件 `工作流 UI · v1.1`，ID `0cd08774-6431-804f-8008-ba2cb9b897b0`，页面 `v1.1 · 工作流 UI`，ID `1e014535-63c9-815f-8008-b8c5536d5bb0`。实际有 10 个顶层画板，并非 ux 编号对应的 15 个页面；画板尚无原型 Interaction。原稿包含重复 ux-06/ux-07 编号，修订稿应采用语义名称区分，不改变原稿编号。

本记录区分原稿偏差、已完成的候选设计与待确认的后端只读支持。MCP 无文件级 Duplicate 接口，跨页 clone 被 Penpot 拒绝；此前仅创建的空副本页已移除，校验原页的文本、位置、尺寸、填充、描边、阴影与字体未变化。用户随后 Duplicate 并连接独立文件「工作流 UI · v1.2」，ID `e618892f-b98f-815a-8008-c1c303e0740f`；后续每个写入入口均校验此 ID，未向 v1.1 写入。

## v1.2 候选稿交付

用户确认七个核心旅程后，已在 v1.2 副本应用修订：10 个原画板与 23 个补充状态，共 33 个顶层画板；页面名为 `v1.2 · 交互对齐（待确认）`。已提供三条明确的 Prototype Flow：从登录开始、核心用户流程、历史继续。原视觉属性沿用；状态板复制原组件，仅改文案、节点/候选交互与必要的显示状态。已导出工作流编辑及执行详情的 SVG/截图检查，修正旧依赖线残留、输入标签与状态徽章重叠；导出运行环境字体不替代 Penpot 的原字体资产。

- 保存草稿不等于发布；发布注册中、注册就绪与权限变化单独表达。
- 四节点样例对齐图片→视频能力，图上不再有汇总输入节点；输入与常量进入节点配置。
- 当前轮 A/B 候选可切换选择后确认；决定已接受、等待新结果、重新生成反馈与失败继续均有独立状态。
- 修改输入整次重跑有专用表单，不复用普通新建提交。历史继续有只读成功前缀与缺失输入表单，不伪造既有原生执行或视频结果。
- 节点图片、最终视频、Product、Virtual IP 四图、素材上传与资源共享有相应查看/操作入口。图片查看返回来源页面，不假装执行已完成。
- 原稿的保温杯示例与服装生成链不一致，样例统一为通勤外套，不修改服务端业务数据。

原型仅用于设计确认，不发业务 API，不表示真实生成、上传、授权或发布已经执行。缺少的只读支持仍按下文记录，未因此新增后端接口或开发前端。用户确认后才固化实施规格与计划。

最终 MCP 复核：33 个顶层画板、161 条交互连接，导航目标不存在的数量为 0；三条原型入口均存在，交付标记为 `pending-user-confirmation`。这是原型结构校验，不是业务 API 或前端 E2E 验收。

失败证据保留：插件多次挂起/断开，整批写入及连线请求曾超时；没有据此宣称保存成功。随后核对副本状态，按画板拆分写入并使用完成标记防重。Penpot 拒绝空 Text characters，改为隐藏不再使用的文字后通过；未修改服务代码以绕过 MCP 限制。

## 页面对齐矩阵

`partial` 表示画板表达与当前后端不完全一致，不能据此声称页面可实现或已验收。

| 画板 | 当前问题 | v1.2 拟修订 | 实现证据 | 状态 |
|:---|:---|:---|:---|:---|
| ux-01 登录 | 固定 SSO 按钮；“端到端加密”并非当前部署保证；缺密码显示状态 | 由 Kratos flow 的 ui.nodes 决定可用方法；有配置才显示 SSO；密码眼睛、提交中/错误/分步注册；登出走真实 browser flow | `docs/ddd/ui_design/authentication-flow.md`、`docker-compose/auth/kratos/kratos.yml`、现有 OryFlowForm | partial |
| ux-02 工作流列表 | 假定全局周统计/趋势接口；把管理员示例当资源 owner；可见性“公开”含义不清 | 显示当前 Workspace 与用户可访问结果；统计须有真实数据或“暂无统计”；public 文案为企业内公开；发布就绪与 published 分开 | Workflow List/Get/Publish handler，权限 spec | partial |
| ux-03 工作流编辑器 | 仍有开始输入节点和参数连线；示例含文案/配音/音频与未接入能力；发布被表达成一次即时成功 | 使用已验证的四节点图片→视频示例；只画 step_output 执行依赖；输入/常量进入节点配置；校验→保存新版本→owner 发布→注册中/就绪/失败；企业和 Workspace 不可重新绑定 | `workflow.proto`、Workflow Service、Publication Flow、migration023 | partial |
| ux-06 模板编辑器 | 能力是 digital-human-video@2.1.0；managed target 像可改字段；“每次一个产物”忽略候选 | Provider + generation_method + generation_type + 整数版本表达 Capability；target 只读属于能力版本；可产生多候选，接受后单输出；版本保存 draft、owner 发布；提示词本地校验不冒充后端预览 | `template.proto`、Template Service、Capability registry、Semantic Compiler | partial |
| ux-07 运行工作流 | 全局 reuse 开关与消耗预估没有 Submit 字段；viewer 被当完整执行许可；失败任意重试承诺过强 | 复用策略作为定义只读摘要，不给 Submit 全局开关；同时校验资源可读、Workspace 与 workflow 功能；冻结版本、企业/Workspace、稳定提交意图键；超时先查原回执，不换键新建 | `job.proto` SubmitJobReq、Job Submit、Start Delivery、权限 spec | partial |
| ux-06 执行详情 | need-review 非领域状态；跨 attempt 候选混入可确认列表；审批前即显示 Generated Asset；租约倒计时被当调度权威 | need_confirm；只选择当前 HUMAN 冻结轮候选；旧轮仅可读；明确决定已接受但尚未完成；最终产物待 Finalize 后展示；原生失败继续、审批重新生成、整次输入覆盖重跑分开 | Job Review/Retry Service、Review Delivery、Finalize、`job.proto` | partial |
| ux-07 素材库 | Product 详情叠加抽屉与子页混用；生成物被标为上传素材；展示 material_bindings、服装套数、存储统计等无来源事实 | 分类保持 Material/Generated Asset/Product/Virtual IP；Product 详情进独立子页；生成来源与上传来源区分；无接口的关联/统计不填假数据；保持类型多选及服务端分页 | `media_materials/*.proto`、Material/Asset/Product Service、素材 UI 契约 | partial |
| ux-14 模板列表 | 输出媒体、引用数、状态筛选假定都有服务端能力；样本混用未接入能力 | 区分输出 Asset Kind 与媒体；支持接口已有 scope/name/visibility/page 查询；客户端筛选必须说明当前数据范围，不冒充全库总数；已发布历史版本与最新 draft 分开 | ListTemplatesReq/handler、TemplateVersion 发布状态 | partial |
| ux-15 任务列表 | 少 awaiting_resume/paused；完成任务显示多条最终输出；重试含义不明 | 列表优先 Workflow/版本、当前步骤与审批；增加待继续/暂停；一个最终输出，多个候选不是多个完成结果；失败继续保留成功旁支；补历史继续执行入口 | Job/Step 状态枚举、Retry/Resume API、Conductor 原生执行 plan | partial |
| ux-13 Product 详情 | 商品“归属工作流”；ResolveAssetByKind/Bindings/SKU 被当既有关系；使用记录与全量审计无查询支撑 | 改为“可用工作流”：已注册且 Input Schema 包含 PRODUCT 的候选，不等于商品已经被引用；预填 product 后仍补齐其他输入；商品信息按字段显示；无来源的使用记录/审计不编造 | Product proto、Workflow Input Schema、Job input 快照、当前 CRUD 路由 | partial |

## 不得用画板假设补齐的查询缺口

1. **Capability 目录**：当前公开 IDL 没有普通用户 Capability List/Get；Worker 注册接口不是目录查询。四类已发布 Provider 契约可以作为已核对的设计样例，但动态选择器的数据源仍需确认，不能硬编码样例当运行事实。
2. **权限读模型**：Get/List 返回归属和 created_by，但没有 my_permission、允许动作或 Grant 列表。created_by 与当前用户一致只能证明 owner 关系，不能代替功能授权；非 owner 的 viewer/editor 无法仅凭管理员身份或可见性准确推断。精准角色徽章及完整授权管理需读接口，或首期明确使用服务端拒绝后的只读交互，不显示虚构角色。
3. **冻结执行拓扑**：Job 保存 workflow_version，但公开 GetJob 只返回 Job/StepRun，未暴露版本化 Workflow 的依赖边。不能把 Workflow 当前头版本的图无条件画成旧 Job 执行图。应补冻结拓扑只读接口，或在版本不匹配时显示真实步骤状态列表并明确“旧版本拓扑不可用”，不伪造依赖。
4. **历史轮次候选**：当前 StepRun 只给当前 candidates、retry_count 和最近 Review 回执，没有完整 generation/candidate history 查询。设计不能承诺任意旧轮候选列表和跨轮采用操作。
5. **统计与追溯**：引用次数、趋势、存储占用、Product 全量使用历史、通用审计与实时 Worker 在线状态没有相应公开查询。只能显示有来源、覆盖范围明确的数据，或保留不可操作空态。StepRun 进度需要实际 GetJob，不能把 ListJobs 的 Job 行当完整节点快照。

这些是需要在设计确认时选择的最小接口支持/首期降级项，**本轮不直接开发接口或扩大 Goal**。

## 必须明确的用户动作

- 多候选永远审批，always 还审批单候选；接受只针对当前候选位置，不让用户输入原生引擎标识。
- 重新生成只在当前 need_confirm 审批轮提交 feedback 和意图键；确认请求超时先查询最近回执，不能重置选择轮次后盲目提交。
- 普通“失败继续”代理 Conductor retry，保留已完成任务；从节点发起仍恢复所属执行，不保证只重跑单节点。
- “修改输入重新运行”是显式整次 rerun，会重跑已成功节点，与失败继续分开提示；不得用于审批等待中的执行。
- awaiting_resume 的“继续执行”补缺少输入，保留历史成功前缀；导入完成记录没有伪造原生运行，不显示不存在的实时调度信息。
- 单输出是节点最终接受结果，不限制 Candidate 数；审批前 Candidate 上传不等于最终业务 Generated Asset 登记。
- private 仅 owner；team 按有效同企业部门 Grant；public 是企业内公开。跨企业、退出部门、功能撤销和权限源故障都以服务端当前检查为准。
- 运行可读的已发布冻结 Workflow 不要求用户重新取得源 Template 权限；修改和发布定义仍需检查引用权限与版本。

## 交付门禁

- v1.2 副本已核对文件 ID，原 v1.1 内容未变。
- 所有修改都对应后端事实；不支持的动作隐藏/解释或明确标为待确认，不伪装可执行。
- 保留视觉属性；新增必要状态复用原组件，不引入第二套视觉风格。
- 主流程原型至少连通列表→定义/模板→运行输入→执行详情→当前审批/失败继续/历史继续；不得用原型跳转假装真实 API 成功。
- 用户确认后再更新正式 UI 契约、spec/plan 并开发前端。未确认前不改 `ui_workflow/src`，不覆盖用户的 `ui_workflow/DESIGN.md`。
