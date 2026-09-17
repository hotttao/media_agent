# job-execution-loop workflow 变更记录

## 2026-07-21 · 初始建模（media_gen v1）

- 新建 job-execution-loop workflow：单 worker 主循环 StartWorker -> Claim -> ExecuteOnce -> Complete/Fail -> StopWorker。
- 明确**worker loop 本期 stub**：goroutine 启动后立即 `select { <-ctx.Done() }`，不实际跑 Claim 循环，避免本地启动空转。真实实现需替换为 loop 逻辑 + backoff。
- 明确**in-process goroutine**：worker 与 HTTP server 同进程，不独立 listener。回退成本低（拆为独立进程只需把 StartWorker/StopWorker 换成 IPC），未达 ADR 阈值。
- 幂等：`StartWorker` 对 STOPPED 状态 worker 复用重启；`StopWorker` 对已 STOPPED 状态幂等返回。
- 错误处理：Complete 失败不本地 retry（`media_workflow` scanner 回收 claimed 超时 step_run）。
# 2026-08-06

- 执行循环改为 River 按 Generation Task ID 唤醒；不再由 media_gen 领取 media_workflow StepRun。
- Task 状态和 attempt 归 media_gen，Provider/Storage 调用在数据库事务外执行。

## 2026-09-17 · 多候选与重新生成 attempt

- Worker 一次 Provider 调用可以保存多条 Candidate；单候选直接 succeeded，多候选进入 needs_confirm。
- 用户 regenerate 由 Generation 聚合记录反馈并创建新 attempt，再由同一执行循环处理。
- 主动重新生成与基础设施失败恢复分离：新 attempt 不复用上一 attempt 的候选，未知 Provider 结果仍先查询而不盲目重发。
