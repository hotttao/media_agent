---
name: session_analytics
description: 近期会话、活动和 Usage 查询投影
dependencies:
  - agent_session
  - agent_task
related_workflow:
  - run_turn
  - agent_task_dispatch
---

## 一句话职责

Session Analytics 从 Gateway 运行事实构建用户和 Workspace 维度的可重建查询投影。

## 边界

### 做什么
- 提供近期 Session、活动摘要和 Usage 查询。
- 消费 Session、Turn 和 Task 结果更新投影。

### 不做什么
- 不作为计费账本或审计日志。
- 不改变 Session、Turn 或 Task 状态。

## 能力清单

| 能力 | 说明 | 对外接口/函数 |
|:---|:---|:---|
| 查询近期会话 | 返回用户最近访问的 Session | 待 IDL 定义 |
| 查询活动 | 返回日期维度活动摘要 | 待 IDL 定义 |
| 查询 Usage | 返回模型与工具使用聚合 | 待 IDL 定义 |

## 聚合与不变量

本 Domain 仅包含可重建读模型，无命令侧聚合；同一来源事件通过 event_id 幂等应用。

## 领域事件

无。

## 数据模型

### `session_activity_daily`
| 字段 | 类型 | 含义 |
|:---|:---|:---|
| workspace_id | text | Workspace ID |
| user_id | text | User ID |
| activity_date | date | 活动日期 |
| session_count | bigint | 活跃 Session 数 |
| turn_count | bigint | 完成 Turn 数 |
| message_count | bigint | Message 数 |
| last_session_id | text nullable | 最近 Session |
| updated_at | timestamptz | 投影更新时间 |

### `usage_daily`
| 字段 | 类型 | 含义 |
|:---|:---|:---|
| workspace_id | text | Workspace ID |
| user_id | text | User ID |
| usage_date | date | 使用日期 |
| model | text | 模型标识 |
| input_tokens | bigint | 输入 Token 数 |
| output_tokens | bigint | 输出 Token 数 |
| tool_calls | bigint | 工具调用数 |
| updated_at | timestamptz | 投影更新时间 |

## 能力详细说明

查询必须按 Cookie JWT 或 Service Token 中的用户范围过滤；管理员查询另行校验管理权限。

## 关联关系

### 依赖的 Domain
- [agent_session](../0001-agent_session/agent_session.md) - 提供会话和交互事实。
- [agent_task](../0002-agent_task/agent_task.md) - 提供异步任务事实。

### 被依赖的 Domain
- 无。

### 参与的 Workflow
- `run_turn`
- `agent_task_dispatch`

### 关联术语
- `Session Analytics`
