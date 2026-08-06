---
name: teamwork
description: 多 Agent 成员与协作拓扑的版本化控制配置
dependencies:
    - workspace
    - agent_configuration
related_workflow:
    - teamwork_runtime_change
---

## 一句话职责

Teamwork 负责多 Agent 的成员集合、协作边和拓扑版本，是运行期协作配置的权威来源。

## 边界

### 做什么
- 创建、更新、查询和删除 Teamwork Topology。
- 校验成员引用、边引用、入口成员和拓扑可达性。
- 发布版本变化供 Gateway 限制新任务。

### 不做什么
- 不管理用户 Team；用户团队归 `media_auth`。
- 不保存运行中的 Turn 或 Session Link。

## 能力清单

| 能力 | 说明 | 对外接口/函数 |
|:---|:---|:---|
| 管理拓扑 | CRUD 版本化 Teamwork | `TeamworkService.Create/Get/List/Update/Delete` |
| 管理成员 | 修改 Agent 成员和协作边 | `UpdateMembers/UpdateEdges` |
| 获取运行配置 | 返回指定版本的只读拓扑 | `GetRuntimeContext` |

## 数据模型

### `teamwork_topologies`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| id | text PK | Teamwork Topology ID |
| workspace_id | text | 所属 Workspace |
| name | text | 拓扑名称 |
| entry_member_id | text | 入口成员 ID |
| version | bigint | 拓扑版本 |
| status | text | active/archived/deleted |
| created_at | timestamptz | 创建时间 |
| updated_at | timestamptz | 最后更新时间 |

### `teamwork_members`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| topology_id | text | 所属 Topology |
| member_id | text | Topology 内成员 ID，与 topology_id 联合主键 |
| agent_config_id | text | 成员使用的 Agent Configuration |
| name | text | 成员显示名称 |
| created_at | timestamptz | 创建时间 |

### `teamwork_edges`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| topology_id | text | 所属 Topology |
| from_member_id | text | 上游成员 ID |
| to_member_id | text | 下游成员 ID |
| mode | text | delegate/handoff/broadcast 等协作模式 |
| configuration | jsonb | 协作边参数 |

### UUID 生成规则

Topology 和 Member 使用 ULID；更新以 `expected_version` 乐观并发控制。

## 能力详细说明

### Update

1. 校验 Workspace 与 expected_version。
2. 校验成员引用同一 Workspace，边端点存在且满足拓扑规则。
3. 原子写入新版本并发布 `TeamworkTopologyChanged`。
