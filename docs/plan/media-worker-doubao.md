# media_worker 正式迁移计划

对应 [spec](../spec/media-worker-doubao.md)。正式工作区：media_agent。

## P0 文档和规范

先完成 spec/plan；核对当前 harness、media_worker、能力和上传协议。补充非 HTTP worker 的 DDD 适用说明。先核查目标工作树，保留其他改动，不复制 .env、账号、Cookie 或运行状态。

## P1 结构与接口（已确认）

1. internal/gateway → biz/client/worker_gateway；internal/fixture → biz/client/fixture；internal/results → biz/dal/repo/results。
2. domain/execution 定义 Task、Capability、交付/检查点事实和 Gateway/Executor/Uploader/仓储端口；协议与磁盘 DTO 独立映射。
3. domain/doubao 定义 Generator 和生成值对象；studio 只建立边界，不接真实生成。
4. CLI 的注册、恢复、轮询、执行、心跳、交付抽至 workflow/execute，端口注入，CLI 构造 Command。
5. 能力选择、失败分类迁 policy；trace 工具迁 shared；删除旧 internal，不加兼容转发。
6. 保留现有 fixture、HTTP wire、幂等键及状态文件格式。补旧格式兼容、fake workflow 恢复/持久化顺序、错误退出测试；跨进程组合测试放 e2e。
7. 更新 README 和失效的代码链接；检查架构门禁实际有效。

## R1 Review 与用户确认

检查领域泄漏、CLI 业务残留、具体客户端耦合、状态兼容及完整迁移准入条件。记录发现、修正、检查结果。跑完格式、编译、单测、race、vet、架构和死代码检查；环境受阻则列为未完成。

随后向用户确认，未确认不进入 P2–P5。

## P2 Studio 基线（已授权）

固定本地版本与修改集，验证控制面和任务导入协议。实现专用实例管理、精确会话绑定、readiness 与串行容量准入。证明稳定 Key 的提交/查询恢复能力；协议不足先修 studio，不盲目重发。

## P3 首帧真实生成（已授权）

接共享配置及生产 profile，完成有序输入下载、参数配置、提交、确认、观察、精确产物下载。只开放 doubao.i2v.first_frame_prompt@1，独立真实任务验证全自动闭环及转人工行为。

## P4 中断恢复（已授权）

实现检查点仓储、提交意图和引用绑定、产物 manifest、上传续接。补齐恢复时资格确认，必要的协议变化按 IDL 生成流程处理。注入各副作用前后退出/丢响应，验证同次不重复生成、不同 attempt 不误绑定。

## P5 公开网关验收（已授权）

独立 Worker 身份，启动前核对待执行队列；真实首帧→视频→授权上传→object_path→候选审批。记录失败/恢复证据及运行文档，区分技术测试和部署验收。

## P6 迁移完成后维护用户 fork

用户已提出此后续任务：确认 GitHub 身份，fork dorakoo/doubao-studio，审查并直接提交本地适配修改至用户仓库，保留上游来源、固定 commit 并运行 studio 检查。

## 当前交接状态（2026-10-26）

P0、P1 已在正式 media_agent 工作区落地。旧 internal 已删除，无兼容 alias；执行与豆包端口、独立 DTO、policy/shared 和端口注入 workflow 已建立，现有 fixture 和旧日志格式保留。DDD 文档与能力覆盖矩阵见 [Execution review](../review/media_worker/domain/0001-execution/execution.md)。

R1 代码 review 已完成并修正：CLI 业务循环、具体 HTTPError 耦合、业务协程退出进程、心跳回收、领域/磁盘模型混用，以及 archcheck 目标路径归一化覆盖问题。真实临时 workspace 的允许/拒绝依赖回归已通过。

验证结果与环境说明以 [Execute review](../review/media_worker/workflow/0001-execute/execute.md) 为准；本轮重新执行，不沿用旧 /tmp 副本证据。Go/C 工具链临时置于 /tmp，不写项目配置或私密运行数据。

用户已确认 P1（回复“进入”），P2–P5 已完成首个生产能力实现与真实闭环验收。P3 独立 Studio 视频全自动生成，用户明确确认没有手动操作；P4 同身份/检查点重启续接原引用；P5 独立 Worker 经公开网关领取、生成、授权上传、候选审批并登记成功。通用登记补传可信 Job 企业；存储公开 origin 已统一，原生 retry 只继续失败 Finalize。

- [x] P2 专用实例、认证控制协议、目录归属证明、串行准入与持久化单次意图。
- [x] P3 一张首帧、5 秒、9:16 的自动生成与精确产物下载。
- [x] P4 检查点各阶段故障测试、真实 Worker 重启、同一 Key 单一 Studio Task。
- [x] P5 Job `job_20938866-9ba7-4844-84a4-bf7a4d77f1fe` SUCCEEDED；原生成/审批各一次，产物 `gen_beb5adc8-aedb-4e2a-8c72-36d22c9953fc`。
- [x] P6 用户 fork 分支 `codex/media-worker-studio`，commit `840117a`；完整 validate 通过。

最终实现、检查范围、历史架构问题及真实证据见 [Generate review](../review/media_worker/workflow/0002-generate/generate.md)。只开放 `doubao.i2v.first_frame_prompt@1`；多图、生图及平台挑战仍需要各自验收，不能从本次结论外推。


## 2026-10-26 结构更正

用户明确移除 execution Domain：公共技术执行协议现位于 `biz/shared/worker_runtime`，Provider 检查点分别归各自 Domain。原 `workflow/generate` 正名为 `workflow/doubao_generate`，仓储正名为 `dal/repo/doubao_checkpoints`；物理检查点目录与格式保持兼容。GPT 成图独立实现，不能通过 Recipe 合入豆包流程。旧 P1 review 保留其当时审查记录，当前结构见 [运行契约](../ddd/media_worker/runtime.md)。
