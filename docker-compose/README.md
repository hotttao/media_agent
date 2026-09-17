# 本地 Compose 分组

默认不启动任何服务。编码和测试只运行各模块自己的测试命令。

## 分组

- `docker-compose.yml`：基础设施，仅 PostgreSQL。
- `docker-compose.auth.yml`：认证与鉴权服务的接入约定（Ory/业务认证独立部署）。
- `docker-compose.gateway.yml`：Traefik 统一入口，配置集中在 `traefik/`。
- `docker-compose.observability.yml`：OTel、Jaeger、Prometheus、Loki、Grafana、Promtail、Consul；全部使用 `observability` profile。

## 明确启动时

```bash
# 基础数据库
docker compose -f docker-compose.yml up -d postgres

# 统一网关
docker compose -f docker-compose.gateway.yml up -d

# 可观测性（只有明确需要联调/观测时才启动）
docker compose -f docker-compose.yml -f docker-compose.observability.yml --profile observability up -d
```

网关地址：`http://192.168.2.41:18080/`。

认证与鉴权：Ory Kratos/Hydra/Keto/Oathkeeper 的权威编排仍在
`/home/tao/code/github/ory-compose` 或 auth deployment；这里保留分组边界和网关路由，
不会在编码或测试时自动启动。当前业务服务的开发认证仍由 `media_auth` 提供，迁移到 Ory
需要在联调前显式启动并配置客户端回调地址。
