# Execution Capability 变更记录

## 2026-09-28：冻结 Personal/Managed Execution Target

- Capability Version 新增不可变 `execution_target=personal_worker|managed_worker` 与可选 `managed_pool_key`。
- Personal Capability 由带 Delegated Actor API Token 的客户端 media_worker 执行；Managed Capability 由 media_workflow 内部 Worker 执行，不使用 Talos/API Token。
- Semantic Compiler 把 Target 冻结进 Executable IR；Job 启动按 Target 生成 Personal 或 Managed Task Domain，两类队列不得互相或向 `NO_DOMAIN` 回退。
- 相同协议需要不同执行位置时优先发布不同 Capability Key，避免用 version 表达部署位置变化。

## 2026-09-27：Capability Worker 改用 Worker Gateway API

- Worker 声明 `(capability_key, version, concurrency)`，不再配置或提交原始 Conductor Task Definition。
- media_workflow 通过冻结的 Capability Registry 映射 taskDef，并阻止客户端领取 Renderer/Finalizer。
- 客户端幂等键改为 `dispatch_id + attempt`，Conductor taskId 只在服务端保存。
- archived 版本只禁止新 Workflow 引用，旧 Task Definition 保留至在途执行排空。

## 2026-09-17：建立版本化 Worker 能力契约

- 新增 Execution Capability 聚合，把 Provider 路由、schema、输出类别和 Conductor Task Definition 从 Template 自由字符串中分离。
- published 版本不可变；Template 必须固定引用一个版本。

## 2026-09-25：明确 Capability 与 Invocation Mapping 边界

- Capability 只声明 Worker 固定输入 schema 与 Task Definition，不保存业务 Slot 映射。
- Template Version 的 Invocation Mapping 负责生成 Capability 参数；Semantic Compiler 在发布期完成覆盖性与类型检查。

## 2026-09-25：Capability Worker 移到客户端

- Execution Capability 对应 Worker 由客户端 `media_worker` 承载，允许访问本地模型、CLI、GPU 与 Provider 凭证。
- 服务端仅运行 Renderer、finalizer 等 Internal Execution Task。
- 联调增加显式 Mock Profile，用冻结的 `media_ai` 样本完整执行一个视频生成 Workflow；确认后再迁移全部数据。
## 2026-10-06：纠正Provider能力与业务用途的混淆

- 用Provider操作契约代替业务步骤独占能力；一个生图能力供多个业务模板复用。
- 技术输出Media Type与Template业务Asset Kind解耦；旧字段/版本只作兼容，新注册/模板/Workflow使用新版本，不改既有IR和执行。
- 本轮保持mock联调边界，真实Provider适配、UI、历史全量迁移及Conductor启动幂等不自动扩入。
- 重新以001原始四类Task为基线，generation_method区分媒体转换，generation_type区分首帧/多图模式；规范Key绑定三维，version固定契约，首帧一图、多图有序至少一图。先前三种种子草稿已撤回，不作为最终能力目录。
## 2026-10-09 公开只读能力目录

为 standalone 模板编辑器提供已发布 Provider 契约的分页目录和指定版本查询，隐藏引擎任务名/Managed Pool/凭据；不新增能力写入或运行语义。纯查询审查无需新增 Workflow。
