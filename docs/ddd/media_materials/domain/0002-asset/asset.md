---
name: asset
description: material（curated 原料）+ generated_asset（workflow 产物）两类物理资源的存储与 CRUD
dependencies:
    - asset_kind
related_workflow:
    - worker_artifact_upload
    - library_object_upload
    - reachability
    - media_workflow.workflow_validation
    - media_workflow.workflow_execution
---

## 一句话职责

Worker Artifact Upload只准备受控企业/Scope下的对象写入凭据，不创建Material或Generated Asset。路径同时绑定原生任务尝试、候选位置与内容清单，相同内容稳定重放、不同文件不覆盖旧引用；对象存在且元数据匹配时可HEAD确认上传。资源登记、可见性和来源校验仍由既有Asset能力持有，参见[上传流程](../../../workflow/0001-worker_artifact_upload/worker_artifact_upload.md)。
asset domain 管理**两类物理资源**，分落两张表：
- **material**（`materials` 表，resource_type=MATERIAL）：curated 原料（scene/pose/makeup/movement/music/caption），用户上传/策展，**无生成记录**；可由 Storage 对象或内联文本承载。
- **generated_asset**（`generated_assets` 表，resource_type=ASSET）：workflow 执行产物（model_image/style_image/first_frame/ai_video）。原生产物持 Job/StepRun/Template 生成记录；历史导入产物持旧系统来源键、摘要与原时间，不伪造原生 Job 链接。

两者都引用 [asset_kind](../0005-asset_kind/asset_kind.md) 注册表；都持 Organization、Scope、owner 和 visibility；可见性沿用 [共享资源访问语言](../../../resource_access/CONTEXT.md) 的 `private/team/public` 三档。

跨域可达性校验（`AssetReachability` / `ResolveAssetByKind` / `AssetRead` 等）**不在本 domain**--归 [reachability workflow](../../workflow/0001-reachability/reachability.md) 编排 + policy 决策（见 [ADR 0006](../../../adr/0006-reachability-layering.md)）。

### 不做什么
- 不存素材二进制（只保存 `object_path`，文件由 `media_storage` 提供）；文本类素材可保存内联 `content`。
- 对受信内部服务提供按企业命名空间解析 Object Path 的只读 Storage 入口；只负责对象存在性与签名，不代替资源级可达性授权。
- 不在 CRUD 内调用 media_storage 上传或删除对象；调用方先经 [Library Object Upload](../../../workflow/0008-library-object-upload/library_object_upload.md) 取得 `object_path`，本 domain 只保存逻辑引用。
- 不管理旧系统 `IpMaterial`；IP 专属素材已明确不在本轮迁移范围，也不得用普通 Asset Binding 冒充其派生语义。
- 不执行 workflow / job / render（归 `media_workflow`）。
- 不把 material 与 generated_asset 混在一张表--generated_asset 必须记录可信的生成或导入来源，material 不需要，字段与生命周期不同。
- 不做跨域可达性决策（归 [reachability workflow](../../workflow/0001-reachability/reachability.md)）；本 domain 仅暴露只读查询能力供其编排。

## 边界

### 做什么
- material 增删改查 + `material_bindings`（material ↔ virtual_ip / material ↔ product，N:N）。
- 维护 Material Compatibility；当前只允许 pose → movement，并保留动作素材的服装适用范围。
- generated_asset 增（由 `media_workflow.workflow_execution` 经 reachability workflow 调 `UpsertGeneratedAsset`）/ 查 / 软删；持生成记录链接。
- 对 reachability workflow 暴露 material / generated_asset 的**只读查询**（scope / visibility / owner / team / bindings / source_*_id）。

### 不做什么
- 不做 user 级 Scope 创建授权（见 [Scope](../../../resource_access/CONTEXT.md#scope业务范围)）。
- 不管理 publication_plan。
- 不暴露跨服务可达性 RPC（归 reachability workflow）。

## 能力清单

原生已接受产物的内部复用核验只读查询现存 Generated Asset：要求明确的企业/Scope/owner 与真实产物一致、Origin 为 native、生成来源完整，且对象仍存在。私有产物不能因复用证据绕过 owner 归属；删除/不匹配返回不可用，数据库/存储暂时故障返回可重试错误，不将故障当成不存在。该入口由已有内部服务凭据保护，不经公开网关，不返回签名 URL，不创建资源或决定 Conductor 分支。

内部对象解析附带上传时保存的内容 SHA-256；缺失时保持为空，不能使用路径、ETag 或业务 ID 替代。对象摘要不授予资源权限，资源核验与企业命名空间校验仍分别执行。

| 能力 | 说明 | 对外接口 |
|:---|:---|:---|
| UpsertMaterial | 新建/更新 material；asset_kind.resource_type 必须=MATERIAL | `POST /api/v1/materials`、`PUT /api/v1/materials/{id}` |
| ListMaterials | 按 scope/asset_kind/owner/visibility 过滤；默认 `only_mine=true` | `GET /api/v1/materials?scope=&asset_kind=&only_mine=&visibility=` |
| SoftDeleteMaterial | 软删 | `DELETE /api/v1/materials/{id}` |
| AddMaterialBinding | Material ↔ Virtual IP / Product；两端同 Organization、同 Scope | `POST /api/v1/materials/{id}/bindings` |
| RemoveMaterialBinding | 解绑 | `DELETE /api/v1/materials/{id}/bindings/{owner_kind}/{owner_id}` |
| ReplacePoseCompatibilities | 原子替换 movement 可兼容的 pose 集合 | `PUT /api/v1/materials/{movement_id}/compatible-poses` |
| GetGeneratedAsset | 按 id 取（含生成记录链接） | `GET /api/v1/generated-assets/{id}` |
| VerifyReusableGeneratedAsset | 内部核验原生已接受产物的归属、来源与可读性 | `GET /api/internal/v1/generated-assets/reusable/{id}` |
| ListGeneratedAssets | 按 scope/asset_kind/owner/job_id 过滤；默认 `only_mine=true` | `GET /api/v1/generated-assets?scope=&asset_kind=&job_id=&only_mine=` |
| SoftDeleteGeneratedAsset | 软删 | `DELETE /api/v1/generated-assets/{id}` |
| UpsertGeneratedAsset | 由 workflow_execution 经 reachability 调用，落 step 产物；带生成记录字段 | `UpsertGeneratedAsset`（跨服务 RPC） |
| ImportGeneratedAsset | 受信迁移入口登记旧图片/视频；按源记录 ID/摘要幂等，不要求预先存在 Job | 内部迁移 API |

> 跨服务可达性 RPC（`AssetReachability` / `ResolveAssetByKind` / `AssetRead` / `Check*Accessible`）归 [reachability workflow](../../workflow/0001-reachability/reachability.md)，不在本 domain 能力清单。`generated_asset` 不暴露 HTTP 给前端直接创建，仅由 `media_workflow.workflow_execution` 经 reachability workflow upsert。

## 数据模型

### Tables - `materials`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| id | bigserial | 主键 |
| organization_id | text not null | 所属 Organization；跨服务 ID，不建外键 |
| scope | text not null | 业务范围；1:1；校验 `IsKnownScope`（service 调 `shared/scope`） |
| asset_kind | text not null | 素材类别；引用 `asset_kinds.name`，且其 resource_type=MATERIAL；校验已注册未停用 |
| name | text not null | 素材名；Organization + Scope 内同 kind 唯一 |
| description | text nullable | 人类可读说明 |
| prompt | text nullable | 策展时保存的提示词；不是 Workflow 渲染后的 prompt |
| object_path | text nullable | Storage 对象路径；域名无关，不保存完整 URL |
| content | text nullable | 文本载荷；与 object_path 至少一个非空 |
| metadata | jsonb | 渲染 hint；default '{}' |
| owner_user_id | text not null | 创建者 |
| visibility | text not null | 可见性；private / team / public；默认 private |
| team_id | text nullable | 可见 team；仅 visibility=team 时使用 |
| created_at / updated_at / deleted_at | timestamptz | 时间戳 |

约束：UNIQUE `(organization_id, scope, asset_kind, name)` WHERE `deleted_at IS NULL`；CHECK `object_path IS NOT NULL OR content IS NOT NULL`；INDEX `(organization_id, scope, asset_kind, updated_at desc)`；INDEX `(owner_user_id)`。

movement 类型 Material 的稳定字段为 `metadata.applicable_clothing` 与 `metadata.is_general`。`applicable_clothing` 对应旧系统同名为 `scope` 的服装适用范围，**不是业务 Scope**；`is_general=false` 时至少存在一个 pose → movement Compatibility，`is_general=true` 时不保留 Compatibility。

Text-only Movement 以非空 `content` 承载动作描述，允许 `object_path=NULL`；这与媒体型 movement 同属 Material 聚合、共用 `asset_kind=movement`，不是待修复的缺失文件。调用方若选择它作为 Workflow 输入，只能取得内容而不能请求不存在的对象 URL。旧系统同一记录没有 URL 时按此语义迁移，保留来源 ID 和原文字，不伪造媒体。

实现状态：现有服务尚未落地 `material_compatibilities` 表和 ReplacePoseCompatibilities 接口。旧动作素材迁移时先将来源 `pose_id` 列表暂存于 `metadata.legacy_pose_ids`；这只是迁移追溯信息，不是可达性或工作流可用的兼容关系。正式关联与 `is_general=false` 校验须待该能力实现后补齐。

### Tables - `generated_assets`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| id | bigserial | 主键 |
| organization_id | text not null | 所属 Organization；跨服务 ID，不建外键 |
| scope | text not null | 业务范围；沿用 job.workflow.scope |
| asset_kind | text not null | 产物类别；引用 `asset_kinds.name`，且其 resource_type=ASSET；= 产出该 asset 的 template.output_asset_kind |
| name | text not null | 产物名；Organization + Scope 内同 kind 唯一；语义见下「UpsertGeneratedAsset 与 retry 的 name 语义」 |
| object_path | text not null | Storage 对象路径；域名无关，不保存完整 URL |
| metadata | jsonb | 渲染 hint；default '{}' |
| owner_user_id | text not null | 提交者；= job.submitted_by |
| visibility | text not null | 可见性；默认 private（产物默认仅提交者可见） |
| team_id | text nullable | 可见 team |
| job_id / step_run_id | text nullable | 原生产物的生成 Job/StepRun；历史导入产物为空并由 Execution StepRun 反向引用本 Asset |
| template_id / template_version | text / int nullable | 原生产物的 Template 版本；导入产物不得伪造 |
| origin | text not null | native / legacy_import |
| source_system / source_record_id | text nullable | 历史导入来源键；同一源记录只对应一个 Asset |
| source_sha256 | text nullable | 摄取对象摘要；同源 ID 不同摘要为冲突 |
| source_created_at / imported_at | timestamptz nullable | 旧系统创建时间与目标摄取时间，不能互相覆盖 |
| source_virtual_ip_id | bigint nullable | 源虚拟形象；job 输入里的 virtual_ip（若有），用于可达性 |
| source_product_id | bigint nullable | 源产品；job 输入里的 product（若有） |
| created_at / updated_at / deleted_at | timestamptz | 时间戳 |

约束：UNIQUE `(organization_id, scope, asset_kind, name)` WHERE `deleted_at IS NULL`；INDEX `(organization_id, scope, asset_kind, created_at desc)`；INDEX `(job_id)`；INDEX `(source_virtual_ip_id)`；INDEX `(source_product_id)`。

> **来源互斥**：`origin=native` 必须具有 Job/StepRun/Template 链接；`origin=legacy_import` 必须具有 source_system/source_record_id/source_sha256 和受控对象引用，不得填写伪造的 Job/Template。历史 StepRun 可引用同一 Imported Generated Asset，因此 Asset 不保存“唯一导入 Job”。

### Tables - `material_bindings`

material ↔ virtual_ip / material ↔ product 的 N:N 绑定。

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| organization_id | text not null | 企业归属；必须与两端一致 |
| material_id | bigint not null | FK materials；复合 PK 一段 |
| owner_kind | text not null | virtual_ip / product；复合 PK 二段 |
| owner_id | bigint not null | vip_id 或 product_id；复合 PK 三段 |
| bound_by | text not null | 谁绑定 |
| bound_at | timestamptz | 绑定时间戳 |
| bindings_end_at | timestamptz nullable | 失效时间戳；默认 NULL=生效中；owner（vip/product）软删时置 `now()`，绑定即时失效但保留可查 |

约束：PK `(material_id, owner_kind, owner_id)`；INDEX `(organization_id, owner_kind, owner_id)`；PARTIAL INDEX `(organization_id, owner_kind, owner_id) WHERE bindings_end_at IS NULL`（生效绑定查询）。写入时校验两端同 Organization、同 Scope。

> generated_asset 不走 material_bindings--它的"容器"是生成记录里的 `source_virtual_ip_id` / `source_product_id`，由 execution 在 upsert 时填入。
> `bindings_end_at` 替代"软删级联删除"：owner 软删时 binding 不删，置失效时间戳；可达性查询过滤 `bindings_end_at IS NULL`，既保留历史追溯又让绑定即时失效（与 [virtual_ip](../0003-virtual_ip/virtual_ip.md) / [product](../0004-product/product.md) 的软删规则统一）。

### Tables - `material_compatibilities`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| organization_id | text not null | 企业归属 |
| scope | text not null | 业务范围 |
| source_material_id | bigint not null | pose Material |
| target_material_id | bigint not null | movement Material |
| relation_kind | text not null | 当前固定 `pose_movement` |
| created_by / created_at | text / timestamptz | 审计 |

约束：PK `(source_material_id, target_material_id, relation_kind)`；两端必须同 Organization、同 Scope，且 Asset Kind 分别为 pose、movement。关系与 Material 同属 asset domain，在一个本地事务内整组替换。

## UpsertGeneratedAsset 与生成结果身份

内部原生登记接收可信 Job 的 `organization_id`，不要求通用生成能力额外携带商品或虚拟 IP。提供来源时必须校验其 Scope 和 Organization 与产物一致；兼容旧调用方从来源推导归属。两者都缺失时拒绝登记。对象路径仍须属于该企业、同 Scope，且通过存储元数据与媒体类型校验。

generated_asset 保留唯一键 `(organization_id, scope, asset_kind, name) WHERE deleted_at IS NULL`。原生新结果使用包含结果身份的稳定名称；名称不作为跨轮覆盖入口。

- 每个 Generation Result 对应独立、不可变 Generated Asset；Retry 不覆盖上一轮的资源 ID、对象或内容，已存在资源引用不改变含义。
- `(step_run_id, result_key)` 的登记回执属于 asset 聚合，一次事务内创建 Asset 与回执；同键同稳定摘要返回原 Asset ID，同键不同内容或来源拒绝。
- 相同媒体对象可被不同轮 Generated Asset 引用，不以内容相同推断结果身份相同。业务节点当前指向哪个产物由 Execution 管理，不由 Materials 保存“最新结果”别名。
- 软删除不会清除登记回执或允许同键重新生成新 Asset。历史记录保留原身份，不伪造轮次、不合并或覆盖。

### 生成结果登记回执

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| step_run_id | text | 稳定业务节点来源；跨服务逻辑引用 |
| result_key | text | 当前被接受生成结果的稳定身份 |
| asset_id | text | 对应不可变 Generated Asset |
| request_digest | text | 规范化对象引用、内容及可信来源的摘要 |
| created_at | timestamptz | 首次登记时间 |

主键 `(step_run_id, result_key)`，asset_id 唯一并引用本库 Generated Asset；并发首次登记只创建一个资源。回执不得随 Retry、网络断线或软删除而改写。不建立对 Workflow 数据库的 FK。

## 校验细则

1. material.asset_kind / generated_asset.asset_kind 必须 ∈ asset_kinds 注册表且 status∈{SYSTEM, ENABLED}；resource_type 与表一致（material->MATERIAL，generated_asset->ASSET）。
2. 创建后 scope 不可改；asset_kind 不可改。scope 校验由 service 层调 `shared/scope.IsKnownScope` 完成（见 [scope](../0001-scope-del/scope.md) 降级说明）。
3. material_bindings 强制两端同 Organization、同 Scope；Binding 不在第一阶段 CRUD 实现范围。
4. visibility=team 时 team_id 必填。
5. 原生 generated_asset 的 job_id / step_run_id / template_id / template_version 在 upsert 时由 execution 填入，不可改；历史导入产物用来源键与对象摘要幂等，不填这些生成记录字段。重复来源 ID 且摘要一致返回同一 Asset，不同摘要拒绝。
6. SoftDelete material：不级联删 material_bindings，保留历史；可达性查询忽略已软删 material。
7. 后续实现 Virtual IP/Product Binding 后，owner 软删必须结束有效 Binding，但不得删除 Generated Asset；第一阶段没有 Binding 写入。
8. movement 的 `applicable_clothing` 不得写入 `scope` 字段；迁移时必须显式改名。
9. ListMaterials/ListGeneratedAssets 默认 `only_mine=true`，owner 从可信 Principal 推导；只有显式传 false 才返回其他有权资源。

## 关联关系

### 依赖的 domain
- [asset_kind](../0005-asset_kind/asset_kind.md) - asset_kind 注册表校验

### 被依赖的 domain / workflow
- [reachability workflow](../../workflow/0001-reachability/reachability.md) - 提供 material / generated_asset 只读查询供其编排可达性决策
- [`media_workflow.workflow_execution`](../../../media_workflow/workflow/0002-workflow_execution/workflow_execution.md) - 经 reachability workflow 调 `UpsertGeneratedAsset` 落 step 产物
- [`media_workflow.execution`](../../../media_workflow/domain/0003-execution/execution.md) - generated_asset 反向链接 job_id / step_run_id

### 关联术语
[Material](../../CONTEXT.md)、[Generated Asset](../../CONTEXT.md)、[Asset](../../CONTEXT.md)、[Asset Kind](../../CONTEXT.md)、[Asset Binding](../../CONTEXT.md)、[Resource Type](../../CONTEXT.md)、[Media Type](../../CONTEXT.md)。
