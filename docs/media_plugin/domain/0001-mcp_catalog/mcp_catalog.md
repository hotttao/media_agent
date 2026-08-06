---
name: mcp_catalog
description: 平台批准 MCP 服务的受控目录
dependencies: []
related_workflow:
    - agent_session_startup
    - plugin_binding_snapshot
---

## 一句话职责

MCP Catalog 负责登记可被 Workspace 启用的受控 MCP Definition。

## 边界

### 做什么
- 由管理入口登记、启停和查询 MCP Definition。
- 固定服务身份、Endpoint、Transport 与认证类型。

### 不做什么
- 不接受普通用户登记任意 URL 或上传代码。
- 不复制 MCP Tool 列表，Tool 由 MCP 协议发现。

## 能力清单

| 能力 | 说明 | 对外接口/函数 |
|:---|:---|:---|
| 查询目录 | 列出和读取已批准定义 | `McpCatalogService.ListDefinitions/GetDefinition` |
| 登记定义 | 管理员新增或更新受控定义 | `McpCatalogAdminService.RegisterDefinition` |
| 改变状态 | 启用、禁用或退役定义 | `UpdateDefinitionStatus` |

## 数据模型

### `mcp_definitions`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| id | text PK | 仓库配置中的稳定 Definition slug |
| service_name | text | 部署服务身份 |
| endpoint | text | MCP 内部或外部地址 |
| transport | text | streamable_http/sse 等 Transport |
| authentication_type | text | session_token/oauth/none |
| status | text | enabled/disabled/retired |
| version | bigint | Definition 版本 |
| created_at | timestamptz | 创建时间 |
| updated_at | timestamptz | 最后更新时间 |

### UUID 生成规则

Definition ID 使用仓库配置中的稳定 slug；数据库记录使用该 ID，不自动生成另一个公开标识。

## 能力详细说明

### RegisterDefinition

1. 只允许平台管理身份调用。
2. 校验 Endpoint 属于部署允许范围，认证类型在闭集合内。
3. 写入版本并发布 Catalog 变更事件。
