# Agent Task 落地计划

## P1 · 管理切片

- [x] 统一 Domain/Workflow 状态机与 change 记录。
- [x] IDL、迁移、Hertz 生成代码。
- [x] AgentTask 聚合、owner/JSON/幂等不变量。
- [x] Repository 与 Create/Get/List/Cancel Service。
- [x] 轻薄 Handler、Session middleware、main wiring。
- [x] Domain/Service 单测、`go test ./...`、`make arch-check`。
- [x] media-cli HTTP E2E：创建、幂等复用、跨用户不可见、取消。
- [x] 独立提交。

## P2 · Dispatch 切片

- [ ] Task + Outbox 原子创建，Relay 至少一次投递。
- [ ] queued/retry_wait 原子领取与 attempt CAS。
- [ ] `task_id` 幂等调用 Agent SubmitTurn，保存 turn_id/waiting_turn。
- [ ] Turn 终态事件收敛与 Replay/GetTurn 补偿。
- [ ] 取消 queued/running/waiting_turn 的不同语义。
- [ ] Workflow 单测、故障/重复投递测试、media-cli E2E。
- [ ] 更新能力覆盖矩阵为无 missing，并独立提交。
