# video-publish workflow 变更记录

## 2026-07-21 · 初始建模（media_gen v1）

- 新建 video-publish workflow：PublishVideo RPC -> publish 域 mock 执行 -> 返回 PublishResult。
- 明确**不被 generation 自动触发**：调用方负责在 BFF 层串接 generation -> 用户选图 -> publish。
- 明确**本期无跨服务 RPC**：mock 不调 `media_materials` / `media_crawler`，留 TODO。
- 状态机：`PENDING -> SUCCEEDED`（跳过 PUBLISHING 异步阶段）。
- 真实实现幂等键：`(asset_id, target)` 复合；本期 mock 不持久化，无幂等要求。
