# asset domain 变更记录

## 2026-09-21 · media_ai 素材能力复核

- 补齐 Material 的 description、prompt、内联 content 与 object_path 二选一承载方式。
- 将旧 Movement Material 归入 `asset_kind=movement`，把其同名异义字段 `scope` 改为 `applicable_clothing`，并新增 pose → movement 的 Material Compatibility。
- 按 `docs/human/research/004_matrials.md` 收窄第一阶段：只实现 Material CRUD，Binding 与 Pose/Movement Compatibility 延后。
- 用户明确旧 `IpMaterial` 暂不考虑；本轮不把 IP 专属素材折叠进 Material 或 Asset Binding。
- 所有业务唯一键补入 `workspace_id`，文件引用统一为 Storage `object_path`。
- Material 与 Generated Asset 列表默认 `only_mine=true`；CRUD 只保存预先上传得到的 object path，不编排 Storage 上传。

## 2026-07-03 · 初始建模（media_agent v1）
- 由 `media_ai` 的 `materials` + `generated_assets` 设计迁入。
- 当时把旧版 media_ai 的 `Material` / `IpMaterial` / `MovementMaterial` 归纳为 `materials` + `material_bindings`；该结论已被 2026-09-21 复核取代：Movement Material 保留 Compatibility 语义，`IpMaterial` 本轮暂不迁移且不得折叠为 Binding。
- 历史生成产物（ModelImage / StyleImage / FirstFrame / Video 等）统一为 `generated_assets` 表，持生成记录链接。

## 修改记录

### 2026-07-21 · 跨域可达性 RPC 上提 + 数据模型修正
- 把 `AssetReachability` / `ResolveAssetByKind` / `AssetRead` 从本 domain 能力清单移除，上提到 [reachability workflow](../../workflow/0001-reachability/reachability.md)（见 [ADR 0006](../../../adr/0006-reachability-layering.md)）。理由：可达性跨 asset / virtual_ip / product 三 domain，留在 asset domain 违反同层 domain 隔离。
- `material_bindings` 补 `bindings_end_at` 列：owner（vip/product）软删时置 `now()` 使绑定失效但保留可查，替代"级联删除"，与 virtual_ip / product domain 的软删规则统一（原 virtual_ip.md 提到该列但本表未声明，本次补齐）。
- 明确 `UpsertGeneratedAsset` 与 retry 的 name 语义：name 是业务维度稳定标识，retry 默认经 upsert 覆盖同一业务产物行，旧 attempt url 入 execution.retry_history；不自动加 attempt 后缀。
- `dependencies` 去掉 `scope`（已降为 shared）与 `docs/research/permission.md`（非 domain）；`related_workflow` 补 reachability + media_workflow.validation/execution。
- 关联术语补 Asset / Resource Type / Media Type（CONTEXT 已新增）。

### 2026-07-03 · Review 反馈：拆 materials + generated_assets 两张表
- 撤回此前"统一 assets 表"的有意简化——用户纠正：generated_asset 必须记录"怎么生成的"（job / template / inputs），material 是 curated 原料无此需求，两者字段与生命周期不同，**不能混在一张表**。
- 拆为 `materials`（resource_type=MATERIAL）+ `generated_assets`（resource_type=ASSET）两张表。
- 生成记录不另建 generation_records 表——`generated_asset` 持 `job_id` + `step_run_id` 反向链接 execution 域的 StepRun（已存完整生成上下文），避免冗余。
- `generated_asset` 持 `source_virtual_ip_id` / `source_product_id`（从 job 输入填入），作为可达性的"容器"——与 material 的 `material_bindings` 角色对齐。
- `AssetReachability` 改为接收 `(resource_type, id)` 列表，按 resource_type 分派到 materials / generated_assets / products / virtual_ips。
- `ResolveAssetByKind` 按 asset_kind.resource_type 分发到 materials 或 generated_assets 表。

### 2026-07-03 · Review 反馈：asset_kind 升为注册实体
- `asset.asset_kind` 从代码枚举改为引用 `asset_kinds.name`（[asset_kind domain](../0005-asset_kind/asset_kind.md)）；UpsertMaterial 时校验 kind 已注册、未停用、resource_type 与表一致。
- 去掉 `AssetKindsByScope[scope]` 代码常量（asset_kind 全局注册，不绑 scope）。

## 来自业务规则的强约束
1. material 与 generated_asset 物理隔离（两张表），因生成记录需求不同。
2. asset_kind 引用注册表，全局不绑 scope；resource_type 决定落表。
3. Visibility 与 scope 正交：scope 管业务归属，visibility（private/team/public）管受众。
4. generated_asset 的生成记录字段（job_id / step_run_id / template_id / template_version）由 execution 填入不可改。
5. SoftDelete 不级联删 material_bindings，保留历史追溯。
# 2026-08-06

- Material 与 Generated Asset 增加 `workspace_id`。
- 文件位置由完整 URL 改为 Storage `object_path`；job/step 引用不再声明跨服务外键。
