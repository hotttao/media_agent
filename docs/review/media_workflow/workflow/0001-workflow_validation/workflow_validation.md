# Workflow Validation 实现一致性审查

审查基线：`docs/ddd/media_workflow/workflow/0001-workflow_validation/workflow_validation.md`、`docs/spec/enterprise-media-production.md`。审查对象：`media_workflow/biz/service/workflow/validator.go`、Workflow 领域对象与数据库约束。

## 结论

当前实现能拒绝空 DAG、重复 step key、非法前向引用、多 sink、离群节点、缺失或归档 Template 以及 Scope 不一致；它可以作为结构校验雏形，但尚未达到发布和执行前安全闸的设计要求。

## 差异

| 严重度 | 设计要求 | 当前实现 | 影响 |
|:---|:---|:---|:---|
| P0 | Step 的前向引用以 `step.order` 判定 | `validateDAG` 用 Step 在请求数组中的下标判定前后 | 请求数组顺序与 order 不一致时会误拒绝或误接受，Validator 与 Compiler 的排序语义不同 |
| P0 | caller 对每个 Template 必须有 viewer+ 权限 | Validator 只加载 Template 快照，不接收 principal，也不检查 Template ACL | 可把无权访问的 Template 引入 Workflow |
| P0 | required Template Slot 必须全部绑定 | 只验证已有 binding 的 `to_input`，未检查 required slot 是否缺失 | Workflow 可保存，但执行时缺少必填输入 |
| P0 | `step_output` 两端 asset kind/media type 必须兼容 | 未比较前驱输出与消费槽的类型 | 非法边直到 Worker 执行才暴露 |
| P1 | Workflow Input 是显式定义，binding 用 `ref` 指向入口名 | 当前 `WORKFLOW_INPUT` 强制 `ref` 为空，并以 `to_input` 派生全局 input schema | 同一个外部输入无法映射到不同 Template slot 名；同名 slot 被错误合并为同一入口 |
| P1 | Asset Kind 必须存在、未停用且 resource type 匹配 | 当前快照仅使用旧 `resource_type`，没有在 Workflow Validator 中批量校验 Asset Kind | 类型契约与 Materials 注册表可能漂移 |
| P1 | Approval/Reuse Policy 枚举在领域入口校验 | 主要依赖数据库 CHECK，Validator 未拒绝非法值 | `ValidateWorkflow` 与最终保存结果可能不一致 |
| P1 | 返回所有可定位 RejectReason | 首个错误立即返回 | UI 无法一次展示完整的 DAG 修复清单 |

## 已覆盖能力

| 能力 | 状态 |
|:---|:---|
| DAG step key 唯一、单 sink、无离群节点 | covered |
| Template 存在、未归档、Scope 相同 | covered |
| Template ACL | missing |
| required slot 覆盖 | missing |
| Step 输出到输入的类型安全 | missing |
| 显式 Workflow Input 定义与绑定 | missing |
| Asset Kind 注册表校验 | missing |

## 建议

先把 Workflow Input 从“由 slot 名隐式派生”改成聚合内显式值对象，再统一 Validator 和 Compiler 对 `step.order` 的解释。发布时必须复用同一个完整 Validator，不能只调用 Compiler 做结构检查。
