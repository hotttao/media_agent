# API Token 变更记录

## 2026-09-28 · Talos 与 Delegated Actor

- 统一使用 API Token，删除自行生成 `tos_`、保存 SHA-256 Hash 的 Service Token 目标设计。
- media_auth 保存管理元数据与 `talos_key_id`，Talos 成为 Secret 创建、验证、轮换、撤销和过期的唯一事实源。
- Token 固定绑定 User、Agent、Personal Worker 或 Service Actor；Agent/Personal Worker 使用 `sub=User + act.sub=Actor`。
- Agent API Token 由 media_agent_gateway 在创建 Agent 流程中请求，Secret 只返回一次；Managed Worker 不使用 API Token。
- `VerifyAPIToken` 组合 Talos 结果与当前 User、Actor、Organization 和 scope，替代 `IntrospectServiceToken`。

## 2026-08-06 · Platform Token 拆分

- Platform Token 从通用客户端凭证中拆出；它继续作为独立加密托管凭证，不纳入 Talos API Token 生命周期。
