# Organization Materials Library CRUD Plan

该计划横跨 media_auth、media_materials 与 media_storage。

## P1 — Contract and schema

- [x] 修改 Material、Virtual IP、Product IDL，移除第一阶段 Binding 入口。
- [x] 增加 Organization Membership 内部授权契约。
- [x] 添加向前/向后 Migration 与 Product Image 表。
- [x] 重新生成 Hertz model/router/handler 并通过编译。

## P2 — Domain and persistence

- [x] 三个聚合增加 organization_id 与新字段。
- [x] Material 实现 object_path、metadata、默认 only_mine。
- [x] Virtual IP 实现 Persona Profile 与身份图片引用。
- [x] Product 实现 Product Image、tags、external commerce 的聚合事务。
- [x] Repository 所有 Get/List/Update/Delete 反查或限定 Organization。

## P3 — Authorization and handlers

- [x] media_auth 增加内部 Organization Membership check。
- [x] media_materials 增加 fail-closed authz client。
- [x] Handler 从认证上下文取得 caller，禁止 body/query 覆盖 owner。
- [x] 统一 400/404/409/503 错误映射。

## P4 — Media storage local backend

- [x] 创建 media_storage module、Stored Object Driver 端口和 LocalFile Backend。
- [x] 使用 `harness/tools/new_server` 创建 media_storage Hertz 骨架、Health IDL、配置和 compose。
- [ ] 确认最终用户鉴权边界后固化 Stored Object HTTP IDL。
- [ ] 实现上传与读取接口，防止绝对路径和 `..` 逃逸。
- [x] compose 默认挂载可写本地目录；MinIO/OSS 只保留 Driver 扩展口。

## P5 — Verification

- [x] Domain/Service 单元测试。
- [ ] PostgreSQL Migration/Repository 集成测试。
- [x] media_storage LocalBackend 测试。
- [x] `go test ./...`、`go vet ./...`、`make arch-check`（media_materials、media_auth 与 media_storage）。
- [ ] 按 `harness/contributing/e2e.md` 增加 Go HTTP E2E 和 media-cli 命令链路。

## Risks

- 旧表没有 Organization：开发数据使用 legacy 占位迁移，生产导入前必须提供真实映射。
- Organization 授权是跨服务读取：超时或 media_auth 不可用时拒绝访问，不能降级为信任请求参数。
- 上传边界尚待确认；若采用上传与业务 CRUD 两个请求，必须处理未引用对象回收，不能把临时对象永久遗留。

## Acceptance

- 两个 Organization 可创建相同 Scope/name 且互不可见。
- 同 Organization 内 Product 共享；Material 默认只显示 owner 自己，可显式显示其他有权素材。
- 三类资源只保存 object_path，不保存完整 URL。
- 第一阶段公开路由中不存在 Binding、Workflow、生成或发布入口。
