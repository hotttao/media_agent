# 客户端 Media Worker、统一网关与 Conductor 代理方案记录

本文记录围绕“客户端 `media_worker` 如何安全领取并完成 Conductor Capability Task”的讨论过程与最终方案。它用于解释方案如何形成；当前有效的领域边界和实现契约仍以 `docs/ddd`、`docs/spec` 为准。

## 1. 问题起点

工作流发布后会被编译成 Conductor Workflow Definition。其中存在两类任务：

- `media_render_template_v1`、`media_finalize_step_v1` 等 Internal Execution Task，由服务端 `media_workflow` 执行；
- 豆包生图、视频生成等 Capability Task，由企业或用户环境中的客户端 `media_worker` 执行。

Capability Task 的类型在编译时已经由不可变的 Execution Capability Version 决定，但某个 Worker 此刻会领取哪个任务、任务何时出现以及是否重试，都是运行时动态的。因此问题不是让 `media_workflow` 预先知道一个固定任务，而是为一组已声明的 Capability 建立安全、可恢复的代理调度协议。

## 2. 方案演进

### 2.1 最初设想：客户端直接连接 Conductor

最初把 `media_worker` 视为普通 Conductor Worker：客户端配置 Conductor 地址，按 Task Definition 长轮询，并直接调用 Conductor Task Update API 回写结果。

这个方式验证执行链路很直接，但不适合作为正式边界：

- Conductor 会被暴露到客户端网络，绕过统一网关；
- Conductor 不理解平台的 User、Organization、Service Token 与资源权限；
- 客户端能看到内部 task id、workflow id 和内部任务类型；
- Token 撤销、租约、重放、审计与限流只能散落在客户端和 Conductor 之间；
- 无法保证用户只能领取自己的任务。

因此保留“Capability Worker 在客户端运行”，但否决“客户端直连 Conductor”。

### 2.2 第二步：由 media_workflow 代理 Conductor

随后把 `media_workflow` 作为业务代理：客户端只调用稳定的 Worker API，`media_workflow` 在内部轮询和更新 Conductor。这样 Conductor 可以保持内网部署，业务服务也能执行任务归属和输出校验。

这个方向正确，但还缺少入口认证边界：客户端是外部调用方，不应直接请求内部服务，也不应把 Service Token 原样传入 `media_workflow`。

### 2.3 最终修正：统一网关完成认证与身份交换

最终确认的调用链为：

```text
media_worker
  -- opaque Service Token --> Unified Gateway / Traefik
  --> Oathkeeper 或 Edge Auth 适配器
  -- short-lived Internal Identity JWT --> media_workflow Worker API
  --> internal Conductor
```

其中：

- Traefik 负责路由、ForwardAuth 接入及剥离客户端伪造的内部身份头；
- Oathkeeper 或专用 Edge Auth 适配器调用 `media_auth` 的 Service Token Introspection，并签发短时 Internal Identity JWT；
- `media_workflow` 验证 JWT 后，执行任务归属、Organization、Capability 与输出 schema 授权；
- Conductor 只在内网接受 `media_workflow` 的代理调用，不识别用户 Token。

“网关校验 Token”是逻辑职责，不表示由 Traefik 自己实现 opaque Token 内省和 JWT 签发。Traefik 是流量入口，认证实现属于 Oathkeeper/Edge Auth，凭证事实属于 `media_auth`。

## 3. 为什么动态任务仍然可以代理

Conductor 的 Worker API 不是“领取任意任务”，而是按 Task Type 领取：

```text
GET /api/tasks/poll/{taskType}
GET /api/tasks/poll/batch/{taskType}
POST /api/tasks
```

所以 `media_workflow` 无法建立一个真正的“任意类型”轮询器。但任务类型也不是无限或未知的：每个已发布的 Execution Capability Version 都冻结一个 `conductor_task_def`，编译后的 Workflow 只会创建这些已登记的 Capability Task。

客户端注册或轮询时声明的是 `(capability_key, capability_version, concurrency)`，不能提交任意 Conductor Task Definition。`media_workflow` 通过 Capability Registry 映射出 `conductor_task_def`，并明确排除 Internal Execution Task。

MVP 可为每个已启用 Capability 启动一个服务端长轮询；规模扩大后，改为由服务端 Poll Multiplexer 仅针对活跃的 `(Execution Domain, taskDef)` 组合维护轮询器。两种实现不改变外部 Worker API。

## 4. 任务隔离必须发生在领取之前

不能先从共享 Conductor 队列领取任务，再检查它是否属于当前用户。因为一旦 poll 成功，任务已经进入 `IN_PROGRESS`：即使随后拒绝，也会造成任务泄露、阻塞和饥饿。

正式方案使用 Conductor Task Domain：

- 启动 Workflow 时，`media_workflow` 通过 `taskToDomain` 为 Capability Task 设置 Execution Domain；
- MVP 的 Execution Domain 从 Job 的提交用户派生，确保用户只能领取自己的任务；
- 企业共享 Worker Pool 后续可以显式选择 Organization/Worker Pool Domain，但不能隐式扩大范围；
- Domain 值由服务端派生，客户端不能传入；
- 不允许回退到 `NO_DOMAIN`；
- Renderer 与 Finalizer 留在服务端内部 Domain，不会进入客户端队列。

这意味着启动 Conductor Workflow 时必须使用能携带 `taskToDomain` 的 StartWorkflowRequest，而不是只传 name/version/input 的简化调用。

## 5. Worker API 与持久调度记录

### 5.1 Capability 注册与任务领取

```http
POST /api/worker/v1/registrations
POST /api/worker/v1/tasks:poll
```

Worker 上报实例标识及支持的 Capability Version。服务端验证：

1. Internal JWT 的 `sub`、`organization_id`、`credential_id` 与 scope；
2. 当前 Organization Membership 与 Capability 使用权限；
3. Capability Version 已发布且允许客户端执行；
4. 由服务端计算 Execution Domain 与 Conductor Task Definition；
5. 在正确 Domain 内执行 Conductor batch long-poll。

领取到任务后，服务端不能只把它原样返回。需要先持久化一个 `WorkerDispatch`，再向客户端返回经过清洗的 `WorkerTask`：

```text
WorkerDispatch
  dispatch_id                 # 对外不透明 ID
  conductor_task_id           # 仅内部保存
  conductor_workflow_id       # 仅内部保存
  job_id / step_run_id
  attempt
  capability_key / version
  user_id / organization_id
  credential_id / worker_instance_id
  status
  lease_expires_at
  created_at / delivered_at / completed_at
```

建议状态为：

```text
claimed -> delivered -> running -> result_received
    |          |           |
    +-------- expired / failed
```

若任务已经持久化但 HTTP 响应丢失，下一次 poll 返回同一有效 Dispatch。若 Conductor poll 后服务在持久化前崩溃，则依靠 Conductor `responseTimeoutSeconds` 回收任务，不能创建一个无期限的隐形租约。

### 5.2 心跳

```http
POST /api/worker/v1/tasks/{dispatch_id}:heartbeat
```

心跳延长本地 Dispatch lease，并由异步适配器更新 Conductor 的 `IN_PROGRESS`、`callbackAfterSeconds` 或租约。服务端同时检查 Job/StepRun 是否取消。Worker 的本地租约必须与 Conductor response timeout 对齐，否则会出现一边认为仍在执行、另一边已经重投的双执行窗口。

### 5.3 完成与失败

```http
POST /api/worker/v1/tasks/{dispatch_id}:complete
POST /api/worker/v1/tasks/{dispatch_id}:fail
```

请求必须携带 attempt 和幂等键。`media_workflow` 先验证主体、Organization、Worker 实例/凭证、租约、attempt 和 Capability 输出 schema，然后在本地事务中：

- 插入 `WorkerResult Inbox`，唯一键为 `(dispatch_id, attempt, operation)`；
- 将 Dispatch 标为 `result_received`；
- 返回 `202 Accepted` 和稳定 receipt。

后台 Conductor Adapter 再以至少一次方式调用 `POST /api/tasks`，成功后记录已提交。这样 Conductor 暂时不可用不会迫使客户端猜测是否需要重新执行 Provider 调用。重复完成返回原 receipt；旧 attempt 返回 `409`，过期租约返回 `410`。

## 6. 输入、输出与对象存储

编译产物和 Conductor payload 只保存稳定业务 ID、Object Path 与非敏感参数，不能保存临时 URL。

- 领取任务时，`media_workflow` 为输入 Object Path 生成新的短时 presigned GET URL；
- 输出由 Worker 使用服务端签发的 presigned PUT URL 上传；
- Worker 只回传 `object_path`、checksum、content_type、size 和 provenance，不接受任意外部 URL 直接成为平台资产；
- Provider 只返回远端 URL 时，Worker 必须先将结果摄取到受控对象存储；
- Finalizer 校验对象并决定唯一 Generated Asset，随后由 `media_workflow` 调用 `media_materials` 登记。

这里有两个不同的“回写”，不能混为一谈：

1. Worker 执行结果：`media_worker -> Gateway -> media_workflow -> Conductor`；
2. 最终业务资源登记：`media_workflow -> media_materials`。

前者是客户端调度协议，后者是服务间业务协作。

## 7. 身份、权限与撤销

客户端使用的“API Token”在统一语言中仍称 Service Token，不引入第二种凭证聚合。用于 Worker 的 Service Token 必须绑定一个 Organization，并至少支持：

- `workflow.worker.poll`
- `workflow.worker.heartbeat`
- `workflow.worker.complete`
- `workflow.worker.fail`
- `workflow.worker.upload`

Internal Identity JWT 至少携带：

```text
iss, aud, sub, organization_id, scope,
credential_id, auth_source, iat, exp, jti
```

`sub` 统一为业务 User ID；Kratos Identity ID 如需审计使用独立 claim，不能与业务 User ID 混用。JWT audience 固定为 `media-workflow`，TTL 建议 1 至 5 分钟。Worker 路由不要求浏览器 Kratos Cookie。

最终有效权限是以下条件的交集：

```text
Token scope
∩ 当前 Organization Membership / Entitlement
∩ Job 所有权或显式 Worker Pool 授权
∩ Capability allowlist
```

每次 poll、heartbeat 和 completion 都经过 Gateway，因此 Token 撤销后不能继续领取或续租。已发出的短时 JWT 只在 TTL 内有有限滞后，现有租约到期后由 Conductor 重投。

## 8. Capability 版本演进

- 已发布 Capability Version 不可修改；
- 老 Workflow/Task 继续引用老版本和老 `conductor_task_def`；
- 新 Worker 可同时声明多个版本；
- 新协议通过发布新 Capability Version 和 Task Definition 演进；
- archived 只禁止新的 Workflow 发布引用，不能立即删除仍有在途任务的旧定义。

Worker 声明业务 Capability Version，而不是原始 Task Definition，防止客户端越权领取内部任务或未来新增任务。

## 9. 错误与审计语义

建议稳定错误语义：

| 状态码 | 含义 |
|:---|:---|
| 401 | Service Token 或 Internal JWT 无效 |
| 403 | scope、Organization、Job 或 Capability 权限不足 |
| 204 | 当前无可领取任务 |
| 409 | attempt 过期、归属冲突或重复操作冲突 |
| 410 | Dispatch lease 已过期 |
| 422 | 输出不符合 Capability schema |
| 429 | Worker 并发或速率超限 |
| 503 | Conductor/调度器暂时不可用 |

审计至少记录 user、organization、credential、worker_instance、dispatch、step_run、attempt、capability、trace_id 与结果摘要。原始 Token、Provider 密钥、完整 presigned URL 和敏感提示词不得写入日志。

## 10. 被否决或延后的方案

| 方案 | 结论 | 原因 |
|:---|:---|:---|
| 客户端直连 Conductor | 否决 | 绕过统一网关、暴露内部协议、无法完整授权和审计 |
| 从共享队列 poll 后再检查 owner | 否决 | 任务已被占用，会泄露并造成队列饥饿 |
| 客户端提交任意 task type | 否决 | 可能领取内部任务，绕过 Capability Registry |
| completion 同步直写 Conductor 后才响应 | 否决 | Conductor 短暂失败会让客户端重复 Provider 副作用 |
| API Token 写入 Workflow/IR/Task Input | 否决 | 长期凭证会被定义、历史与日志持久化 |
| MVP 引入独立 Agent 权限模型 | 延后 | 当前身份仍是 User；`credential_id` 和 `worker_instance_id` 仅用于凭证与审计 |
| Organization 共享 Worker Pool | 延后 | MVP 按 User Execution Domain 隔离，未来以显式授权扩展 |

## 11. 当前实现差距

本节只记录审查时的实现差距，不改变目标设计：

- `media_worker` 当前仍配置 `CONDUCTOR_URL` 并直接 poll/update；
- Gateway/Oathkeeper 当前主要处理浏览器 Cookie Session，尚无完整 Service Token -> Internal JWT Worker 链路；
- `media_workflow` 的 Internal JWT 中间件仍依赖 Kratos Cookie，不适用于 token-only Worker；
- Service Token 的现有持久化尚未完整实现 scope、Organization 与 expiry；
- 尚无 Worker Registration、WorkerDispatch、Result Inbox、heartbeat 与代理 Worker API；
- StartWorkflow 尚需确认并改为携带 `taskToDomain` 的请求形式。

这些差距属于后续实现计划，不能通过让客户端临时直连 Conductor 规避。

## 12. 能力覆盖矩阵

| 原能力/需求 | 目标限界上下文 | Domain | 聚合/读模型 | Workflow | 状态 |
|:---|:---|:---|:---|:---|:---|
| Service Token 创建、撤销、内省 | media_auth | service_token | ServiceToken | service_token_verification | covered |
| Token 换取 Internal JWT | Edge + media_auth | service_token | Principal | service_token_verification | covered |
| Worker Capability 声明 | media_workflow | execution_capability | ExecutionCapability Version | workflow_execution | covered |
| 按用户隔离领取任务 | media_workflow | execution | WorkerDispatch | workflow_execution | covered |
| 心跳、租约与取消感知 | media_workflow | execution | WorkerDispatch | workflow_execution | covered |
| 幂等完成/失败并代理 Conductor | media_workflow | execution | WorkerDispatch + WorkerResult Inbox | workflow_execution | covered |
| Internal Renderer/Finalizer | media_workflow | template/execution | Internal Execution Task | conductor_compilation | covered |
| 输出对象读写 | media_storage | stored_object | StoredObject | library_object_upload | external-owner |
| Generated Asset 登记 | media_materials | asset | Generated Asset | media_workflow_execution | external-owner |

矩阵没有 `missing`；“covered”表示目标设计已有明确归属，并不表示当前代码已经完成。
