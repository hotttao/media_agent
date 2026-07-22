# 可达性决策归层：上提到 workflow + policy

media_materials 的 `AssetReachability` 等跨域可达性 RPC 从 asset domain 上提到独立的 `biz/workflow/reachability`（编排 asset / virtual_ip / product 事实加载）+ `biz/policy/reachability`（纯决策）。

**核心理由**：可达性决策需跨 asset / virtual_ip / product 三个 domain 加载事实，留在 asset domain 会产生 `asset -> virtual_ip` / `asset -> product` 的同层 domain 互依，违反架构 🔴 强制规则（`biz/domain/<a> ─X-> biz/domain/<b>`，同层隔离）。workflow 是跨 domain 编排的唯一合法入口，policy 承载无副作用纯决策，二者组合是唯一合规落点。

**备选方案考量**：
- ① service 稳定接口（asset service 通过稳定接口读他域查询）：架构允许单 domain 用例内读他域数据，但可达性跨 4 个 domain 事实加载，asset service 会过胖且决策混入 service 层。
- ② 维持 asset domain + 豁免同层隔离：直接违反强制规则，否决。

**衍生影响**：asset / virtual_ip / product domain 的能力清单移除上述跨域 RPC，仅保留各自 CRUD + 只读查询能力供 reachability workflow 调用。

**调用方**：reachability workflow 的调用方是 **caller（BFF / 编排层）**，在 SubmitJob 前校验可达性 + 组装 JobInput bundle（含 url/content）；media_workflow 不调本 workflow（采用 bundle 模式，execution 用 bundle 渲染，仅 `BatchGetAssetKind` + `UpsertGeneratedAsset` 由 media_workflow 直接调 asset_kind / generated_asset domain）。
