## 2026-09-28 · 并发轮换

撤销允许从 rotating 进入 revoking；轮换持久化尝试继续由 Rotation Workflow 清理，不因当前 Key 撤销成功而删除该尝试。
