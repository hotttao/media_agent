# ui_platform 与 agent-platform 源码对照

2026-11-10。范围：先对齐平台自身的 UI 实现，再提取应用 UI SDK。此记录是源码审查，不代表已经运行浏览器验收。

## 结论

media_agent 的 `ui_platform` 已有 Shell/Slot、会话列表、聊天、Agent Request、SSE 回放、中断与暂停恢复；尚未形成 agent-platform 中“平台自身和嵌入应用共用同一个 session store、Transcript、Composer”的实现结构。当前 ChatPage 同时承担宿主组合、会话编排和事件投影。直接另写独立 UI SDK 会产生第二套状态机，应先完成平台内部收敛。

对齐目标是状态生命周期、共享组件和可注入端口，不是逐文件照搬。保留本仓库 L0/L1/L2/L3 架构、后端事实所有者及既有 Session Context Version / execution_epoch 规则。

## 证据与差距

| 能力 | agent-platform 实现 | 当前 ui_platform 实现 | 状态与处理 |
|---|---|---|---|
| Workspace 宿主与 Slot | `web/src/pages/WorkspacePage.tsx`、workspace 组件 | `src/pages/chat/index.tsx`、`foundation/providers/slot-provider.tsx`、WorkspaceLayout | partial：已有宿主抽象；本轮不重建 Shell，先减薄页面 |
| 平台与应用共享会话状态机 | `web/src/stores/agent-session-store.ts` 直接导出 UI SDK 的 createAgentSessionStore | ChatPage 内 useState/useRef、selectSession、send、applyEvent | missing：先在 L2 chat 内提取并由平台页面实际使用，验证后才提取 SDK |
| API / SSE / effects / drafts 注入 | `internal/ui-sdk/src/session/agent-session-store.ts` 的 AgentSessionDeps | chat-client 直接 fetch；页面编排、路由和状态互相引用 | partial：client 已分离；补消费侧端口，宿主提供导航、草稿和刷新回调 |
| 快速切会话 | shared store 管理切换生命周期 | requestVersion、activeSessionId、historyReadySessionId | partial：已有过时结果保护；统一多次加载、取消、错误路径及 A→B→A 场景验证 |
| 历史与实时增量投影 | shared store / to-chat-message / Transcript | 页面 itemMessageIds、itemTexts、seedCompletedItems、applyEvent | partial：提取消息投影规则；保护已持久化项去重与增量合并 |
| Session Event 恢复 | store 的 SSE 端口与恢复逻辑 | chat-client decodeSessionEventFrame、按 Session cursor、持续订阅及 500ms 重连 | partial：已有恢复，不得重写成只订阅当前 Turn；补连接错误分类及游标生命周期测试 |
| 后端发起的新 Turn | useRemoteTurnSync 按可见性和业务窗口轮询 | 当前订阅是持续 Session Event，不局限当前浏览器发起的 Turn | partial：先验证持续事件能否满足发现需求；不能因为参考有 5 秒轮询就重复加轮询 |
| Transcript / 工具呈现 | Transcript、ToolCallBlock、renderer registry、markdown 注入 | MessageBubble 对 content_json 提取纯文本，tool 只区别样式 | partial：补结构化消息与工具呈现契约，再对齐可复用组件；工具结果不是业务 Job 状态 |
| Composer / 草稿 | 受控 Composer、DraftStore、平台 preset | ChatApp 本地 input；提交前立即清空 | partial：先明确失败保留和按 Session 隔离，再做受控组件；不能丢失被拒绝的输入 |
| Agent Request | AskUserQuestionPanel 与 store request 流程 | 独立 AgentRequestCard、DTO 解码、resolve，已有组件测试 | partial：复用现有能力；保持 permission/confirm/input 与服务端 JSON 契约，不硬套旧 respond 接口 |
| 中断与暂停恢复 | 平台 store 的执行控制 | interrupting、resuming、execution_epoch、稳定 resume operation_id | partial：提取时保留当前更明确的 paused/resume 规则；中断不等于取消显示流 |
| 排队输入 / 文件 / 删除 / commands / skills | 参考 store 和 Composer 提供多类能力 | 应用 Relay 只有当前已定义的固定端点 | missing/协议待核实：先核对平台公开 API、权限及持久化语义，不能在 UI 模拟成功或先显示无后端能力的入口 |
| 身份/Workspace 隔离 | store/context 生命周期与 destroy | Workspace 路由、Auth Provider、组件 effect cleanup | partial：增加退出、切身份、Workspace 切换的生命周期验证；游标与消息不能跨边界复用 |

表中 missing 表示没有相应共享抽象或公开协议；partial 不表示已确认存在运行缺陷，尚需测试定位。

## 当前代码中优先验证的位置

1. `ChatApp.submit` 在等待 send 前清空 input，失败时仅撤销乐观消息并显示错误，需明确并补齐草稿恢复。
2. `selectSession` 使用列表缓存的 chat_status，并与 URL 变化触发的加载 effect 并行，需统一详情/历史加载和过时结果保护。
3. SSE 订阅错误后统一 500ms 重连，需要区分认证失效、权限拒绝与临时网络故障。
4. 页面聚合多种状态及异步流程，先提取到 L2 并保持行为，不另起未接入页面的状态机。

## 改造顺序

P0：确认本对照范围和能力优先级。

P1：在 ui_platform 内收敛会话运行模型。`domains/chat/types.ts` 维护契约，`domains/chat/lib/` 放 session store、事件投影与 API/SSE 适配，`domains/chat/hooks/` 放 React 订阅。页面只装配 Workspace、身份、URL、Slot 和 effects。先保留现有外观与 API，并用状态机测试验证切换、恢复、发送失败、请求回应、中断和 resume。

P2：平台页面实际接入共享状态与消息/输入组件，对齐 Transcript、Composer、工具与请求呈现。按 UI 四层规则测试宿主组合，补浏览器刷新、切换和权限异常场景。

P3：从平台已经使用并通过验收的实现中提取 packages/platform-ui-sdk；平台和应用共用其核心。分别注入平台 API adapter 与 Go Relay adapter，不共享凭证或 URL 全局配置。

P4：应用接入及后续能力扩展。pending/files 等能力先按后端协议与权限建模补齐，再进入 UI，不把参考的旧协议当成本项目已有能力。

## 本次工作区处理

此前刚创建但未完成的新 UI SDK 草稿已移出工作区暂存；未修改平台页面。已提交并推送的应用认证、Go/JS Server SDK 不受影响。UI 改造按本对照重新起步。
