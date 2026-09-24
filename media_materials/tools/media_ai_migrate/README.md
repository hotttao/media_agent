# media_ai 全量迁移工具

该工具把旧 `media_ai` 的 `materials`、`movement_materials`、`virtual_ips`、`products` 及其媒体迁移到
`media_materials`。目标端只使用公开 HTTP API，不直接写目标数据库。

迁移具有可重入性：图片使用由来源记录和字段生成的稳定 `Idempotency-Key`；实体通过
`metadata.migration.source_id`（Virtual IP 通过名称兜底）匹配后更新。任务中断后可以直接重跑。

```bash
cd media_materials
export MEDIA_AI_DATABASE_URL='mysql://user:password@host:3306/media_ai'
export MEDIA_MATERIALS_ORGANIZATION_ID='<organization-id>'
export MEDIA_E2E_KRATOS_SESSION_COOKIE='<ory_kratos_session value>'

go run ./tools/media_ai_migrate --dry-run
go run ./tools/media_ai_migrate
go run ./tools/media_ai_migrate --only virtual-ip # 仅重试虚拟形象
go run ./tools/media_ai_migrate --only movement   # 仅迁移动作素材
```

可选环境变量：

- `MEDIA_AI_ASSETS_URL`：旧图片服务，默认 `http://192.168.2.38`
- `MEDIA_MATERIALS_URL`：目标统一网关，默认 `http://127.0.0.1:18080`
- `MEDIA_MATERIALS_SCOPE`：目标 scope，默认 `ai_clothing`
- `MEDIA_MATERIALS_BEARER_TOKEN`：也可替代 Kratos cookie 使用 Bearer token
- `MEDIA_MATERIALS_SESSION_COOKIE_NAME`：会话 cookie 名，默认 `ory_kratos_session`

动作素材映射为 `asset_kind=movement` 的 Material，旧 pose 关联 ID 保存在
`metadata.legacy_pose_ids`。正式 pose → movement Compatibility 接口尚未实现，
因此它们目前不能作为正式兼容关系使用。

单个媒体文件超过目标服务当前的 16 MiB 限制时，工具会报告对应来源记录并以非零状态退出；
已成功的记录保持可重入，修正图片后重跑即可。
