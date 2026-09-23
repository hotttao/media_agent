# Organization Materials Library CRUD Plan

该计划横跨 media_auth、media_materials 与逻辑 media_storage 基础设施。

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

## P4 — S3-compatible media storage

- [x] 用 `media_materials/biz/shared/objectstorage` 的 S3 Storage Port 替换自研 media_storage HTTP client。
- [x] 接入 AWS SDK for Go v2，实现 PutIfAbsent、Stat、Delete 与 PresignGet。
- [x] 保留确定性 Object Path；用条件 Put 和 checksum metadata 保持幂等冲突语义。
- [x] 新增 SeaweedFS `media-storage` Compose，使用本地持久化目录、独立凭证和 bucket。
- [x] 暴露 SeaweedFS S3 数据端口供 Presigned URL 直读；对象字节不经过 media_materials 或业务 API 网关。
- [x] 实现 `library-objects:resolve`：Organization/path 授权后返回五分钟 Presigned URL。
- [x] 前端先 resolve 再直接加载 Provider Presigned URL，不使用 media_materials 文件代理。
- [x] 删除自研 LocalFile Driver、对象元数据库、签名算法和 media_storage Hertz 运行入口。
- [x] 提供 SeaweedFS、MinIO、Aliyun OSS、AWS S3 的配置示例；业务代码不得按 provider 分支。

## P5 — Verification

- [x] Domain/Service 单元测试。
- [ ] PostgreSQL Migration/Repository 集成测试。
- [x] S3 adapter 条件写、幂等、冲突、not found 与 presign 测试，并通过真实 SeaweedFS 兼容性测试。
- [x] Library Object Upload workflow 与 Library Object Resolution handler 单元测试。
- [x] `go test ./...`、`go vet ./...`、`make arch-check`（media_materials、media_auth）。
- [x] 按 `harness/contributing/e2e.md` 增加经统一网关的 Go HTTP 上传 E2E（服务外部启停）。
- [ ] 启动联调环境后执行 Go HTTP E2E，并补齐/执行 media-cli 命令链路。

## Risks

- 旧表没有 Organization：开发数据使用 legacy 占位迁移，生产导入前必须提供真实映射。
- Organization 授权是跨服务读取：超时或 media_auth 不可用时拒绝访问，不能降级为信任请求参数。
- 上传与业务 CRUD 是两个请求；首期保留未引用对象，后续 Retention/GC 必须按回收期清理，不能永久遗留。
- S3 凭证必须 fail closed，并通过 Bucket/Prefix Policy 只允许 `media-materials -> materials/*`；不同环境不得复用根凭证。
- SeaweedFS、MinIO 与 Aliyun OSS 的条件请求和寻址细节存在差异，必须使用兼容性测试锁定所依赖的 S3 子集。

## Acceptance

- 两个 Organization 可创建相同 Scope/name 且互不可见。
- 同 Organization 内 Product 共享；Material 默认只显示 owner 自己，可显式显示其他有权素材。
- 三类资源只保存 object_path，不保存完整 URL。
- 浏览器能通过统一网关上传图片并取得稳定 object_path；相同 Idempotency Key + 内容可安全重试，不同内容冲突。
- 缺失/错误 S3 凭证或越权 Prefix 时 Provider 拒绝对象操作。
- resolve 返回短期 URL，Browser 可直接读取正确 Content-Type；URL 到期或被篡改后 Provider 拒绝。
- 第一阶段公开路由中不存在 Binding、Workflow、生成或发布入口。
