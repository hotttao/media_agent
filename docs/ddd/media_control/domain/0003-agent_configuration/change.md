# Agent Configuration 变更

## 2026-08-22：启用 Codex 与 Claude Code 双 Agent 配置

- 将 Agent Type 明确为 `codex | claude-code` 受控枚举。
- Provider API Key 继续由 Control PostgreSQL 加密托管，公开接口仅投影是否已配置。
- 规定更新留空保留、切换 Agent 共用 Workspace Runtime Directory、现有 Session 不变。
- Runtime Context 按 Agent Type 路由并仅在单次运行调用链传递明文密钥。

### 能力覆盖矩阵

| 原能力 | 目标上下文 | Domain | 聚合/读模型 | Workflow | 状态 |
|:---|:---|:---|:---|:---|:---|
| 创建 Workspace/Codex 配置 | media_control | agent_configuration | AgentConfiguration | workspace_lifecycle | covered |
| 更新配置且留空保留 API Key | media_control | agent_configuration | AgentConfiguration | runtime_context_assembly | covered |
| 选择 Claude Code | media_control | agent_configuration | AgentConfiguration | runtime_context_assembly | covered |
| 公开查询配置状态 | media_control | agent_configuration | WorkspaceResponse | 无：纯查询 | covered |
| 解密并下发运行上下文 | media_control | agent_configuration | RuntimeContext | runtime_context_assembly | covered |
| Codex/Claude Turn 执行 | media_agent_gateway/media_agents | agent_session/runtime adapter | Session/Agent process | agent turn | external-owner |
## 2026-10-03 · 稳定委托身份解析

区分类型标签 agent_id 与配置记录的稳定 ID，以 AgentConfiguration ID 形成 Agent Actor。增加内部只读身份投影，回查 Workspace 归属和当前 owner 的部门权限，不返回 Provider 秘密。只读能力不创建 Application Workflow；Token 签发/撤销仍由既有跨服务 Workflow 协调。没有把运行环境 stopped 隐式改成身份停用。
## 2026-10-04 · 失效事实与查询不确定性

- 明确不存在/已删除返回404；不完整或未知身份事实返回503，防止后台误撤销。
