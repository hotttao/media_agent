# media_agent_gateway Agent 交互上下文

管理 Agent 的持久化会话、交互记录和执行任务，并提供 Platform MCP。它不拥有 Workspace 配置，也不是业务服务的通用代理。

## 业务术语词典

**Agent Session（Agent 会话）**:
一个用户在一个 Workspace 中与一个 Agent 持续交互的边界，固定绑定用户、Workspace 和 Agent。
_禁用替代词_：Web Session、Runtime Instance、Chat

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
