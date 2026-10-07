# Workflow 与 Conductor 实现说明

完整设计说明见 [编译、执行边界与 Conductor 交互](workflow-conductor-design.md)。该文档包含抽象架构、复杂并行示例、DSL/IR/Definition 转换、事务与幂等边界、重试/审批/重新生成、客户端回写、对象存储以及实际验收证据。

当前验收使用客户端 mock，真实厂商调用未接入。UI、全量历史迁移、权限扩展与引擎级启动幂等扩展不属于本轮核心验收，不能据本文推断这些工作已完成。
