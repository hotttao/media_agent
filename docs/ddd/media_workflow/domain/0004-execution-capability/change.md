# Execution Capability 变更记录

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
