---
name: conductor_compilation
description: 把已发布 Workflow 版本可靠编译并注册为 Conductor Definition
participants: [workflow, template, execution_capability, conductor_adapter]
domains: [workflow, template, execution_capability]
---

## 一句话说明

Workflow 发布时冻结所有引用版本，先经 Semantic Compiler 生成不可变 Executable Workflow IR，再由 Conductor Backend Compiler 按拓扑波次生成 Definition；经 Outbox 注册成功后才进入可执行的 published 状态。

## 执行时间线

1. 校验 DAG、Template Version、Execution Capability Version、Scope 与权限并冻结快照。
2. Semantic Compiler 解析每个 Binding 与 Invocation Mapping，检查 Mapping 覆盖 Capability 必填参数且类型兼容，生成 `ExecutableWorkflowIR`。
3. Conductor Backend Compiler 计算拓扑波次并降低每个 `ExecutableStepIR`：自动插入 `media_render_template_v1`，再调用固定 Capability Task，最后进入候选持久化、审批和唯一输出收口。
4. 事务内把 Workflow 置 `publishing`，创建 Compilation 与注册 Outbox；Definition 内嵌降低后的执行语义，Compilation 仅保存发布映射、状态和 hash。
5. 计算 canonical JSON 的 `definition_hash`，以稳定 name/version 向 Conductor 注册。
6. 事务内记录目标映射并把 Workflow 置 `published`；不可恢复校验错误置 `publish_failed`，网络失败保留 publishing 并重试。

## 状态与幂等

Compilation 状态为 `pending -> registering -> registered | failed`。幂等键是 `(workflow_id, workflow_version, definition_hash)`；同一 hash 重放返回既有映射，不创建新 Conductor version。Conductor 返回冲突时必须读取远端定义并比 hash：相同视为成功，不同视为不可自动覆盖的冲突。

## 编译输出约束

- task reference name 为 `step__<sanitized_step_key>`，原始 step_key 保留在 input。
- `ExecutableWorkflowIR` 是 Semantic Compiler 与 Backend Compiler 的稳定边界；Conductor Definition 和 FlowGram 文档均不得充当 IR。
- `ExecutableStepIR` 固定 Template/Capability 版本、context bindings、Invocation Mapping、唯一输出和 reuse/approval policy。
- `media_render_template_v1` 固定接收 `template:string` 与 `context:map<string,json>` 并返回 `prompt:string`；Slot 数量和名称只改变 context 内容，不改变 Task Definition。
- Capability 的 `images[]` 等参数由 Backend Compiler 按 Invocation Mapping 生成；Renderer 和 Worker 均不得按 Slot 名猜测。
- Definition 只含稳定 ID、不可变模板内容、Object Path 表达式与非敏感参数；不含二进制、Provider 密钥、cookie、Token 或临时 URL。
- Step 间传递稳定业务资源 ID；二进制与 Provider 密钥不得进入 Definition 或执行 payload。
- Conductor Definition 不是编辑源，禁止从其反向覆盖业务 Workflow。

## 内部 Renderer Worker

`media_render_template_v1` 由 `media_workflow` 限界上下文拥有，使用同一代码库中的纯 Template Renderer，但以独立 Worker 进程轮询 Conductor；它不是独立微服务。Worker 输入中的 `template` 与 `context` 已由 Backend Compiler 和运行时引用组装完成，执行期间不查询 `media_materials`。同一输入必须产生相同 `prompt` 与 `prompt_digest`，以便 Conductor 重投安全重放。API 进程不承载轮询循环，避免 HTTP 生命周期、扩缩容与 Task 并发互相绑定。

## Capability Worker 交互

```mermaid
sequenceDiagram
    participant User as 用户/UI
    participant MW as media_workflow
    participant C as Conductor
    participant IW as Internal Worker
    participant CW as 客户端 media_worker

    User->>MW: SubmitJob(workflow_version, inputs)
    MW->>C: StartWorkflow(compiled_name/version, inputs)
    C-->>IW: poll media_render_template_v1
    IW->>C: COMPLETED(prompt, digest)
    C-->>CW: poll capability task
    CW->>CW: 调本地能力或显式 Mock Profile
    CW->>C: COMPLETED(candidates, provenance)
    C-->>IW: poll media_finalize_step_v1
    IW->>C: COMPLETED(unique output_resource)
    C-->>MW: execution state（查询/同步）
    MW-->>User: Job/StepRun 投影
```

- Capability Task 的幂等键为 Conductor `taskId`；客户端重投同一结果不得重复产生外部副作用。
- 客户端离线时 Task 保持可重试状态，不改变 Workflow 或 Job 定义；恢复后继续轮询。
- Mock Profile 只用于联调，按 capability key 返回冻结样本，并在 provenance 中标记 `media_ai_fixture`。
- Internal Worker 与 Capability Worker 生命周期独立；任一方重启不得要求重新发布 Workflow。
