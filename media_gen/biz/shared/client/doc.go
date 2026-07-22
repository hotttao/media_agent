// Package client 是 media_gen 业务级下游 client 实例装配层。
//
// 按客户端调用指南（contributing/client-cn.md）§7：
//   - 决定当前服务需要哪些下游 client；
//   - 消费 clienthertz 的治理能力（BuildBaseSuite / BuildAppSuite / BuildOptions / ApplyMiddlewares）；
//   - 把初始化后的 client 暴露为全局变量供 service / workflow 使用。
//
// 当前依赖：media_workflow（调 ClaimStepRun / CompleteStepRun / FailStepRun）。
package client
