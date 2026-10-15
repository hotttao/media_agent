---
name: mcp_catalog
description: 平台批准 MCP 服务的受控目录
dependencies: []
related_workflow:
    - agent_session_startup
    - media_plugin/plugin_binding_snapshot
    - mcp_catalog_publication
---

## 一句话职责

Gateway 的 MCP Catalog 负责登记可被 Workspace 启用的受控 MCP Definition，是 MCP 服务目录的唯一事实源。

## 边界

### 做什么
- 由管理入口登记、启停和查询 MCP Definition。
- 平台管理员维护平台内置 MCP；企业管理员可登记并管理本企业的外部 MCP，企业归属不可变。
- 固定服务身份、Endpoint、Transport 与认证类型。

### 不做什么
- 不接受普通用户登记任意 URL 或上传代码。
- 不拥有具体 Plugin 的工具实现、Workspace 启用关系或用户 OAuth 凭据。
- 不把 Tool 列表作为目录事实；工具描述由 MCP 协议发现，发现缓存是可重建读模型。

## 能力清单

| 能力 | 说明 | 对外接口/函数 |
|:---|:---|:---|
| 查询目录 | 列出和读取已批准定义 | GET /api/v1/mcp-definitions 与 /internal/mcp-definitions |
| 登记定义 | 管理员新增或更新受控定义 | POST/PUT /api/v1/mcp-definitions |
| 改变状态 | 启用、禁用或退役定义 | PUT /api/v1/mcp-definitions/{id} |

## 聚合与不变量

MCP Definition 是聚合根，拥有稳定 ID、Endpoint、服务身份、认证类型、状态和版本。更新在 Gateway 本地事务中完成；每次修改递增版本。具体 Plugin 和 Workspace Binding 只引用 Definition ID，跨服务不建立数据库外键。

企业外部 Definition 只能向所属企业内有权用户呈现，不得用于其他企业的 Workspace。普通成员按授权查看和选用，为 Workspace 配置还需其 manage 权限。平台角色不替代企业范围校验；企业管理员身份来自 Auth 的当前事实，不信任请求声明。

## 领域事件

目录创建、配置或状态变化产生 Catalog 变更事件，携带 Definition ID 与版本；Plugin 快照构建和 Gateway 每次请求读取当前定义，不以事件载荷替代权威配置。

## 数据模型

### `mcp_definitions`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| id | text PK | 仓库配置中的稳定 Definition slug |
| organization_id | text nullable | 企业外部 MCP 的不可变归属；空仅用于平台维护的定义 |
| service_name | text | 部署服务身份 |
| endpoint | text | MCP 内部或外部地址 |
| transport | text | platform/streamable_http/sse |
| authentication_type | text | session_context/oauth/none/bearer/api_key |
| status | text | enabled/disabled/retired |
| version | bigint | Definition 版本 |
| created_at | timestamptz | 创建时间 |
| updated_at | timestamptz | 最后更新时间 |

### UUID 生成规则

Definition ID 使用仓库配置中的稳定 slug；数据库记录使用该 ID，不自动生成另一个公开标识。

## 能力详细说明

### RegisterDefinition

1. 平台定义仅平台管理员可写；企业外部定义要求所属企业的当前管理员身份。
2. 校验 Endpoint 属于部署允许范围，认证类型在闭集合内。
3. 在 Gateway 本地事务 CAS 写入当前 Definition、不可变历史、目录 revision 和 Outbox；发布见 mcp_catalog_publication。
4. 外部 MCP 工具发现和 Plugin 快照构建不进入目录写事务，参见关联 Workflow。

### 查询与状态更新

目录查询由 Gateway 接口提供；Plugin 不读取 Gateway 数据库。管理查询与状态更新按平台/企业归属校验当前管理身份及期望版本；普通用户选用列表只呈现获准范围。版本冲突要求重读，禁用/退役定义不得作为新启用关系的有效目标。

## 关联关系

- [Plugin Binding](../../../media_plugin/domain/0002-plugin_binding/plugin_binding.md)：通过服务接口引用目录定义。
- [Plugin Binding Snapshot](../../../media_plugin/workflow/0001-plugin_binding_snapshot/plugin_binding_snapshot.md)：目录变化后的跨服务快照协调。
- [统一 MCP 调用](../../workflow/0004-platform_mcp_invocation/platform_mcp_invocation.md)：消费目录与绑定，发现并路由后端工具。


## 版本与目录查询

Definition ID 使用1–48字符安全 slug，不能包含命名空间分隔符 `__`。平台内置定义固定为 platform；不能更新为远端 Transport。远端 URL 只允许 http/https 和部署批准的精确 host[:port]，不允许 userinfo/query/fragment，协议适配器禁止重定向。

历史 `mcp_definition_versions` 以 id/version 为键，每次更新只追加；Plugin 固定快照引用该版本。当前 disabled/retired 立即禁止该来源调用；retired 不复活。`mcp_catalog_events` 以 definition_id/version 唯一并保存可重试投递状态，`mcp_catalog_revision` 是目录变更的可重建读版本。目录表位于 Gateway 自有 schema，不与外部服务共表。

内部 GET /internal/mcp-definitions/{id}?version=V 查询固定历史；省略 version 查询当前。内部列表使用 after_id/limit；公开管理查询和写入按定义归属要求当前平台或企业 admin，内部查询使用部署服务凭证，内部入口不提供写命令。内部调用也须以真实 Workspace 企业归属核对企业外部定义，不能因服务身份绕过企业边界。

企业定义不得使用平台 session_context 认证。bearer/api_key 的 Workspace 凭据由 Plugin 加密管理，OAuth 用户凭据由 Auth 管理，目录不保存秘密。企业当前成员可读本企业及平台目录；管理企业定义要求当前企业 admin。所有归属在创建后不可修改。
