# asset_kind domain 变更记录

## 2026-07-03 · 初始建模（media_agent v1）
- 对齐 media_ai v3.3 的 asset_kind 注册实体模型。
- **反转第二轮"代码枚举"决策**：asset_kind 从 `config.AssetKindsByScope[scope]` 代码常量升级为 `asset_kinds` 注册表实体 + CRUD（Register/Get/List/Update/Delete）。
- **反转第一轮"asset_kind 属于 scope"决策**：asset_kind 改为**全局注册**，无 scope 字段。理由：`scene`/`pose` 等类别语义跨业务线一致，业务线归属由 asset 实例自身的 scope 承担，类型不该重复绑 scope。
- 字段对齐 media_ai：`name` / `resource_type` / `media_type` / `source_table` / `status`。
- `resource_type ∈ {MATERIAL, ASSET, PRODUCT, VIRTUAL_IP, TEXT}`——asset_kind 是覆盖所有资源类别（含 product / virtual_ip / text）的统一注册表，不只服务于 asset。
- `media_type ∈ {IMAGE, TEXT, VIDEO}`——取代旧的 `output_resource_type` 概念；Template 的媒体形态由所引 asset_kind.media_type 推导。

## media_agent 与 media_ai 的差异
- 无差异：material / generated_asset 同样拆两张表（`materials` / `generated_assets`），source_table 指向实际表。

## 来自业务规则的强约束
1. asset_kind 全局注册，无 scope。
2. `name` / `resource_type` / `media_type` / `source_table` 创建后锁定不可改。
3. SYSTEM 不可删；DISABLED 仍列出但拒绝被新 Template 引用。
4. Delete 前校验无 asset / template 引用，防悬挂。
5. resource_type 决定落表：MATERIAL→materials，ASSET→generated_assets，PRODUCT→products，VIRTUAL_IP→virtual_ips，TEXT→inline。
