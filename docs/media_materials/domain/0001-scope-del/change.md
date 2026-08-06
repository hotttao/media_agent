# scope domain 变更记录

## 2026-07-03 · 初始建模（media_agent v1）
- `media_ai` 的 spec 没有 scope，本期**新引入**。
- scope 值集合 = `config.SupportedScopes`，**不**落表、不暴露注册接口——由 ops 改 yaml 即可，service 启动期加载。
- asset_kind 从 `asset_kind_registrations` 表下移到代码配置 `AssetKindsByScope[scope] → []string`，理由：避免"接口注册 + DB 验证"增加运行时开销，且当前业务场景下 asset_kind 范围是研发 vs 业务共同决定的稳定集合，不是运营运行时配置。
- 1:1 严格相等是产品决议：用户明确不要父子层级。
- 删去最初构想的 `workspace` domain；workspace 仅留术语解释，不必为它单建 domain。

## 修改记录

### 2026-07-21 · 废弃 domain，降为 shared 值对象
- scope 无表、无接口、无实体，仅 `SupportedScopes` 闭集常量 + `IsKnownScope` / `IsSameScope` 纯函数，本质是值对象 + 纯校验工具，符合 `shared` 定位，不具备 domain 持久化与行为特征。
- 降级为 `biz/shared/scope`（常量 + 工具函数），由 service 层调用；domain 实体不直接依赖 shared（架构禁止 domain -> shared）。
- scope 业务规则（1:1 严配、跨 scope 引用非法、scope 正交 visibility、scope 不授权创建）改由各资源 domain（asset / virtual_ip / product）校验细则承载为不变式。
- `IsSameScope` 不再以跨服务 RPC 暴露；scope 为字符串常量，各 service 从同一份 `supported_scopes` 配置加载，本地比较即一致。
- 目录改名 `0001-scope-del` 标记废弃。
- 未单独立 ADR：轻量、可回退决策。相关可达性归层决策见 [ADR 0006](../../../adr/0006-reachability-layering.md)。

### 2026-07-03 · Review 反馈：user 不持 scope，创建不限 scope
- P0-2 决策：user **没有 scope 概念**；user 通过拥有的 [virtual_ip](../0003-virtual_ip/virtual_ip.md) 间接关联 scope（virtual_ip 带 scope）。
- 创建不限制 scope：任何 user 可在任何 scope 下创建 virtual_ip / template / workflow / asset（暂不限制）。scope 仅作资源属性盖章 + 引用一致性校验，**不是授权边界**。
- 据此删除所有"user scope 上下文 / workspaceID / 调用方 scope"残留：`AssetReachability` 锚点改为 `workflowScope`（workflow 的 scope）；List 接口的 `?scope=x` 是显式过滤器，无"user 默认 scope"。
- 既不需要 `user.scope` 字段，也不需要 `user_scopes` 授权表——比方案 B/D 都简单。后续若要收口"谁能创建哪个 scope"，再加 user↔scope 授权。

### 2026-07-03 · Review 反馈：asset_kind 升为独立注册实体（全局，不绑 scope）
- 把 `MatchAssetKind` / `AssetKindsByScope` 从 scope domain 移除——asset_kind 现归独立的 [asset_kind](../0005-asset_kind/asset_kind.md) domain，全局注册，无 scope 字段。
- 反转第一轮"asset_kind 属于 scope"决策：`scene`/`pose` 等类别语义跨业务线一致，业务线归属由 material / generated_asset 实例自身的 scope 承担。
- scope domain 现仅保留 `SupportedScopes` / `IsKnownScope` / `IsSameScope`。

## 来自业务规则的强约束
1. scope 不可改。理由同 template / workflow 域：版本化与可达性验证都依赖稳定语义。
2. scope 正交于 visibility，不要把"私有工作空间"用 scope 区分，而用 visibility。
3. 第三轮反馈：Asset / VirtualIp / Template / Workflow 的可见性统一为 `private / team / public`（去掉原本的 `workspace` 档位）——见 [media_auth §资源级 ACL](../../../media_auth/CONTEXT.md#资源级-acl-所有媒体资源通用)。scope 保持 1:1 严格相等的业务线隔离。
4. user 不持 scope；scope 是资源属性。创建不限 scope（暂不限制）。
5. asset_kind 全局注册，不绑 scope；scope domain 不再校验 asset_kind。
