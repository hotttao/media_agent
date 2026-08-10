---
name: scope
description: 已废弃（2026-07-21）— 降为 biz/shared/scope 值对象
status: deprecated
---

## ⚠️ 已废弃（2026-07-21）

本 domain 已废弃，scope 降级为 `biz/shared/scope` 值对象，不再作为独立 domain。

**降级理由**：scope 无表、无接口、无实体，仅 `SupportedScopes` 闭集常量 + `IsKnownScope` / `IsSameScope` 纯函数，本质是值对象 + 纯校验工具，符合 `shared`（跨 domain 应用工具）定位，不具备 domain 的持久化与行为特征；保留 0001 domain 编号会误导为 peer domain。

**降级后的承载**：

| 内容 | 迁移去向 |
|:---|:---|
| `SupportedScopes` 常量 + `IsKnownScope` / `IsSameScope` / `RejectIfDifferentScope` 工具 | `biz/shared/scope`，由 service 层调用（架构禁止 domain -> shared，故 domain 实体不直接调，scope 校验在 service 创建用例完成） |
| scope 业务规则（1:1 严配、跨 scope 引用非法、scope 正交 visibility、scope 不授权创建） | 各资源 domain（[asset](../0002-asset/asset.md) / [virtual_ip](../0003-virtual_ip/virtual_ip.md) / [product](../0004-product/product.md)）的校验细则，作为不变式承载 |
| `IsSameScope` 跨服务一致性 | 不再以 RPC 暴露；scope 是字符串常量，各 service 从同一份 `supported_scopes` 配置加载，本地比较即一致（跨 service scope 校验由 `media_workflow.workflow_validation` 调 `AssetReachability` / `Check*Accessible` 间接完成，见 [reachability workflow](../../workflow/0001-reachability/reachability.md)） |
| Scope 术语 | [CONTEXT.md §业务范围](../../CONTEXT.md) |

**未单独立 ADR**：属轻量、可回退决策（恢复为 domain 仅需迁回目录 + 补 domain 文档）。与之相关的可达性归层决策见 [ADR 0006](../../../adr/0006-reachability-layering.md)。
