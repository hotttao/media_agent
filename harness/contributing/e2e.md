# E2E 测试

## 测试分工

| 测试类型 | 位置 | 驱动方式 | 覆盖 |
|---------|------|---------|------|
| Go HTTP E2E（需启动服务、打 HTTP） | `media-cli/e2e/` | `go test` + 裸 HTTP + 进程内 cookie jar（被测服务由 docker-compose 起） | 可重复执行的接口断言、异步收敛和错误分支 |
| media-cli 命令 E2E | `media-cli/cmd/<service>/` | 连续执行 `media-cli <service> <command>`，使用落盘 Cookie 跨进程复用登录态 | CLI 命令树、参数映射、生成 client、中间件和真实用户操作链路 |
| 接口无关测试（domain 逻辑/算法/迁移） | `<app>/e2e/` | go test + testcontainers | domain 业务逻辑、事务、纯算法 |

Go HTTP E2E 与 media-cli 命令 E2E 是两道独立门禁，不能互相替代：`media-cli/e2e/` 中的测试通过，不代表对应 CLI 子命令存在或可用；CLI 命令执行成功，也不能替代 Go 测试对失败、幂等和并发分支的断言。

## 启动流程（4 步）

```bash
# 1. tearup 基础服务（按需起 infra 子集，dev=只起 pg，local=全部）
INFRA=dev  docker compose -f docker-compose/docker-compose.yml up -d postgres
# 或全部 infra：
INFRA=local docker compose --profile infra -f docker-compose/docker-compose.yml up -d

# 2. tearup 被测试服务
INFRA=local docker compose -f media_auth/docker-compose.yml up -d media_auth

# 3a. 执行 Go HTTP E2E
MEDIA_AUTH_ADDR=http://localhost:8001 \
  go -C media-cli test ./e2e -run TestAuthE2E -count=1 -v

# 3b. 执行 media-cli 命令 E2E（cookie 跨进程复用）
MEDIA_AUTH_ADDR=http://localhost:8001 \
  go -C media-cli run . media_auth auth login --username alice --password secret123
MEDIA_AUTH_ADDR=http://localhost:8001 \
  go -C media-cli run . media_auth auth me
# 或 service 特有子命令（如 media-cli media_example 自己的命令）

# 4. teardown 被测试服务
docker compose -f media_auth/docker-compose.yml down
# 基础服务保留常驻（除非确定不再测试）
```

## docker-compose profiles 与 env_file

### 基础环境（`docker-compose/docker-compose.yml`）

- `postgres` 无 profile（默认启，单元测试/单服务调试必需）
- `otel-collector`/`jaeger`/`prometheus`/`loki`/`grafana`/`promtail`/`consul` 标 `profiles: [infra]`（默认不启，联调时按 profile 启）

### 切换 infra 子集

```bash
# 单元测试/单服务调试：只起 pg（INFRA=unit 用 .env.unit，关其他 infra）
INFRA=unit  docker compose -f docker-compose/docker-compose.yml up -d postgres
# 联调全部：起 pg + 所有 infra（INFRA=e2e 用 .env.e2e，开所有）
INFRA=e2e   docker compose --profile infra -f docker-compose/docker-compose.yml up -d
# 联合启动（基础 + 服务）：
INFRA=e2e   docker compose -f docker-compose/docker-compose.yml -f media_auth/docker-compose.yml up -d
```

### env_file 共享（`docker-compose/.env.<infra>`）

- `.env.unit`：只开 pg（其他 infra=false）
- `.env.e2e`：开全部 infra

被测服务 compose 引用共享 env_file：
```yaml
services:
  media_auth:
    env_file:
      - ../docker-compose/.env.${INFRA:-e2e}   # INFRA 切换 → 自动选 unit/e2e
    environment:
      SERVER_ADDR: ":8001"                    # 服务特有变量（不随 INFRA 变）
      MEDIA_AUTH_DB_DSN: postgres://...@host.docker.internal:5432/media_auth?sslmode=disable
      LOG_DIR: /media_logs/media-auth
    volumes:
      - ../media_logs/media-auth:/media_logs/media-auth   # 日志映射 host（grep debug）
    extra_hosts:
      - "host.docker.internal:host-gateway"             # 容器内访问 host pg
```

## 文件组织

```text
docker-compose/
  docker-compose.yml              # 基础环境（pg + infra services with profiles）
  .env.unit                         # INFRA=unit：只开 pg
  .env.e2e                          # INFRA=e2e：开全部 infra
media_auth/
  Dockerfile
  docker-compose.yml                # media_auth 独立 compose（build + env_file + ports + volumes）
media_example/
  Dockerfile
  docker-compose.yml                # 同上
media-cli/
  cmd/<service>/                    # media-cli 命令 E2E 入口（落盘 cookie 跨进程复用）
  e2e/                              # Go HTTP E2E（裸 HTTP + 进程内 cookie jar）
```

## 关键约定

### 通过统一网关使用 API Token

Ory 认证环境的接口测试使用预先签发的 API Token。业务服务接收网关交换的内部 JWT；测试应请求网关地址。
当前本地测试凭据保存在未跟踪的 `ui_workflow/e2e/.env` 的 `E2E_API_TOKEN` 中。

```bash
set -a
. ui_workflow/e2e/.env
set +a
export MEDIA_API_TOKEN="$E2E_API_TOKEN"
export MEDIA_GATEWAY_ADDR=http://192.168.2.41:18080
export MEDIA_AUTH_ADDR="$MEDIA_GATEWAY_ADDR"
export MEDIA_E2E_ORGANIZATION_ID=8afeb26b9df643952f4f816576eb2504

# HTTP 生命周期、scope 与组织隔离测试（不模拟登录）
go -C media-cli test ./e2e -run '^TestAPITokenGateway$' -count=1 -v

# CLI 跨进程调用（MEDIA_API_TOKEN 优先于 Cookie）
go -C media-cli run . media_auth auth me
go -C media-cli run . media_auth api-token list
```

CLI 提供 `api-token create/list/rotate/revoke`。创建和轮换要求 `--env-out` 指向新文件，Secret 只写该文件（0600），控制台只返回元数据。测试不会输出 Secret；`.env` 不得提交。旧的本地 Session 测试仍适用于没有启用 Ory 的独立服务环境。

### API Token 轮换恢复集成测试

`media_auth/e2e/api_token_rotation_test.go` 使用 testcontainers 的隔离 PostgreSQL；设置 `MEDIA_E2E_TALOS_URL` 后还会连接已由仓库启动的 Talos Admin API。测试为唯一临时 User Actor 创建 Key，不使用测试账号的既有凭据；结束后撤销自己创建的 Key。

```bash
rotation_talos_ip=$(docker inspect --format '{{(index .NetworkSettings.Networks "media-auth").IPAddress}}' media-agent-auth-talos-1)
MEDIA_E2E_TALOS_URL="http://${rotation_talos_ip}:4420" \
  go -C media_auth test -tags=e2e -race ./e2e -run '^TestAPITokenRotation' -count=1 -v
```

这个测试让真实 Talos 提交轮换，然后在 transport 层丢弃响应并模拟清理不可用；重建 Workflow 后只用持久化操作恢复，验证旧 Key 与未知 replacement 均 revoked、不会重发 Rotate、已完成操作不被补偿。普通 `go test ./...` 不会连接 Talos。此测试与浏览器 E2E 串行执行。

### API Token 签发恢复上游契约测试

`media_auth/e2e/api_token_issuance_test.go` 为唯一临时 Actor 验证真实 Talos 的 request_id 重放规则，不使用既有测试账号的 Key：签发提交后丢弃响应，恢复只取得元数据；恢复先于原请求到达时也仅创建一个 Key；撤销后重放不能复活凭据，正常 Issue 不能再次取得 Secret。

```bash
issuance_talos_ip=$(docker inspect --format '{{(index .NetworkSettings.Networks "media-auth").IPAddress}}' media-agent-auth-talos-1)
MEDIA_E2E_TALOS_URL="http://${issuance_talos_ip}:4420" \
  go -C media_auth test -tags=e2e -race ./e2e -run '^TestAPITokenIssuanceTalosRecoveryContract$' -count=1 -v
```

未设置地址时该测试明确 skip。它只证明 Adapter 与锁定 Talos 的契约，不替代创建 Workflow 的数据库持久意图/CAS/重启恢复测试，也不替代 HTTP 与 CLI 生命周期门禁。

- **`media-cli` 不用管服务启停**——那是 docker-compose 的事。media-cli 只发 HTTP 请求。
- **两个 E2E 层级都要验收**——功能存在用户/管理 HTTP 入口时，既要运行 `media-cli/e2e` 的 Go HTTP E2E，也要实际执行对应 media-cli 命令链路；纯内部事件入口可以由面向用户的 CLI 流程间接覆盖，不要求暴露危险的内部命令。
- **基础环境常驻**（除非确定不再测试才停），避免每次 e2e 重复 build/起。
- **cookie 跨命令复用**：`media-cli media_auth auth login` 下发 cookie 落盘 `media-cli/.cookies/`，后续 `auth me`/`change-password` 自动带。
- **日志映射 host**：`LOG_DIR=/media_logs/<app>` + volume 映射 `../media_logs/<app>/`，便于 `grep` debug。

## 边界

- 不再提供 media-cli `e2e` 编排子命令（用 docker-compose 直管，更灵活）
- 接口无关测试（migration / 事务 / 纯算法）继续用 `<app>/e2e/` 的 go test + testcontainers
