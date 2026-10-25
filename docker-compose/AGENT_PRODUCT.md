# 本机 Agent 服务与持久化目录

Agent 私有部署材料和工作区存放在 `/data/agent-product`，不进入 Git。

| 路径 | 用途 |
| --- | --- |
| `product.env` | 服务凭据、镜像和运行路径 |
| `runtime-policy.json` | Controller 策略 |
| `runtime-public-keys.json` | Agent 身份验证公钥 |
| `runtime/` | Agent 工作区、身份令牌、会话文件及运行状态 |
| `.data-migration/` | 迁移前容器配置及私有配置备份 |
| `network-overrides/` | 本机地址变更后的 Compose 环境覆盖 |

原位置 `/home/tao/code/github/media_agent/docker-compose/.agent-product` 仅保留指向新目录的兼容软链接。运行中的 Agent 和 Controller 直接挂载 `/data/agent-product`。数据库和对象存储各自保持原有目录或 Docker volume；备份业务 PostgreSQL 不能代替备份 Agent 工作区。

## 启动

从项目根目录运行。此文件使用本机已有镜像；Agent 源码及完整镜像构建流程位于 `agent` 分支。

```bash
docker compose --env-file /data/agent-product/product.env \
  -f docker-compose/docker-compose.agent-product.yml \
  up -d --no-build --no-deps runtime-controller media_plugin agent-ui

docker compose --env-file /data/agent-product/product.env \
  -f docker-compose/docker-compose.agent-product.yml \
  --profile model-proxy up -d --no-build --no-deps agent-model-proxy
```

Broker 使用已有的外部 volume `docker-compose_product_broker`。已有四个 Agent 实例由 Controller 管理，不作为 Compose 的固定服务重复启动。恢复其他机器时还需备份 Gateway 的签名/加密密钥及其持久化 volume。

认证辅助服务使用已保存的私有环境覆盖，避免覆盖已有配置：

```bash
docker compose -p media-agent-auth \
  -f docker-compose/docker-compose.auth.yml \
  -f /data/agent-product/auth-services.override.json \
  -f /data/agent-product/network-overrides/media-agent-auth.json \
  up -d --no-deps login-ui consent mailpit
```

当前本机地址为 `192.168.124.66`。Workflow、Materials、Hydra 等服务的地址覆盖保存在 `network-overrides/<Compose项目名>.json`；后续用对应服务原有 Compose 启动时，通过额外的 `-f` 参数加载。网络变更时同步检查 Kratos 回跳/CORS、Oathkeeper 匹配规则、Hydra URL 和素材公开地址。

`product.env`、策略及迁移快照包含私有凭据，保持仅所有者可读。不要将这些内容粘贴到日志或提交到仓库。
