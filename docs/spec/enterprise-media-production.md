# 企业素材与工作流平台 Spec

状态：Accepted（2026-09-17）。

本 spec 将 [002_materials.md](../human/research/002_materials.md) 固化为第一阶段可实施边界，覆盖 `media_materials`、`media_workflow`、Ory 登录鉴权、企业/部门/用户管理，以及可迁移到 UI Platform Slot 的 `ui_workflow`。它优先于既有 service impl 文档中与认证、执行调度和 UI 宿主冲突的段落；未被本 spec 改写的资源 CRUD 继续沿用原 spec。

## 1. 目标与非目标

### 1.1 目标

- 企业用户通过 Ory 登录，在 Organization → Team → Workspace 上下文中使用素材和工作流。
- Organization 持有功能授权上限；Team 默认继承，也可被企业管理员限制为子集。
- Template、Workflow、Material 等资源沿用 owner/visibility/grant 模型，不把企业功能授权混入资源 ACL。
- Workflow 是可版本化 DAG；每个 Step 最终只输出一个 Generated Asset，但一次生成可产生多个 Candidate 供用户接受或带反馈重生成。
- 发布的 Workflow 被可靠编译到 Conductor，支持并行、重试、人工等待、取消和恢复。
- `ui_workflow` 同一套业务组件可运行在 standalone Shell，也可作为 Materials/Workflow App 注入 UI Platform Slot。

### 1.2 非目标

- 本期不引入 Ory Hydra，不重做第三方 OAuth Provider/B2B Service Token。
- 不允许浏览器直接访问 Conductor，也不允许业务 UI 自由创建 Conductor Task Definition。
- 不实现任意第三方前端插件加载；只交付仓库内静态注册的 Materials/Workflow App。
- 不在 P0 优化任意 DAG 的最短关键路径；先保证编译语义正确和可恢复。
- 不改变现有资源共享语义：企业、部门功能授权不是 Template/Workflow/Material 的访问授权。

## 2. 系统边界

| 组件 | 权威职责 |
|:---|:---|
| Ory Kratos | 登录 Identity、凭证、自助登录/恢复、浏览器 Session |
| Ory Oathkeeper | 外部 API 策略执行；校验 Session；签发短时 Internal Identity JWT |
| Ory Keto | 企业、部门和资源关系的授权判定投影 |
| media_auth | User Profile、Organization、Team、Membership、Entitlement 与授权 Outbox |
| media_materials | Material、Generated Asset、Candidate、Product、Virtual IP、Asset Kind 与资源 ACL |
| media_workflow | Execution Capability、Template、Workflow、Compilation、Job、StepRun、Review Decision |
| Conductor | 技术工作流调度、任务重试、动态并行、WAIT 与恢复 |
| Capability Worker | 实现已注册的生成能力；幂等提交业务结果 |
| ui_workflow | Materials/Workflow 两个 App、standalone host adapter、plugin host adapter |

业务服务只信任经 Oathkeeper 校验的内部身份断言。Internal Identity JWT 只携带 `sub/user_id`、会话与请求追踪等稳定上下文；Organization/Team/Workspace 权限必须调用 Keto 或事实源检查，禁止把可变角色长期放进 JWT。

## 3. 企业、部门与权限

层级固定为：

```text
Organization（公司）
└── Team（部门，1..n）
    └── Workspace（工作空间，0..n）
```

- User 先成为 Organization Member，才能成为其 Team Member。
- Organization Entitlement 是企业可用功能上限。
- Team `entitlement_mode=inherit` 时有效功能等于企业授权；`restricted` 时等于企业授权与 Team 显式集合的交集。
- 企业管理员只能授予企业已有功能，不能抬高上限。
- Workspace 继承所属 Team 的有效功能；本期不增加 Workspace 级功能覆写。
- 资源访问继续使用 owner → team grant → public 的既有推导；Team entitlement 只回答“能否使用某功能”，不回答“能否读写某个资源”。

授权变更在事实源事务内写 Outbox，再幂等投影到 Keto。管理写入和撤权敏感操作应回查事实源；普通请求可使用 Keto，必须监控投影延迟。

Team 功能接口固定为：

- `GET /api/teams/{id}/entitlements`：Team 成员或 Organization admin 可读，返回 `entitlement_mode`、`configured_feature_keys`、`effective_feature_keys`。
- `PUT /api/teams/{id}/entitlements`：仅 Organization admin 可写，请求同时携带 `entitlement_mode` 与 `feature_keys`；restricted 集合必须是 Organization Entitlement 子集，inherit 模式清空显式集合。
- Team 创建、成员和角色变化、状态变化、功能变化均递增 `authorization_version`，并与 Outbox 原子提交。Organization Entitlement 变化还要触发其所有 Team 重新投影。

## 4. Workflow 的业务定义

### 4.1 Execution Capability

Capability 是 Worker 能力的注册契约，而不是用户可编辑节点：

```json
{
  "key": "jimeng.image.first_frame_prompt",
  "version": 1,
  "provider": "jimeng",
  "mode": "image_generation",
  "input_schema": {"type": "object", "required": ["prompt"]},
  "output_asset_kind": "image",
  "conductor_task_def": "media_capability_jimeng_image_v1",
  "retry_policy": {"count": 3, "backoff": "exponential"},
  "timeout_seconds": 900,
  "status": "published"
}
```

Task Definition 必须预注册并由平台管理。Capability published 后不可修改；Provider 密钥由 Runtime Environment/secret owner 提供，不进入 Capability、Workflow Definition 或 task payload。

### 4.2 Template

Template 是一次语义转换的不可变版本，定义输入槽、prompt、Capability 和唯一输出类别：

```json
{
  "template_id": "tpl_storyboard",
  "version": 4,
  "inputs": [
    {"name": "character", "asset_kind": "character_image", "required": true},
    {"name": "scene", "asset_kind": "scene_text", "required": true}
  ],
  "prompt_body": "在 {{.scene}} 中使用角色 {{.character}}",
  "capability": {"key": "jimeng.image.first_frame_prompt", "version": 1},
  "output_asset_kind": "image"
}
```

旧 `platform/gen_way/gen_type` 只用于迁移映射；新建和更新必须引用 Capability。

### 4.3 Step

Step 只表达业务节点，不暴露 Conductor 类型：

```json
{
  "key": "make_cover",
  "order": 20,
  "template": {"id": "tpl_storyboard", "version": 4},
  "bindings": [
    {"to_input": "character", "source": "workflow_input", "ref": "hero"},
    {"to_input": "scene", "source": "step_output", "ref": "write_scene"}
  ],
  "execution_policy": {
    "reuse": "reuse_if_available",
    "approval": "when_multiple_candidates"
  }
}
```

约束：

- `key` 在 Workflow Version 内稳定且唯一；只能引用更早 order 的 Step，DAG 无环且只有一个 sink。
- Template 和 Capability 都固定到不可变版本。
- binding source 仅为 `workflow_input | step_output | const`；一个前驱 Step 只暴露唯一最终 asset，不暴露 Candidate 列表。
- `execution_policy.reuse` 为 `reuse_if_available | force_new`；Workflow 默认前者，用户“重新生成”强制下一 attempt 为后者。
- `execution_policy.approval` 取值 `when_multiple_candidates | always`，默认前者。0 个 Candidate 失败；2 个及以上无条件等待用户审批；1 个在 `always` 时等待审批，否则自动确认。不存在允许多 Candidate 跳过审批的配置。

### 4.4 多 Candidate 与重新生成

Capability Worker 可以返回多张图，但它们先保存为当前 `step_run_id + attempt` 下的 Candidate：

- `accept(candidate_id)`：验证 Candidate 属于当前 attempt，登记/确认唯一 Generated Asset，StepRun 成功并解锁下游。
- `regenerate(feedback)`：保存反馈和决定，旧 Candidate 只读保留，`attempt + 1`，设置 `force_new` 并重新执行。
- 同一 attempt 只有一个 Review Decision；旧 attempt 晚到结果不得覆盖新 attempt。
- 下游 Step 永远只接收 `output_resource_id`，因此“多张图片”不会变成 Workflow 的多输出节点。

## 5. 编译到 Conductor

### 5.1 为什么不是 Step = SIMPLE Task

一个业务 Step 内包含缓存命中、Provider 执行、Candidate 持久化、用户确认和重新生成循环。若直接映射成 SIMPLE Task，会让业务节点受技术重试状态污染，也无法稳定表达人工等待。因此固定映射为：

```text
Business Step
└── SUB_WORKFLOW media_step_execution_v1
    └── DO_WHILE attempt_loop
        ├── resolve_inputs_and_reuse_v1 (SIMPLE)
        ├── if cache miss: capability task (DYNAMIC，解析到已注册 SIMPLE Task)
        ├── persist_candidates_v1 (SIMPLE)
        ├── if multiple or approval=always: review_wait (WAIT, externally completed)
        ├── accept -> finalize_candidate_v1 + loop complete
        └── regenerate -> persist feedback + attempt++ + force_new + next loop
```

Conductor 片段只返回：

```json
{"step_run_id": "sr_01...", "output_resource_id": "asset_01..."}
```

### 5.2 主 DAG 编译算法

P0 使用拓扑波次：

1. 以 `step_output` binding 构边并进行 Kahn 拓扑排序。
2. `wave(step) = 0`（无前驱），否则为 `1 + max(wave(predecessor))`。
3. 每个 wave 的节点可并行；波次之间设置完整屏障。
4. wave 只有一个 Step 时生成 SUB_WORKFLOW；多个 Step 时生成 `FORK_JOIN_DYNAMIC`，通过 `forkTaskWorkflow` 让每个输入分支调用 `media_step_execution_v1`，随后 JOIN。
5. 最后一波必须只含 `output_step_key`；其 `output_resource_id` 成为 Job 最终输出。

例：`A -> C`、`B -> D`、`C,D -> E` 编译为：

```text
Wave 0: dynamic fork [A, B] -> join_0
Wave 1: dynamic fork [C, D] -> join_1
Wave 2: subworkflow E
```

其中多节点波次的 Definition 形状固定为（省略通用字段）：

```json
[
  {
    "name": "wave_0_fork",
    "taskReferenceName": "wave_0_fork",
    "type": "FORK_JOIN_DYNAMIC",
    "inputParameters": {
      "forkTaskWorkflow": "media_step_execution_v1",
      "forkTaskWorkflowVersion": 1,
      "forkTaskInputs": "${workflow.input.wave_0_step_inputs}"
    }
  },
  {
    "name": "wave_0_join",
    "taskReferenceName": "wave_0_join",
    "type": "JOIN",
    "joinOn": []
  }
]
```

SubmitJob 先创建 StepRun，再按编译产物生成每个 `wave_n_step_inputs`，因此 runtime input 中已有稳定 `step_run_id`；Definition 本身不包含某次 Job 的 ID。

该结果语义正确，但 C 会等待 B，即使 C 不依赖 B。P0 明确接受这点；后续编译器可优化成依赖感知调度，业务 Workflow 无需迁移。

跨 Step 不依赖 Conductor fork 输出的 JSON 形状：每个 Step Fragment 以 `step_run_id` 从 media_workflow 读取 binding，`step_output` 解析为前驱 StepRun 已提交的唯一 `output_resource_id`。JOIN 只提供调度屏障，业务数据始终从事实源读取。

### 5.3 Step 子工作流输入

每个分支输入至少包含：

```json
{
  "job_id": "job_01...",
  "step_run_id": "sr_01...",
  "step_key": "make_cover",
  "workspace_id": "ws_01...",
  "scope": "ai_clothing",
  "principal_ref": "user_01...",
  "template": {"id": "tpl_storyboard", "version": 4},
  "capability": {"key": "jimeng.image.first_frame_prompt", "version": 1},
  "bindings": [],
  "reuse_policy": "reuse_if_available",
  "approval_policy": "when_multiple_candidates",
  "attempt": 1
}
```

`taskReferenceName = step__<sanitized_step_key>`；原始 key 始终保留在 input。payload 只传稳定 ID、内容 digest、Object Path 和非敏感参数，不传二进制、Provider secret、浏览器 cookie 或临时 URL。

### 5.4 发布与版本

Workflow 的保存不调用 Conductor；发布才编译：

```text
draft -> publishing -> published
                 └-> publish_failed
```

发布事务冻结 Workflow/Template/Capability 版本并写 Compilation + Outbox。注册成功后保存：

```text
(workflow_id, workflow_version)
  -> (conductor_name, conductor_version, definition_hash)
```

`conductor_name = media_workflow_<workflow_id>`，`conductor_version` 使用业务 Workflow version；若远端同 name/version 已存在，只有 canonical definition hash 相同才视为幂等成功，不同则失败且禁止覆盖。

### 5.5 Job 启动、状态与恢复

- SubmitJob 事务内创建 Job、全部 StepRun 和 Start Outbox。
- Adapter 使用 `correlationId=job_id` 启动映射的 Definition；启动超时先按 correlationId 查询。
- `media_workflow.jobs/step_runs` 是 API/UI 的状态源；Conductor 是调度源和运维诊断数据，不反向覆盖业务记录。
- Worker 先按 `(step_run_id, attempt, operation)` 幂等提交业务结果，再完成 Conductor Task；重投返回既有结果。
- 用户决定先写 `step_run_reviews + Outbox`，Adapter 再外部完成对应 WAIT Task；多 Candidate 始终进入 WAIT，单 Candidate 是否进入 WAIT 由 Step 的 approval policy 决定。
- Step Fragment 的 DO_WHILE 以持久化决定 `regenerate` 为继续条件；Conductor iteration 只用于技术循环，业务 attempt 从 StepRun 读取。设置部署级 `max_regenerations_per_step` 防止无限循环，达到上限后仍保留候选但拒绝继续生成。
- Job cancel 先提交本地 cancelled，再尽力 terminate Conductor；晚到完成只审计。

### 5.6 复用指纹

复用由 `media_gen`/生成领域负责，不使用 Conductor cache。指纹为 canonical JSON 的 SHA-256，至少含：

```text
workspace_id, scope, capability key/version,
rendered_prompt_digest, input_content_digests,
model, normalized_params, seed, output_asset_kind
```

只复用已成功、仍可达且内容完整的结果。用户 regenerate 必须 `force_new`，反馈进入 prompt/参数和新指纹。

## 6. Worker 接入与安全

Conductor 只部署在内部网络。普通用户和外部 Worker 不获得 Conductor API/全局 queue 凭证。`media_workflow` 提供 Worker Gateway：

- Worker 用 Oathkeeper 校验的服务身份连接；
- 根据 Runtime Environment、Workspace 和允许的 Capability 校验领取权；
- Gateway 代为 poll/update，或在受控内部部署中为 environment 映射 Conductor task domain；
- 每次完成都验证 `step_run_id`、attempt、Workspace 和 capability version；
- Provider secret 从 Runtime Environment 的 secret owner 按调用读取，不落日志/Conductor payload。

## 7. API 增量

### media_auth

- `GET /api/v1/me/context`：返回 User Profile、可见 Organization/Team/Workspace 摘要和有效功能。
- Organization/Team/Membership/Entitlement CRUD 沿 DDD 定义；写接口要求 organization admin 或 platform admin。
- Kratos webhook/import endpoint 以 `kratos_identity_id` 幂等创建或关联 User Profile。

### media_workflow

- `POST /api/v1/execution-capabilities`、`.../{key}/versions`：仅 platform admin。
- `POST /api/v1/workflows/{id}/publish`：异步返回 compilation id/status。
- `GET /api/v1/workflows/{id}/versions/{version}/compilation`。
- `POST /api/v1/jobs`：只接受 published 且 compilation registered 的版本。
- `POST /api/v1/step-runs/{id}/decisions`：`accept` 或 `regenerate`，必须带 idempotency key。
- Worker Gateway 的 poll/heartbeat/complete API 放 internal 路由，不进入浏览器 SDK。

所有外部写请求使用 `Idempotency-Key`，错误码稳定区分 `AUTH_REQUIRED`、`FORBIDDEN`、`ENTITLEMENT_DENIED`、`COMPILATION_PENDING`、`WORKFLOW_NOT_PUBLISHED`、`STALE_ATTEMPT` 和 `DECISION_CONFLICT`。

## 8. UI 架构与页面

### 8.1 Shell Context

```ts
type ShellContextValue = {
  principal: { userId: string }
  organization: { id: string; name: string }
  team: { id: string; name: string }
  workspace: { id: string; name: string }
  capabilities: ReadonlySet<string>
  navigate(intent: RouteIntent): void
}
```

RouteIntent 至少包含 `openWorkspace`、`openMaterials`、`openWorkflow`、`openOrganizationAdmin`、`openTeamAdmin`、`openUserAdmin`。业务 App 不拼接宿主 URL、不渲染 Menubar、不直接引用 `window.tos`。

- standalone adapter：本地 Router + Menubar，实现 `/o/:orgId/t/:teamId/w/:workspaceId`。
- plugin adapter：把 Host 当前上下文与导航 API 转成相同接口，不嵌套第二套 Desktop。
- App 局部标题和操作继续通过 App Header Portal 注入。
- Organization/Team/User 管理属于 Shell/管理页面；Materials/Workflow 属于 Workspace App。

### 8.2 第一阶段页面

- 登录/恢复：使用 Kratos browser flow，不自建密码表单协议。
- 企业与部门管理：企业资料、成员、Team、Team entitlement、Workspace 入口。
- Materials App：Material/Asset/Product/Virtual IP/Asset Kind 的既有 CRUD 和共享。
- Workflow App：Capability（管理员）、Template、Workflow DAG、发布状态、Job、Candidate Review。
- Candidate Review 必须同时展示当前 attempt、候选集、接受动作、反馈输入和重新生成动作；决定提交后锁定本 attempt。

## 9. 配置、部署与迁移

新增配置：Kratos public/admin URL、Oathkeeper URL/JWKS/audience、Keto read/write URL、Conductor API URL、task domain、poll timeout、definition namespace、Worker Gateway audience。admin endpoint 和 secret 只允许服务网络访问。

迁移顺序：

1. 引入 Ory 并建立 User ↔ Kratos Identity 映射，旧登录只读共存。
2. 建 Organization/Team/Workspace 层级及 Keto projection，先 shadow check 再 enforce。
3. 建 Capability 并把 legacy route 字段映射为版本化引用。
4. 建 Compilation 表、Conductor Adapter、Step Fragment 和 Worker Gateway。
5. 双写/对比现有执行路径，按 Workspace 灰度切换；保留回滚开关。
6. UI 切 Shell Context，交付 standalone，再验证 plugin adapter。
7. 停止旧 Cookie JWT 登录和旧 Dispatcher 新流量；历史 Job 仍由旧路径收尾。

## 10. 可观测性与验收

必须以 `request_id/job_id/step_run_id/conductor_workflow_id/task_id/attempt` 串联日志和 trace。指标至少覆盖 compilation latency/failure、outbox lag、Keto projection lag、task queue depth、worker heartbeat、WAIT age、candidate decision latency、cache hit、regeneration count 和 orphan reconciliation。

验收用例：

- 企业授权缩减后，Team 有效权限不会超出上限；资源 grant 结果不被错误改变。
- 任意合法单 sink DAG 编译后结果正确；并行波次有 JOIN 屏障，版本/hash 可重复注册。
- Worker 完成响应丢失并重投时只创建一个 Generated Asset。
- 两张 Candidate 时 Job 停在 needs_confirm；accept 只推进一个 asset；regenerate 保留旧候选并使用新 attempt。
- 服务重启后 WAIT、Outbox 和 Job 可恢复；取消后晚到结果不反转终态。
- standalone/plugin 共享业务组件，plugin 不渲染第二个 Shell，导航只经 Shell Context。
- 浏览器无法访问 Conductor/admin API，日志和 task payload 不出现密码、cookie、Provider key 或临时 URL。

## 11. 已确认决策

1. **DAG P0 使用拓扑波次屏障**：支持任意合法 DAG，接受部分过度等待；不把 P0 限制为线性/严格结构化 DAG。
2. **多 Candidate 强制审批，节点可要求单 Candidate 审批**：`approval=when_multiple_candidates` 时单 Candidate 自动确认；`approval=always` 时单 Candidate 也必须由用户选择接受或带反馈重新生成。
3. **重生成发生在同一 StepRun 的新 attempt**：保留旧 Candidate/反馈审计，不新建 Job，也不把它记作失败 Retry。
