---
name: plugin_binding_snapshot
description: 将 Workspace Plugin Binding 发布为 Agent MCP 配置快照
participants: [media_agent_gateway/mcp_catalog, plugin_binding, media_control]
domains: [media_agent_gateway/mcp_catalog, plugin_binding]
---

## 一句话说明
Workspace 启用关系变化后，Plugin 校验 MCP Definition 与配置并生成版本化候选快照。用户明确确认选择后，Control 才发布新的 Runtime Context。

## 触发与结果
| 项目 | 内容 |
|:---|:---|
| 触发 | Binding 创建、更新、禁用或 Definition 变更 |
| 前置条件 | Definition 已批准，配置通过 schema 校验 |
| 成功结果 | 发布不可变 snapshot_version |
| 失败结果 | 保留上一有效快照并报告无效项 |

## 执行时间线
```mermaid
sequenceDiagram
    participant U as Workspace Admin
    participant G as Gateway MCP Catalog
    participant P as media_plugin
    participant DB as Plugin DB
    participant C as media_control
    U->>P: Enable/Update/Disable Binding
    P->>G: 查询受控 Definition 与当前版本
    G-->>P: Definition / 不存在 / 禁用
    P->>DB: TX1: 更新 Binding/version + Outbox
    P->>DB: 读取有效 Binding 版本
    P->>G: 查询 Definition 配置与版本
    G-->>P: 版本化 Definition
    Note over P,G: 构建后再次核对来源版本；冲突则放弃旧结果并重组
    P->>P: 校验 schema、endpoint、auth type
    P->>DB: TX2: 保存不可变 Snapshot + Outbox
    P-->>U: 返回候选 snapshot_version
    U->>C: 明确确认固定快照选择
    C->>P: 校验快照、Workspace 与当前目录权限
    P-->>C: 不含凭据的固定快照
    C->>C: 保存不可变 Release 并通知 Agent
```

### 步骤说明
Binding 是用户期望，Snapshot 是可重建发布结果。配置无效时保留 Binding 错误和上一有效 Snapshot；Gateway 使用版本化配置解析可访问的 MCP 实现；Agent 只连接 Gateway MCP 入口，由 Gateway 代理目标 MCP。

## 关键参数
`workspace_id` 固定作用域；`snapshot_version` 供 Runtime 检测配置变更。

## 状态与不变量
Binding 的 enabled/disabled 由 Plugin Binding 聚合决定；Snapshot 创建后不可变。同一 Workspace 可保留多个不可变候选版本；Agent 使用 Control 已确认 Release 中的固定选择，不能在运行时直接读取最新候选版本。

## 一致性边界
TX1 原子提交 Binding/version/Outbox；TX2 原子提交 Snapshot/候选指针。Gateway 目录查询、Control 通知均在事务外并允许重复；新配置不原地修改已有 Session 的固定版本。

## 幂等、重试与补偿
`workspace_id + binding_version_vector` 唯一标识快照。构建失败可在相同向量下重试；版本已变化则放弃旧结果。Outbox 重试不回滚 Binding；非法配置需用户修正。

## 权限与失败
Edge 传递已验证用户身份，Plugin 调 Control 检查 Workspace manage；服务间使用可配置 mTLS。Definition 未批准、schema 不匹配和 endpoint 禁止均不可重试；Gateway/Control 暂时不可用可有限重试，失败时保留上一有效快照，不发布半成品。

## 结果事件
候选快照可产生 `McpConfigurationSnapshotChanged` 供界面提示；不能据此自动升级 Agent。只有用户确认后由 Control 发布已确认配置变更通知。
