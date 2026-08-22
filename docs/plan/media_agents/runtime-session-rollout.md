# Agent Runtime Session 落地计划

依据 `docs/spec/media_agents/runtime-session-rollout.md`，逐个垂直切片开发、验证和提交。
`media_workflow` 暂不开发。

## P0：设计—实现基线

- [x] 建立 Harness 分层映射。
- [x] 建立全能力覆盖矩阵并保留所有 `missing`。
- [x] 明确 Gateway 旧写能力的迁移终点。
- [x] 通过 ADR-0010 统一为 Agent 自有 PostgreSQL database/schema。

## P1：Runtime Session 注册

- [x] 实现 Runtime Session 聚合、不变量和 repository 协议。
- [x] 增加数据库配置、Alembic migration、ORM/repository 与 TxRunner。
- [x] 实现幂等 Register/Get service 和轻薄 HTTP API/OpenAPI。
- [x] 增加 `media-cli media_agents runtime-session register/get`。
- [x] 完成领域、service、DAL、API 与 media-cli E2E 测试。
- [x] 通过 Python 全量质量门禁并独立提交。

## P2：Submit Turn 与 FIFO

- [x] 实现 Turn Queue、幂等提交、enqueue sequence 与 running 唯一约束。
- [x] 原子保存 queued Turn 和 user Message；无 running 时声明队首。
- [x] 完成并发、事务回滚、HTTP/media-cli E2E，独立提交。

## P3：Execute Turn 与终态

- [x] Codex ACP 与 Claude Code Runtime 接入 execute_turn workflow，事务外执行。
- [x] 持久化 Runtime Handle；运行凭据只在执行期解析且不进入 Turn input。
- [x] 实现 completed/failed/interrupted 与队列暂停/自然完成续跑。
- [x] 完成故障、中断、排队顺序 HTTP/media-cli E2E，独立提交。

## P4：事件、历史与回放

- [x] Agent 原子分配 Session sequence 并保存 Universal Event。
- [x] 迁移 Message、完成项、tool result 聚合/截断规则。
- [x] 实现持久化后总线发布和 `after_sequence` 回放。
- [x] 完成临时帧不落库、断线恢复、多订阅者 HTTP/media-cli E2E，独立提交。

## P5：Gateway 切换

- [ ] Gateway 改为 Agent 命令/查询代理和 SSE framing。
- [ ] 实现先订阅后回放、sequence 去重、缺口/终态/周期补查。
- [ ] 迁移历史数据并删除 Gateway 旧写入口。
- [ ] 完成跨 Gateway 节点及发布失败恢复 E2E，独立提交。

## P6：Agent Request 与重启恢复

- [ ] 实现 Agent Request 持久化与 resolve。
- [ ] 实现 running Turn 启动协调和安全恢复规则。
- [ ] 完成响应幂等、晚到事件、结果不明确 E2E，独立提交。

## 完成门禁

- [ ] 能力覆盖矩阵无 `missing`。
- [ ] 所有写能力均映射到可实现 Workflow，且 Domain/Harness 分层一致。
- [ ] Python/Go/UI 相关门禁与所有 media-cli E2E 通过。
- [ ] 每个功能均有独立提交，文档与实现同步。
