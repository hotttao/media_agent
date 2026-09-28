---
name: platform_administration
description: 平台设置及 Cluster、统计管理读模型
dependencies:
  - workspace
related_workflow: []
---

## 一句话职责

Platform Administration 持有平台级设置，并向管理员提供 Cluster、Workspace 和统计投影。

## 边界

### 做什么
- 维护版本化平台设置。
- 保存 Cluster 状态和平台统计的只读投影。
- 聚合管理员所需的 System/Admin Workspace 查询。

### 不做什么
- 不拥有 Workspace 和 Runtime Environment 事实。
- 不直接执行基础设施变更。

## 能力清单

| 能力 | 说明 | 对外接口/函数 |
|:---|:---|:---|
| 管理平台设置 | 创建、查询和乐观锁更新平台级配置 | `PlatformAdministrationService.Create/List/Get/UpdatePlatformSetting` |
| 查询集群 | internal-system 单调更新并向管理员返回 Cluster 状态投影 | `Report/List/GetClusterProjection` |
| 查询平台统计 | internal-system 刷新并按指标/周期返回聚合投影 | `Report/ListPlatformStatistic` |
| 查询管理工作空间 | 按 owner/state/agent type 分页组合 Workspace、Agent Context 与 Placement 摘要 | `ListAdminWorkspaces` |

## 聚合与不变量

| 聚合根 | 包含对象 | 一致性边界 | 核心不变量 |
|:---|:---|:---|:---|
| PlatformSetting | 无 | 单个设置项 | 更新必须使用 version 乐观锁 |

Cluster 和统计为可重建读模型，不作为命令侧聚合。

## 领域事件

| 事件 | 产生条件 | 关键载荷 | 消费方 |
|:---|:---|:---|:---|
| PlatformSettingChanged | 设置更新提交 | key, version | 配置订阅方 |

## 数据模型

### `platform_settings`
| 字段 | 类型 | 含义 |
|:---|:---|:---|
| key | text PK | 设置键 |
| value | jsonb | 设置值 |
| version | bigint | 乐观锁版本 |
| updated_by | text | 管理员 User ID |
| updated_at | timestamptz | 更新时间 |

### `cluster_projections`
| 字段 | 类型 | 含义 |
|:---|:---|:---|
| cluster_id | text PK | Cluster 标识 |
| status | text | 观测状态 |
| capacity | jsonb | 容量快照 |
| observed_at | timestamptz | 观测时间 |

### `platform_statistics`
| 字段 | 类型 | 含义 |
|:---|:---|:---|
| metric_key | text | 指标键 |
| period_start | timestamptz | 统计周期起点 |
| dimensions | jsonb | 维度集合 |
| dimensions_hash | text | canonical dimensions 的 SHA-256，参与复合主键 |
| value | numeric | 指标值 |
| refreshed_at | timestamptz | 刷新时间 |

## 能力详细说明

管理查询可组合 Workspace 与投影数据，但不得绕过各 Domain 的命令接口修改事实。

Platform Setting 管理入口只接受 `admin/system` 角色；内部调用必须同时具有 Control 工作负载身份、acting user 和受控 acting role。创建与更新在同一事务写入 `PlatformSettingChanged` Outbox，配置订阅方不得依赖同步回调。

Cluster Projection 上报只接受内部 `system` Principal；`observed_at` 必须单调递增，迟到或重复观察不得覆盖较新状态。管理员只能查询该读模型，不能通过普通管理请求上报基础设施事实。

Platform Statistics 使用 `metric_key + period_start + dimensions_hash` 唯一定位投影。维度 JSON canonicalize 后计算 SHA-256，数值以十进制字符串进入 PostgreSQL `numeric`；只有更大的 `refreshed_at` 可以覆盖当前值。

Admin Workspace Query 只组合 Workspace、Agent Configuration 和 Placement 的管理摘要，不选择或返回 `encrypted_api_key`、Provider API Key 或 Session 凭据；普通用户不能借此跨 owner 查询。

## 关联关系

### 依赖的 Domain
- [workspace](../0001-workspace/workspace.md) - 提供 Workspace 权威事实。

### 被依赖的 Domain
- 无。

### 参与的 Workflow
- 无。

### 关联术语
- `Platform Administration`
