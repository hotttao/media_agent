# API Token 变更记录

## 2026-09-29 · 创建意图与未知签发结果恢复

- 创建先持久 issuing 意图，再调用 Talos；无法交付的凭据进入 issuance_cleanup，跨请求取消与服务重启保留。
- 使用服务生成的 Token ID 作为 Talos request_id，客户端幂等键只在创建者范围内去重。恢复接受 Talos 无 Secret 的幂等响应，正常签发仍拒绝缺失 Secret。
- 创建副作用和恢复时序归 api_token_issuance Workflow；本轮先落恢复 Adapter 契约，持久模型和运行时接线仍由 plan 跟踪，不将目标设计冒充已实现。

### 持久化与编排实现

- 聚合增加 deadline、创建者范围内客户端键和 nullable Key；数据库 shape/唯一约束与 CAS 阻止重复接受、过期激活和终态覆盖。
- Service 创建委托 Issuance Workflow，后台从持久状态扫描恢复；撤销未完成创建复用该流程。普通撤销不再允许直接终结 issuing/cleanup 意图。

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
