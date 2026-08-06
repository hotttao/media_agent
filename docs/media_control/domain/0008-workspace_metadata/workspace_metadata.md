---
name: workspace_metadata
description: Workspace 资源的标签、分享记录与公共导出元数据
dependencies:
  - workspace
related_workflow: []
---

## 一句话职责

Workspace Metadata 保存控制面通用的资源分类与发布记录，但不拥有资源内容和访问凭证。

## 边界

### 做什么
- 管理 Workspace 内 Tag 及资源关联。
- 记录资源 Share 和 Public Export 的生命周期与对象路径。

### 不做什么
- 不签发匿名分享 Token；由资源所有服务签发并校验。
- 不保存导出二进制；对象由 `media_storage` 保存。

## 能力清单

| 能力 | 说明 | 对外接口/函数 |
|:---|:---|:---|
| 管理标签 | 创建、查询、更新、删除 Tag 及资源关联 | 待 IDL 定义 |
| 管理分享记录 | 记录或撤销资源分享状态 | 待 IDL 定义 |
| 管理公共导出 | 记录导出对象路径和发布状态 | 待 IDL 定义 |

## 聚合与不变量

| 聚合根 | 包含对象 | 一致性边界 | 核心不变量 |
|:---|:---|:---|:---|
| Tag | ResourceTag | Tag 及其关联 | 同一 Workspace 内 Tag 名称唯一 |
| ShareMetadata | 无 | 单条分享记录 | owner 与 workspace 创建后不可变 |
| PublicExport | 无 | 单次导出记录 | 仅 ready 状态可对外发布 |

## 领域事件

无。

## 数据模型

### `tags`
| 字段 | 类型 | 含义 |
|:---|:---|:---|
| id | text PK | Tag ID |
| workspace_id | text | 所属 Workspace |
| name | text | Workspace 内唯一名称 |
| color | text nullable | 展示颜色 |
| created_by | text | 创建用户 |
| created_at | timestamptz | 创建时间 |

### `resource_tags`
| 字段 | 类型 | 含义 |
|:---|:---|:---|
| tag_id | text | Tag ID |
| workspace_id | text | 所属 Workspace |
| resource_type | text | 资源类型 |
| resource_id | text | 资源 ID |
| created_at | timestamptz | 关联时间 |

### `share_metadata`
| 字段 | 类型 | 含义 |
|:---|:---|:---|
| id | text PK | 分享记录 ID |
| workspace_id | text | 所属 Workspace |
| resource_type | text | 资源类型 |
| resource_id | text | 资源 ID |
| owner_user_id | text | 创建分享的用户 |
| status | text | active/revoked |
| created_at | timestamptz | 创建时间 |
| revoked_at | timestamptz nullable | 撤销时间 |

### `public_exports`
| 字段 | 类型 | 含义 |
|:---|:---|:---|
| id | text PK | 导出记录 ID |
| workspace_id | text | 所属 Workspace |
| resource_type | text | 来源资源类型 |
| resource_id | text | 来源资源 ID |
| object_path | text | 域名无关的存储路径 |
| status | text | pending/ready/failed/revoked |
| created_by | text | 创建用户 |
| created_at | timestamptz | 创建时间 |

## 能力详细说明

所有写操作先校验 Workspace action；资源是否可分享或删除仍由资源所有服务校验。

## 关联关系

### 依赖的 Domain
- [workspace](../0001-workspace/workspace.md) - 提供租户边界与访问判断。

### 被依赖的 Domain
- 无。

### 参与的 Workflow
- 无。

### 关联术语
- `Workspace Metadata`
