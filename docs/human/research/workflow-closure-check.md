# 流程闭环检查

> 本 doc 用具体场景依次走完 `media_materials` + `media_workflow` 主流程，逐条验证业务规则在端到端上是否漏点。每条场景以"动作序列 → 规则触点 → 期望输出"呈现。
>
> **权限模型**：以第三轮反馈为准——Template / Workflow / VirtualIp 都按 [media_auth §资源级 ACL](../../ddd/media_auth/CONTEXT.md#资源级-acl-所有媒体资源通用) 走 `visibility ∈ {private, team, public}` + `grants (resource_id, team_id, permission)` + 推导 `my_permission`，Asset 仅 `visibility + team_id`。
>
> **Job**：状态机由 v1 的"failed 终态"扩为可 in-place retry（retry_count 上限默认 3）。

## 场景 A — AI 服装业务，单人完整闭环

**前置条件**：
- `config.SupportedScopes = ["ai_clothing", "ai_guoxue"]`
- asset_kind 注册表（全局）已注册：`model_image`(ASSET/IMAGE) / `scene`(MATERIAL/IMAGE) / `pose`(MATERIAL/IMAGE) / `movement`(MATERIAL/VIDEO) / `product_image`(MATERIAL/IMAGE) 等

**人物**：Alice（user）

**动作序列**：

1. Alice 登录 media_auth → 拿 session。
2. Alice 在 media_materials 创建 VirtualIp V1，`scope=ai_clothing`，owner=Alice。  
   **触点**：scope.md rule 1（IsKnownScope）✓ → 创建成功。
3. Alice 上传 2 个 material 到 V1（每个 material.scope=ai_clothing，asset_kind.resource_type=MATERIAL）：
   - `scene_1`（asset_kind=scene）
   - `pose_1`（asset_kind=pose）  
   visibility 均为 `public`（便于场景 A 单人闭环；team/private 同理可推）。  
   触发 AddMaterialBinding：`IsSameScope(material.scope, V1.scope) == true` ✓ → 绑成功。
4. 创建 Template T1（scope=ai_clothing）：
   ```json
   {
     "inputs": [
       {"name": "scene_image", "kind": "scene", "required": true},
       {"name": "pose_image",  "kind": "pose",  "required": true}
     ],
     "output_asset_kind": "first_frame",
     "platform": "gpt", "gen_way": "i2i",
     "prompt_body": "以 {pose_image.url} 的人物置于 {scene_image.url}，输出首帧图"
   }
   ```
   触点：template.md 不变式全过（slot.kind 在 asset_kinds 注册表已注册）；status=draft；version=1。
5. 创建 Workflow W1（scope=ai_clothing）：
   ```json
   {
     "inputs": [
       {"name": "scene_in",  "type": "material", "required": true},
       {"name": "pose_in",   "type": "material", "required": true}
     ],
     "output_step_key": "gen_first_frame",
     "steps": [
       {
         "key": "gen_first_frame",
         "template_id": T1.id,
         "template_version": T1.version,
         "bindings": [
           {"to_input": "scene_image", "source": "workflow_input", "ref_workflow_input_name": "scene_in"},
           {"to_input": "pose_image",  "source": "workflow_input", "ref_workflow_input_name": "pose_in"}
         ]
       }
     ]
   }
   ```
   **触点**：workflow.md §校验细则 — step_key 唯一 ✓；template 存在且未 archive ✓；binding 三选一互斥 ✓；slot.kind.resource_type=MATERIAL 与 workflow_input.type=material 自洽 ✓；DAG 1 节点 1 sink ✓。
6. 提交 Job J1，inputs:
   ```json
   {
     "scene_in": {"material_id": scene_1.id},
     "pose_in":  {"material_id": pose_1.id}
   }
   ```
   触发 [workflow_validation](../../ddd/media_workflow/workflow/0001-workflow_validation/workflow_validation.md)：
   - DAG 校验 ✓。
   - Template 校验：scope 严格相等 ✓，未 archive ✓。
   - **AssetReachability**：scope_match(material.scope==workflow.scope==ai_clothing) ✓；visibility=public ✓；material_bindings(V1) + V1 对 Alice 可见（owner）✓ → reachable。
7. Job 进入 running：
   - workflow_execution 解析 binding → ResolvedSlot：`source=asset`，`AssetURL=scene_1.url / pose_1.url`（AssetRead(MATERIAL, id)）。
   - Render T1 → prompt 字符串（含 url 替换）；output_asset_kind=`first_frame`，media_type=IMAGE。
   - 按 T1.platform=gpt 调下游生成器 → 产出单一 generated_asset；UpsertGeneratedAsset（带 job_id/step_run_id/template_id/template_version/source_virtual_ip_id=V1）→ `generated_asset_v1`。
   - StepRun succeeded（output_asset_id = asset_v1.id）；Job succeeded。
8. Alice 通过 `GET /api/v1/jobs/{J1.id}` 看到 sink step 的 `output_asset_id` = `generated_asset_v1`，再到 media_materials `GET /api/v1/generated-assets/{id}` 拿到 url。

**结论**：单业务、单人、纯 material 输入 → 产出 generated_asset 的场景闭环。✓

## 场景 B — 跨 scope 引用（应被拒）

**前置条件**：同 A，但额外有 V2（scope=ai_guoxue）。

**动作**：
- Alice 创建模板 T2（scope=ai_clothing），引用 V2 上传过的素材 X（scope=ai_guoxue，asset_kind=illustration）。
- Alice 创建 Workflow W2，把 T2 与 X 拼起来。

**触点**：[workflow_validation](../../ddd/media_workflow/workflow/0001-workflow_validation/workflow_validation.md) → AssetReachability：
- scope_match(ai_clothing vs ai_guoxue) → reject `SCOPE_MISMATCH`。
- 不写库。

**结论**：跨 scope 引用被拒。✓

## 场景 C — 可见性卡住（应被拒）

**前置条件**：Asset A 由 Bob 创建，visibility=private。Bob 在 V_user 里。

**动作**：Alice 想把 A 用到自己的 workflow。

**触点**：AssetReachability：
- visibility=private && owner_user=Bob≠Alice → reject `NOT_VISIBLE`。

**结论**：私有素材不能被外人用。✓

## 场景 D — const 满足 required（按反馈允许）

**动作**：
```json
{
  "template T3": {
    "inputs": [
      {"name": "language", "resource_type": "text", "required": true}
    ],
    "output_asset_kind": "caption",
    "prompt_body": "用 {language} 语言生成一段描述"
  }
}
```
绑定：`{to_input: "language", source: "const", const_value: "zh-CN"}`。

**触点**：
- workflow.md §校验细则 step 4（已放宽）：required + const ✓。
- Render：ResolvedSlot.source=const, ConstValue="zh-CN" → 渲染 prompt = "用 zh-CN 语言..."。

**结论**：const 满足 required 合法路径。✓

## 场景 E — const 类型不匹配（应被卡在 Render 阶段）

继承 D，但 const_value = 123（数字）。

**触点**：
- workflow 校验阶段：source=const、value 非 nil → 通过。
- Render 阶段：resource_type="text" 但 ConstValue 是 number → 序列化规则按实现：
  - 实现若严格匹配 → reject ErrTypeMismatch；
  - 实现若宽松做 strconv → 渲染为 "123"。

**当前设计**：不强制类型严格匹配（template.md §Render 第 4 点"其它类型不严格要求"）。如果你希望严格对齐 resource_type，告诉我，我把它升级为 hard rule。

## 场景 F — 模板 archive 后引用（应被拒）

**动作**：场景 A 第 7 步完成；后来运营 archive Template T1（status=archived）。

- 此时 Alice 想 **新建**一个 workflow W3 引用 T1：workflow_validation → Template archive → reject `TEMPLATE_ARCHIVED`。
- 已存在的 J1 在 workflow.version + template.version 上有版本快照，archive 不影响 J1 重跑（按 execution.change.md 的设计）。

**结论**：archive 只阻止新建引用，不破坏历史 Job 可重现。✓

## 场景 G — Job 提交时模板已被改版（应被拒）

**前置**：W1 引用 T1@version=1，已 SubmittedJob J1 还没起 run。

**动作**：有人 Update T1 → version=2；J1 仍写的是 T1@version=1。

**触点**：J1 入 running 时 workflow_execution 会**重读 Workflow 版本 + Template 版本**——J1.workflow_version/W1.version，J1 的 StepRun.template_version/T1@1。LoadTemplate(T1.id, 1) 仍能找到旧版本快照 → 渲染用旧模板 → 结果可重现。

若 J1 引用的是 T1@version=2，但 T1@version=2 已被删除（极端情况）：Render 阶段找不到 TemplateVersion → reject，StepRun failed。

**结论**：版本快照保证可重现，前提是旧 version 不能硬删（template.md §版本化）。

## 场景 H — Job 失败后 in-place retry

**前置**：场景 A 中 J1 跑到 gen_first_frame 这一 Step Run 时 media_materials 返回 5xx（场景渲染 → 资产 upsert 失败），StepRun.failed，Job.failed，`failure_reason="downstream timeout"`，retry_count=0。

**动作**：Alice 改用 `scene_2`（同一 scope/visibility），调 `POST /api/v1/jobs/{J1.id}/retry` body `{retry_inputs: {scene_in_workflow_input: {asset_id: scene_2.id}}}`。

**触点**：
- workflow_validation 调用：retry user 必须 viewer+ on workflow ✓（Alice 是 owner）。
- Job.status=failed ✓ → 进入 retry 流。
- retry_count += 1 = 1。
- 所有 succeeded StepRun 保留（本例只有 input 解析，model_in_step run 实际没独立 StepRun；若有多步，已 succeeded 的跳过）。
- 对 `binding.source=workflow_input` 且 `to_input=scene_image` 的 step：覆盖其 `resolved_inputs[scene_image].asset_id = scene_2.id`，其它 input 不变。
- 重新走 AssetRead(scene_2)、Render → Run → success → output_asset_id 覆盖。
- Job.status=succeeded。

**期望**：`GET /api/v1/jobs/{J1.id}` 看到 retry_count=1、`retry_history=[{retry_at, reason, retry_inputs.scene_in_workflow_input}]`、`status=succeeded`、sink step 的 `output_asset_id` 为 scene_2 跑的产物。

**结论**：in-place retry 闭环。✓

## 场景 I — retry 上限耗尽

**前置**：J1 已经 retry_count=3，再次 retry。

**触点**：service 校验 `retry_count < MaxRetries` (3) → 拒 `ErrRetryLimitExceeded`，建议改 Submit 新 Job。

**结论**：retry 上限保护。✓

## 场景 J — Template 跨业务线可见，但 workflow 拒绝引用

**前置**：T1 在 ai_clothing，业务员把 visibility 改为 public。Alice 想在 ai_guoxue 的 workflow W 里引用 T1。

**触点**：
- `GET /api/v1/templates?scope=ai_clothing&name=T1`：Alice 显式按 `scope=ai_clothing` 过滤即可看到 T1（visibility=public，任何 user 可读；user 无 scope，scope 只是过滤器，不卡 user）。
- Alice 创建/更新 W（scope=ai_guoxue）时 step 引用 T1 → workflow_validation 阶段 2 校验 `Template.Scope == Workflow.Scope`：ai_clothing ≠ ai_guoxue → reject `SCOPE_MISMATCH`，workflow 不写库。

**结论**：scope 是资源引用一致性边界，不是 user 可见性边界——Alice 能"看到"跨 scope 的 public template，但**不能**把它绑进另一 scope 的 workflow。✓

> List 接口策略：`?scope=x` 是调用方显式传的过滤器，无"user 默认 scope"概念；不传 scope 则跨 scope 列出所有可见（visibility/grants）资源。

## 场景 K — Team Grant 共享 Template

**动作**：Alice 创建 T1（visibility=private），调 `GrantTemplate {team_id: "team-x", permission: "viewer"}`；Bob 在 team-x 内。

**触点**：
- T1.visibility 仍为 private，T1 不在 public 列表里。
- Bob 调 `ListTemplates`：`_acl.listVisibleToUser(userID=bob)` 触发 grants 查询：
  ```
  with my_templates = SELECT t.id FROM templates t JOIN template_grants g ON g.template_id=t.id JOIN team_members tm ON tm.team_id=g.team_id AND tm.user_id=$1 WHERE g.permission IN ('viewer','editor')
  ```
  → T1 命中 → visible。
- myPermission(bob, T1) = 'viewer'（grant max）。
- Bob 可 `GET / T1.id`，可 `SubmitJob` 引 T1，但 `UpdateTemplate` 失败（需 editor）。

**结论**：team grants 共享生效，editor/viewer 分权生效。✓

## 场景 L — Product 资产按 scope 共享（无 user 校验）

**前置**：product P1（scope=ai_clothing，无 owner）。P1 绑定 asset `product_img_1`（scope=ai_clothing，visibility=private，owner=Bob）。Alice（非 Bob）创建 workflow W（scope=ai_clothing），input slot 引用 `product_img_1`。

**触点**：SubmitJob 时 workflow_validation 调 `AssetReachability(alice, workflowScope=ai_clothing, [product_img_1])`：
- `asset.scope == workflowScope` ✓（都 ai_clothing）。
- `asset.visibility=private` 且 owner=Bob≠Alice → visibility 规则**不通过**……但 product 路径：
  - exists binding owner_kind=product（P1） → product 路径命中，**绕过 user visibility 校验**。
- → reachable。

> 这正是"product 没有 user 概念，scope 一样就能用 product 的素材"的体现：即使 asset 本身 private，只要它绑到 product 且 scope 匹配，就可在 workflow 中引用。

**对比场景 C**：同一 `product_img_1` 若**只**绑到 virtual_ip（不绑 product），Alice 非owner、非可见 vip → unreachable。product 绑定是"额外开了一条 scope 共享通道"。

**结论**：product 资产 scope 共享闭环。✓

## 场景 M — Product CRUD 无 user 限制

**动作**：Alice 创建 P1（scope=ai_clothing）。Bob（任意其它 user）调 `PUT /api/v1/products/{P1.id}` 改名、`DELETE` 软删。

**触点**：product domain 不做 user 级权限校验（无 owner/visibility/grants）→ Bob 可改可删，`updated_by=bob` 记审计。

**结论**：product CRUD 全开放（与"暂不限制"立场一致）。✓ 若后续要收紧，加 owner/team ACL。

## 场景 N — product / virtual_ip 作为 workflow 输入

**前置**：virtual_ip V1（scope=ai_clothing，owner=Alice）绑 asset `model_image_1`(asset_kind=model_image)；product P1（scope=ai_clothing）绑 asset `product_img_1`(asset_kind=product_image)。

**Workflow W**（scope=ai_clothing）：
```json
{
  "inputs": [
    {"name": "the_vip",     "type": "virtual_ip", "required": true},
    {"name": "the_product", "type": "product",    "required": true}
  ],
  "output_step_key": "compose",
  "steps": [
    {
      "key": "compose",
      "template_id": T1, "template_version": 1,
      "bindings": [
        {"to_input": "model_image",  "source": "workflow_input", "ref": "the_vip"},
        {"to_input": "product_image","source": "workflow_input", "ref": "the_product"}
      ]
    }
  ]
}
```
T1 的 inputs：`model_image`(asset_kind=model_image) + `product_image`(asset_kind=product_image)。

**校验**（workflow_validation）：
- `the_vip`(virtual_ip)：`CheckVirtualIPAccessible(alice, ai_clothing, V1)` → V1.scope 匹配 ✓ + Alice 是 owner ✓。
- `the_product`(product)：`CheckProductAccessible(ai_clothing, P1)` → P1.scope 匹配 ✓（无 user 校验）。
- 两个 slot 都有 asset_kind ✓（product / virtual_ip 输入要求）。

**执行**（workflow_execution）：
- `the_vip` → `ResolveAssetByKind(virtual_ip, V1, model_image)` → `model_image_1`（唯一）✓。
- `the_product` → `ResolveAssetByKind(product, P1, product_image)` → `product_img_1`（唯一）✓。
- 渲染、下游生成、产出 asset。

**反例**：若 P1 绑了 2 个 `product_image` asset → `ResolveAssetByKind` 返回 >1 → StepRun failed (ErrAssetResolveAmbiguous)，Alice 须改传具体 asset_id（把 workflow_input 改 type=asset）。

**结论**：product / virtual_ip 作 workflow 输入、按 slot.kind 解析的闭环成立。✓

## 跨场景遗漏点自检

逐条看 `media_auth` 全局规则和 media_materials / media_workflow 各域规则：

| 规则 | 出现的位置 | 闭环到位 |
|:---|:---|:---|
| scope 不可改 | 各域 change.md | 创建后 Scope 只读字段 |
| 跨 scope 引用 reject | scope/asset/workflow | IsSameScope RPC + multi-layer enforce |
| AssetKind 注册实体校验 | [asset_kind](../../ddd/media_materials/domain/0005-asset_kind/asset_kind.md) Register/Get/List；Template 引用 kind 时 BatchGetAssetKind 校验 | 全局注册表，无 scope |
| visibility ∈ {private, team, public} | resource-acl 全局 | template/workflow/virtual_ip 的 visibility 字段 + grants |
| 资源权限统一走 my_permission | 各域 §权限规则 | 每入口 `LoadMyPermission` 校验 |
| const 满足 required | workflow.md §4 + template.md Render | 已允许 |
| 单 sink 必须等于 output_step_key | workflow.md §校验 2 | 校验推导 sink 唯一且匹配 |
| step 只能 ref 之前 step | workflow.md §校验 3 | 按 step.order 严格小于 |
| archive 不影响旧 Job | execution.change.md | Job/StepRun 都快照 version |
| 跨资源权限一致性 | workflow.md §跨资源权限一致性 | UpdateWorkflow 引用 template 要求 caller ≥ viewer on template |
| Job in-place retry | execution.md §Retry | retry_count 上限、retry_history、AssetReachability 重走 |
| AssetReachability 复合规则 | asset.md §AssetReachability | scope_match ∧ visibility ∧ (vip_visible ∨ product_binding) |
| Product 资产 scope 共享（无 user 校验） | product.md §可达性 + asset.md product 路径 | product binding 绕过 user visibility，仅 scope 匹配 |
| Product CRUD 无 user 限制 | product.md §权限 | 无 owner/visibility/grants，任意 user 可 CRUD + 审计 |
| Scope ≠ Visibility | scope.md | 业务归属 vs 受众正交 |
| Template / Workflow / VirtualIp 资源权限统一 | media_auth §资源级 ACL | visibility + grants + my_permission 三件套 |
| user 不持 scope | media_auth §Scope | scope 是资源属性；锚点 workflow.scope；创建不限 scope |

## 一致性 / 闭环未覆盖项

| # | 项目 | 状态 |
|:---|:---|:---|
| 1 | Render 对 const 的类型适配严格化 | 用户决定**不要严格**，维持宽松 |
| 2 | publication_plan | **本期不上**，下次迭代专题 |
| 3 | Job in-place retry | **已实现**（execution domain retry_count / retry_history + RetryJob / RetryStepRun 接口） |
| 4 | 跨业务线共享 Template | **已回答**：用 `visibility + grants` 解决——把 Template 设为 `visibility=team` 并 grants 多个 team；scope 仍 1:1 锁业务线，跨 scope 引用仍拒 |
| 5 | Job 优先级 / 抢占 | **不做**（worker FIFO），已从待决移除 |
| 6 | user 的 scope 上下文载体（原 workspace） | **已回答**：user 不持 scope；scope 是资源属性，锚点是 workflow.scope；创建不限 scope。无需 user.scope / user_scopes / workspace |
