# 从 FlowGram 工作流到 Conductor 可执行工作流：完整示例

> 状态：方案已确认。正式模型使用 `ExecutableWorkflowIR` 与 `ExecutableStepIR`，不再使用 `CompiledStepPlan`；详细约束以 `docs/ddd/media_workflow` 与对应 Spec 为准。

## 1. 本文要解释的问题

本文只回答一个核心问题：

> 用户在可视化编辑器里定义的是业务节点及其输入输出依赖；执行 Worker 接受的却是固定的技术参数。系统如何把前者可靠地编译成后者？

FlowGram 解决的是**可视化编辑、节点表单、变量引用和依赖展示**。它提供画布、表单和带作用域及类型推导的变量引擎，但它是用于构建工作流产品的框架，不是本系统的业务模型或执行引擎。[FlowGram 官方说明](https://github.com/bytedance/flowgram.ai)

本系统仍然需要自己的两层抽象：

1. **业务 Workflow DSL**：表达用户想做什么，以及节点之间传递什么业务资源。
2. **Executable Workflow IR**：表达每个业务节点应如何调用固定的 Execution Capability。

Conductor 只消费编译后的执行定义，不能成为工作流编辑源。

```text
FlowGram 编辑文档
       │  UI Adapter：去掉坐标、颜色、折叠状态等展示信息
       ▼
业务 Workflow DSL
       │  Semantic Compiler：解析引用、检查类型、编译参数映射
       ▼
Executable Workflow IR
       │  Conductor Backend Compiler：展开执行片段和拓扑
       ▼
Conductor Workflow Definition
```

---

## 2. 示例业务

用户创建一个“服装营销视频”工作流：

```text
Workflow 输入
├── virtual_ip.full_body_image   全身图
├── product.main_image           衣服图
└── movement.description         动作描述

节点 A：生成模特图
├── 输入：全身图、衣服图
└── 输出：模特图

节点 B：生成视频
├── 输入：节点 A 的模特图、动作描述
└── 输出：视频
```

节点 A 使用“即梦多图生图”能力。该 Worker 的固定契约是：

```json
{
  "prompt": "string",
  "images": ["ImageRef"]
}
```

节点 B 使用“豆包首帧生视频”能力。其固定契约是：

```json
{
  "prompt": "string",
  "images": ["ImageRef"]
}
```

虽然两个 Worker 都接受 `prompt + images[]`，业务节点不会要求用户直接填写这两个技术字段。用户看到的是“全身图”“衣服图”“动作描述”等业务端口。

这正是需要编译层的原因。

---

## 3. 第零层：先注册固定的 Execution Capability

在用户编辑 Workflow 之前，系统已经注册了 Worker 能实现的能力。Capability 是编译器的“目标函数签名”。

```yaml
capabilityKey: jimeng.image.multi_image_prompt
version: 1
conductorTaskDef: jimeng_image_multi_v1
inputs:
  type: object
  required: [prompt, images]
  properties:
    prompt:
      type: string
    images:
      type: array
      minItems: 1
      items:
        $ref: ImageRef
output:
  candidates:
    type: array
    items:
      $ref: GeneratedImage
```

```yaml
capabilityKey: doubao.video.first_frame_prompt
version: 1
conductorTaskDef: doubao_video_first_frame_v1
inputs:
  type: object
  required: [prompt, images]
  properties:
    prompt:
      type: string
    images:
      type: array
      minItems: 1
      maxItems: 1
      items:
        $ref: ImageRef
output:
  candidates:
    type: array
    items:
      $ref: GeneratedVideo
```

Capability 只描述稳定的执行契约，不包含某次 Workflow 的全身图、衣服图，也不包含 Provider 密钥。

---

## 4. 第一层：用户在 FlowGram 中看到的定义

下面是**本项目基于 FlowGram 保存的概念性编辑文档**，不是声称 FlowGram SDK 原生规定了这些业务字段。`nodes`、`edges`、坐标由 FlowGram 管理；`data.domain` 是本项目挂在节点表单上的业务配置。

```json
{
  "nodes": [
    {
      "id": "workflow_input",
      "type": "workflow-input",
      "position": { "x": 80, "y": 160 },
      "data": {
        "outputs": [
          { "key": "full_body", "kind": "virtual_ip.full_body_image" },
          { "key": "clothing", "kind": "product.main_image" },
          { "key": "movement", "kind": "movement.description" }
        ]
      }
    },
    {
      "id": "model_image",
      "type": "generation-step",
      "position": { "x": 420, "y": 80 },
      "data": {
        "domain": {
          "stepKey": "model_image",
          "templateRef": { "id": "tpl_model_image", "version": 3 },
          "inputs": {
            "person": "${workflow_input.full_body}",
            "garment": "${workflow_input.clothing}"
          },
          "approvalPolicy": "when_multiple_candidates"
        },
        "outputs": [
          { "key": "result", "kind": "model_image" }
        ]
      }
    },
    {
      "id": "video",
      "type": "generation-step",
      "position": { "x": 760, "y": 160 },
      "data": {
        "domain": {
          "stepKey": "video",
          "templateRef": { "id": "tpl_product_video", "version": 2 },
          "inputs": {
            "first_frame": "${model_image.result}",
            "movement": "${workflow_input.movement}"
          },
          "approvalPolicy": "always"
        },
        "outputs": [
          { "key": "result", "kind": "product_video" }
        ]
      }
    }
  ],
  "edges": [
    { "sourceNodeID": "workflow_input", "sourcePortID": "full_body", "targetNodeID": "model_image", "targetPortID": "person" },
    { "sourceNodeID": "workflow_input", "sourcePortID": "clothing", "targetNodeID": "model_image", "targetPortID": "garment" },
    { "sourceNodeID": "model_image", "sourcePortID": "result", "targetNodeID": "video", "targetPortID": "first_frame" },
    { "sourceNodeID": "workflow_input", "sourcePortID": "movement", "targetNodeID": "video", "targetPortID": "movement" }
  ]
}
```

FlowGram 在这里负责：

- 显示节点、端口和连线；
- 在节点表单中选择 Template、审批策略和上游变量；
- 用变量作用域阻止节点引用尚不可见的下游输出；
- 提供即时的类型提示。

FlowGram 不负责：

- 决定 `person` 和 `garment` 在 Worker 参数中的排列方式；
- 渲染最终提示词；
- 生成 Conductor Definition；
- 执行业务审批、结果复用或资源持久化。

### 为什么节点里既有 `inputs` 又有 `edges`

`edges` 用于画布展示和拓扑交互，`inputs` 是具有业务含义的绑定。保存时 Adapter 必须检查二者一致，然后只生成一份规范化的 Slot Binding，避免后端存在两个事实源。

---

## 5. 第二层：转换成业务 Workflow DSL

UI Adapter 去掉坐标等展示数据，并把 FlowGram 变量引用规范化为领域模型：

```yaml
workflow:
  key: clothing_marketing_video
  version: 1
  scope: ai_clothing

  inputs:
    - name: full_body
      kind: virtual_ip.full_body_image
      required: true
    - name: clothing
      kind: product.main_image
      required: true
    - name: movement
      kind: movement.description
      required: true

  steps:
    - key: model_image
      templateRef: tpl_model_image@3
      slot_bindings:
        person: workflow_input.full_body
        garment: workflow_input.clothing
      approvalPolicy: when_multiple_candidates
      reusePolicy: reuse_if_available

    - key: video
      templateRef: tpl_product_video@2
      slot_bindings:
        first_frame: step_output.model_image
        movement: workflow_input.movement
      approvalPolicy: always
      reusePolicy: reuse_if_available

  output: step_output.video
```

这一层只说：

- Workflow 有哪些业务输入；
- Step 使用哪个不可变 Template 版本；
- Template Slot 的值来自哪个 Workflow Input 或上游 Step Output；
- 哪个 Step 是最终输出。

它仍然没有出现 `jimeng_image_multi_v1`，因为业务 Workflow 不应依赖 Conductor 的 Task 名称。

### 两个 Template 快照

节点 A 引用的 Template：

```yaml
id: tpl_model_image
version: 3
inputs:
  - { name: person, kind: virtual_ip.full_body_image, required: true }
  - { name: garment, kind: product.main_image, required: true }
promptBody: >-
  保持人物 {{person.description}} 的身份特征，穿上图二对应的服装，生成电商模特图。
outputAssetKind: model_image
capabilityRef: jimeng.image.multi_image_prompt@1

# Template Slot 到 Capability 参数的转换规则
invocationMapping:
  prompt:
    expression: render_prompt()
  images:
    expression: list(asset_ref(person), asset_ref(garment))
```

节点 B 引用的 Template：

```yaml
id: tpl_product_video
version: 2
inputs:
  - { name: first_frame, kind: model_image, required: true }
  - { name: movement, kind: movement.description, required: true }
promptBody: >-
  模特以 {{movement.text}} 的方式展示服装，保持人物、服装和背景一致。
outputAssetKind: product_video
capabilityRef: doubao.video.first_frame_prompt@1
invocationMapping:
  prompt:
    expression: render_prompt()
  images:
    expression: list(asset_ref(first_frame))
```

`invocationMapping` 就是缺失的关键抽象。它定义了：

```text
业务 Slot 值
    + Prompt Template
    ──────────────────> Capability 的固定输入参数
```

它属于 Template 的不可变版本快照，而不是 FlowGram 连线，也不是 Worker 自己猜测的规则。

---

## 6. 第三层：Semantic Compiler 生成 Executable Workflow IR

Semantic Compiler 在发布 Workflow 时加载并冻结：

- Workflow 版本；
- 所有 Template 版本；
- 所有 Execution Capability 版本；
- Asset Kind 的类型信息。

然后执行以下检查：

1. `model_image.person` 是否绑定，类型是否兼容 `virtual_ip.full_body_image`；
2. `model_image.garment` 是否兼容 `product.main_image`；
3. `model_image.result` 的 `model_image` 类型是否兼容 `video.first_frame`；
4. Template 的 `invocationMapping` 是否覆盖 Capability 的全部必填参数；
5. `images` 表达式结果是否为 `ImageRef[]`，而不是单个字符串或视频；
6. DAG 是否无环，并且 `video` 是否为唯一最终输出。

通过后得到与 UI 和 Conductor 都无关的 Executable IR：

```yaml
executableWorkflow:
  source: clothing_marketing_video@1
  output: video.output_resource_id

  steps:
    - key: model_image
      dependsOn: []
      capability:
        key: jimeng.image.multi_image_prompt
        version: 1
        taskDef: jimeng_image_multi_v1
      resolve:
        person: workflow.inputs.full_body
        garment: workflow.inputs.clothing
      invoke:
        prompt:
          op: render_template
          template: tpl_model_image@3
        images:
          op: list
          items:
            - { op: asset_ref, value: step.inputs.person }
            - { op: asset_ref, value: step.inputs.garment }
      output:
        candidatesKind: model_image
        selectedOutput: output_resource_id
      policies:
        approval: when_multiple_candidates
        reuse: reuse_if_available

    - key: video
      dependsOn: [model_image]
      capability:
        key: doubao.video.first_frame_prompt
        version: 1
        taskDef: doubao_video_first_frame_v1
      resolve:
        first_frame: steps.model_image.output_resource_id
        movement: workflow.inputs.movement
      invoke:
        prompt:
          op: render_template
          template: tpl_product_video@2
        images:
          op: list
          items:
            - { op: asset_ref, value: step.inputs.first_frame }
      output:
        candidatesKind: product_video
        selectedOutput: output_resource_id
      policies:
        approval: always
        reuse: reuse_if_available
```

这里的 `invoke` 不是发布时已经算好的值，而是一棵**类型检查过的求值表达式**。发布 Workflow 时还没有具体的用户图片，因此编译器只能冻结转换计划；真正的 `prompt` 和 `images[]` 在 Execution 运行时求值。

### 编译前后的本质变化

```text
编译前：
  “person 来自全身图，garment 来自衣服图”

编译后：
  prompt = render(tpl_model_image@3, person, garment)
  images = [asset_ref(person), asset_ref(garment)]
  call   = jimeng_image_multi_v1(prompt, images)
```

这一步才真正解决“业务节点输入不等于 Conductor Task 输入”的问题。

### 6.1 为什么运行时仍然需要转换计划

发布 Workflow 时，编译器知道转换规则：

```yaml
step: model_image
capability: jimeng.image.multi_image_prompt@1
providerInput:
  prompt: render(tpl_model_image@3)
  images:
    - asset_ref(person)
    - asset_ref(garment)
```

但它还不知道用户本次选择的是哪一个 Virtual IP 和 Product，因此不能在发布时算出最终值：

```json
{
  "prompt": "保持人物小雅的身份特征，穿上图二对应的服装……",
  "images": [
    { "object_path": "virtual-ip/xiaoya/full-body.png" },
    { "object_path": "products/dress-001/main.png" }
  ]
}
```

所以发布期保存的不是最终参数，而是一个已经通过类型检查、等待实际资源参与求值的转换计划。运行到 `model_image` Step 时：

```text
转换计划
  + person = 本次 Job 的 full_body 资源
  + garment = 本次 Job 的 clothing 资源
  + tpl_model_image@3 的不可变快照
  ↓ 求值
prompt:string + images:ImageRef[]
  ↓
jimeng_image_multi_v1 Worker
```

这里“必须保留”的是这段转换语义，而不是必须新建一个名为 `Executable Workflow IR` 的数据库实体。

### 6.2 方案一：把计划嵌入 Conductor Definition

> 本节到 6.5 保留的是持有方式对比，其中 `media_evaluate_ir_v1` 是用于解释差异的假设任务，不属于最终实现。最终实现见第 7 节：`media_render_template_v1` 只渲染模板，其他参数由 Backend Compiler 直接降低。

编译器直接把转换计划放进准备任务的输入：

```json
{
  "name": "media_evaluate_ir_v1",
  "taskReferenceName": "model_image__prepare",
  "type": "SIMPLE",
  "inputParameters": {
    "step_key": "model_image",
    "plan": {
      "template_ref": "tpl_model_image@3",
      "capability_ref": "jimeng.image.multi_image_prompt@1",
      "provider_input": {
        "prompt": { "op": "render_template" },
        "images": {
          "op": "list",
          "items": [
            { "op": "asset_ref", "slot": "person" },
            { "op": "asset_ref", "slot": "garment" }
          ]
        }
      }
    },
    "person": "${workflow.input.full_body}",
    "garment": "${workflow.input.clothing}"
  }
}
```

在这个未采用的变体中，运行时 `media_evaluate_ir_v1` 不需要查询编译结果，只读取输入中的 plan 并求值。

优点是执行定义自包含、容易从 Conductor 查看实际执行规则；缺点是 Definition 会变大，而且修改计划必须注册新的 Conductor Workflow 版本。

### 6.3 方案二：把计划保存在 `workflow_compilations`

数据库保存完整编译快照：

```json
{
  "workflow_id": "wf_clothing_video",
  "workflow_version": 1,
  "compiler_version": 1,
  "compiled_steps": {
    "model_image": {
      "template_ref": "tpl_model_image@3",
      "capability_ref": "jimeng.image.multi_image_prompt@1",
      "provider_input": {
        "prompt": { "op": "render_template" },
        "images": {
          "op": "list",
          "items": [
            { "op": "asset_ref", "slot": "person" },
            { "op": "asset_ref", "slot": "garment" }
          ]
        }
      }
    }
  }
}
```

Conductor 中只传引用：

```json
{
  "name": "media_evaluate_ir_v1",
  "inputParameters": {
    "compilation_id": "comp_wf_clothing_video_v1",
    "step_key": "model_image",
    "person": "${workflow.input.full_body}",
    "garment": "${workflow.input.clothing}"
  }
}
```

准备任务用 `compilation_id + step_key` 读取不可变计划，再结合本次 Job 输入求值。

优点是计划集中、便于审计和重新生成 Conductor Definition；缺点是执行时多一次读取，并且必须保证 Compilation 永远不可变、可用，不能让运行中的 Job 读到后来修改的规则。

### 6.4 方案三：为 Step 生成不可变子工作流

主 Workflow 只引用具体的 Step 子工作流：

```json
{
  "name": "media_step_wf_clothing_video_v1_model_image",
  "taskReferenceName": "model_image",
  "type": "SUB_WORKFLOW",
  "inputParameters": {
    "person": "${workflow.input.full_body}",
    "garment": "${workflow.input.clothing}"
  }
}
```

这个不可变子工作流内部已经固定转换和能力调用：

```text
prepare(tpl_model_image@3, person, garment)
  → jimeng_image_multi_v1(prompt, images)
  → persist candidates
  → approval/reuse
  → output_resource_id
```

优点是每个业务 Step 的执行边界清晰，可以单独查看和重试；缺点是每次发布会产生较多 Conductor Workflow Definition，版本和清理工作更复杂。

这里需要区分：当前设计中的通用 `media_step_execution_v1` 是所有 Step 共用的执行骨架；本方案生成的是绑定了具体 Template、Capability 和 Mapping 的不可变 Step 子工作流。

### 6.5 方案四：运行时根据冻结版本重新构造

Conductor 只携带来源引用：

```json
{
  "name": "media_evaluate_ir_v1",
  "inputParameters": {
    "workflow_ref": "wf_clothing_video@1",
    "step_key": "model_image",
    "template_ref": "tpl_model_image@3",
    "capability_ref": "jimeng.image.multi_image_prompt@1"
  }
}
```

运行时重新加载 Workflow、Template 和 Capability 的不可变快照，再执行一次 Mapping 编译：

```text
load workflow@1 + template@3 + capability@1
  → compile mapping
  → evaluate with actual resources
  → prompt + images[]
```

优点是不重复保存编译计划；缺点是每次运行都依赖编译器，启动更慢，而且编译器升级后可能让同一 Workflow 版本产生不同结果。若采用这种方式，必须同时冻结 `compiler_version`，并保证旧版编译器可以长期重放。

### 6.6 四种方式不是四份都保存

它们是同一个问题的四种替代方案：**执行到 Step 时，从哪里取得已经验证过的转换计划？**

| 方式 | 运行时从哪里取得计划 | 主要特点 |
|:---|:---|:---|
| 嵌入主 Conductor Definition | 当前 Workflow Definition | 自包含，最直观 |
| 保存到 `workflow_compilations` | media_workflow 的不可变编译快照 | 审计和重编译方便 |
| 生成 Step 子工作流 | 具体 Step Definition | 执行边界清楚，Definition 数量多 |
| 运行时重新构造 | 冻结的源版本 + 指定编译器版本 | 少存储，运行期复杂度最高 |

本项目确定采用显式 `ExecutableWorkflowIR`，由 Conductor Backend Compiler 把每个 `ExecutableStepIR` 降低并内嵌到 Definition；`workflow_compilations` 不作为运行时计划仓库。

---

## 7. 第四层：Conductor Backend Compiler 生成执行定义

Conductor 后端编译器不再理解 `virtual_ip.full_body_image` 这样的业务概念。它只把 Executable IR 展开成技术任务。

为了突出关键关系，下面省略重试、超时、持久化和审批任务的部分字段：

```json
{
  "name": "media_workflow_clothing_marketing_video",
  "version": 1,
  "tasks": [
    {
      "name": "media_render_template_v1",
      "taskReferenceName": "model_image__render",
      "type": "SIMPLE",
      "inputParameters": {
        "template": {
          "id": "tpl_model_image",
          "version": 3,
          "body": "保持人物 {{.person.description}} 的身份特征，穿上图二对应的服装，生成电商模特图。"
        },
        "context": {
          "person": "${workflow.input.inputs.full_body}",
          "garment": "${workflow.input.inputs.clothing}"
        }
      }
    },
    {
      "name": "jimeng_image_multi_v1",
      "taskReferenceName": "model_image__invoke",
      "type": "SIMPLE",
      "inputParameters": {
        "prompt": "${model_image__render.output.prompt}",
        "images": [
          "${workflow.input.inputs.full_body.asset_ref}",
          "${workflow.input.inputs.clothing.asset_ref}"
        ]
      }
    },
    {
      "name": "media_finalize_step_v1",
      "taskReferenceName": "model_image__finalize",
      "type": "SUB_WORKFLOW",
      "inputParameters": {
        "step_key": "model_image",
        "candidates": "${model_image__invoke.output.candidates}",
        "approval_policy": "when_multiple_candidates"
      }
    },
    {
      "name": "media_render_template_v1",
      "taskReferenceName": "video__render",
      "type": "SIMPLE",
      "inputParameters": {
        "template": {
          "id": "tpl_product_video",
          "version": 2,
          "body": "模特以 {{.movement.text}} 的方式展示服装。"
        },
        "context": {
          "first_frame": "${model_image__finalize.output.output_resource}",
          "movement": "${workflow.input.inputs.movement}"
        }
      }
    },
    {
      "name": "doubao_video_first_frame_v1",
      "taskReferenceName": "video__invoke",
      "type": "SIMPLE",
      "inputParameters": {
        "prompt": "${video__render.output.prompt}",
        "images": [
          "${model_image__finalize.output.output_resource.asset_ref}"
        ]
      }
    },
    {
      "name": "media_finalize_step_v1",
      "taskReferenceName": "video__finalize",
      "type": "SUB_WORKFLOW",
      "inputParameters": {
        "step_key": "video",
        "candidates": "${video__invoke.output.candidates}",
        "approval_policy": "always"
      }
    }
  ],
  "outputParameters": {
    "output_resource_id": "${video__finalize.output.output_resource_id}"
  }
}
```

在节点 A 运行时，`model_image__render` 接收变化的 context，但固定只输出渲染结果：

```json
{
  "prompt": "保持人物小雅的身份特征，穿上图二对应的服装，生成电商模特图。",
  "prompt_digest": "sha256:..."
}
```

图片数组顺序由 Backend Compiler 直接从 Invocation Mapping 降低。随后固定契约的 `jimeng_image_multi_v1` Worker 收到 `prompt + images[]`，不需要理解 Product、Virtual IP、Template Slot 或 FlowGram。

### 为什么下游拿到的是一个输出，而 Provider 可以返回多张图片

Provider 可以返回 `candidates[]`，但这不是多个 Step Output。`media_finalize_step_v1` 根据审批策略：

- 多个候选：必须让用户选择或要求重新生成；
- 单个候选且策略为 `always`：仍需用户确认；
- 单个候选且策略为 `when_multiple_candidates`：自动确认；
- 零个候选：Step 失败。

只有确认后的一个 Candidate 被注册为 `output_resource_id`，因此下游 Step 始终消费单一、稳定的业务输出。

---

## 8. 发布期与运行期必须分开

这是实现中最容易混淆的地方。

| 阶段 | 已知信息 | 产物 |
|:---|:---|:---|
| FlowGram 编辑期 | 节点、连线、用户选择的 Template | 编辑文档 |
| Workflow 保存期 | 业务绑定关系 | Workflow DSL |
| Workflow 发布期 | 固定的 Workflow/Template/Capability 版本 | 类型检查过的 Executable IR + Conductor Definition |
| SubmitJob | 实际 Product、Virtual IP、Material、Asset ID | 自闭环 Job Input Snapshot |
| Step 运行期 | 实际资源元数据、上游唯一输出 | `prompt + images[]` 等 Provider Input |

发布期不能生成最终 Prompt 和 URL，因为实际输入尚不存在。运行期也不应重新解释未经校验的自由 JSON，而应只对发布期产生的已校验表达式求值。

---

## 9. 失败分别由哪一层发现

### FlowGram/UI 即时提示

- 必填端口没有连线；
- 引用了不可见的下游变量；
- 图片端口连接到了文本端口。

UI 校验只改善体验，不能作为最终可信校验。

### Semantic Compiler 发布失败

- Template Slot 与 Workflow Binding 类型不兼容；
- `invocationMapping` 没有生成 Capability 必填参数；
- `images` 的结果不是 `ImageRef[]`；
- Capability 或 Template 版本不存在、未发布；
- DAG 有环或最终输出不唯一。

### 运行期 Step 失败

- 资源 ID 已失效或调用者不可访问；
- 对象存储文件不存在；
- Provider 超时或返回非法结果；
- Prompt 求值依赖的实际 metadata 缺失。

同一错误不能依赖 UI 独占发现；后端发布必须重新完整校验。

---

## 10. 对当前仓库实现的 Review

### 已经存在的部分

当前实现已经具备：

- `Workflow -> Step -> Binding` 的基本领域结构；
- `WORKFLOW_INPUT / STEP_OUTPUT / CONST` 三种来源；
- DAG、唯一 sink 和拓扑波次校验；
- `approval_policy` 与 `reuse_policy`；
- 把拓扑波次编译成 Conductor `SUB_WORKFLOW` 或 `FORK_JOIN_DYNAMIC + JOIN`；
- Conductor Definition 的稳定 hash。

对应代码主要位于：

- `media_workflow/biz/domain/workflow/workflow.go`
- `media_workflow/biz/service/workflow/validator.go`
- `media_workflow/biz/workflow/compiler/compiler.go`

### 当前缺失的关键部分

当前代码中的 `compiler.Compile` 实际是**拓扑编译器**，不是本文定义的完整 Semantic Compiler。它把每个 Step 编译为统一的 `media_step_execution_v1`，但尚未完成：

1. 可持久化、版本化的 `ExecutionCapability` 实现；
2. Template 对 `(capability_key, capability_version)` 的真实引用；
3. Template Slot 到 Capability 参数的 `invocationMapping`；
4. Mapping 表达式的类型检查器；
5. 独立的 Executable Workflow IR；
6. IR 到具体 Capability Task Definition 的后端编译；
7. 发布期冻结 Template、Capability 与 Mapping 的完整快照。
8. `WORKFLOW_INPUT` Binding 对具体 Workflow Input 名称的显式引用。当前模型只允许 `Ref` 保存上游 Step Key，因此只能隐式使用 `to_input` 作为入口名，无法完整表达示例中的 `full_body -> person` 重命名绑定。

此外，当前 Go `template.Template` 仍使用 `Platform / GenWay / GenType`，而 DDD 文档已经声明目标模型是 `CapabilityKey / CapabilityVersion`，说明代码尚未追上目标领域设计。

### 当前 `wave_n_step_inputs` 的问题

现有 Conductor Compiler 假设 SubmitJob 已经提供：

```text
workflow.input.wave_<n>_step_inputs
```

这会把最关键的语义转换隐藏在 SubmitJob 组装逻辑里：

- 不清楚哪个版本的规则把业务 Slot 转换成了 `prompt + images[]`；
- 无法在发布期证明 Capability 的必填参数一定能得到；
- 转换逻辑容易分散到 Job Service、Template Render 和 Worker；
- Conductor Definition 只能看见通用子工作流，看不见具体 Capability 契约。

`wave_n_step_inputs` 可以作为运行时载体继续存在，但它必须是 **Executable IR 求值后的结果**，不能代替 IR 和 Semantic Compiler。

---

## 11. 建议用这个例子 Review 后续实现

实现完成时，至少应能用自动化测试证明下面这条链路：

```text
FlowGram 文档
  -> 规范化 Workflow DSL
  -> 加载 tpl_model_image@3
  -> 加载 jimeng.image.multi_image_prompt@1
  -> 类型检查 invocationMapping
  -> 生成 model_image Executable Step IR
  -> 编译为 jimeng_image_multi_v1 Conductor Task
  -> 运行时得到固定的 prompt:string + images:ImageRef[]
```

建议的核心验收断言：

1. 调换 `person`、`garment` 在 FlowGram 画布上的位置，不影响 `images[]` 顺序；顺序只由 `invocationMapping` 决定。
2. 把 `movement.description` 错接到 `images` 时，后端发布失败，而不是运行到 Worker 才失败。
3. Template 更新为版本 4 后，已发布 Workflow 仍使用版本 3。
4. Capability 发布版本 2 后，已发布 Workflow 仍调用版本 1 的 Task Definition。
5. 即梦返回三张图片时，节点 B 不会启动，直到用户确认其中一张。
6. 用户选择重新生成时，旧 Candidate 保留审计，新 attempt 不改变业务 DAG。
7. 相同输入命中复用策略时，可以跳过 Provider 调用，但仍得到同样的单一 `output_resource_id`。

---

## 12. 最终应保持的边界

```text
FlowGram
  负责“怎么画、怎么配置、怎么展示引用”

Workflow DSL
  负责“用户定义了什么业务流程”

Template + Invocation Mapping
  负责“业务 Slot 如何变成 Capability 参数”

Semantic Compiler
  负责“这个转换是否完整且类型安全，并生成可执行计划”

Conductor Backend Compiler
  负责“如何把可执行计划展开成 Conductor Definition”

Conductor
  负责“按定义可靠调度”

Worker
  负责“严格执行固定 Capability 契约”
```

其中最关键的设计对象不是 FlowGram 的 Edge，也不是 Conductor 的 Task，而是：

> **Template Version 中从业务 Slot 到 Execution Capability 参数的 Invocation Mapping，以及它编译出的 Executable Step IR。**
