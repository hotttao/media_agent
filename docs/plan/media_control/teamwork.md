# Teamwork Topology 垂直切片

- [x] Teamwork Topology/Member/Edge IDL、迁移与生成代码。
- [x] Workspace owner 校验与 active topology CRUD。
- [x] 成员唯一性、入口成员、边端点和模式校验。
- [x] `expected_version` 乐观并发与删除状态机。
- [x] 单测、`go test ./...`、`make arch-check`、media-cli HTTP E2E。
- [x] Runtime Context 只读投影（见 `docs/spec/media_control/teamwork-runtime-context.md`）。
- [ ] Gateway 拓扑变更通知与新任务版本准入（后续独立切片）。
