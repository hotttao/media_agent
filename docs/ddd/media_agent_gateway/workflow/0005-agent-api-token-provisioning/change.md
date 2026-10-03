# Agent API Token Provisioning 变更记录

## 2026-09-28 · 初始建模

- Agent 创建后由 media_agent_gateway 请求 media_auth 为 `Agent:<id>` 创建 Talos API Token。
- Token 固定 owner User、Agent Actor、Organization 和 scope；Secret 只返回一次，Gateway 不持久化。
- 运行时 Agent 只经 Gateway 代理内部服务，Internal JWT 使用 `sub=User + act.sub=Agent`。
- Agent 与 Token 跨服务创建不做分布式回滚；Agent 可保留，Token 创建按幂等键重试，孤立 Talos Key 通过撤销补偿收敛。
## 2026-10-03 · 明确稳定 Actor 和签发一致性

Actor 绑定配置记录 ID，不使用 Runtime 类型标签。Control 提供无 Provider Secret 的内部身份投影；签发时序改为复用既有 API Token Issuance 持久意图及 CAS，再调 Talos，未知结果由持久后台恢复。不把 stopped 自动定义为身份停用。

## 2026-10-03 · 私有签发入口与一次性响应

- 签发使用专用 Gateway 服务凭据，身份由可信 User 和 Control 当前事实派生；scope 仅允许 `agent.gateway`。
- 明确配置/purpose/客户端键的有界摘要与重复请求 409；响应丢失只查元数据，不重交付 Secret 或自动轮换。
- Auth 签发接口复用原 Issuance 流程；Gateway 申请/代理与生命周期撤销仍须分别实现验收。
