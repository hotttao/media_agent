---
name: plugin_binding
description: Workspace 对受控 MCP Definition 的启用与配置
dependencies:
    - mcp_catalog
related_workflow:
    - agent_session_startup
    - plugin_binding_snapshot
    - external_mcp_oauth_proxy
---

## 一句话职责

Plugin Binding 负责 Workspace 启用了哪些已批准 MCP，以及其非敏感配置版本。

## 边界

### 做什么
- 启用、停用和配置 Workspace MCP Binding。
- 提供 Gateway 构建 MCP Snapshot 所需数据。
- 发布 Binding 版本变化。

### 不做什么
- Binding 不代表调用接口的授权结果。
- 不保存用户 OAuth Token，不进入平台内 MCP 请求路径。

## 能力清单

| 能力 | 说明 | 对外接口/函数 |
|:---|:---|:---|
| 查询绑定 | 返回 Workspace 的 MCP 启用关系 | `WorkspacePluginService.ListBindings` |
| 启停绑定 | 改变受控 Definition 的启用状态 | `Enable/Disable` |
| 更新配置 | 更新非敏感运行配置 | `UpdateConfiguration` |
| 获取快照输入 | 供 Gateway 批量读取绑定 | `PluginBindingInternalService.GetWorkspaceBindings` |

## 数据模型

### `workspace_plugin_bindings`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| id | text PK | Workspace Plugin Binding ID |
| workspace_id | text | 所属 Workspace |
| definition_id | text | 已批准 MCP Definition ID |
| enabled | boolean | 是否向 Workspace Agent 下发 |
| configuration | jsonb | 不含 Secret 的运行配置 |
| version | bigint | Binding 版本 |
| updated_by | text | 最近修改用户 ID |
| created_at | timestamptz | 创建时间 |
| updated_at | timestamptz | 最后更新时间 |

### UUID 生成规则

Binding 使用 ULID；每次变化递增 version 并发布 `PluginBindingChanged`。

## 能力详细说明

### Enable

1. 验证 Definition 当前 enabled，Workspace 授权由调用链完成。
2. 对配置应用 Definition schema，禁止写入 Token/Secret。
3. 原子 upsert Binding 与 Outbox 事件。
