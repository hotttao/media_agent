# Agent Session 变更记录

## 2026-10-12 · DDD 收拢与核心接口模型验证

- 核心六能力与旧入口纳入覆盖矩阵，保持完整目标与 Plugin/Auth 外部所有者。
- 实例注册、执行所有权、配置发布/应用分开建模；迁移持久意图与 Agent epoch 双侧协调。
- queued 接管重取凭据，浏览器 Cookie 与稳定 Session Token 分离；原Session配置产物保持。
- 接口伪代码22场景通过，完整spec与plan已形成；模型验证不替代实际数据库/Broker/环境集成测试。

## 2026-10-11 · 参考 agent-platform 的副本路由与共享存储

- Session 绑定副本持久化；Ready 且非 draining 时沿用，扩容不改变有效绑定。
- 支持缩容及不可用/draining 副本上的 Session 重绑，替代本次讨论中较早的禁止缩容设想。
- 多副本共享 Workspace RWX 卷及 HOME；执行互斥在 Session 级，共享文件采用独立的原子发布规则。
- 保留本项目 Agent-owned FIFO；恢复扫描限定负责范围，重绑不得自动重复未知工具副作用。
- 运行所有权与排空准入的实现细节待 spec 明确，本次仅更新目标设计。

## 2026-08-23 · Workspace Agent 切换后的 Session 兼容性

- Session Response 公开创建时固定的 Context Version，供客户端识别与 Workspace 当前配置是否兼容。
- 配置不兼容的既有 Session 保留历史读取、事件订阅和运行中 Turn 中断能力，但新 Turn 返回明确冲突并要求创建新 Session。
- Workspace Agent/API Key 变更不再让 UI 继续向旧 Registration 提交 Turn；UI 创建并选中当前版本的新 Session。

## 2026-08-22 · Agent Request 只读代理落地

- Gateway 新增授权后的 Agent Request 查询与 resolve 路由，通过 Agent Runtime Session API 代理。
- Request、响应、幂等和终态不在 Gateway 建立事实副本，页面仅通过该代理处理交互。

## 2026-08-18 · Turn 与交互事实下沉 Agent

- Gateway Agent Session 收敛为 Registration、访问控制、Session Token 与 UI 元数据。
- Turn、pending message、Message、Universal Event、Agent Request、Session sequence 和 Chat Status 的权威所有权迁移到 `media_agents/runtime_session`。
- `pending_message` 单槽位被 Agent 持久化 Queued Turn 取代；Gateway 不再维护 Session 执行锁或 active Turn。
- 事件回放只通过 Agent 查询接口完成，禁止 Gateway 共享或直连 Agent 数据库。

## 2026-08-10 · 最小 Chat 默认上下文

- 增加 `GetOrCreateDefault`，让已登录但尚未配置 Workspace/Agent 的用户可以直接进入对话。
- media_control 空响应解释为“使用平台 Default Agent Context”，而不是启动失败。
- 明确 Universal Event 必须完整持久化，assistant/tool Message 由已提交的完成事件生成，不允许只保存 UI 摘要。

## 2026-08-11 · 多客户端会话事件订阅

- Universal Event sequence 统一为 Session 内严格递增游标。
- 增加按游标回放能力；进程内广播只负责低延迟分发，不替代持久化事实。
- 一个 Session 的多个页面订阅相互独立，任何客户端断开都不得取消 Turn。

## 2026-08-17 · 事件持久化分级与 Chat Status

- 把非持久化 Live Event Frame 与带 sequence 的 Universal Event 分开，恢复承诺收敛到最终 Message、工具快照和生命周期事实。
- tool result 改为首次立即写、300ms 聚合追加快照、终态强制 flush，并在持久化边界统一截断超大输出。
- Session 增加 `chat_status=human|agent` 交互控制投影；它不替代 Session 生命周期或 Turn 执行状态。

## 2026-08-17 · Turn 软中断

- 增加查询当前 running Turn 的领域能力，作为中断前的 Session 归属与执行状态事实。
- 用户中断不创建新的持久化实体，也不直接把 Turn 改为终态；Runtime 确认取消后，由既有执行流提交 `interrupted`。
- `interrupted` 与 `completed`、`failed` 同为不可互相覆盖的终态；中断前已提交的 Message、Universal Event 和 Tool Result Snapshot 全部保留。


### 2026-10-13 · P3 路由与执行归属对齐

Session Token 使用持久密钥版本和 MAC，浏览器 Cookie 不再作为会话执行凭证。Registration 保存 CAS 绑定和可恢复转移意图；执行所有权与 epoch 由 Agent 原子维护。固定版本历史上下文用于 queued 和后续 Turn，配置确认更新不改变已有 Session。历史读取走同范围事实端口，执行响应走实际 owner。
