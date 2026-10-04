## 2026-09-28 · 并发轮换

撤销允许从 rotating 进入 revoking；轮换持久化尝试继续由 Rotation Workflow 清理，不因当前 Key 撤销成功而删除该尝试。
## 2026-10-04 · Agent 当前委托关系兜底

- 事务外查询 Control，再以短事务保存精确 Actor/owner/企业清理意图。
- 明确失效与来源故障分开处理；ID 分页、失败后继续、重启重试复用既有恢复。
- 可靠 disabled/deleted/owner 解绑事件仍未覆盖。
