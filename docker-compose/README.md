# 本地 Compose 分组

默认不启动任何服务。编码和测试只运行各模块自己的测试命令。

## 分组

- `docker-compose.yml`：基础设施，仅 PostgreSQL。
- `docker-compose.auth.yml`：当前仓库维护的 Ory 认证鉴权栈（Kratos、Hydra、Keto、Oathkeeper、Login UI）。
- `docker-compose.gateway.yml`：Traefik 统一入口，配置集中在 `traefik/`。
- `docker-compose.workflow.yml`：Conductor 工作流编排服务；只执行编排，不包含客户端 capability worker。
- `docker-compose.observability.yml`：OTel、Jaeger、Prometheus、Loki、Grafana、Promtail、Consul；全部使用 `observability` profile。

## 明确启动时

```bash
# 基础数据库
docker compose -f docker-compose.yml up -d postgres

# Ory 认证与鉴权（必须先于网关启动，以创建 media-auth 网络）
docker compose -f docker-compose.auth.yml up -d

# 统一网关
docker compose -f docker-compose.gateway.yml up -d

# 工作流联调（Conductor API 仅绑定本机 127.0.0.1:8080）
docker compose -f docker-compose.workflow.yml up -d

# 可观测性（只有明确需要联调/观测时才启动）
docker compose -f docker-compose.yml -f docker-compose.observability.yml --profile observability up -d
```

工作流执行时还需要启动两个不同信任边界的进程：

```bash
# media_workflow 内部任务：模板渲染与结果收敛
CONDUCTOR_URL=http://127.0.0.1:8080 go run ./media_workflow/cmd/internal_worker

# 客户端 capability worker；fixture profile 仅用于迁移联调
go run ./media_worker/cmd/media_worker \
  -conductor-url http://127.0.0.1:8080 \
  -profile media_ai_fixture
```

`media_worker` 不允许注册或执行 `media_render_template_v1`、
`media_finalize_step_v1`；这两个内部任务始终由 `media_workflow` 维护。

网关地址：`http://192.168.2.41:18080/`。

认证与鉴权配置全部位于 `auth/`，不再依赖外部仓库。Kratos 浏览器流程与 Hydra
Public API 经统一网关访问；Keto Write API、Kratos/Hydra Admin API 和 Oathkeeper
Decision API 仅在 `media-auth` Docker 网络内开放。

本地入口：

- 登录：`http://192.168.2.41:18080/login`
- 注册：`http://192.168.2.41:18080/registration`
- Hydra OIDC discovery：`http://192.168.2.41:18080/ory/hydra/.well-known/openid-configuration`
- Mailpit 仅在容器网络内运行，需要调试邮件时再显式增加受限端口映射。

本地开发账号由幂等 seed job 创建，可通过环境变量覆盖：

- 用户名：`admin@media-agent.local`
- 密码：`Media-agent-2026`
- 覆盖变量：`ORY_DEV_USER_EMAIL`、`ORY_DEV_USER_PASSWORD`

Traefik 对 `/api/auth/me`、`/api/v1/*`、`/api/users`、`/api/teams` 和
`/api/organizations` 使用 Oathkeeper ForwardAuth。Oathkeeper 校验 Kratos Session 后
签发 5 分钟 RS256 Internal JWT；业务服务从 `/.well-known/jwks.json` 获取公钥并验证
issuer/audience，再把 Kratos Identity 映射为 `media_auth.users.id`。本地启动业务服务需设置：

```bash
ORY_KRATOS_PUBLIC_URL=http://192.168.2.41:18080
INTERNAL_JWKS_URL=http://192.168.2.41:18080/.well-known/jwks.json
INTERNAL_JWT_ISSUER=http://oathkeeper:4456/
MEDIA_AUTH_INTERNAL_TOKEN=dev-media-auth-internal-token
KETO_READ_URL=http://keto:4466
KETO_WRITE_URL=http://keto:4467
```

`auth/oathkeeper/id_token.jwks.json` 只用于本地开发，包含私钥；生产部署必须改用
Secret/Vault 挂载并轮换，业务容器只能访问 Oathkeeper 发布的公钥。Keto 投影和按 route
权限 enforce 仍未完成，当前 Oathkeeper 只执行认证，不替代业务授权检查。
