# Conductor Compilation 变更记录

## 2026-09-29 · 旧联调定义恢复

- 明确 Capability Task 必须携带 Dispatch 校验所需的 Job/Step/Capability 元数据。
- 旧定义缺字段时不降级鉴权、不猜测归属、不覆写运行快照；经业务 API 取消旧样本并重新发布新样本定义。
- Mock 样本资源服务根地址独立配置，保留冻结路径与来源；状态门禁不替代资源访问验收。

## 2026-09-28：Execution Target 进入 IR 与 taskToDomain

- Semantic Compiler 将 Capability Version 的 Personal/Managed Target 冻结进 Executable Step IR。
- Job 启动按 Target 生成 User Domain 或 Managed Pool Domain；Definition 继续可复用，不持久化某次 Job Domain。
- Personal/Managed 不能互相或向 `NO_DOMAIN` 回退，具体 Worker Instance 不进入 IR。

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
