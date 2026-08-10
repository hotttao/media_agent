# media_materials 素材库上下文

素材与归属物（虚拟形象 / 产品）的统一管理微服务：管理原料（Material，curated）与生成产物（GeneratedAsset，workflow 产出）两类资源、资源类别注册表（AssetKind）、虚拟形象（VirtualIp）、产品（Product）、以及 material ↔ vip/product 绑定关系。Scope 维度隔离不同业务线（AI 服装 / AI 国学 等），所有资源强制绑定 1 个 scope（AssetKind 除外，全局注册）。对外以 Hertz + Protobuf IDL 暴露，自治数据库。

## 业务术语词典

### 租户边界

**Workspace（工作空间）**:
所有 Material、Generated Asset、Virtual IP 和 Product 的租户归属；资源读取与写入必须限定 `workspace_id`。
_禁用替代词_：Scope（Scope 是业务线分类，不是租户）

### 业务范围

**Scope（业务范围）**:
见 [共享资源访问语言](../resource_access/CONTEXT.md)。本服务所有资源都强制绑定一个 Scope；跨 Scope 引用不合法。

### 素材与素材类别

**Material（原料）**:
一份 curated 可复用资源实体（scene/pose/makeup/movement/music/caption 等），以 `material_id` 标识；归属一个 Workspace、Scope 和 owner，声明一个 Asset Kind；可见性遵循 [共享资源访问语言](../resource_access/CONTEXT.md)。
_禁用替代词_：raw asset、input asset。

**Generated Asset（生成产物）**:
workflow 执行产出的资源（model_image/style_image/first_frame/ai_video 等），落 `generated_assets` 表（asset_kind.resource_type=ASSET）；持生成记录（job_id / step_run_id / template_id / template_version / source_virtual_ip_id / source_product_id）；可见性同 Material 三档。由 `media_workflow.workflow_execution` upsert，不开放前端直接创建。
_禁用替代词_：output asset、result、generation（generation 留给 generation_record 语义）。

**Asset（资源）**:
Material 与 Generated Asset 的统称，作为跨域引用与可达性校验的对象。两类分落 `materials` / `generated_assets` 表，由所属 Asset Kind 的 `resource_type` 区分；跨服务引用统一用 `(resource_type, id)` 二元组定位，**不直读对方库**。归 [asset](domain/0002-asset/asset.md) domain。
_禁用替代词_：resource（留给 Resource Type）、item、entity。

**Asset Kind（素材类别）**:
全平台资源类别的注册实体（`asset_kinds` 表），全局注册、**不绑 scope**。每条声明 `name` / `resource_type ∈ {MATERIAL, ASSET, PRODUCT, VIRTUAL_IP, TEXT}` / `media_type ∈ {IMAGE, TEXT, VIDEO}` / `source_table` / `status`。Template 的 input slot `kind` 与 output `asset_kind` 引用此处的 `name`。归 [asset_kind](domain/0005-asset_kind/asset_kind.md) domain。
_禁用替代词_：category、type（type 留给 service 内部枚举）。

**Resource Type（资源类别 / 落表）**:
Asset Kind 的属性，标识物理资源落哪张表，枚举 `{MATERIAL, ASSET, PRODUCT, VIRTUAL_IP, TEXT}`：`MATERIAL→materials`、`ASSET→generated_assets`、`PRODUCT→products`、`VIRTUAL_IP→virtual_ips`、`TEXT→无表（inline 常量）`。与 media_workflow 的 Workflow Input Type 区分：后者是 workflow 入口传哪种 id 的**载体类型**（material/asset/product/virtual_ip），本属性是 asset_kind 自身的**物理落表类别**；product/virtual_ip 输入经 ResolveAssetByKind 解析后落点仍由所绑 slot 的 Resource Type 决定。
_禁用替代词_：kind（留给 asset_kind）、table_type、entity_type。

**Media Type（媒体形态）**:
Asset Kind 的属性，枚举 `{IMAGE, TEXT, VIDEO}`，决定 Template 渲染与下游生成器分发。注意与 Resource Type 的 `TEXT`（无表 inline 常量）**正交**：`media_type=TEXT` 指内容形态是文本，`resource_type=TEXT` 指该 kind 不落表；两者枚举值同名但语义无关。一个 asset 的 media_type 由其 asset_kind.media_type 推导，不另存。
_禁用替代词_：format、content_type、mime。

**Asset Binding（素材绑定）**:
material ↔ virtual_ip / material ↔ product 的关联关系（N:N），落 `material_bindings` 表；两端必须同 scope。generated_asset 不走此表——它的"容器"是生成记录里的 `source_virtual_ip_id` / `source_product_id`。归 [asset](domain/0002-asset/asset.md) domain。
_禁用替代词_：relation、link、attach。

### 虚拟形象 / 产品

**Virtual IP（虚拟形象）**:
一个归属 Workspace、Scope 和 owner 的 AI 形象（模特/IP），由 [virtual_ip](domain/0003-virtual_ip/virtual_ip.md) domain 管理。

**Product（产品）**:
一个归属 Workspace 和 Scope、可用于媒体生产的业务产品，由 [product](domain/0004-product/product.md) domain 管理。

### 跨域

**Asset Reachability（素材可达性）**:
判定一个 material / generated_asset 在指定 workflow（其 scope 为锚点）下能否被 **引用** 的复合规则：`scope_match == workflow.scope ∧ visibility 通过 ∧ 容器可达`。容器可达 = 绑定的 vip 对 user 可见，或绑定的 product scope 匹配（material 走 material_bindings；generated_asset 走 source_*_id）。作为 RPC 暴露给 `media_workflow`，**不允许直接读 materials 库**。
_禁用替代词_：access check、permission check。
