# 初次启动恢复收口

依据 executable-workflow-compilation.md 初次启动章节，仅覆盖既定后端启动恢复任务，不涉及UI/历史/Agent功能。

- [x] 精确实例对账代码：完整冻结命令传入恢复端口；13组单位/race覆盖异版本/名称/输入/路由/ID/correlation、缺字段、重复实例、空查询及不可用，纯GET且错误不泄露。启动/取消编排端口传递冻结参数；真实Conductor `TestConductorStartCommittedResponseLossRecovery` 0.31s通过：实际启动成功后代理503，correlation候选+精确实例GET收口，POST仅一次。Workflow全包/相关race/架构/gofmt/diff通过；本次对账补强尚未部署，不代表引擎级幂等已实现。
- [ ] 引擎级幂等启动（用户于2026-10-06要求暂缓，本轮不实施）：验证同键并发/终态重放/发送前崩溃/提交后丢响应/本地收口崩溃。当前3.32.4普通POST不执行RETURN_EXISTING；仅查询不足以完成发送前崩溃恢复。不得在失败门禁前自动补发。
- [ ] 标准部署、公开HTTP及独立CLI启动恢复门禁与已有执行回归。

引擎补强需要明确部署方案；外部Conductor源码保持只读。不能把精确对账完成标作整体启动幂等完成。

用户已决定本轮不开发引擎扩展；问题与限制见[缺陷记录](../../review/media_workflow/workflow/0002-workflow_execution/conductor-start-idempotency.md)。此项是明确暂缓，不作为当前开发的新增阻塞，也不视为已实现。

当前普通启动链路为WorkflowResource→WorkflowServiceImpl.startWorkflow→WorkflowExecutorOps.startWorkflow；读取到的外部源码包含另一条startWorkflowIdempotent路径，但普通入口未调用。真实3.32.4 `TestConductorStartIdempotencyContract`重新核对仍失败，同RETURN_EXISTING键得到两个不同实例；测试只使用独立HUMAN定义并终止自身实例，未触发模型/业务Worker。发送前崩溃后的自动推进仍需要引擎内部原子去重，数据库Outbox/锁加空correlation查询不能代替该能力。
