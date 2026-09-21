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
- [x] 确认最终用户鉴权边界：浏览器经 media_materials 上传，media_storage 只接受服务身份。
- [x] 固化 Library Object Upload 与 Stored Object Internal HTTP IDL。
- [x] 实现 media_materials 上传 workflow、确定性 Object Path 与 Storage client。
- [x] 实现 media_storage PutObject 内部接口、服务凭证与 Namespace 限制，防止绝对路径和 `..` 逃逸。
- [x] 增加 Traefik 的 public upload 路由；内部 Storage API 不进入公共网关。
- [ ] 通用读取接口延后到资源级授权/短期 URL 方案，不开放按任意 object_path 读取。
- [x] compose 默认挂载可写本地目录；MinIO/OSS 只保留 Driver 扩展口。

## P5 — Verification

- [x] Domain/Service 单元测试。
- [ ] PostgreSQL Migration/Repository 集成测试。
- [x] media_storage LocalBackend 测试。
- [x] Library Object Upload workflow、Storage client、内部服务凭证与 PutObject service 单元测试。
- [x] `go test ./...`、`go vet ./...`、`make arch-check`（media_materials、media_auth 与 media_storage）。
- [x] 按 `harness/contributing/e2e.md` 增加经统一网关的 Go HTTP 上传 E2E（服务外部启停）。
- [ ] 启动联调环境后执行 Go HTTP E2E，并补齐/执行 media-cli 命令链路。

## Risks

- 旧表没有 Organization：开发数据使用 legacy 占位迁移，生产导入前必须提供真实映射。
- Organization 授权是跨服务读取：超时或 media_auth 不可用时拒绝访问，不能降级为信任请求参数。
- 上传与业务 CRUD 是两个请求；首期保留未引用对象，后续 Retention/GC 必须按回收期清理，不能永久遗留。
- 共享内部凭证是首期服务身份实现，必须 fail closed 且只允许 `media-materials -> materials/*`；生产可替换 mTLS。

## Acceptance

- 两个 Organization 可创建相同 Scope/name 且互不可见。
- 同 Organization 内 Product 共享；Material 默认只显示 owner 自己，可显式显示其他有权素材。
- 三类资源只保存 object_path，不保存完整 URL。
- 浏览器能通过统一网关上传图片并取得稳定 object_path；相同 Idempotency Key + 内容可安全重试，不同内容冲突。
- media_storage 内部 PutObject 缺少/错误服务凭证或越权 Namespace 时拒绝访问。
- 第一阶段公开路由中不存在 Binding、Workflow、生成或发布入口。
