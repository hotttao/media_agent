# Step Result Reuse 实现规格

目标：新执行的节点在实际生成输入相同且已有接受结果时，不向 Capability Worker 派发生成任务。保持 SlotBinding、两阶段编译、Conductor 原生分支/循环/技术重试和既有 Finalize 结果登记；不引入业务调度器。

## 匹配与安全

本规格横跨 media_workflow 与 media_materials：前者拥有复用证据、编译与内部任务；后者继续拥有产物、权限和对象存储元数据。不得把素材查询移入 Workflow 数据库。

使用带 schema version 的规范化摘要：冻结 Template ID/version、Capability key/version、实际渲染 prompt、有序媒体内容 checksum、生成参数、输出 asset_kind。递归对象 key 排序，保留数组顺序及值类型；短期签名 URL、Task/Job 身份、重试次数不参与。无法取得可信 checksum 或旧历史缺少生成来源证据时不能自动命中，不能猜测旧模板版本。

复用证据与执行归属分开：默认查找同企业、Workspace、提交用户的可信生成证据；缓存命中还须通过 Materials 当前授权与对象可读性复核。不能把同内容摘要当作跨租户读取授权。未来共享复用须显式扩展授权查找契约，不扩大本轮范围。

只登记经过自动接受或人工选定、已成功登记的结果；等待审批、拒绝/重新生成中的候选不是命中。复用记录包含源 Asset/StepRun 与匹配输入摘要，不伪造新生成结果键。不改变旧产物或旧步骤当前引用。

## 编译及内部任务

新原生发布使用 IR schema v4，v3 和历史 Resume 仍按旧执行图降低，不原位改写冻结版本。v4 节点编译为初始 Render → `media_check_result_reuse_v1` → Conductor SWITCH（复用空分支／原生成审批循环）→ 统一 Finalize；唯一输出引用保持不变。初始 Render 仅准备查询输入，不是 Capability。Finalize 对命中再次核验资源；未命中使用本轮真实成功 Invoke 的实际参数建立指纹，不能把审批反馈后的结果登记到初始提示词身份下。

新增内部复用检查任务，输入由冻结 IR 与真实渲染结果生成；返回命中标识及稳定单输出，不属于 Capability。Conductor SWITCH 命中分支返回已有产物，未命中分支执行现有 Capability、审批循环及 Finalize。两条分支输出统一供真实下游绑定和唯一 sink 消费。

策略取已有 `reuse_if_available | force_new`；force_new 及审批重新生成不得被缓存吞掉。技术 retry 按 Conductor 原生语义重试，不修改 Step DAG 或新建业务重跑周期。已发布定义不原位改写，新增编译输出须更新可辨识 schema/编译证据并通过重新发布使用。

Finalize 在 Materials 幂等登记后写复用来源证据，再完成原生 Task。Materials 调用不持有本地事务；失败重交付仍使用成功 invoke 的稳定 result_key。复用来源记录用独立仓储端口/版本化 SQL migration 实现，不在应用层直接写 SQL，不能依赖 Get/List 才补写索引。

## 查询与 UI

执行投影暴露明确的结果来源（新生成/复用）及源产物，不把准备向导状态冒充运行成功。节点配置支持优先复用/强制新生成；运行视图标注复用已有结果。复用选项不能只改变前端显示而不改变编译执行分支。

StepRun 响应追加 `result_source`（generated/reused，旧定义及未完成节点为空）与 `reused_from_step_run_id`。从同次已授权查询的当前完成 Finalize 输出派生，仅在投影产物 ID 一致时展示；不新增运行状态表或第二次引擎请求。UI 在结果面板标注来源，源步骤 ID 仅按需展开；不猜测旧记录的来源。

## 错误与验收

- 确定不存在、已撤权或已失效的资源：未命中后按现有生成契约执行。
- 鉴权/存储网络失败：可重试内部 Task 失败，不视为未命中并产生新生成费用。
- 取消/精确 Task 被替代：拒绝晚到回写，不能推进新 Task。
- 并发未命中：允许各自生成及独立产物，不引入全局生成锁。
- 验证摘要稳定及参数变更失配、权限隔离、文本输入、失效对象、force_new、审批重新生成绕过、Finalize 重交付；真实 Conductor 验证命中时零 Capability Task、下游正常取值、未命中及并行分支正常完成。

## 持久化与对象摘要契约

`media_workflow.step_result_reuse` 保存不可变证据，字段：`id`（稳定来源键）、`organization_id`、`workspace_id`、`owner_user_id`、`scope`、`fingerprint`（64 位小写 SHA-256）、`source_job_id`、`source_step_run_id`、`source_result_key`、`asset_id`、`object_path`、`content`、`asset_kind`、`created_at`。同来源结果键唯一；同键不同证据冲突。按企业/Workspace/owner/Scope/fingerprint 查询，最新接受结果优先；不存签名 URL 或执行调度状态。来源 Job/StepRun 删除时清理证据，不对 Materials 建跨库外键。

`ResolveLibraryObjectResp` 追加 `checksum_sha256`，来自受控存储 HEAD 的不可变对象元数据；没有摘要时返回空值，不能用 ETag/路径/业务 ID 猜测。内部解析继续验证企业对象命名空间。Workflow 的媒体摘要端口只接受受控引用，SHA-256 缺失返回明确不可匹配，不把存储网络错误当未命中。

新增 Materials 内部 `GET /api/internal/v1/generated-assets/reusable/{id}?organization_id=&scope=&owner_user_id=`，使用既有 `X-Media-Materials-Internal-Token`，公开网关不暴露。响应包含产物 ID、企业/Scope/owner、Asset Kind、Object Path/Content、Job/StepRun/Template 版本来源，不含 URL 或凭据。不存在、已删除、非原生或归属不匹配为 404；存储/数据库暂时故障为 503。工作流还须比较该真实来源与证据记录，不把记录本身当授权。

Fingerprint 使用 capability 声明字段，剔除 job_id/step_key 等投递元数据；文本参数原文参与，资源参数替换为真实内容 SHA-256（有序数组不排序）。输入不存在与存在空值不合并；禁止未声明 Provider 参数被静默排除。模板/能力版本及输出 Asset Kind 一并参与匹配。

当前状态：实现及验收完成，见 [Plan 验收记录](../plan/step-result-reuse.md)。新定义的运行契约参数规范为纯 JSON 值，持久化/解码不改变定义哈希；不更改旧冻结定义的哈希算法。
