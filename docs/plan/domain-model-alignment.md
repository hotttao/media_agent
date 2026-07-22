# 领域模型对齐落地计划

> 基于 [ADR 0006](../adr/0006-reachability-layering.md) + 领域文档修订（2026-07-21）。把领域模型对齐（bundle 模式 + 可达性归层 + 单步能力 + Publish + bindings_end_at + scope 降级）落地到实现。
> 依据 spec：[media_materials/impl.md](../spec/media_materials/impl.md)、[media_workflow/impl.md](../spec/media_workflow/impl.md)。
> 领域文档：`docs/media_materials/`、`docs/media_workflow/`（含 [reachability workflow](../media_materials/workflow/0001-reachability/reachability.md)）。

## 总览

本次对齐涉及的实现变更（横跨 media_materials + media_workflow + caller）：

- **bundle 模式确认**：media_workflow 不调 reachability；caller（BFF）组装 JobInput bundle。实现已是 bundle（spec §6 对齐），无需改实现，仅文档对齐（已完成）。
- **可达性归层**（media_materials）：新建 `biz/workflow/reachability` + `biz/policy/reachability` + `reachability.proto`，供 caller 组装 bundle 时校验可达性 + 解析 asset。
- **material_bindings.bindings_end_at**（media_materials）：加列 + owner 软删置失效。
- **scope 降级**（media_materials）：`biz/domain/scope` -> `biz/shared/scope` 值对象。
- **单步能力**（media_workflow）：`job.proto` 加 `RetryStepRun` / `GetStepRun` + 实现。
- **Publish**（media_workflow）：`template` / `workflow.proto` 加 `PublishTemplate` / `PublishWorkflow` + 实现。
- **asset_kind TEXT 限制**（media_materials）：`resource_type=TEXT` 不可作 output_asset_kind / asset 实例的 asset_kind。

依赖方向：caller -> media_materials.reachability（组装 bundle）；media_workflow -> media_materials（`BatchGetAssetKind` + `UpsertGeneratedAsset`）。

## 阶段划分

```
P1 materials 数据层 + scope  ──> P2 materials reachability workflow  ──> P4 caller 组装 bundle  ──> P5 集成 + e2e
                                         │
P3 workflow 单步 + Publish  ──────────────┘（P2/P3 可并行）
```

P2 依赖 P1（`bindings_end_at` + `shared/scope`）；P3 独立（可与 P2 并行）；P4 依赖 P2 + P3；P5 依赖全部。

## P1 · media_materials 数据层 + scope 降级

**目标**：`bindings_end_at` 列 + scope 降 shared + asset_kind TEXT 限制。

### Checklist
- [ ] migration `media_materials/migrations/000NNN_material_bindings_end_at.{up,down}.sql`：`ALTER TABLE material_bindings ADD COLUMN bindings_end_at timestamptz;` + `CREATE INDEX idx_material_bindings_active ON material_bindings (owner_kind, owner_id) WHERE bindings_end_at IS NULL;`。
- [ ] virtual_ip / product 软删 service：置 `material_bindings.bindings_end_at = now()`（按 owner_kind 匹配）。
- [ ] 可达性 / ListByOwner 查询过滤 `bindings_end_at IS NULL`。
- [ ] scope 降级：`biz/domain/scope` -> `biz/shared/scope`（`SupportedScopes` 常量 + `IsKnownScope` / `IsSameScope` 纯函数）；service 层调用；domain 实体不直接依赖 shared（架构禁 domain -> shared）。
- [ ] asset_kind service：`resource_type=TEXT` 拒绝作 `output_asset_kind`（TEXT kind 仅 step_binding const 源）。
- [ ] config 加载 `supported_scopes`（shared/scope 启动期读）。
- [ ] arch-check + gofmt + go test。

### 验收
- `material_bindings` 加列；软删 vip/product 后对应 binding `bindings_end_at` 非 null、可达性查询忽略。
- scope 校验由 `shared/scope` 完成，`IsKnownScope` 拒绝未知 scope。
- `asset_kind` TEXT 不可作 output。

## P2 · media_materials reachability workflow + policy

**目标**：新建 reachability workflow + policy + proto，供 caller 组装 bundle。

### Checklist
- [ ] IDL `idl/media_materials/reachability.proto`（spec §2.6）：`AssetReachability` / `CheckProductAccessible` / `CheckVirtualIPAccessible` / `ResolveVirtualIpForReachability` / `ResolveAssetByKind` / `AssetRead`；`make hz-model` + `make -C media_materials hz-update`。
- [ ] `biz/policy/reachability`：纯决策（scope ∧ visibility ∧ 容器可达）；接收 service 加载的事实，返回 `ReachabilityResult`；不查 DB。
- [ ] `biz/workflow/reachability`：编排 asset / virtual_ip / product service 只读查询加载事实 -> 调 policy 决策；跨 domain 编排唯一合法入口（[ADR 0006](../adr/0006-reachability-layering.md)）。
- [ ] handler 内部 RPC（不暴露 HTTP，供 caller 调用）。
- [ ] `fetchTeams` 接入 media_auth（team 可见性；当前 stub 返 nil--前置依赖，见风险）。
- [ ] arch-check（验证 workflow -> 多 service、policy 纯决策无 DB、domain 同层隔离）。
- [ ] 单测：policy 决策各分支 + workflow 编排。

### 验收
- `AssetReachability` 返回 reachable / blocked + reason（SCOPE_MISMATCH / NOT_VISIBLE / CONTAINER_UNREACHABLE / NOT_FOUND）。
- `ResolveAssetByKind` 按 resource_type 分派，0 / >1 报错。
- `AssetRead` 返 url + metadata（不返二进制）。
- policy 无 DB 调用；workflow 只读不开事务。

## P3 · media_workflow 单步能力 + Publish

**目标**：`RetryStepRun` / `GetStepRun` + `PublishTemplate` / `PublishWorkflow`。

### Checklist
- [ ] IDL `idl/media_workflow/job.proto` 加 `RetryStepRun` / `GetStepRun`（spec §2.3）；`template` / `workflow.proto` 加 `PublishTemplate` / `PublishWorkflow`；hz 生成。
- [ ] service：`RetryStepRun`（failed/skipped -> pending + `retry_count+1` + 下游重置 + `input_override`）；`GetStepRun`（只读）；`PublishTemplate` / `PublishWorkflow`（draft -> published，owner，状态机守卫）。
- [ ] 状态机守卫：archived 不可逆；published -> archived -> 不可回；draft ↔ published 双向。
- [ ] bundle 模式确认：execution 不调 reachability（实现已是，确认）；`RetryStepRun` 不调 reachability（caller 责任）。
- [ ] 路由 auth 中间件注册（前置依赖，见风险）：`biz/shared/auth/middleware.go` 挂到 `biz/router/<service>/middleware.go`，否则 `CallerID` 恒空、权限全失效。
- [ ] arch-check + gofmt + go test。

### 验收
- `RetryStepRun` 重跑单步 + 下游；`RetryJob` 重跑整 Job；`RegenerateStepRun` 仅 need_confirm；三者边界清晰。
- `GetStepRun` 返回单条 step_run。
- `PublishTemplate`：draft -> published；archived 不可 publish。
- execution 不调 media_materials（除 `BatchGetAssetKind` + `UpsertGeneratedAsset`）。

## P4 · caller（BFF）组装 bundle

**目标**：caller 调 reachability + 组装 JobInput bundle + 提交 SubmitJob。

### Checklist
- [ ] 确定 caller 落点（`media-cli` 或独立 BFF）；实现组装逻辑。
- [ ] caller 调 `media_materials.reachability`：`AssetReachability` 校验 + `ResolveAssetByKind` 解析 product/vip 输入 + `AssetRead` 拿 url/content。
- [ ] caller 组装 JobInput bundle（material/asset/product/virtual_ip data）。
- [ ] caller 调 `media_workflow.SubmitJob` 提交 bundle。
- [ ] retry 时 caller 重新校验可达性 或 信任原 bundle + `input_override`。

### 验收
- caller 组装的 bundle 自包含 url/content。
- 不可达 asset 被 reachability 拒绝（不进 bundle）。
- media_workflow 收到 bundle 后不调 reachability。

## P5 · 集成 + e2e

**目标**：端到端跑通。

### Checklist
- [ ] e2e：caller 组装 bundle（reachability 校验）-> SubmitJob -> worker claim/complete/confirm -> 产物落 generated_assets。
- [ ] e2e：`RetryStepRun` / `RetryJob` / `Publish` / `bindings_end_at` 失效场景。
- [ ] docker-compose 含 media_materials + media_workflow + caller stub。

### 验收
- 端到端全绿；media_workflow 不调 reachability（仅 `BatchGetAssetKind` + `UpsertGeneratedAsset`）；caller 调 reachability。

## 风险与对策

| 风险 | 对策 |
|:---|:---|
| `fetchTeams` stub（media_auth 未接入 internal teams endpoint） | reachability team 可见性降级为 false；**P2 前置**：media_auth 加 `/internal/teams?user_id=xxx` 供下游服务查 team |
| media_workflow 路由 auth 中间件未注册（`CallerID` 恒空） | **P3 前置**：注册 `auth.Middleware` 到 `biz/router/<service>/middleware.go`；否则 owner/grant 权限全失效 |
| scope 降级影响各 domain service 调用 | P1 集中迁移 `shared/scope`；arch-check 验证 domain 不依赖 shared |
| reachability 跨 domain 编排事务 | workflow 只读，不开事务；policy 纯决策无副作用 |
| caller 组装逻辑散落 | 收口到 media-cli 或 BFF；reachability 集中校验避免散落 |
| `RetryStepRun` / `RegenerateStepRun` / `RetryJob` 语义混淆 | 状态机单测覆盖所有转移；文档固化边界（workflow_execution §7-8） |
| generated_asset upsert 与 retry name 语义 | name 是业务维度稳定标识，retry 默认覆盖同产物行；旧 attempt url 入 retry_history |

## 依赖

- P1 无外部依赖（除 media_auth team 概念，预留 team_id 字段）。
- P2 依赖 P1（`shared/scope` + `bindings_end_at`）+ media_auth `fetchTeams`（前置）。
- P3 独立（可与 P2 并行）；依赖 media_workflow 路由 auth 注册（前置）。
- P4 依赖 P2（reachability）+ P3（SubmitJob + 单步能力）。
- P5 依赖全部。

## 开发顺序

P1 -> P2 ‖ P3 -> P4 -> P5。P2 与 P3 可并行（不同服务）。P2 前置 media_auth `fetchTeams`；P3 前置 media_workflow auth 中间件注册。每阶段 PR 独立 review。
