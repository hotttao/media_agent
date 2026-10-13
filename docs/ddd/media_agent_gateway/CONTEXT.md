# media_agent_gateway Agent 接入上下文

管理 Agent Session Registration、Web 用户访问、执行任务接入、MCP 服务注册目录和统一 MCP 入口，并把 Agent 命令、查询及事件适配为客户端 API/SSE。它不拥有 Runtime 执行事实，也不决定同一 Session 的 Turn 顺序。

## 业务术语词典

**Platform Prompt（平台提示词）**:
平台管理的运行指导正文，说明平台能力、使用约定及已授权能力的上下文；它与用户配置的角色或任务提示词是不同内容。
_禁用替代词_：Configured Prompt、User Message、Final Prompt

**Session Replica Binding（会话副本绑定）**:
Agent Session 在所属 Workspace 内选定的运行副本引用；后续调用沿用该引用，副本不可用或进入排空时可重新分配。
_禁用替代词_：Placement、Agent Identity、Runtime Handle

**Draining Replica（排空副本）**:
为缩容而停止接收新会话及新执行分配、等待已有执行结束的运行副本。
_禁用替代词_：Deleted Session、Interrupted Turn、Offline Replica

**MCP Definition（MCP 定义）**:
由平台运维登记的 MCP 服务描述，包含稳定标识、服务身份、Endpoint 和认证类型。
_禁用替代词_：Plugin Instance、MCP Connection、Tool List

**MCP Gateway（MCP 网关）**:
Agent 访问平台工具的统一 MCP 入口，向调用方呈现有权访问的工具，并将调用路由到具体 MCP 实现。
_禁用替代词_：Agent Runtime、Plugin Implementation、MCP Catalog

**Agent Instance Registration（Agent 实例注册）**:
Gateway 持有的稳定副本与当前运行代次、所属 Workspace/Agent 配置及通信身份之间的受控关联。
_禁用替代词_：Agent Identity、Agent Session Registration、Pod Health

**Agent Session（Agent 会话）**:
一个用户在一个 Workspace 中与一个 Agent 持续交互的跨上下文稳定标识；Gateway 与 Agent 分别维护自身模型，不共享数据表。
_禁用替代词_：Web Session、Runtime Process、Chat

**Agent Session Registration（Agent 会话注册）**:
Gateway 拥有的 Session 身份、用户/Workspace/Agent 绑定、访问凭证和 UI 元数据；不包含 Turn、Message 或 Universal Event。
_禁用替代词_：Runtime Session、Turn Store、Chat History

**Default Agent Context（默认 Agent 上下文）**:
当用户尚未配置 Workspace 或 Agent 时，由内置 media_control mock 配置提供的可立即对话上下文；只用于创建 Agent Session Registration 和对应 Runtime Session。
_禁用替代词_：Anonymous Session、Unowned Workspace、Temporary Chat

**Session Token（会话令牌）**:
每个 Agent Session 唯一且稳定的 opaque 凭证，仅供该会话调用 Platform MCP 或平台内业务 MCP；它绑定 Session，不代表 Agent 调用通用内部 API，也不替代创建 Agent 时签发的 Agent API Token。
_禁用替代词_：Platform Token、Cookie、Access Token

**Agent API Client（Agent API 客户端）**:
以 `Agent:<agent_id>` 作为 Delegated Actor、代表其 owner User 调用平台内部服务的客户端形态。创建时 Gateway 请求 media_auth 签发绑定 Agent 的 API Token；运行时 Agent 只调用 Gateway，由 Gateway 验证 Token、校验委托关系并代理内部服务。
_禁用替代词_：Agent Session、Personal Worker、Service Account

**Agent Delegation（Agent 委托）**:
Agent 代表 owner User 发起请求的关系。有效 Internal Identity JWT 使用 `sub=User:<id>` 与 `act.sub=Agent:<id>`；Agent 初期继承 User 权限上限，但仍受 API Token scope、Organization 和资源服务当前授权约束。
_禁用替代词_：Impersonation、User Token、Session Ownership

**Session Context Version（会话上下文版本）**:
Agent Session Registration 创建时固定引用的运行配置版本；Workspace 后续切换 Agent 或更新凭证不会原地改变该版本，继续使用新配置必须创建新的 Agent Session。
_禁用替代词_：Workspace Version、Current Agent Version、Mutable Session Config

**Session Event Subscription（会话事件订阅）**:
客户端从指定 Session Sequence 开始订阅一个 Agent Session；Gateway 将会话事件分发给本机订阅者，并通过 Agent 查询接口回放已提交事件、转换为 SSE。
_禁用替代词_：Turn Stream、Runtime Connection、Web Session

**Event Replay Query（事件回放查询）**:
Gateway 通过 Agent 服务接口按 `session_id + after_sequence` 读取已提交 Universal Event 的只读操作；Gateway 不直连 Agent 数据库。
_禁用替代词_：Database Sharing、Redis Replay、SSE Resume

**Agent Task（Agent 任务）**:
需要异步调度 Agent 完成的一次持久化执行请求，拥有独立生命周期和幂等键；执行时向 Agent 提交 Turn。
_禁用替代词_：Queued Turn、River Job、Schedule

**Batch Run（批量运行）**:
由多个 Agent Task 组成并统一跟踪进度与取消状态的批处理。
_禁用替代词_：Teamwork、Turn Queue

**Teamwork Session Link（协作会话关联）**:
Teamwork 拓扑版本与实际 Agent Session 之间的运行期关联。
_禁用替代词_：Teamwork Topology、Team Membership

**Allowed Topology Version（允许拓扑版本）**:
Gateway 已确认可供新 Agent Task 使用的 Teamwork Topology 最新版本；旧版本只影响新任务准入，不中断已经开始的 Turn。
_禁用替代词_：Runtime Context、Topology Snapshot、Current Turn Version

**Platform MCP（平台 MCP）**:
Gateway 内置、通过 Session Token 调用的平台级工具集合。
_禁用替代词_：Business MCP、External MCP

**Session Analytics（会话分析）**:
由 Session Registration、Agent Turn 终态和 Agent Task 事实派生的近期会话、活动与 Usage 查询投影。
_禁用替代词_：Billing Ledger、Audit Log

Turn、Queued Turn、Message、Universal Event、Live Event Frame、Session Sequence 和 Chat Status 使用 [media_agents 统一语言](../media_agents/CONTEXT.md)，其事实所有者均为 Agent。
