# media_agents Agent 执行上下文

管理 Agent Runtime Session、Turn 串行调度、模型与工具执行，以及执行产生的可恢复交互事实。它不负责 Web 用户鉴权、页面协议适配或 Workspace 期望配置。

## 业务术语词典

**Agent Runtime Instance（Agent 执行实例）**:
承载 Agent 执行的一份可替换运行实例；它与稳定 Agent Identity、Runtime Session 及底层 Runtime Handle 是不同概念。
_禁用替代词_：Agent Identity、Agent Session、Runtime Handle

**Runtime Incarnation（运行代次）**:
同一稳定运行副本每次进程启动形成的一次运行身份，用于区分重启前后的执行与观测。
_禁用替代词_：Replica ID、Agent Identity、Runtime Handle

**Runtime Session（运行会话）**:
Agent 内与一个 Agent Session ID 对应的持久化执行边界，拥有 Turn 队列、交互记录和事件游标。
_禁用替代词_：Web Session、Gateway Session、Process

**Runtime Handle（运行时句柄）**:
底层 Agent Runtime 创建并返回、由 Runtime Session 持久化用于后续 Turn 加载和重启恢复的不透明会话标识；它与业务 Session ID 属于不同命名空间且不包含运行凭据。
_禁用替代词_：Runtime Session、API Key、Process ID

**Execution Ownership（执行所有权）**:
Runtime Session 当前允许执行和提交事实的运行实例及其代次；会话副本绑定只定位请求，不能替代执行所有权。
_禁用替代词_：Session Replica Binding、User Ownership、Gateway Lock

**Execution Epoch（执行代次）**:
同一 Runtime Session 执行归属的递增版本，与副本进程启动代次分别标识。
_禁用替代词_：Incarnation、配置版本、事件序号

**Session Handoff（会话移交）**:
一个 Runtime Session 从当前执行归属转交给另一执行实例的稳定操作。
_禁用替代词_：Instance Drain、Turn Retry、配置升级

**Runtime Metadata Application（运行元数据应用）**:
一个运行实例对用户已确认配置的准备、发布与结果记录；它不修改已有 Session 的固定配置。
_禁用替代词_：Metadata Selection、Turn、RabbitMQ ACK

**Turn（交互轮次）**:
Runtime Session 内由一次用户输入触发的一轮持久化执行请求，以 completed、failed 或 interrupted 之一结束。
_禁用替代词_：Message、Agent Task、Job

**Queued Turn（排队轮次）**:
已经被 Agent 接受并持久化、等待同一 Runtime Session 当前 Turn 结束后执行的 Turn。
_禁用替代词_：Pending Message、Concurrent Turn、In-memory Job

**Session Turn Queue（会话轮次队列）**:
同一 Runtime Session 内按 enqueue sequence 排列的持久化 FIFO Turn 集合；任一时刻最多一个 Turn 可以 running。
_禁用替代词_：Global Queue、Pending Message Slot、Gateway Queue

**Message（消息）**:
Turn 中由用户、Agent 或工具产生、面向历史查询的持久化交互内容。
_禁用替代词_：Universal Event、Live Event Frame、Notification

**Live Event Frame（实时事件帧）**:
Turn 执行期间产生、通过 Session Event Bus 即时呈现的临时传输帧；它可以不持久化，也不推进恢复游标。
_禁用替代词_：Universal Event、Message、Domain Event

**Universal Event（通用事件）**:
Agent 已持久化并分配 Runtime Session sequence 的权威事件快照，用于历史恢复、重连和订阅去重。
_禁用替代词_：Live Event Frame、Message、Broker Message

**Session Sequence（会话事件序号）**:
由 Agent 在单个 Runtime Session 内为每个 Universal Event 原子分配的严格递增恢复游标。
_禁用替代词_：Kafka Offset、Redis ID、Message Sequence

**Message Sequence（消息序号）**:
由 Runtime Session 为历史 Message 原子分配的严格递增查询次序；它不承担 Universal Event 的断线恢复游标语义。
_禁用替代词_：Session Sequence、Broker Offset、Turn Sequence

**Session Event Bus（会话事件总线）**:
把 Live Event Frame 和已提交 Universal Event 从 Agent 低延迟扇出到 Gateway 的瞬时通知通道；它不是事实源。
_禁用替代词_：Event Store、Turn Queue、SSE Connection

**Chat Status（交互控制状态）**:
Runtime Session 当前由 human 还是 agent 掌控的可查询投影；Turn 状态仍是执行事实来源。
_禁用替代词_：Runtime Session Status、Turn Status、User Presence

**Turn Interrupt（轮次中断）**:
停止 Runtime Session 当前 running Turn 的软中断命令；它不删除排队 Turn，也不回滚已经发生的工具副作用。
_禁用替代词_：Session Delete、Rollback、Queue Clear
