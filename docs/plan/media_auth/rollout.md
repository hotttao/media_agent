# media_auth 落地计划 (plan)

> 状态：P1/P2 已落地；P3 已具备迁移工具与兼容投影，待生产迁移窗口；P4 已具备 RS256 双验与远程会话校验，事件化撤销缓存仍待评估
> 范围：按 [spec/media_auth/impl.md](../../spec/media_auth/impl.md) 把 control-plane 认证鉴权能力抽成独立 media_auth Go/Hertz 微服务。
> 规范依据（裁决优先级）：`harness/contributing/architecture.md`（层边界/import，冲突时优先）> `server.md`（服务端治理收口）> `idl.md`（IDL 约束）> `config.md`（配置 schema）。
> 领域模型：`docs/ddd/media_auth/`（CONTEXT / 6 domain / 4 workflow）；架构决策：`docs/ddd/adr/0001`、`docs/ddd/adr/0002`。

## 0. 已完成（本批次）

- [x] IDL 7 proto：`docs/research/media_auth/idl/media_auth/*.proto`（含 `token.proto`）
- [x] SQL：`docs/research/media_auth/sql/0001_init.sql`（mcp_oauth 宽限列已对齐 control-plane migration 117）
- [x] 领域模型：`docs/ddd/media_auth/`（CONTEXT + 6 domain + change + 3 service workflow）
- [x] 跨服务 workflow：`docs/ddd/workflow/0001-api-token-verification.md`
- [x] ADR：`docs/ddd/adr/0001`（token 校验契约）、`docs/ddd/adr/0002`（platform token 明文列）
- [x] spec：`docs/spec/media_auth/impl.md`（自包含）

> 注：上述 IDL/SQL 仍位于 `docs/research/media_auth/`（个人前期调研稿，非正式）。P1 第 1 步将其落到代码规范位置 `idl/media_auth/` 与 `media_auth/migrations/`，正式内容以 `docs/spec/media_auth/impl.md` 为准，research 稿保留作历史。

## 1. P1 — media_auth 独立 Go service 骨架与实现

### 1.1 workspace 接入
- [x] `go.work` 追加 `media_auth` module（`media_agent/media_auth`）
- [x] 创建 `media_auth/main.go` / `router.go` / `router_gen.go` / `Makefile`（提供 `hz-update`），参照 `media_example/`
- [x] `media_auth/conf/{local,dev,prod}.yaml`（spec §7），`.env` 放 `APP_ENV` + 覆盖变量

### 1.2 IDL 落位 + 代码生成
- [x] `docs/research/media_auth/idl/media_auth/*.proto` → `idl/media_auth/*.proto`（7 个，含 `token.proto`）
- [x] 仓库根 `make hz-model` → `hertz_gen/model/media_auth/<service>/`
- [x] 仓库根 `make hz-client` → `hertz_gen/media_auth/`（含 `token_service.go`，供 control_plane 调 VerifyToken）
- [x] `cd media_auth && make hz-update` → `biz/handler/<service>/` + `biz/router/<service>/`
- [x] `make swagger-gen`（`token.proto` 不进 OpenAPI）
- [x] `make arch-check --strict-idl` 校验 `go_package` 末段==service

### 1.3 数据层
- [x] `media_auth/migrations/0001_init.sql`（spec §4 全文，8 表 + `media_auth` schema）
- [x] `biz/dal/db.go`（postgres 连接，复用 `hertz_infra` 模式）+ `transaction.go`
- [x] `biz/dal/model/`（8 张表 GORM 行模型）
- [x] GORM Gen 生成 `biz/dal/query/`（🔴 禁改）
- [x] `biz/dal/repo/<domain>/`：实现 domain 仓库接口，行模型↔domain 转换

### 1.4 domain / policy / service / workflow（按 spec §5 / §6）
按依赖顺序（无依赖的先）：
- [x] `domain/identity/` + `service/identity/`（Identity 值对象、双唯一不变量、findUserIdentity/bindIdentity）
- [x] `domain/auth/` + `policy/auth/` + `service/auth/`（Session/User、JWT 签发/验签、Login 双路、ChangePassword、auth_source 推导只读 identity）
- [x] `domain/service_token/` + `policy/service_token/` + `service/service_token/`（Token 聚合、platform 唯一、ensurePlatformToken/getServiceTokenByHash/getPlatformToken）
- [x] `domain/wecom/` + `service/wecom/` + `workflow/wecom_login_bind.go`（corp token 缓存、authorize/callback、编排 identity+auth+service_token）
- [x] `domain/oauth/` + `policy/oauth/` + `service/oauth/` + `workflow/oauth_authorize_code.go`（Client/AuthCode/RefreshToken、PKCE S256、轮转、token-exchange introspect）
- [x] `domain/mcp_oauth/` + `policy/mcp_oauth/` + `service/mcp_oauth/` + `workflow/mcp_oauth_connect.go`（discover/authorize/callback、getValidAccessToken + 宽限算法、notifyAgentReload）
- [x] internal `service/token/` + `handler/token/`：VerifyToken 复用 `service_token.getServiceTokenByHash`；路由注册到独立 mTLS listener（`/internal/*`，不进公共网关）

### 1.5 handler 填充 + DI
- [x] `biz/handler/<service>/*_service.go`：请求绑定 → DTO→Command → 调 service/workflow → 响应（保持轻薄）
- [x] `biz/middleware/`：session 解析注入（cookie JWT 验签 + 滑动续期 + skip list）、`c.Redirect`/HTML 返回（wecom/mcp callback）
- [x] `InitDI()`：注入 service/workflow 到 handler（register 之前）

### 1.6 外部 client 装配
- [x] `biz/shared/client/`：LDAP client、企微 corp API client、RS introspect client、MCP metadata/token client、notifyAgentReload client（→ control_plane）；消费 `hertz_infra/clienthertz` 治理能力

### 1.7 P1 验收
- [x] `gofmt -l .` / `go test ./...` / `make arch-check`
- [x] e2e：6 service 关键路径 + VerifyToken RPC + mcp 宽限算法（参考 control-plane `e2e/{auth,service-tokens,oauth-provider,mcp-oauth}.test.ts`）

## 2. P2 — control_plane 接入 media_auth

- [x] control_plane 网关反代 4 前缀（`/api/auth` `/api/oauth` `/api/service-tokens` `/api/mcp-oauth`）→ media_auth
- [x] control_plane 中间件：Bearer service_token 可切换为调用 `VerifyToken` 内部 API，不再直读 `service_tokens`
- [x] control_plane 取 platform token 可切换为调用 media_auth 内部接口
- [x] 前端适配 identity 路径：`DELETE /api/auth/wecom/identity` → `DELETE /api/auth/identities/:provider`（破坏性）
- [x] `default_prompt_name` 由网关层补（media_auth 只返 `default_prompt_id`）
- [x] 灰度：4 前缀、service token、session、platform token 均有独立 feature flag，回滚可控

## 3. P3 — 数据所有权迁移

- [x] media_auth 接管 8 表（`media_auth` schema）
- [x] 存量数据迁移脚本（默认 dry-run，`--apply` 执行；保留 id，无主键变更）
- [x] control_plane 运行时认证链路可取消对 `service_tokens` 的直读
- [x] control_plane 保留 `users` 业务只读投影，远程 session 验证时按原 id JIT upsert，以维持历史业务外键与展示关联
- [ ] 在生产迁移窗口执行 dry-run、备份、apply、计数校验和灰度切换
- [ ] 生产稳定后清理 control_plane 已失效的认证写路径；历史 migration 不删除，保证旧环境可重放

## 4. P4 — JWT 非对称签名与即时撤销

- [x] media_auth 支持 RS256 私钥签名、JWKS 公钥发布和精确算法校验
- [x] control_plane 支持 JWKS 缓存与 RS256 本地验签，并可配置 HS256 双验期
- [x] control_plane 支持远程 session introspection；切换时可获得用户软删除/改密后的即时撤销语义
- [ ] 生产执行双验期（新旧皆可验）→ 切签 → 撤旧，并完成密钥轮换演练
- [ ] 评估 auth 事件消费者；在未实现前，不启用可能延迟撤销的边缘 session 缓存
- [ ] 评估 VerifyToken 短 TTL 本地缓存（撤销失效及时性）

## 5. 风险与对策

| 风险 | 对策 |
|---|---|
| `JWT_SECRET` 跨服务共享轮换不同步 | P1 起明确轮换流程；P4 迁非对称签名根治 |
| `identity` 路径破坏性变更 | P2 灰度 + 前端同步适配，回滚 flag |
| mTLS VerifyToken 内网暴露面 | 不挂网关/不出 OpenAPI；独立 listener；仅 control_plane 可达 |
| token-exchange 依赖外部 RS `/introspect` | 5s 超时；不可达返回 invalid_grant；不阻塞主流程 |
| mcp_oauth 明文存第三方 token | DB 访问受限；断开即删；审计 |
| platform token 明文列 | ADR-0002；code review 禁读非 platform 的 `token` 列；远期拆 `platform_tokens` 表 |
| auth_source 修正（wecom 账号 ldap→wecom） | P1 即修正；若前端依赖旧值需协调 |

## 6. 依赖关系

```text
P1.2 (IDL 生成) ──> P1.3 (dal) ──> P1.4 (domain/service/workflow) ──> P1.5 (handler/DI) ──> P1.7 (e2e)
                                                                              │
P1.6 (外部 client) ─────────────────────────────────────────────────────────┘
P1 完成 ──> P2 (control_plane 接入) ──> P3 (数据所有权) ──> P4 (非对称签名)
```
