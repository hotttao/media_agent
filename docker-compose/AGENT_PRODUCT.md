# 本机 Agent 服务与持久化目录

Agent 私有部署材料和工作区存放在 `/data/agent-product`，不进入 Git。

| 路径 | 用途 |
| --- | --- |
| `product.env` | 服务凭据、镜像和运行路径 |
| `site.env` | 固定的浏览器公开地址、素材公开地址 |
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
docker compose --env-file /data/agent-product/site.env -p media-agent-auth \
  -f docker-compose/docker-compose.auth.yml \
  -f /data/agent-product/auth-services.override.json \
  -f /data/agent-product/network-overrides/media-agent-auth.json \
  up -d --no-deps login-ui consent mailpit
```

## 局域网固定网址

统一入口为 `http://mini-001.local:18080`，素材公开地址为 `http://mini-001.local:8333`。本机的 Avahi 服务已启用，会在当前局域网发布主机名。局域网客户端通过 mDNS 解析当前 IP，DHCP 地址变化不需要调整项目配置。不要在客户端 hosts 文件中把该名称写死到某个 IP。

`site.env` 保存 Compose 使用的 `ORY_PUBLIC_URL` 与 `MEDIA_STORAGE_PUBLIC_ENDPOINT`，示例见 `site.env.example`。Kratos 回跳/CORS、Oathkeeper URL 匹配规则及服务默认公开 URL 均使用同一主机名。

容器中的 `mini-001.local:host-gateway` 映射让服务通过宿主机网关访问公开 URL，无需容器自行解析 mDNS，也不依赖宿主机局域网 IP。容器之间继续使用 Docker 服务名。

Workflow、Materials、Hydra 等服务的部署覆盖保存在 `network-overrides/<Compose项目名>.json`；后续用对应服务原有 Compose 启动时，通过额外的 `-f` 参数加载并传入 `--env-file /data/agent-product/site.env`。这些覆盖保留既有环境配置，不能用默认 Compose 重建所有服务来替代。

本方案需要客户端与服务器位于支持 mDNS 的同一局域网。访客 Wi-Fi 的设备隔离或阻止组播会影响名称发现；这类网络应由路由器提供固定 DNS 名称。跨网络访问另行配置 VPN 或正式域名。

`product.env`、策略及迁移快照包含私有凭据，保持仅所有者可读。不要将这些内容粘贴到日志或提交到仓库。
