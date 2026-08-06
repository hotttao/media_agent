# publish domain 变更记录

## 2026-08-06 · 确定异步发布模型与 Adapter 所有权

- Publish 改为持久化异步 Task，保存稳定平台请求标识和最终结果。
- 平台发布 Adapter 明确归 `media_gen`，不归 `media_crawler`。
- `media_crawler` 只负责抓取，不参与发布链路。
- 增加 `result_unknown`，平台调用结果不明确时先查询协调，禁止盲目重发。

## 2026-07-21 · 初始建模（media_gen v1）

- 新建 publish domain：mock 视频发布，返回假 publish_url + SUCCEEDED 状态。
- 明确**不持久化**：发布结果一次性返回，真实实现需新增 `publishes` 表 + 异步轮询，本期不预建表。
- 明确**不调 media_materials / media_crawler**：本期 mock，留 TODO；真实实现预期调 `AssetRead` 校验 + 调 crawler 平台 API。
- 状态机：`PENDING -> PUBLISHING -> (SUCCEEDED | FAILED)`；本期 mock 跳过 PUBLISHING，直接 SUCCEEDED。
- 支持的 target 静态列表：`xhs / douyin / bilibili`，与 `media_crawler` 平台概念对齐。
