# API Token 与 Worker Execution Target 落地计划

> 规格：[API Token 与 Worker Execution Target 实现规格](../spec/api-token-worker-execution.md)
>
> 领域依据：ADR 0017、ADR 0018，以及 `media_auth/api_token`、`media_workflow/execution_capability`、`media_workflow/execution`。

## P1 · Capability Target 与 IR

- [x] migration 增加 `execution_target`、`managed_pool_key` 和约束，旧数据回填 Personal。
- [x] Domain、DAL Model、Repository 映射 Target。
- [x] Semantic Compiler 快照并校验 Target。
- [x] Executable Workflow IR 冻结 Target，canonical JSON 保持确定性。
- [x] 单元测试覆盖非法 Target、Pool 约束和发布版本冻结。

## P2 · Personal/Managed Domain 调度

- [x] Job 启动按 Target 生成 Personal/Managed `taskToDomain`。
- [x] 检测相同 Task Definition 的 Domain 冲突，禁止 `NO_DOMAIN` 回退。
- [x] Worker Gateway 只接受 Personal Capability。
- [x] Dispatch 拆分 subject、actor、credential、instance 和 auth source。
- [x] 实现 Managed Worker 入口、配置 allowlist 和 Managed Domain poll。
- [x] 共用 lease、Result Inbox 与 Conductor result adapter。
- [x] 单元/集成测试覆盖双 Target 隔离与审计字段。

## P3 · Talos 基础设施与 Adapter

- [x] 按 Talos `v2alpha1` OpenAPI 锁定 Issue/Verify/Get/Rotate/Revoke adapter 契约；部署镜像版本仍需在接入 compose 时固定。
- [x] 在当前仓库增加 Talos OSS 配置和独立数据 volume。
- [x] 增加 `talos` typed adapter、配置 schema、超时、trace 和错误映射。
- [x] Adapter 使用 fake HTTP server 覆盖签发、验证、查询、轮换、撤销、超时和敏感信息不落错误。
- [x] 默认编码测试不启动 Talos；仅显式启动认证栈时运行。

## P4 · API Token 聚合

- [x] 新增 `api_tokens` migration、行模型、Repository 和领域不变量。
- [x] 新增 API Token IDL：create/list/rotate/revoke，以及 internal verify。
- [x] 实现 Talos Issue + 本地事务失败撤销补偿。
- [x] 实现 VerifyAPIToken，联查 User、Actor、Organization 和 scope。
- [x] 实现 revoke/rotate pending operation 与幂等恢复。
  - [x] revoke 使用持久 revoking 意图，后台分页恢复，并查询确认不确定结果。
  - [x] rotate 的持久操作、超时结果确认与未知 replacement Key 清理。
    - [x] migration、聚合操作实体、Token/操作事务 CAS。
    - [x] Talos metadata Rotate 与 Actor 分页 List adapter。
    - [x] 同步 Workflow、独立有界清理、后台重启恢复。
    - [x] 单元/PG 并发与失败测试，真实 Talos 丢失响应联调，HTTP/CLI 回归。
- [x] 生成 Hertz model/client/router/swagger，不手改生成产物。
- [ ] 单元、DAL、HTTP E2E 覆盖生命周期与失败分支。
  - [ ] 补齐创建时签发结果不确定、撤销补偿失败后的持久化恢复；不能只返回组合错误后丢失补偿意图。
    - [x] 独立 RecoverIssue Adapter：同 request_id 重放，仅返回元数据，严格核对 Actor/签发标识，单元与真实 Talos 契约测试。
    - [ ] issuing/issuance_cleanup、nullable Key、期限与客户端去重迁移和聚合 CAS。
    - [ ] Issuance Workflow、撤销未完成创建、后台重启恢复与 wiring。
    - [ ] PG 并发/失败与真实丢失响应恢复测试；部署后 HTTP/CLI/浏览器回归。

## P5 · 委托身份与 Personal Worker 切换

- [x] Edge Auth 把 API Token 交换为短时 Internal Identity JWT。
- [x] JWT 支持 RFC 8693 `act.sub`，并校验 audience、TTL 和 issuer。
- [x] Worker Gateway 使用 `sub=User`、`act.sub=Worker`，不再把 credential 当 Actor。
- [x] `media_worker` 配置和文案由 Service Token 改为 API Token。
- [ ] 换发现有测试 Worker Token，删除本地 Hash 验证旁路。
- [x] 为测试 User 签发受限测试环境 API Token，Secret 仅保存到未跟踪的 `.env`；HTTP/CLI E2E 支持 Bearer API Token，不再要求预先模拟登录。
- [ ] 覆盖撤销、scope、Organization、Actor disabled 和 trace_id 联调测试。

## P6 · Agent Token 申请与代理

- [ ] 定义 Agent API Client/代理 IDL，不与 Agent Session Token 混用。
- [ ] Agent 创建后由 Gateway 幂等调用 media_auth 创建 Actor Token。
- [ ] Gateway 只回传一次 Secret，不持久化 Secret。
- [ ] Gateway 验证 Agent Token、校验 owner 委托并代理内部服务。
- [ ] Agent disabled/deleted/owner 解绑触发 Token 撤销。
- [ ] E2E 覆盖申请失败重试、代理调用和撤销。

## P7 · 删除旧 Service Token

- [ ] 明确 Platform Token 独立存储和调用方后，删除 `service_token` domain/policy/service/repo。
- [ ] 删除旧 IDL、路由、生成代码与 `tos_` 本地 Hash 工具。
- [ ] 删除旧表或改为只读迁移归档；历史 migration 保留可重放。
- [ ] 更新 OpenAPI、CLI、配置、部署和所有文档引用。
- [ ] 运行全仓 `make arch-check`、相关模块 `go test ./...` 和 E2E。

## 实施顺序与提交边界

```text
P1 -> P2 -> P3 -> P4 -> P5 -> P6 -> P7
```

每个阶段单独提交。P1/P2 不依赖 Talos，可先完成并保持现有 Personal Worker 可用；P3/P4 完成后再切换凭证，避免半成品认证链路阻塞 Workflow 开发。P7 只有在所有调用方和测试完成切换后执行。

## 2026-09-28 联调记录

- 为测试用户签发组织受限的 `api.access` Token，Secret 保存到未跟踪的 `ui_workflow/e2e/.env`（`E2E_API_TOKEN`），文件权限 0600。
- 修正 Oathkeeper introspection 的容器内服务地址，并通过 `ext` 把 credential、actor、organization、auth source 传入 Internal JWT。
- 修正 materials 的 API Token 分支缺失组织授权上下文；组织受限 Token 只能访问所绑定组织。
- `TestAPITokenGateway` 通过：用户、workflow、materials、非法凭据、跨组织拒绝、创建/轮换/撤销、scope 不足。
- CLI `api-token create/list/rotate/revoke` 与 `auth me` 实际执行通过，Secret 只写显式指定的新 `.env` 文件。
- 撤销支持 `revoking` 状态重试及重复撤销；持久 pending operation、轮换失败恢复、Worker/Agent Token 和 UI 优化仍待完成，以上记录不代表 P4/P5 整体完成。
- Managed Worker 增加内部进程入口及显式启用 profile，仅允许发布的 Managed Capability/Pool，不携带 API Token。Provider 通过配置的内部执行器接入，未配置真实实现时保持关闭。
- 真实 Conductor + testcontainers PostgreSQL 联调通过领取、续租、模拟模型结果写入 Inbox、结果重放及执行完成；结果并发/失败重试另有数据库集成测试。
- 联调修复 Conductor OSS 返回原始 UUID 的解析、5 秒长轮询上限和 Inbox 幂等回查错误；同一次 attempt 不能同时接受 complete/fail。
- API Token 轮换的本地提交改为对旧 Key ID 和 active 状态做 CAS；PostgreSQL 测试证明不能复活 revoking/revoked Token，两个并发轮换只能提交一个。
- Talos 错误响应正文、任意 error_code 和异常字段不再进入错误信息；缺失 Secret 的轮换响应被拒绝。持久 rotation pending operation 与自动恢复仍未完成，不以这些加固代替该验收项。
- Vite 补齐 Recovery/Settings/Verification 等 Ory 页面和明确的静态资源代理；浏览器验证注册、稳定 User、独立 Session 登出、恢复入口通过。FlowGram 拖动/节点详情/进入编辑单独复测通过；Docker/testcontainers 操作与浏览器测试串行执行，避免 Chromium `ERR_NETWORK_CHANGED`。
- 撤销恢复后台已部署：持久 revoking、keyset 扫描、失败记录不阻塞后页、错误后查询确认 Talos 撤销终态。PostgreSQL 集成测试（含 race）验证进程对象重建后恢复、响应丢失、分页和终态不重放；单元测试覆盖本地提交失败、查询失败及 Key ID 不匹配。
- 更新认证服务后 `TestAPITokenGateway` 通过；CLI 实际执行创建、轮换、撤销、重复撤销，Secret 文件权限为 0600。企微登录测试原有异步状态读取竞态改用通道同步，相关 service/workflow race 测试通过。轮换持久恢复仍未完成。
- 轮换恢复随后完成：rotating + 依附操作记录、同步事务 CAS、metadata 定位 replacement、旧 Key 撤销屏障、分页清理和后台恢复已部署。服务/Workflow/Talos adapter race 测试、9 项 API Token PostgreSQL/真实 Talos集成测试通过；真实 Talos 提交后丢弃响应并阻断即时清理，再重建 Workflow 验证仅靠数据库恢复，所有本次 Key 均 revoked，且晚到 Rotate 被拒绝。
- 更新认证服务后网关 API 生命周期与 CLI 创建/轮换/撤销/重复撤销回归再次通过；未把创建补偿意图丢失问题、Worker/Agent 切换或 UI 整体验收并入该完成项。
- 轮换恢复部署后的浏览器回归 6 项通过：登录 setup、注册入口、稳定业务 User、独立 Session 登出、密码恢复、FlowGram 节点拖动/详情/结构化编辑。仅证明这些旅程仍可用，不替代完整视觉与执行交互验收。

## 2026-09-29 联调记录

- 创建恢复契约已通过真实 Talos 验证：签发提交后丢失响应，以及恢复先于原请求到达，均只产生一个 Key；撤销后相同 request_id 重放仍返回同一 revoked Key，正常 Issue 不重发 Secret。此处仅完成恢复 Adapter，不代表创建意图、后台恢复或 P4 已完成。
- 重启后发现临时 Vite unit 消失；改为仓库维护的持久 user-service 模板，启动脚本 link/enable，显式 stop 同时 disable。当前机器启用 Linger，让用户服务管理器随主机启动，不依赖终端会话；仍采用 Vite :5173 → Traefik 后端的开发拓扑。
- 验证记录：media_auth 全模块单元测试、Talos/API Token service race 与架构检查通过；真实 Talos 恢复契约两种场景通过。5173 返回 200、持久 unit enabled/active、Linger=yes；网关 API Token HTTP 生命周期与认证/FlowGram 浏览器回归6项通过。没有执行整机重启验证，不以 enabled 状态宣称已完成重启实测；创建 Workflow 持久恢复和完整 UI 验收仍未完成。
