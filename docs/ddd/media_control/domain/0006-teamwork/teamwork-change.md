# Teamwork 变更记录

## 2026-08-22：Topology 管理首个垂直切片

- 新增 Workspace-owned Topology、Member、Edge 三表及原子更新。
- 服务层校验成员唯一性、入口成员存在、边端点存在且不能自环；写入使用版本锁。
- Runtime Context 投影和 Gateway 任务限制留待运行时集成。
