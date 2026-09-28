# Talos API Token 与 Delegated Actor 身份模型

平台统一使用 API Token 作为 CLI、Agent 和 Personal Worker 的长期 opaque 凭证。`media_auth` 是 Token Manager，保存 Token 管理 ID、Talos Key ID、被代表 User、Actor、Organization、scope、状态和过期时间；Talos 是 Secret 生命周期的唯一事实源，负责签发、验证、轮换、撤销和过期。原来自行生成 `tos_` 并保存 SHA-256 Hash 的 Service Token 模型退出，不与 Talos API Token 并存。

API Token 必须绑定一个稳定 Actor。User 直接调用时 Internal Identity JWT 使用 `sub=User:<id>`；Agent 或 Personal Worker 代表 User 调用时采用 RFC 8693 委托语义：`sub=User:<id>`、`act.sub=Agent:<id>|Worker:<id>`。Credential ID、Actor ID 和 Worker Instance 分别表达“哪把凭证”“谁实际调用”“哪个进程实例”，不能互相替代。`client_id` 只保留给 OAuth Client，不用于表达 Agent 或 Worker。

Agent 创建流程由 `media_agent_gateway` 编排：确认 Agent/owner/Organization 后请求 `media_auth` 为 `Agent:<id>` 创建 API Token，Secret 只在成功响应中返回一次。Agent 运行时只能调用 Agent Gateway；Gateway 调用 media_auth/Talos 验证 Token、重新校验 Agent 委托关系并代理内部服务。第一阶段 Agent 与 Personal Worker 可继承 User 权限上限，但有效权限仍是 User 当前权限、Actor 当前状态、API Token scope、Organization 和资源服务授权的交集。

Managed Worker 不属于外部委托 Actor。它运行在 `media_workflow` 服务边界内，不申请 API Token或请求 Talos；信任来自进程/工作负载边界，执行范围由不可变 Capability `execution_target`、Managed Execution Domain、配置 allowlist 和 WorkerDispatch 约束。若以后拆成独立服务边界，使用 mTLS、Workload Identity 或短期内部凭证，而不是用户 API Token。只有确实需要代表 Job User 调用内部资源服务时，才由受信服务签发 audience/scope/dispatch/lease 均受限的短期委托 JWT。

选择该方案的原因：

- Talos 已提供长期 API Key 的安全生命周期，media_auth 无需维护第二套 Secret 实现；
- RFC 8693 `act` 能同时保留业务 User 与实际 Agent/Worker，满足授权和审计；
- Agent Gateway、Personal Worker Gateway 和内部 Managed Worker 具有不同信任边界，不应为形式统一强制使用同一种凭证；
- 任务 User、执行 Actor、Credential 与进程实例分离后，服务端 Worker 无需读取任务 User 后冒充登录。

未采用的方案：把 Agent/Worker 直接写入 `sub` 会丢失被代表 User；只用 User `sub` 会丢失实际 Actor；使用 `client_id` 会混淆 OAuth Client；Managed Worker 持有用户长期 API Token会扩大泄露与冒用范围；media_auth 与 Talos 同时签发两套同用途 Token 会产生撤销和事实源冲突。

代价是 media_auth 需要维护 Talos 元数据映射和 Actor 状态联查，业务服务需要识别 `act.sub`，Agent/Worker 生命周期变化需要撤销关联 Token。Talos OSS 的单节点 SQLite 是当前部署约束，替换实现不得改变 API Token 与 Delegated Actor 的领域契约。

**状态：accepted**
