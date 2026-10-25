# 任务列表稳定分页

依据 `docs/spec/media_workflow/ui-query-support.md`。

- [x] 提交时间相同的任务增加唯一 ID 排序，保持原筛选和总数。
- [x] 隔离 PostgreSQL 回归验证同时间记录跨页完整且无重复，核对用户隔离；修复前失败、修复后通过。
- [x] 新机真实历史任务分页、详情与步骤读取通过；输出 review 并提交。
