# Platform Administration 落地计划

## P1 · Platform Setting

- [x] IDL、迁移、Domain/Repository/Service 与 Harness 路由。
- [x] Admin/System 双层授权与普通用户拒绝。
- [x] 创建、查询、列表与 expected_version 更新。
- [x] `PlatformSettingChanged` Outbox 同事务写入。
- [x] 单测、架构检查和 media-cli 真实 HTTP E2E。
- [x] 回填 P1 能力矩阵并独立提交。

## 后续读模型

- [x] Cluster 状态投影：内部单调上报、Admin 查询与迟到观测保护。
- [ ] 平台统计投影。
- [ ] Admin Workspace 查询。
