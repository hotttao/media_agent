---
name: scope
description: scope 归属校验 + 跨 service 契约（`config.SupportedScopes` 闭枚举）
dependencies: []
related_workflow: []
---

## 一句话职责
scope domain 在 materials 内承载"业务归属字段"的**校验与跨域门面**：scope 是个字符串，值取自 `config.SupportedScopes` 闭集合，不开接口、不开表。本域**不创建数据**——只在 materials / virtual_ips / products 创建时校验 scope 合法性，对外提供 `IsKnownScope` / `IsSameScope` 工具。

### 不做什么
- 不引入 scope 注册表（值在代码常量里固化）。
- 不管理 asset_kind（asset_kind 是全局注册实体，归 [asset_kind](../0005-asset_kind/asset_kind.md) domain，**不绑 scope**）。
- 不管理用户或工作空间（用户身份归 `media_auth`）。
- 不暴露 owner / visibility 检查（归 [asset](../0002-asset/asset.md)）。

## 边界

### 做什么
- 提供 `SupportedScopes() []string` —— 当前部署支持的 scope 闭集合。
- 提供 `IsKnownScope(s string) bool` —— 创建资源时校验非空且在闭集合内。
- 提供 `IsSameScope(a, b string) bool` —— 严格相等；跨 service 同名调用结果必须一致（同一份常量）。
- 提供 `RejectIfDifferentScope(want, got)` —— 在 binding 创建时立即抛 `ErrScopeMismatch`，db 不写。

### 不做什么
- 不对 scope 取值本身做语义解释（"AI 服装"的运营策略归 ops）。
- 不允许父子层级或集合包含关系——scope 是 1:1 严格相等。
- 不校验 asset_kind（asset_kind 全局注册，与 scope 正交）。

## 能力清单

| 能力 | 说明 |
|:---|:---|
| `SupportedScopes()` | 返回闭集合 |
| `IsKnownScope(s)` | 校验值合法 |
| `IsSameScope(a, b)` | 严格相等 |

## 数据模型

本域**不引入任何新表**。所有 scope 字段都已存在于 materials / generated_assets / virtual_ips / products / templates / workflows 表上（asset_kind 无 scope 字段）。

`SupportedScopes` 在代码层定义（启动期从 yaml / Consul KV 加载），不读 DB：

```yaml
# conf/local.yaml — 仅示意；具体配置方式见后续 spec
supported_scopes: [ai_clothing, ai_guoxue]
```

## 业务规则

1. **1:1 严配**：所有带 scope 字段的记录，scope 不可改；scope 必须 `IsKnownScope(scope)`。
2. **跨 scope 引用 = 非法**：
   - `material_bindings` 强制两端 scope 一致——由 service 在 INSERT 前比较。
   - `media_workflow` 与 step 引用的 template / material / asset / product / virtual_ip 的 scope 必须相等——由 `media_workflow.workflow_validation` 跨 service 校验，调 `IsSameScope` RPC。
   - asset_kind **不参与 scope 校验**（全局注册，无 scope）。
3. **scope 字符串大小写敏感**；service 不归一化。
4. **scope 正交于 visibility**：scope 管业务归属（引用一致性），visibility（private/team/public）管受众。两者同时生效。
5. **scope 不授权创建**：当前版本不限制 user 能在哪些 scope 下创建资源（见 [Scope 术语](../../media_auth/CONTEXT.md#scope业务范围)）。scope 仅作资源属性盖章；user 通过拥有的 [virtual_ip](../0003-virtual_ip/virtual_ip.md) 间接关联 scope，但创建 virtual_ip 本身也不限 scope。后续若要收口"谁能创建哪个 scope"，再加 user↔scope 授权表。

## 关联关系

### 被依赖的 domain
- [asset](../0002-asset/asset.md) — 创建时校验 IsKnownScope；material_bindings INSERT 前 IsSameScope。
- [`media_workflow.workflow_validation`](../../../media_workflow/workflow/0001-workflow_validation/workflow_validation.md) — 跨域 RPC `IsSameScope`。

### 关联术语
[Scope](../../media_auth/CONTEXT.md#业务划分与归属)、[Asset Reachability](../../CONTEXT.md)、[Asset Kind](../../CONTEXT.md)（全局注册，归 asset_kind domain）。
