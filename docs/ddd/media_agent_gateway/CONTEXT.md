# media_agent_gateway Agent 交互上下文

管理 Agent 的持久化会话、交互记录和执行任务，并提供 Platform MCP。它不拥有 Workspace 配置，也不是业务服务的通用代理。

## 业务术语词典

**Agent Session（Agent 会话）**:
一个用户在一个 Workspace 中与一个 Agent 持续交互的边界，固定绑定用户、Workspace 和 Agent。
_禁用替代词_：Web Session、Runtime Instance、Chat

**Default Agent Context（默认 Agent 上下文）**:
当用户尚未配置 Workspace 或 Agent 时，由内置 media_control mock 配置提供的可立即对话上下文；当前默认 Agent 为 Codex。它只用于创建 Agent Session，创建后 Session 仍固定绑定解析出的 Workspace 与 Agent。
_禁用替代词_：Anonymous Session、Unowned Workspace、Temporary Chat

**Session Token（会话令牌）**:
每个 Agent Session 唯一且稳定的 opaque 凭证，仅供 Agent 调用 Platform MCP 或平台内业务 MCP。
_禁用替代词_：Platform Token、Cookie、Access Token

**Turn（交互轮次）**:
Agent Session 内由一次用户输入触发并自然结束的一轮处理。
_禁用替代词_：Session、Agent Task

**Pending Message（待执行消息）**:
同一 Agent Session 已有 active Turn 时保存的唯一 follow-up 输入；它不是 Turn，仅在当前 Turn 自然完成后转换为下一 Turn。
_禁用替代词_：Concurrent Turn、Agent Task、Message Queue

**Message（消息）**:
Turn 中由用户、Agent 或工具产生的持久化内容。
_禁用替代词_：Event、Notification

**Universal Event（通用事件）**:
Agent 交互过程中可恢复、可排序的运行事件记录，用于流式回放和重连。
_禁用替代词_：Domain Event、Message

**Session Event Subscription（会话事件订阅）**:
客户端从指定 Universal Event sequence 开始订阅一个 Agent Session；先回放已提交事件，再持续接收新提交事件。订阅连接不是 Turn，也不拥有或取消 Runtime 执行。
_禁用替代词_：Turn Stream、Agent Connection、Web Session

**Agent Request（Agent 请求）**:
Agent 暂停执行并等待用户确认或补充输入的请求。
_禁用替代词_：HTTP Request、Agent Task

**Agent Task（Agent 任务）**:
需要异步调度 Agent 完成的一次持久化执行请求，拥有独立生命周期和幂等键。
_禁用替代词_：River Job、Schedule、Turn

**Batch Run（批量运行）**:
由多个 Agent Task 组成并统一跟踪进度与取消状态的批处理。
_禁用替代词_：Teamwork、Job Queue

**Teamwork Session Link（协作会话关联）**:
Teamwork 拓扑版本与实际 Agent Session 之间的运行期关联。
_禁用替代词_：Teamwork Topology、Team Membership

**Platform MCP（平台 MCP）**:
Gateway 内置、通过 Session Token 调用的平台级工具集合。
_禁用替代词_：Business MCP、External MCP

**Session Analytics（会话分析）**:
由 Agent Session、Turn 和 Task 事实派生的近期会话、活动与 Usage 查询投影。
_禁用替代词_：Billing Ledger、Audit Log
