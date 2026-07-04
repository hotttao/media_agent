# media_materials 素材库上下文

素材与归属物（虚拟形象 / 产品）的统一管理微服务：管理原料（Material，curated）与生成产物（GeneratedAsset，workflow 产出）两类资源、资源类别注册表（AssetKind）、虚拟形象（VirtualIp）、产品（Product）、以及 material ↔ vip/product 绑定关系。Scope 维度隔离不同业务线（AI 服装 / AI 国学 等），所有资源强制绑定 1 个 scope（AssetKind 除外，全局注册）。对外以 Hertz + Protobuf IDL 暴露，自治数据库。

## 业务术语词典

### 业务范围

**Scope（业务范围）**:
见 [media_auth/CONTEXT.md §业务划分与归属](../media_auth/CONTEXT.md#业务划分与归属)。本服务所有资源都强制绑定 1 个 scope；跨 scope 的资源引用视为不合法。

### 素材与素材类别

**Material（原料）**:
一份 curated 可复用资源实体（scene/pose/makeup/movement/music/caption 等），以 `material_id` 标识，落 `materials` 表（asset_kind.resource_type=MATERIAL）；归属 1 个 scope、1 个 owner_user_id、声明 1 个 asset_kind；可见性遵循 [资源级 ACL](../media_auth/CONTEXT.md#资源级-acl-所有媒体资源通用) 的 `private/team/public` 三档（无 grants 表）。通过 `material_bindings` 绑定到 virtual_ip / product。
_禁用替代词_：raw asset、input asset。

**Generated Asset（生成产物）**:
workflow 执行产出的资源（model_image/style_image/first_frame/ai_video 等），落 `generated_assets` 表（asset_kind.resource_type=ASSET）；持生成记录（job_id / step_run_id / template_id / template_version / source_virtual_ip_id / source_product_id）；可见性同 Material 三档。由 `media_workflow.workflow_execution` upsert，不开放前端直接创建。
_禁用替代词_：output asset、result、generation（generation 留给 generation_record 语义）。

**Asset Kind（素材类别）**:
全平台资源类别的注册实体（`asset_kinds` 表），全局注册、**不绑 scope**。每条声明 `name` / `resource_type ∈ {MATERIAL, ASSET, PRODUCT, VIRTUAL_IP, TEXT}` / `media_type ∈ {IMAGE, TEXT, VIDEO}` / `source_table` / `status`。Template 的 input slot `kind` 与 output `asset_kind` 引用此处的 `name`。归 [asset_kind](domain/0005-asset_kind/asset_kind.md) domain。
_禁用替代词_：category、type（type 留给 service 内部枚举）。

**Asset Binding（素材绑定）**:
material ↔ virtual_ip / material ↔ product 的关联关系（N:N），落 `material_bindings` 表；两端必须同 scope。generated_asset 不走此表——它的"容器"是生成记录里的 `source_virtual_ip_id` / `source_product_id`。归 [asset](domain/0002-asset/asset.md) domain。
_禁用替代词_：relation、link、attach。

### 虚拟形象 / 产品

**Virtual IP（虚拟形象）**:
见 [media_auth/CONTEXT.md](../media_auth/CONTEXT.md#业务划分与归属)；在 `media_materials` 服务中建模，归 [virtual_ip](domain/0003-virtual_ip/virtual_ip.md) domain。VirtualIp 是 user 间接关联 scope 的唯一桥梁——user 不持 scope，通过拥有的 VirtualIp 进入对应 scope。

**Product（产品）**:
见 [media_auth/CONTEXT.md](../media_auth/CONTEXT.md#业务划分与归属)；在 `media_materials` 服务中建模，归 [product](domain/0004-product/product.md) domain。Product 无 user 归属、无 visibility/grants，是 scope 级共享资源；其绑定的 material 在执行时按 scope 匹配使用（无 user 校验）。

### 跨域

**Asset Reachability（素材可达性）**:
判定一个 material / generated_asset 在指定 workflow（其 scope 为锚点）下能否被 **引用** 的复合规则：`scope_match == workflow.scope ∧ visibility 通过 ∧ 容器可达`。容器可达 = 绑定的 vip 对 user 可见，或绑定的 product scope 匹配（material 走 material_bindings；generated_asset 走 source_*_id）。作为 RPC 暴露给 `media_workflow`，**不允许直接读 materials 库**。
_禁用替代词_：access check、permission check。
