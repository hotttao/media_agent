# media_control 控制上下文

管理 Workspace 及 Agent 的期望配置，协调运行环境、计划任务和 Teamwork 拓扑。它是控制面事实源，不承载 Agent 交互或媒体数据面流量。

## 业务术语词典

**Workspace（工作空间）**:
用户开展 Agent 工作的租户边界，持有 owner、状态和控制配置；业务资源通过 `workspace_id` 归属其中。
_禁用替代词_：Tenant、Project、Environment

**Runtime Environment（运行环境）**:
可承载 Agent Runtime 的执行环境定义，包含连接、容量和可用状态。
_禁用替代词_：Workspace、Runner

**Placement（运行放置）**:
Workspace 到 Runtime Environment 的当前调度结果。
_禁用替代词_：Deployment、Assignment

**Agent Configuration（Agent 配置）**:
Workspace 期望的 Agent 模型、工具、Prompt、Skill、Memory 和运行参数集合。
_禁用替代词_：Runtime State、Session Config

**Agent Catalog Item（Agent 目录项）**:
可被 Agent Configuration 引用的 Prompt、Agent Template、Skill、Provider、Credential Reference 或 Memory Store 定义。
_禁用替代词_：Plugin、MCP Definition

**Agent Schedule（Agent 计划）**:
按时间规则代表指定用户在指定 Workspace 发起 Agent Task 的长期离线委托。
_禁用替代词_：River Job、Cron Job、Agent Task

**Schedule Trigger（计划触发）**:
Agent Schedule 的一次幂等触发记录，是创建 Agent Task 前的控制面事实。
_禁用替代词_：Execution、River Job

**Teamwork Topology（协作拓扑）**:
描述多个 Agent 成员及其协作关系的版本化期望配置。
_禁用替代词_：Team、Team Membership、Runtime Session

**AFS Binding（AFS 绑定）**:
Workspace 或 Agent 配置对外部 AFS 能力的引用及平台侧状态投影。
_禁用替代词_：AFS Object、AFS Runtime

**Runtime Context（运行上下文）**:
Control 根据 Workspace、Agent Configuration、Placement 和 Teamwork Topology 生成的只读配置快照。
_禁用替代词_：Agent Session、MCP Snapshot

**Workspace Metadata（工作空间元数据）**:
附着于 Workspace 资源的 Tag、Share Metadata 与 Public Export Metadata；资源本身及分享凭证仍归资源所有服务。
_禁用替代词_：Resource ACL、Public Token

**Platform Administration（平台管理）**:
管理员维护的平台设置，以及 Cluster 与平台统计的只读投影。
_禁用替代词_：Workspace Configuration、Runtime Environment
