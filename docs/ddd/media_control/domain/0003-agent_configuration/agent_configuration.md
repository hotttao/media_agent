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
---

## 一句话职责

Agent Configuration 负责 Workspace 中 Agent 的期望配置，并从控制面事实组装 Runtime Context。

## 边界

### 做什么
- 管理模型、工具、Prompt、Skill、Memory 和运行参数引用。
- 管理受控 Agent Type 与加密的 Provider API Key。
- 校验所有引用存在且允许用于当前 Workspace。
- 生成不可变、带版本的 Runtime Context。

### 不做什么
- 不保存 Agent Runtime 的当前内存状态。
- 不管理 MCP Binding；该配置归 `media_plugin`，由 Gateway 合并。

## 能力清单

| 能力 | 说明 | 对外接口/函数 |
|:---|:---|:---|
| 读取配置 | 获取 Agent 当前期望配置 | `AgentConfigurationService.GetConfig` |
| 更新配置 | 原子替换配置并递增版本；API Key 留空表示保留 | `UpdateConfig` |
| 管理表现配置 | 维护 Commands、Profile 和 Layout | `UpdateCommands/UpdateProfile/UpdateLayout` |
| 生成运行上下文 | 汇总配置、目录资源和 Placement | `GetRuntimeContext` |

## 数据模型

### `agent_configurations`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| id | text PK | Agent Configuration ID |
| workspace_id | text | 所属 Workspace |
| agent_id | text | Agent 稳定标识，Workspace 内唯一 |
| agent_type | text | `codex` 或 `claude-code` |
| encrypted_api_key | bytea nullable | Provider API Key 的认证加密密文 |
| configuration | jsonb | 模型、工具及 Catalog 资源 ID 快照 |
| version | bigint | 乐观锁和配置版本 |
| created_at | timestamptz | 创建时间 |
| updated_at | timestamptz | 最后更新时间 |

### UUID 生成规则

`agent_config_id = ULID()`；`(workspace_id, agent_id)` 唯一。

## 聚合与不变量

`AgentConfiguration` 是聚合根，Workspace 只保存其 ID。一个 Workspace 当前只有一个活动配置：

- `agent_type` 只能为 `codex` 或 `claude-code`，`agent_id` 与当前 `agent_type` 相同；
- 创建时 API Key 可空；更新时空字符串表示保留原密文，非空值必须重新认证加密；
- 公开读模型只能暴露 `api_key_configured`，任何响应、日志和事件不得包含密钥；
- Agent Type 或 API Key 实际改变时递增配置版本；既有 Session 固定原 Agent Type，新 Session 使用新配置；
- 切换 Agent Type 复用 Workspace Runtime Directory，但两个 Runtime 不得同时写入。

## 能力详细说明

### UpdateConfig

1. 校验 Workspace 可写和 expected_version。
2. 批量解析 Catalog 引用并拒绝失效或越权引用。
3. 写入新配置版本与 `AgentConfigChanged` Outbox 事件。

当前 Workspace 创建/更新是 Workspace 与 Agent Configuration 的同库事务；Runtime Context 的解密读取是
只读应用能力，不改变聚合。
