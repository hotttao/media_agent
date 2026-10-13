---
name: agent_configuration
description: Workspace 的 Agent 期望配置与运行快照组装
dependencies:
    - workspace
    - agent_catalog
    - runtime_environment
related_workflow:
    - agent_session_startup
    - runtime_context_assembly
    - agent_metadata_update
---

## 一句话职责

Agent Configuration 负责 Workspace 中 Agent 的期望配置，并从控制面事实组装 Runtime Context。

## 边界

### 做什么
- 管理模型、工具、Prompt、Skill、Memory 和运行参数引用。
- 管理受控 Agent Type 与加密的 Provider API Key。
- 校验所有引用存在且允许用于当前 Workspace。
- 生成不可变、带版本的 Runtime Context。
- 保存用户明确确认的元数据版本选择，目录资源编辑不自动推进运行选择。

### 不做什么
- 不保存 Agent Runtime 的当前内存状态。
- 不管理 MCP Binding；该配置归 `media_plugin`，由 Gateway 合并。

## 能力清单

| 能力 | 说明 | 对外接口/函数 |
|:---|:---|:---|
| 读取配置 | 获取 Agent 当前期望配置 | `AgentConfigurationService.GetConfig` |
| 更新配置 | 原子替换配置并递增版本；API Key 留空表示保留 | `UpdateConfig` |
| 管理表现配置 | 维护 Commands、Profile 和 Layout | `UpdateCommands/UpdateProfile/UpdateLayout` |
| 确认元数据更新 | 校验明确选定资源版本，提交不可变发布及通知意图 | ConfirmMetadataSelection（核心接口规格） |
| 接收应用结果 | 按选择、实例代次保存结果投影 | ReportMetadataApplication（核心接口规格） |
| 生成运行上下文 | 汇总配置、目录资源和 Placement | `GetRuntimeContext` |
| 解析委托身份 | 只读返回稳定 Agent 身份及真实 Workspace、owner、企业和部门归属，不返回 Provider 秘密 | `ResolveAgentIdentity` |

## 数据模型

### `agent_configurations`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| id | text PK | Agent Configuration ID |
| workspace_id | text | 所属 Workspace |
| agent_id | text | 历史运行时类型标签，与 agent_type 相同，不是跨服务委托身份 |
| agent_type | text | `codex`、`claude-code` 或 `goose` |
| encrypted_api_key | bytea nullable | Provider API Key 的认证加密密文 |
| configuration | jsonb | 模型、工具及 Catalog 资源 ID 快照 |
| version | bigint | 乐观锁和配置版本 |
| created_at | timestamptz | 创建时间 |
| updated_at | timestamptz | 最后更新时间 |

### UUID 生成规则

`agent_config_id = ULID()`；`(workspace_id, agent_id)` 唯一。

## 聚合与不变量

`AgentConfiguration` 是聚合根，Workspace 只保存其 ID。一个 Workspace 当前只有一个活动配置：

- `agent_type` 只能为 `codex`、`claude-code` 或 `goose`，`agent_id` 与当前 `agent_type` 相同；
- 创建时 API Key 可空；更新时空字符串表示保留原密文，非空值必须重新认证加密；
- 公开读模型只能暴露 `api_key_configured`，任何响应、日志和事件不得包含密钥；
- Agent Type 或 API Key 实际改变时递增配置版本；既有 Session 固定原 Agent Type，新 Session 使用新配置；
- 切换 Agent Type 复用 Workspace Runtime Directory，但两个 Runtime 不得同时写入。

### 稳定委托身份

不存在或已删除的配置/Workspace 返回404；已读取但字段缺失、配置ID不匹配或未知Workspace状态返回503，不能冒充不存在。Auth 后台以明确404作为撤销证据，因此来源不确定必须与确定失效区分；授权查询故障同样返回503。

`Agent:<agent_configurations.id>` 是 API Token 的 Actor；不得使用类型标签 `agent_id`、Workspace ID、Session ID 或 Token ID。类型切换和配置版本递增不改变配置记录 ID。该事实由 Control 持有，Auth 不复制 Agent 配置表，也不信任签发请求提交的 owner/企业。

内部身份解析仅返回配置 ID、Workspace ID、Workspace 的 owner/Organization/Team、desired_state 及当前 owner 的部门可见权限。身份解析是只读投影，不解密 Provider API Key、不获取 Runtime Context，也不签发 Token。已删除 Workspace 或未知配置返回 NotFound；上游 Team 授权服务错误返回不可用，不能当作授权成功。stopped 只表示运行环境停止，不等于身份删除；是否可执行业务仍由目标资源的 Workspace/action 授权决定。独立 Agent disabled 及 owner 解绑的撤销编排仍须在生命周期入口落实。

## 能力详细说明

### UpdateConfig

1. 校验 Workspace 可写和 expected_version。
2. 批量解析 Catalog 引用并拒绝失效或越权引用。
3. 写入新配置版本与 `AgentConfigChanged` Outbox 事件。

当前 Workspace 创建/更新是 Workspace 与 Agent Configuration 的同库事务；Runtime Context 的解密读取是
只读应用能力，不改变聚合。


### ConfirmMetadataUpdate

Control 设置明确提交选定资源版本，校验 Workspace manage、稳定幂等键和 expected_version。选择作为 Agent Configuration 事实保存，Catalog 编辑不自动推进运行配置。每次实际选择变化递增 version，以 selection_revision 表达次序；用户明确选择旧资源版本仍是一项新选择。

发布通过 [agent_metadata_update](../../../workflow/0010-agent-metadata-update/agent_metadata_update.md) 通知实例，Agent 经 Gateway 拉取，Gateway 不缓存配置。实例刷新不修改已有 Session、queued/running Turn 的固定配置；Skill 版本化/旧 Session 隔离为系统必须满足的契约，不建立可选能力登记。

### Metadata Application Projection

Control 接收 Agent 经 Gateway API 的结果上报，只保存观测；Gateway 核验 Workspace/config/副本及当前代次。投影按 workspace/config/selection_revision/replica/incarnation 分区。相同 report_id 同载荷幂等，载荷冲突拒绝；旧代次/旧选择不覆盖当前视图。applied 后晚到失败不撤销成功，失败可被后续成功收敛。

当前选择与当前目标实例集合决定用户汇总，未上报不等于成功，空集合显示等待运行。新增/替代实例不能继承旧进程结果，排空实例从目标集合退出由 Gateway 工作负载目标确定。

### 不可变 Metadata Release

Release 独立保存用户明确确认的非秘密完整配置及版本引用，发布后不修改。准备外部 Skill 内容在事务外完成并校验源版本，再提交 release、当前选择指针/版本与通知 Outbox；失败不发布半成品。

| 持久化对象 | 关键字段/约束 | 事实归属 |
|:---|:---|:---|
| agent_metadata_releases | workspace/config/context_version、selection_revision、资源版本向量、非秘密 manifest、创建者、幂等键 | Control；唯一 config/selection 和 config/idempotency |
| metadata_notification_outbox | event_id/source、scope、payload、交付状态 | Control 本地通知意图，不是 Workflow 独有事实表 |
| metadata_application_reports | report_id、scope、replica/incarnation、selection/target、status/error | Control 观测投影；同 ID 不同载荷冲突 |
| catalog_resource_versions | kind/id/version、内容/不可变引用、checksum | Catalog 聚合历史；当前行与历史同事务维护 |

Secret 不进入 manifest，凭据按受控加密版本引用执行时取得。已被 Session/release 使用的版本不可按实例 current 指针回收；当前资源授权撤销仍阻止调用。


### 平台模板引用

用户确认的不可变 Metadata Release 固定 Gateway 平台模板版本引用，Control 不渲染或拥有模板正文。已有缺失引用的 release 对应版本1；新确认 release 选择版本2。模板发布本身不自动变更已确认 release、既有 Session 或 queued Turn。
