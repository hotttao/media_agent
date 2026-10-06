# 原生产物 Retry 幂等语义局部审查

用户于 2026-10-05 采纳方案 2。下表记录裁决前发现的冲突；DDD 已更新为每轮独立不可变产物，服务接线仍需实现，不能将设计更新视作运行能力已完成。

范围：GeneratedAsset.Upsert 与原生 Workflow Finalize/Retry，不审查 UI、历史迁移或整个 Materials Domain 的完成度。

## 事实与冲突

| 事实来源 | 当前语义 | 影响 |
|:---|:---|:---|
| `docs/ddd/media_materials/domain/0002-asset/asset.md`「UpsertGeneratedAsset 与 retry 的 name 语义」 | 同一业务 name 的 Retry 默认覆盖 Generated Asset，旧对象由执行历史保存；新 name 才保留独立 Asset | 独立结果资源不是既有默认设计 |
| `media_materials/biz/service/generated_asset/service.go` `Upsert` | 按 StepRunID 查到任何未删除记录即返回，尚未解析新的候选对象，也未比较业务请求 | 新一次执行可能返回旧对象；同轮不同结果也不会冲突 |
| `media_workflow/biz/service/job/service.go` `finalizeStepRun` | 同一 StepRun 使用同一稳定 name/StepRunID 调 Upsert | 整单 Retry 不会通过新 name 区分轮次 |
| `media_workflow/biz/service/job/service.go` `Retry` | 清空节点产物引用后重跑整单，忽略多项本地写入错误；引擎调用先于本地接受 | 仍缺可靠 Retry 意图，也缺对旧产物身份的处理 |

不能以本地 RetryCount 快照 fencing 证明跨服务产物正确：它能拒绝旧查询收口，但无法改变 Materials 按 StepRunID 返回旧记录的行为。StepRun 是稳定业务节点，不等同某次执行结果。

## 能力覆盖

| 原能力 | 所属 Domain / Workflow | 状态 |
|:---|:---|:---|
| 同 StepRun 首次产物登记 | Materials asset / Workflow workflow_execution | covered |
| 同轮结果重放且并发只落一次 | Materials asset / workflow_execution | missing |
| Retry 默认覆盖业务产物并保存旧对象历史 | Materials asset / Workflow execution | deprecated-with-reason：用户选择每轮独立产物，替代覆盖默认值 |
| 每轮独立产物与同轮不可变登记回执 | Materials asset / Workflow execution | missing：结果核心已实现，仓储与接口尚未接线 |
| 同轮不同内容拒绝 | Materials asset | missing |
| Retry 的跨引擎持久协调 | Workflow execution / workflow_execution | missing |

## 需要确定的默认语义

1. 保持既有 DDD：同一业务产物 ID 指向最新结果，Retry 覆盖；另保存不可变执行结果历史，旧结果不会丢失。必须处理晚到轮次不得覆盖新轮次，以及所有已引用该产物的消费者如何理解更新。
2. 每次生成结果是独立不可变 Generated Asset：同轮重放原 ID，新轮落新 ID，StepRun 仅指向当前选定结果；既有产物引用不被修改。需要修改 DDD 的默认覆盖语义，但不要求删除或改写现有记录。

已裁决方案 2，解除本项语义修改暂停。执行规格与计划见 `docs/spec/generation-result-idempotency.md`、`docs/plan/generation-result-idempotency.md`。不得把不同结果默认为同轮幂等重放，也不得为满足新协议给旧记录回填虚构轮次。
