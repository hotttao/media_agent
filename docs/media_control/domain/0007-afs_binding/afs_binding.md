---
name: afs_binding
description: 平台对第三方 AFS 的配置引用与状态投影
dependencies:
    - workspace
related_workflow:
    - workspace_lifecycle
    - runtime_context_assembly
---

## 一句话职责

AFS Binding 负责 Workspace 与第三方 AFS 能力的绑定、分享配置和平台侧状态投影。

## 边界

### 做什么
- 维护 Workspace AFS Binding 与分享成员。
- 调用原有 AFS 接口并保存必要状态投影。
- 对账外部状态并暴露控制面查询。

### 不做什么
- 不复制 AFS 内部对象模型。
- 不修改 `afs-controller` 或 `afs-sidecar` 代码。
- 不改变原 Control Plane 使用 AFS 的交互语义。

## 能力清单

| 能力 | 说明 | 对外接口/函数 |
|:---|:---|:---|
| 管理绑定 | 创建、查询和删除 AFS Binding | `AfsShareService.Create/List/Delete` |
| 管理成员 | 维护 AFS 分享成员投影 | `ListMembers/AddMember/RemoveMember` |
| 对账状态 | 刷新第三方 AFS 状态 | `ReconcileAfsBinding` |

## 数据模型

### `afs_bindings`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| id | text PK | 平台 AFS Binding ID |
| workspace_id | text | 所属 Workspace |
| external_ref | text | AFS 第三方对象标识 |
| desired_state | text | 平台期望状态 |
| observed_state | text | 最近观测到的 AFS 状态 |
| last_synced_at | timestamptz nullable | 最近对账时间 |
| version | bigint | 乐观锁版本 |
| created_at | timestamptz | 创建时间 |
| updated_at | timestamptz | 最后更新时间 |

### `afs_binding_members`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| binding_id | text | 所属 AFS Binding |
| subject_id | text | 平台用户或团队 ID |
| subject_type | text | user/team |
| external_member_ref | text | AFS 侧成员标识 |
| status | text | pending/active/removed/failed |
| updated_at | timestamptz | 最后同步时间 |

### UUID 生成规则

平台 Binding 使用 ULID；`external_ref` 完整保留第三方标识，不参与平台 ID 生成。

## 能力详细说明

### ReconcileAfsBinding

1. 读取平台期望状态，事务外调用 AFS 原有接口。
2. 以外部返回更新 observed_state；失败保留旧投影并记录可重试错误。
3. 不将外部对象当作平台聚合写入其他表。
