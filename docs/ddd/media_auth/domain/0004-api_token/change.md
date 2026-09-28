# API Token 变更记录

## 2026-09-28 · 轮换操作与失败清理

- 增加 rotating 状态和依附的轮换操作实体，操作 ID 写入 Talos replacement metadata 用于定位，不误称为上游幂等键。
- 成功提交与失败恢复通过操作状态/期限互斥。未知结果先撤销旧 Key 作为晚到请求屏障，再分页清理本次 replacement；无法交付的 Secret 不重放，失败 Token 进入 revoked，可新建凭证。

## 2026-09-28 · 撤销恢复与轮换边界

- revoking 是聚合内持久化撤销意图，先于外部调用提交；新增后台分页恢复和错误后查询 Talos 终态的规则。
- 核对 Talos v2alpha1：Rotate 不接受 request_id 且 Secret 仅返回一次，删除“稳定 operation ID 可直接传给轮换接口”的假设。轮换恢复仍需单独实现，不能用撤销恢复冒充。

## 2026-09-28 · Talos 与 Delegated Actor

- 统一使用 API Token，删除自行生成 `tos_`、保存 SHA-256 Hash 的 Service Token 目标设计。
- media_auth 保存管理元数据与 `talos_key_id`，Talos 成为 Secret 创建、验证、轮换、撤销和过期的唯一事实源。
- Token 固定绑定 User、Agent、Personal Worker 或 Service Actor；Agent/Personal Worker 使用 `sub=User + act.sub=Actor`。
- Agent API Token 由 media_agent_gateway 在创建 Agent 流程中请求，Secret 只返回一次；Managed Worker 不使用 API Token。
- `VerifyAPIToken` 组合 Talos 结果与当前 User、Actor、Organization 和 scope，替代 `IntrospectServiceToken`。

## 2026-08-06 · Platform Token 拆分

- Platform Token 从通用客户端凭证中拆出；它继续作为独立加密托管凭证，不纳入 Talos API Token 生命周期。
