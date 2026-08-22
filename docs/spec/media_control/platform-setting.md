# Platform Setting P1 实现规格

## 边界

Platform Setting 是 Platform Administration 的版本化命令侧聚合。本切片只实现设置项管理；Cluster 状态、平台统计和 Admin Workspace 查询仍属于后续读模型切片。

## 能力覆盖矩阵

| 设计能力 | 当前状态 |
|:---|:---|
| Admin/System 创建、查询和列出 JSON 设置项 | covered |
| `expected_version` 乐观并发更新 | covered |
| 普通用户管理入口拒绝 | covered |
| 设置变更与 `PlatformSettingChanged` Outbox 原子提交 | covered |
| Cluster 状态投影 | covered |
| 平台统计投影 | missing |
| Admin Workspace 查询 | missing |

P1 只在前四行全部 `covered` 后完成；Platform Administration 总 Domain 在后三行补齐前不得声明完成。

## HTTP API

- `POST /api/v1/admin/platform-settings`
- `GET /api/v1/admin/platform-settings`
- `GET /api/v1/admin/platform-settings/:setting_key`
- `PUT /api/v1/admin/platform-settings/:setting_key`

所有入口要求 JWT `role=admin|system`，或携带有效 Control 内部服务令牌并明确指定相同受控角色。设置 key 使用小写字母开头的点分/短横线命名；value 必须是最大 64 KiB 的有效 JSON。

## Harness 映射

- IDL：`idl/media_control/platform_administration.proto`
- Domain：`media_control/biz/domain/platform_administration`
- Repository：`media_control/biz/dal/repo/platform_administration`
- Service/Handler：`media_control/biz/service/platform_administration`、`media_control/biz/handler/platform_administration`
- Migration：`media_control/migrations/000017_platform_settings.*.sql`
- E2E：`media-cli/e2e/media_control_platform_setting_e2e_test.go`
