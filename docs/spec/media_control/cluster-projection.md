# Cluster Projection 实现规格

## 边界

Cluster Projection 是可重建管理读模型。基础设施观察者只能使用有效 Control Service Token 和 `system` acting role 上报；Admin/System 只能查询，不能从用户请求体伪造观察者身份。

## 能力覆盖矩阵

| 设计能力 | 当前状态 |
|:---|:---|
| internal-system 上报 Cluster status/capacity/observed_at | covered |
| Admin/System 列表与单项查询 | covered |
| 普通用户查询和非内部上报拒绝 | covered |
| 迟到或重复 observed_at 不覆盖较新投影 | covered |

矩阵存在 `missing` 时不得声明 Cluster Projection 完成。

## HTTP API

- `PUT /api/v1/admin/clusters/:cluster_id/report`：仅内部 system Principal。
- `GET /api/v1/admin/clusters`
- `GET /api/v1/admin/clusters/:cluster_id`

状态受控为 `online/degraded/offline/unknown`，capacity 必须为最大 64 KiB 的 JSON object，`observed_at` 使用 Unix 毫秒且必须为正数。

## Harness 映射

- IDL：`idl/media_control/platform_administration.proto`
- Domain：`media_control/biz/domain/platform_administration`
- Repository：`media_control/biz/dal/repo/platform_administration`
- Service/Handler：`media_control/biz/service/platform_administration`、`media_control/biz/handler/platform_administration`
- Migration：`media_control/migrations/000018_cluster_projections.*.sql`
- E2E：`media-cli/e2e/media_control_cluster_projection_e2e_test.go`
