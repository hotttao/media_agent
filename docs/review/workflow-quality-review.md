# Workflow 质量审查

审查范围：`docs/**/workflow/` 与 `docs/workflow/` 下 26 份权威 Workflow。标准来自 `domain-modeling` Skill：时序与参与者、事务边界、状态所有权、幂等恢复、身份传递和失败结果必须足以指导实现。

## 结果

| 分组 | 数量 | 审查结果 |
|:---|---:|:---|
| Gateway | 4 | covered |
| Control | 2 | covered |
| Plugin / Notification / Storage | 4 | covered |
| 全局跨服务流程 | 5 | covered |
| Auth | 5 | covered |
| Crawler / Gen / Materials | 4 | covered |
| Media Workflow | 2 | covered |
| 合计 | 26 | 无 missing |

## 统一结论

- River Job、Outbox 和网络连接均不是业务状态；业务终态由对应 Domain 聚合决定。
- 外部调用位于本地事务外；结果不明确时优先查询/协调，禁止盲目重放非幂等操作。
- Web 身份在 Edge 终止；内部调用使用 mTLS 与受控 Principal；Agent MCP 使用 Session Token；资源所有服务执行最终授权。
- Workflow Validation 是同步只读决策，不拥有持久化状态机；运行类 Workflow 必须明确取消、晚到结果和终态竞争。
- 已移除权威 Workflow 中旧 `control_plane + HS256`、单进程 `pendingAuths` 和 Control 代理外部 MCP 的目标设计残留。

## 本轮重点修正

- `run_turn`：补齐流式事件先持久化后推送、断线恢复、终态 CAS 和非幂等工具重放限制。
- Agent Task、Schedule、Generation、Publish：区分业务任务和 River 投递，并明确结果不确定状态。
- MCP OAuth：改为 Plugin 负责外部 MCP 连接/代理、Auth 负责持久化授权意图与加密 Credential。
- Service Token：改为 Edge 调 Auth introspection；Cookie/OAuth JWT 通过 Auth JWKS 验证。
- Runtime Context、Plugin Snapshot、Teamwork Topology：使用不可变版本，晚到旧版本不得覆盖新配置。
