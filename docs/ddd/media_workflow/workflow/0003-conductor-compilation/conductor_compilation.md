---
name: conductor_compilation
description: 把已发布 Workflow 版本可靠编译并注册为 Conductor Definition
participants: [workflow, template, execution_capability, conductor_adapter]
domains: [workflow, template, execution_capability]
---

## 一句话说明

Workflow 发布时冻结所有引用版本，按拓扑波次生成不可变 Conductor Definition，经 Outbox 注册成功后才进入可执行的 published 状态。

## 执行时间线

1. 校验 DAG、Template Version、Execution Capability Version、Scope 与权限。
2. 事务内冻结 Workflow 快照，状态置 `publishing`，创建 Compilation 与 Outbox。
3. Compiler 计算拓扑波次；每个 Step 生成一个 `media_step_execution_v1` 输入描述，单节点波次发 SUB_WORKFLOW，多节点波次发 `FORK_JOIN_DYNAMIC + JOIN`。
4. 计算 canonical JSON 的 `definition_hash`，以稳定 name/version 向 Conductor 注册。
5. 事务内记录目标映射并把 Workflow 置 `published`；不可恢复校验错误置 `publish_failed`，网络失败保留 publishing 并重试。

## 状态与幂等

Compilation 状态为 `pending -> registering -> registered | failed`。幂等键是 `(workflow_id, workflow_version, definition_hash)`；同一 hash 重放返回既有映射，不创建新 Conductor version。Conductor 返回冲突时必须读取远端定义并比 hash：相同视为成功，不同视为不可自动覆盖的冲突。

## 编译输出约束

- task reference name 为 `step__<sanitized_step_key>`，原始 step_key 保留在 input。
- 子工作流输入只含 Job/StepRun ID、Template/Capability 快照、binding 描述、reuse/approval policy、Workspace/Scope/Principal 引用。
- Step 间传递稳定业务资源 ID；二进制与 Provider 密钥不得进入 Definition 或执行 payload。
- Conductor Definition 不是编辑源，禁止从其反向覆盖业务 Workflow。
