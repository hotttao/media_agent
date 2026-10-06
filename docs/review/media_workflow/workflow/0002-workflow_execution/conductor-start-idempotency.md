# Conductor 普通启动幂等问题（暂缓实现）

## 实测事实

运行镜像为`conductoross/conductor:3.32.4`。普通`POST /api/workflow`使用相同名称、版本、输入、correlationId及idempotencyKey，并设置`idempotencyStrategy=RETURN_EXISTING`，重复请求仍得到不同实例ID。

证据：`media_workflow/e2e/start_idempotency_test.go`的`TestConductorStartIdempotencyContract`。2026-10-06再次运行失败（0.88s），检测到两个不同实例。测试只注册唯一HUMAN技术流程，终止自己的实例并清理定义，不调用用户模型。源码普通入口为WorkflowResource→WorkflowServiceImpl.startWorkflow→WorkflowExecutorOps.startWorkflow；请求字段存在不等于普通入口实现了去重。

这是仓库内的问题记录，未向上游提交Issue，也不代表其他Conductor版本均有相同行为。

## 保留的保护与限制

已提交但响应丢失时，启动意图保持uncertain，只查询原correlation并核对精确实例的名称、版本、完整输入与Task Domain；不盲目重发。发送标记提交后、实际POST发出前进程退出，可能留下无实例的uncertain记录，当前无法自动继续。公开Submit的本地幂等回执不能代替引擎启动幂等。

## 用户决定

2026-10-06用户要求记录问题，暂不实现启动幂等。本轮不开发Conductor自定义入口、不修改外部源码、不为此升级替换引擎，保留uncertain只读对账。引擎级幂等及发送前崩溃自动补发从本轮开发范围暂缓移出；不标记为已完成，不改失败协议门禁来掩盖限制。
