# Conductor Compilation 变更记录

## 2026-09-27：Capability Task 路由与客户端协议隔离

- Definition 继续冻结 Capability taskDef，但客户端只声明 Capability Version，由 media_workflow 完成内部映射。
- Job 启动时通过 `taskToDomain` 为 Capability Task 设置 User Execution Domain；该运行时路由不写回可复用 Definition。
- Capability completion 改为 Worker Gateway 持久收据后异步更新 Conductor，客户端不再看到 taskId。

## 2026-09-25：引入 Executable Workflow IR

- Workflow 发布时由 Semantic Compiler 生成冻结的 Executable Workflow IR。
- Backend Compiler 把 IR 编译成 Conductor Definition；运行时不重新解释可编辑的 Workflow 与 Template。
- Template 的输入映射统一命名为 `slot_bindings`，与 Capability 固定输入契约分离。

## 2026-09-26：打通服务端内部任务与客户端 Capability Worker

- `media_workflow` 增加 `media_render_template_v1` 与 `media_finalize_step_v1` 内部 Worker。
- 客户端 `media_worker` 仅执行版本化 Capability Task，并显式拒绝内部 Task Definition。
- 发布流程持久化 IR、Conductor Definition 与哈希后注册到 Conductor；Job 通过冻结的 compilation 启动并投影执行状态。
- 使用 `media_ai` 的单个完整视频样本验证八步流程和并行动作视频分支；全量迁移等待用户确认。
