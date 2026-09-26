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

Execution Capability 把 Template 的业务意图绑定到稳定、可版本化的执行契约；它声明输入 schema、唯一输出类别、Provider 模式、Conductor Task Definition 与运行策略，但不保存 Provider 密钥或执行实例。

## 聚合与不变量

- 主键为 `(capability_key, version)`；published 版本不可修改，只能发布新版本或归档。
- `input_schema` 中每个逻辑输入都必须能由 Template Slot 映射；`output_asset_kind` 必须与 Template 一致。
- Capability 只声明目标参数 schema，不拥有业务 Slot 到参数的规则；该规则属于 Template Version 的 Invocation Mapping。
- `conductor_task_def` 指向已注册 SIMPLE Task Definition；Task 输入只传稳定 ID、Object Path 与参数，不传二进制、密钥或临时下载 URL。
- Capability 可返回一个或多个 Candidate，但确认后的业务输出恒为一个 Generated Asset。
- Capability Worker 归客户端 `media_worker`，服务端只保存契约和调度信息；Provider 凭证、本地 CLI 路径及 GPU 配置不进入本聚合。
- Worker 使用 Conductor `taskId` 作为幂等键；相同 taskId 的重复执行必须返回等价结果或读取既有结果。

## 数据模型

### `execution_capabilities`

| 字段 | 含义 |
|:---|:---|
| capability_key, version | 稳定能力名与不可变版本 |
| provider, mode | Provider 与模式，例如 jimeng / image_generation |
| input_schema | JSON Schema；描述 Worker 输入 |
| output_asset_kind | 唯一最终输出类别 |
| conductor_task_def | SIMPLE Task Definition 名 |
| retry_policy, timeout_seconds | 编译时采用的运行策略 |
| status | draft/published/archived |
| created_at, published_at | 审计时间 |

初始能力示例：`jimeng.image.first_frame_prompt.v1`、`jimeng.image.multi_image_prompt.v1`、`doubao.video.first_frame_prompt.v1`。实际 key 以已接入 Worker 能力清单为准，不由 UI 自由输入。

## 客户端实现契约

- `media_worker` 可承载多个 Capability Version，但只能轮询显式启用的 `conductor_task_def`。
- 每次完成返回 `candidates[]` 与 `provenance`；失败返回可观察原因，由 Conductor retry policy 决定重投。
- Mock Profile 必须由启动参数显式开启，fixture 版本固定并可审计；它不改变 Capability Definition。
- Renderer 与 finalizer 是 Internal Execution Task，不在客户端能力清单中。

## 与 Template 的关系

Template 版本必须引用已发布的 `(capability_key, capability_version)`，并提供通过类型检查的 Invocation Mapping。现存 `platform/gen_way/gen_type` 在迁移期只读保留，由数据迁移映射到 Capability；新写接口不得再以这三个自由字符串决定队列。
