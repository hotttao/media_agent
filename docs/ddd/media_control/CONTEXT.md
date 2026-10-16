# media_control 控制上下文

管理 Workspace 及 Agent 的期望配置，协调运行环境、计划任务和 Teamwork 拓扑。它是控制面事实源，不承载 Agent 交互或媒体数据面流量。

## 业务术语词典

**Workspace（工作空间）**:
企业部门开展 Agent 工作的业务资源与运行数据隔离边界；每个 Workspace 只属于一个 Team，一个 Team 可创建多个 Workspace，业务资源通过 `workspace_id` 归属其中。
_禁用替代词_：Organization、Team、Project、Environment、Worker Space

**Runtime Environment（运行环境）**:
可承载 Agent Runtime 的执行环境定义，包含连接、容量和可用状态。
_禁用替代词_：Workspace、Runner

**Runtime Controller（运行控制器）**:
根据 Agent 资源声明创建承载资源的环境实现；Kubernetes 中为创建 StatefulSet 等资源的自定义控制器，Docker 中为具有 Docker 权限的容器创建服务。
_禁用替代词_：Agent Gateway、Agent Runtime、Placement

**Workspace Runtime Directory（工作空间运行目录）**:
由单个 Workspace 独占、跨 Agent 容器重启保留的文件状态边界；同一 Workspace 的 Agent Runtime 在容器内统一通过 `/workspace` 访问。
_禁用替代词_：Shared Volume、Agent Session Store、UI Profile

**Placement（运行放置）**:
Workspace 到 Runtime Environment 的当前调度结果。
_禁用替代词_：Deployment、Assignment

**Agent Configuration（Agent 配置）**:
Workspace 期望的 Agent 模型、工具、Prompt、Skill、Memory 和运行参数集合。
_禁用替代词_：Runtime State、Session Config

**Agent Type（Agent 类型）**:
决定 Workspace 使用哪一种 Agent Runtime 实现的受控枚举；当前支持 Codex、Claude Code 与 Goose。
_禁用替代词_：Model、Provider、Agent ID

**Agent Identity（Agent 身份）**:
由一个 Agent Configuration 代表的稳定委托身份，属于该配置的 Workspace 和创建者 User；不同配置即使采用相同 Agent Type 也具有不同身份。
_禁用替代词_：Agent Type、Agent Session、Provider API Key

**Model Invocation Settings（模型调用设置）**:
Agent Configuration 中选定的模型API协议、模型名称与服务地址，是随会话固定的调用目标设置，不代表模型供应商目录或认证秘密。
_禁用替代词_：Provider、Model Connection、API Key

**Provider API Key（Provider API 密钥）**:
Workspace 为当前 Agent Configuration 提交、由控制面加密托管并仅在运行调用链中解密传递的模型服务秘密。
_禁用替代词_：Platform Token、Credential Reference、Session Token

**Metadata Update（元数据更新）**:
用户明确确认后，将选定的 Prompt、Skill、Memory 或工具配置版本发布为 Agent 可使用运行配置的变更。
_禁用替代词_：Catalog Edit、Automatic Upgrade、Turn Interrupt

**Metadata Release（元数据发布版本）**:
用户明确确认的完整元数据选择所形成的不可变配置，保存当次资源版本与内容引用，供实例加载及 Session 固定引用。
_禁用替代词_：Catalog Latest、Mutable Configuration、Agent State

**Metadata Selection Revision（元数据选择序号）**:
同一 Agent Configuration 中用户已确认的元数据选择的先后次序；显式选择旧资源版本也形成一次新的选择。
_禁用替代词_：Resource Version、Session Sequence、Event ID

**Metadata Application Projection（元数据应用投影）**:
Control 根据各 Agent 运行实例的上报形成的元数据更新结果视图；它与用户已选配置、Session 固定版本是不同事实。
_禁用替代词_：RabbitMQ ACK、Desired Configuration、Turn Status

**Configured Prompt（配置提示词）**:
用户在控制面选择或编辑、用于表达 Agent 角色和任务要求的版本化提示词内容。
_禁用替代词_：Platform Prompt、User Message

**Agent Catalog Item（Agent 目录项）**:
可被 Agent Configuration 引用的 Prompt、Agent Template、Skill、Provider、Credential Reference 或 Memory Store 定义。
_禁用替代词_：Plugin、MCP Definition

**Agent Resource Library（Agent 资源库）**:
企业内管理和共享 Prompt、Skill 的资源集合；资源具有独立所有者与资源权限，可被同一企业的多个 Workspace 引用。
_禁用替代词_：Workspace、Platform Prompt、Plugin Catalog

**Skill Draft（Skill 草稿）**:
Skill 尚未发布的可编辑文件集合，包含入口描述以及脚本、参考资料等资源，不作为 Agent 可选的运行版本。
_禁用替代词_：Skill Version、Metadata Release、Runtime Directory

**Skill Version（Skill 发布版本）**:
Skill 一次发布形成的不可变文件集合，具有稳定版本标识，可独立查看、下载并由 Agent Configuration 明确选择。
_禁用替代词_：Skill Draft、Metadata Selection Revision、Session Version

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

**Agent Workload Target（Agent 工作负载目标）**：
Control 保存的 Agent 承载资源期望，关联配置与 Placement，并确定目标副本数和资源配置；与环境实际资源及执行实例状态分别建模。
_禁用替代词_：Pod 状态、Session Binding、Gateway 配置缓存

**Model Connection（个人模型连接）**:
用户拥有的可复用模型认证配置，Workspace 通过明确选择引用。
_禁用替代词_：企业资源、CredentialReference、模型本身。
