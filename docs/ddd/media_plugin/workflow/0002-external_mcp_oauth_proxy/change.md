# 变更记录

## 2026-10-12 · DDD 收拢与核心接口模型验证

- 核心六能力与旧入口纳入覆盖矩阵，保持完整目标与 Plugin/Auth 外部所有者。
- 实例注册、执行所有权、配置发布/应用分开建模；迁移持久意图与 Agent epoch 双侧协调。
- queued 接管重取凭据，浏览器 Cookie 与稳定 Session Token 分离；原Session配置产物保持。
- 接口伪代码22场景通过，完整spec与plan已形成；模型验证不替代实际数据库/Broker/环境集成测试。


## 2026-10-15 · 接通现有 Auth 服务

使用 Auth 私有可用 access token 端口与既有浏览器授权流程。每次校验固定 Control Release 和当前准入，不新增连接事实副本、后台同步或自动工具重试。外部 OAuth 经 Plugin 独立 MCP 协商代理，其他外部认证由 Gateway 使用受控内部凭据直接连接。
