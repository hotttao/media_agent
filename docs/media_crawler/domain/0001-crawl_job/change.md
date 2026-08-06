# crawl_job domain 首版建立

- 定义 Crawl Job 生命周期（queued→running→completed/failed/cancelled/rejected）、全局串行调度、子进程编排。
- 数据模型：自治 PG `jobs` 表。
- 能力：start_crawler / get_crawl_status / stop_crawler / list_checkpoints / delete_checkpoint / list_platforms / get_config / health_check / ListJobs / GetJob。
- 对齐 [ADR-0003](../../../adr/0003-crawler-subprocess-isolation.md)（子进程隔离）。
# 2026-08-06

- Crawl Job 增加 `workspace_id` 与 `requested_by`，作为结果授权边界。
