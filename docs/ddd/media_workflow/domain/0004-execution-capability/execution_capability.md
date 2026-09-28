---
name: execution_capability
description: 生成 Worker 可实现的版本化执行契约与 Conductor 路由
dependencies:
    - media_materials.asset_kind
related_workflow:
    - workflow_validation
    - conductor_compilation
    - workflow_execution
---

## 一句话职责

Execution Capability 把 Template 的业务意图绑定到稳定、可版本化的执行契约；它声明输入 schema、唯一输出类别、Provider 模式、Conductor Task Definition、Execution Target 与运行策略，但不保存 Provider 密钥或执行实例。

## 聚合与不变量

- 主键为 `(capability_key, version)`；published 版本不可修改，只能发布新版本或归档。
- `input_schema` 中每个逻辑输入都必须能由 Template Slot 映射；`output_asset_kind` 必须与 Template 一致。
- Capability 只声明目标参数 schema，不拥有业务 Slot 到参数的规则；该规则属于 Template Version 的 Invocation Mapping。
- `execution_target` 是不可变枚举 `personal_worker | managed_worker`；它决定任务进入 Personal 或 Managed Execution Domain，不指定具体 Worker。
- `conductor_task_def` 指向已注册 SIMPLE Task Definition；它只在 `media_workflow` 与 Conductor 内部使用，客户端按 Capability Version 声明能力，不能提交或领取任意 task type。Task 输入只传稳定 ID、Object Path 与参数，不传二进制、密钥或临时下载 URL。
- Capability 可返回一个或多个 Candidate，但确认后的业务输出恒为一个 Generated Asset。
- Personal Worker 由客户端 `media_worker` 承载并使用 API Token；Managed Worker 位于 `media_workflow` 内部，不使用 API Token/Talos。Provider 凭证、本地 CLI 路径及 GPU 配置不进入本聚合。
- Published Version 不允许修改 `execution_target`；目标变化发布新的 Capability Key。推荐使用 `.personal`、`.managed` 等稳定 Key 区分相同输入输出协议的不同执行位置，避免让 version 同时表达协议和部署变化。
- Personal 与 Managed Target 不得因 Worker 离线自动回退；同一 Job 的 taskToDomain 必须服从已冻结 Target。
- 客户端使用 `dispatch_id + attempt` 作为幂等键；Conductor `taskId` 仅由服务端 Dispatch 保存。相同 Dispatch 的重复执行必须返回等价结果或读取既有结果。

## 数据模型

### `execution_capabilities`

| 字段 | 含义 |
|:---|:---|
| capability_key, version | 稳定能力名与不可变版本 |
| provider, mode | Provider 与模式，例如 jimeng / image_generation |
| execution_target | personal_worker / managed_worker |
| managed_pool_key | Managed Target 的可选受控路由键；Personal 时必须为空 |
| input_schema | JSON Schema；描述 Worker 输入 |
| output_asset_kind | 唯一最终输出类别 |
| conductor_task_def | SIMPLE Task Definition 名 |
| retry_policy, timeout_seconds | 编译时采用的运行策略 |
| status | draft/published/archived |
| created_at, published_at | 审计时间 |

初始能力示例：`jimeng.image.first_frame_prompt.personal@1`、`jimeng.image.multi_image_prompt.managed@1`、`doubao.video.first_frame_prompt.personal@1`。实际 key 以已接入 Worker 能力清单为准，不由 UI 自由输入。

## Worker 实现契约

- Personal `media_worker` 可承载多个 `execution_target=personal_worker` 的 Capability Version，向 Worker Gateway API 声明 `(capability_key, version, concurrency)`；服务端拒绝 Managed Capability 和 Internal Execution Task。
- Managed Worker 作为 `media_workflow` 内部独立进程或运行组件，只加载配置允许的 `execution_target=managed_worker` Capability 和 Managed Domain；不调用公开 Worker API，不请求 Talos。
- 每次完成返回 `candidates[]`、受控 Object Path/摘要与 `provenance`；失败返回稳定错误。服务端先保存结果收据，再由 Conductor retry policy 决定是否重投。
- Mock Profile 必须由启动参数显式开启，fixture 版本固定并可审计；它不改变 Capability Definition。
- Renderer 与 finalizer 是 Internal Execution Task，不在客户端能力清单中。
- archived Capability Version 禁止新 Workflow 引用，但旧 Task Definition 必须保留到在途 Job 排空。

## 与 Template 的关系

Template 版本必须引用已发布的 `(capability_key, capability_version)`，并提供通过类型检查的 Invocation Mapping。现存 `platform/gen_way/gen_type` 在迁移期只读保留，由数据迁移映射到 Capability；新写接口不得再以这三个自由字符串决定队列。
