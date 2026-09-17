---
name: organization
description: 企业信息、企业成员与企业功能授权上限
dependencies:
  - auth
related_workflow: []
---

## 一句话职责

Organization 负责企业客户主体、企业成员关系和企业可使用功能的授权上限，是 Team 与 Workspace 企业归属的权威来源。

## 边界

### 做什么
- 管理企业名称、状态和基础信息。
- 管理 User 与 Organization 的成员关系及企业级角色。
- 维护平台授予企业的 Organization Entitlement。
- 为 Team 创建、部门授权和 Workspace 准入提供企业归属与功能上限查询。

### 不做什么
- 不管理企业内部门；部门归 [team](../0007-team/team.md) Domain。
- 不拥有 Workspace；Workspace 归 `media_control.workspace` Domain。
- 不保存资源级 Resource Grant，也不替资源所属服务执行最终授权。
- 不把企业功能授权写入用户身份令牌；运行期按权威事实或其受控投影判断。

## 能力清单

| 能力 | 说明 | 对外接口/函数 |
|:---|:---|:---|
| 管理企业 | 创建、查询、更新和停用企业 | `OrganizationService` `/api/organizations*` |
| 管理企业成员 | 加入、移除和调整企业级角色 | `OrganizationService` `/api/organizations/{id}/members*` |
| 管理企业功能 | 设置企业可使用功能的授权上限 | `OrganizationService` `/api/organizations/{id}/entitlements` |
| 查询企业访问事实 | 批量返回成员角色和功能授权 | internal authorization facts API（待落地） |

## 聚合与不变量

| 聚合根 | 包含对象 | 一致性边界 | 核心不变量 |
|:---|:---|:---|:---|
| Organization | Organization Membership、Organization Entitlement | 一次企业配置变更 | Team 只能引用 active Organization；企业至少保留一名 admin；功能授权只能引用平台注册功能 |

- Organization Member Role 首期为 `admin | member`。
- admin 可以管理企业资料、部门和部门功能，但只能授予 Organization Entitlement 已包含的功能。
- 停用 Organization 后不得创建新 Team、Workspace 或发起新的业务执行；历史事实保留。
- Organization 创建只允许平台管理员；创建事务同时把指定首任管理员加入 Organization，禁止产生无管理员企业。
- 删除最后一名 active Organization admin、把其降级或移出企业必须返回冲突。

## 领域事件

| 事件 | 产生条件 | 关键载荷 | 消费方 |
|:---|:---|:---|:---|
| OrganizationStatusChanged | 企业状态发生变化 | organization_id, status, version | Control、授权投影 |
| OrganizationEntitlementsChanged | 企业功能授权发生变化 | organization_id, entitlement_version | Team 授权投影、授权同步器 |
| OrganizationMembershipChanged | 企业成员或角色变化 | organization_id, user_id, role, version | 授权同步器 |

## 数据模型

### `organizations`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| id | text PK | Organization ID |
| name | text | 企业名称 |
| status | text | active/disabled |
| version | bigint | 乐观锁与事件版本 |
| created_at | timestamptz | 创建时间 |
| updated_at | timestamptz | 更新时间 |

#### 约束与索引

- `PRIMARY KEY (id)`。
- `CHECK (status IN ('active', 'disabled'))`。

### `organization_members`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| organization_id | text | Organization ID |
| user_id | text | User ID |
| role | text | admin/member |
| joined_at | timestamptz | 加入时间 |

#### 约束与索引

- `PRIMARY KEY (organization_id, user_id)`。
- `CHECK (role IN ('admin', 'member'))`。
- User 属于同一服务但仍通过 Domain 能力校验，不由外部服务直接写表。

### `organization_entitlements`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| organization_id | text | Organization ID |
| feature_key | text | 平台功能稳定标识 |
| granted_by | text | 授权操作人或平台服务身份 |
| granted_at | timestamptz | 授权时间 |

#### 约束与索引

- `PRIMARY KEY (organization_id, feature_key)`。
- feature_key 必须存在于平台功能目录；功能目录所有权在后续平台管理设计中确定。

## 能力详细说明

### 企业功能授权

Organization Entitlement 表达企业功能上限，不是资源 ACL。企业管理员可以让 Team 继承全部企业功能或限制为子集，但不能越过企业上限。企业功能被平台撤销后，所有 Team 的有效功能立即按交集收缩，不需要逐部门复制撤权。

## 关联关系

### 依赖的 Domain
- [auth](../0001-auth/auth.md) - User 是企业成员主体。

### 被依赖的 Domain
- [team](../0007-team/team.md) - Team 必须属于一个 Organization，部门功能不能超过企业上限。
- [workspace](../../../media_control/domain/0001-workspace/workspace.md) - Workspace 通过 Team 确定企业归属。

### 参与的 Workflow
- 当前无；后续将企业授权投影到外部权限引擎时必须新增同步 Workflow。

### 关联术语
- `Organization`
- `Organization Membership`
- `Organization Entitlement`
