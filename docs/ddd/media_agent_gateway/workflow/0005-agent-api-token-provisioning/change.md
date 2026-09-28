# Agent API Token Provisioning 变更记录

## 2026-09-28 · 初始建模

- Agent 创建后由 media_agent_gateway 请求 media_auth 为 `Agent:<id>` 创建 Talos API Token。
- Token 固定 owner User、Agent Actor、Organization 和 scope；Secret 只返回一次，Gateway 不持久化。
- 运行时 Agent 只经 Gateway 代理内部服务，Internal JWT 使用 `sub=User + act.sub=Agent`。
- Agent 与 Token 跨服务创建不做分布式回滚；Agent 可保留，Token 创建按幂等键重试，孤立 Talos Key 通过撤销补偿收敛。
