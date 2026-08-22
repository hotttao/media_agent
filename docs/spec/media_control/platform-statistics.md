# Platform Statistics 实现规格

## 边界

Platform Statistics 是可重建聚合读模型，不是计费账本。每条投影由 `metric_key + period_start + canonical dimensions hash` 唯一定位；基础设施聚合器使用 internal-system 身份上报，Admin/System 按指标和周期查询。

## 能力覆盖矩阵

| 设计能力 | 当前状态 |
|:---|:---|
| 指标、周期和维度复合幂等身份 | covered |
| internal-system 单调刷新 numeric value | covered |
| Admin/System 按指标和周期过滤查询 | covered |
| 普通用户查询和非内部上报拒绝 | covered |
| 迟到/重复 refreshed_at 不覆盖较新投影 | covered |

矩阵存在 `missing` 时不得声明 Platform Statistics 完成。

## HTTP API

- `POST /api/v1/admin/platform-statistics/report`：仅内部 system Principal。
- `GET /api/v1/admin/platform-statistics?metric_key=...&period_start_from=...&period_start_to=...`

时间字段使用 Unix 毫秒。`dimensions_json` 必须为最大 64 KiB 的 JSON object，服务将其 canonicalize 后计算 SHA-256；`value` 使用最多 30 位整数和 18 位小数的十进制字符串，避免浮点精度丢失。

## Harness 映射

- IDL：`idl/media_control/platform_administration.proto`
- Domain：`media_control/biz/domain/platform_administration`
- Repository：`media_control/biz/dal/repo/platform_administration`
- Service/Handler：`media_control/biz/service/platform_administration`、`media_control/biz/handler/platform_administration`
- Migration：`media_control/migrations/000019_platform_statistics.*.sql`
- E2E：`media-cli/e2e/media_control_platform_statistics_e2e_test.go`
