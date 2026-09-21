# reachability workflow 变更记录

## 2026-09-21 · Workspace 与失败语义补全

- 按 Workflow 规范补齐触发/结果、步骤说明、关键参数、一致性边界、重试与失败矩阵。
- 明确 Product 虽无资源级 user ACL，Principal 仍必须具有 Workspace 使用权。
- `ResolveAssetByKind` 对 0 条或多条结果都失败，不允许隐式选择最新资源。

## 2026-07-21 · 初始建立

- 把原先散落在 asset / virtual_ip / product domain 的跨域可达性 RPC（`AssetReachability` / `ResolveAssetByKind` / `AssetRead` / `CheckProductAccessible` / `CheckVirtualIPAccessible` / `ResolveVirtualIpForReachability`）集中上提到本 workflow。
- **理由**：可达性决策需跨 asset / virtual_ip / product 三个 domain 加载事实。原设计把这些 RPC 放在 asset domain，会导致 asset domain 调用 virtual_ip / product domain（违反架构同层 domain 不可互依的 🔴 强制规则：`biz/domain/<a> ─X-> biz/domain/<b>`）。上提到 workflow（跨 domain 编排唯一合法入口）+ policy（无副作用纯决策）符合架构分层。
- 决策依据见 [ADR 0006](../../../adr/0006-reachability-layering.md)。
- `AssetRead` 明确不返回二进制，仅返回 `url` + `metadata`（与 materials 不存二进制的原则一致，修正原 asset.md「拿资源二进制」的措辞）。
- `AssetReachability` 确立为对 media_workflow 的唯一可达性契约；删除 workflow_validation.md 原先另设的 `BatchGetAssetForValidation`（返回完整 `*Asset` 实体、与不直读 materials 库原则冲突）。

### 2026-07-21 · 调用方对齐 bundle 模式
- 确认 media_workflow 采用 **bundle 模式**（见 [media_workflow spec](../../../../spec/media_workflow/impl.md) §6）：caller（BFF）在 SubmitJob 前调本 workflow 校验可达性 + 组装 JobInput bundle（含 url/content）；media_workflow 不调本 workflow。
- 本 workflow 调用方由 media_workflow 改为 **caller（BFF）**。
- `AssetReachability` / `ResolveAssetByKind` / `AssetRead` 等规则不变，仅调用方变更。
- media_workflow 仅直接调 `BatchGetAssetKind`（asset_kind domain）+ `UpsertGeneratedAsset`（generated_asset domain），不经本 workflow。
- 与上一条"初始建立"的关系：初始建立时假设由 media_workflow 调用，本次按现有 spec/实现的 bundle 决策校正调用方。
# 2026-08-06

- 调用方改为 media_workflow 等受信内部编排服务；Principal 来自认证上下文，不再信任 BFF 传入 user ID。
- 返回 Storage Object Path，不再组装包含完整 URL/content 的客户端 bundle。
