// Package client 是 media_workflow 的业务级下游 client 实例装配层。
//
// 按客户端调用指南（contributing/client-cn.md）§7：
//   - 决定当前服务需要哪些下游 client；
//   - 消费 clienthertz 的治理能力（BuildBaseSuite / BuildAppSuite / BuildOptions / ApplyMiddlewares）；
//   - 把初始化后的 client 暴露为全局变量供 service / workflow 使用。
//
// 当前 media_workflow 模板无下游依赖，clients.go 保留空骨架；
// 新增下游时按 §9 流程：clients.go 加变量 → init_<app>.go 实现 init<App>Clients → init.go 注册。
package client

// 此处声明全局下游 client 变量。
//
// 命名规则：<AppPascalCase><ServicePascalCase>Client
//
// 例：
//   var MediaSchedulerJobClient *hertzgenmedia_scheduler.JobServiceClient
//
// 当前模板无下游，留空。
