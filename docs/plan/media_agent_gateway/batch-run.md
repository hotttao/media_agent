# Batch Run 落地计划

## P1 · 管理切片

- [x] 对齐 Domain/Workflow 术语并建立覆盖矩阵。
- [x] IDL、迁移与 Hertz 生成代码。
- [x] BatchRun 聚合、冻结 Item 与校验不变量。
- [x] Repository 与 Create/Get/List/Cancel Service。
- [x] 轻薄 Handler、Session middleware 与 wiring。
- [x] Domain/Service 单测、`go test ./...`、`make arch-check`。
- [x] media-cli E2E：创建、幂等、越权拒绝、取消。
- [x] 独立提交。

## P2 · Task 拆分与投影

- [ ] Batch Outbox/River Job 与稳定 next_cursor。
- [ ] 每个 Item 幂等创建 Agent Task 并保存 task_id。
- [ ] Task terminal Inbox 去重、Item/计数投影与 Batch 终态。
- [ ] running Batch 取消与未终态 Task 传播。
- [ ] 故障/重复事件单测与 media-cli E2E。
- [ ] 更新能力矩阵为无 missing，并独立提交。
