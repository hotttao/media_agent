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

Execution Capability拥有Provider操作的稳定版本化契约，例如Doubao生图、Doubao生视频、即梦生图；声明固定输入、技术输出媒体类型、Provider模式、Conductor Task Definition、Execution Target与运行策略，不拥有模特图等业务用途，也不保存Provider密钥或执行实例。

## 聚合与不变量

- 主键为 `(capability_key, version)`；published 版本不可修改，只能发布新版本或归档。
- 核心身份为platform/generation_method/generation_type；Personal Key为`platform.generation_method.generation_type`，同契约Managed变体沿用独立Key规则加`.managed`，避免改变已发布Target。唯一版本为Key@version，数据库同时约束结构化字段与Key一致。
- generation_method表示输入到输出的媒体转换(t2i/i2i/t2v/i2v)；generation_type表示首帧(first_frame_prompt)/多图(multi_image_prompt)等输入模式，不是业务Asset Kind。首帧images恰好一张；多图images至少一张且顺序有意义。豆包多图视频是原需求“类似两种”下的项目契约，真实Provider原生协议仍需未来适配验证。
- `input_schema`中每个逻辑输入都必须能由Template Invocation Mapping提供；新Provider契约的`output_media_type`为IMAGE/VIDEO/TEXT，必须与Template业务输出Asset Kind的Media Type兼容，不要求Asset Kind名称相等。旧`output_asset_kind`仅解释既有业务型Capability，不能作为新Provider能力的分类依据。
- Capability 只声明目标参数 schema，不拥有业务 Slot 到参数的规则；该规则属于 Template Version 的 Invocation Mapping。
- `execution_target` 是不可变枚举 `personal_worker | managed_worker`；它决定任务进入 Personal 或 Managed Execution Domain，不指定具体 Worker。
- `conductor_task_def` 指向已注册 SIMPLE Task Definition；它只在 `media_workflow` 与 Conductor 内部使用，客户端按 Capability Version 声明能力，不能提交或领取任意 task type。Task 输入只传稳定 ID、Object Path 与参数，不传二进制、密钥或临时下载 URL。
- Capability 可返回一个或多个 Candidate，但确认后的业务输出恒为一个 Generated Asset。
- Personal Worker 由客户端 `media_worker` 承载并使用 API Token；Managed Worker 位于 `media_workflow` 内部，不使用 API Token/Talos。Provider 凭证、本地 CLI 路径及 GPU 配置不进入本聚合。
- Published Version 不允许修改 `execution_target`；目标变化发布新的 Capability Key。新Provider契约Personal使用基础Key，Managed变体追加`.managed`；不能让version同时表达协议和部署变化。
- Personal 与 Managed Target 不得因 Worker 离线自动回退；同一 Job 的 taskToDomain 必须服从已冻结 Target。
- 客户端使用 `dispatch_id + attempt` 作为幂等键；Conductor `taskId` 仅由服务端 Dispatch 保存。相同 Dispatch 的重复执行必须返回等价结果或读取既有结果。

## 数据模型

### `execution_capabilities`

| 字段 | 含义 |
|:---|:---|
| capability_key, version | 稳定能力名与不可变版本 |
| provider, mode | Provider 与模式，例如 jimeng / image_generation |
| generation_method, generation_type | 生成方式与生成类型；旧记录null，新契约必填并参与唯一性 |
| execution_target | personal_worker / managed_worker |
| managed_pool_key | Managed Target 的可选受控路由键；Personal 时必须为空 |
| input_schema | JSON Schema；描述 Worker 输入 |
| output_media_type | Provider技术输出形态IMAGE/VIDEO/TEXT；新能力必填，旧记录保持null不猜测回填 |
| output_asset_kind | 旧业务型能力的兼容字段；新Provider能力为空，不与Template业务分类耦合 |
| conductor_task_def | SIMPLE Task Definition 名 |
| retry_policy, timeout_seconds | 编译时采用的运行策略 |
| status | draft/published/archived |
| created_at, published_at | 审计时间 |

原始四类基线为`jimeng.i2i.first_frame_prompt@1`、`jimeng.i2i.multi_image_prompt@1`、`doubao.i2v.first_frame_prompt@1`、`doubao.i2v.multi_image_prompt@1`，本轮目录使用Personal Target。输入固定为prompt与images数组，schema声明IMAGE及min_items/max_items；输出固定candidates数组、技术Media Type和候选字段协议。它们不是每个业务节点独占的能力，也不意味着真实Provider调用已经接入。后续Doubao生图等能力按相同身份规则扩展，不覆盖原始四类。

## Worker 实现契约

Provider输入schema限制在领取与重交付时均执行：参数名、类型、必填、资源数组数量以及Capability引用须符合发布版本；声明媒体约束的资源只允许企业受控对象引用。对象解析后以存储HeadObject的ContentType/SizeBytes校验媒体类型与非空，未知媒体或错误媒体拒绝交付，不从文件名推断；签名URL有效期不足也拒绝。客户端接收服务端已Render的prompt与有序images，不执行内部模板任务。

- Personal `media_worker` 可承载多个 `execution_target=personal_worker` 的 Capability Version，向 Worker Gateway API 声明 `(capability_key, version, concurrency)`；服务端拒绝 Managed Capability 和 Internal Execution Task。
- Managed Worker 作为 `media_workflow` 内部独立进程或运行组件，只加载配置允许的 `execution_target=managed_worker` Capability 和 Managed Domain；不调用公开 Worker API，不请求 Talos。
- 每次完成返回 `candidates[]`、受控 Object Path/摘要与 `provenance`；失败返回稳定错误。服务端先保存结果收据，再由 Conductor retry policy 决定是否重投。
- Mock Profile 必须由启动参数显式开启，fixture 版本固定并可审计；它不改变 Capability Definition。
- Mock按Provider Capability契约领取任务，样本结果可按Job/业务Step配置；生产处理器只理解Provider参数，不硬编码模特图、定妆图等业务语义。一个进程可以注册多个Provider处理器，不需要为每个模板启动独立进程。
- Renderer 与 finalizer 是 Internal Execution Task，不在客户端能力清单中。
- archived Capability Version 禁止新 Workflow 引用，但旧 Task Definition 必须保留到在途 Job 排空。

## 与 Template 的关系

Template 版本必须引用已发布的 `(capability_key, capability_version)`，并提供通过类型检查的 Invocation Mapping。现存 `platform/gen_way/gen_type` 在迁移期只读保留，由数据迁移映射到 Capability；新写接口不得再以这三个自由字符串决定队列。
