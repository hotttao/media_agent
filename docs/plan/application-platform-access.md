# 应用平台接入后续计划

- [x] P5 Auth部门Grant、受限业务凭证、内部在线校验与现有成员权限。
- [x] P6 Control幂等Agent绑定与真实Workspace创建。
- [x] P7 Gateway应用会话、消息、Turn、SSE及跨应用隔离。
- [x] P8 MCP Caller可信业务回调与客户端SDK。
- [x] P9 跨服务集成与真实Ory验证；记录部署和外部模型限制。

依赖P5→P6→P7；P8依赖绑定与MCP可信上下文，P9验证最终链路。实现前先更新对应领域与Workflow；跨服务不共享表或跨库事务。

完成依据与边界见 [验收记录](../review/application-platform-access.md)。这些勾选表示本次应用认证协议的代码与测试完成，不表示已部署、已执行生产迁移或已完成真实模型推理验收。
