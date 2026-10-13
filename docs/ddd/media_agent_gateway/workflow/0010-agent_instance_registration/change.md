# 变更记录

## 2026-10-12 · DDD 收拢与核心接口模型验证

- 核心六能力与旧入口纳入覆盖矩阵，保持完整目标与 Plugin/Auth 外部所有者。
- 实例注册、执行所有权、配置发布/应用分开建模；迁移持久意图与 Agent epoch 双侧协调。
- queued 接管重取凭据，浏览器 Cookie 与稳定 Session Token 分离；原Session配置产物保持。
- 接口伪代码22场景通过，完整spec与plan已形成；模型验证不替代实际数据库/Broker/环境集成测试。



### 2026-10-13 · P3 两步原生身份登记

先解析已核验 native generation，进程在注册重试中保持 UID/generation 不变。登记 CAS 与历史原子保存，同 generation 不同 incarnation 拒绝。响应交付可重建的独立 MAC 实例凭证和持久准入状态；Agent 启动恢复该准入后开放受签名保护的执行接口。
