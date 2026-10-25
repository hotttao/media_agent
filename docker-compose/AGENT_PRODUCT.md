# 本机 Agent 服务与持久化目录

Agent 私有部署材料和工作区默认存放在 `/data/agent-product`，不进入 Git。准备脚本和 Compose 均支持通过 `AGENT_PRODUCT_DATA_DIR` 指定其他绝对路径；既有部署更改路径前需要迁移工作区并更新容器挂载。

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

从项目根目录运行。下面的启动命令使用本机已有镜像；Compose 同时保留从当前仓库构建镜像的定义。

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


## 构建镜像与 Agent 产品使用

从仓库根执行，需 Docker 权限、Python3、OpenSSL。准备命令沿用现有部署的服务凭据，生成持久 runtime 签名/Session 密钥、Plugin 加密凭据、创建服务策略与单个 Docker 环境，并创建 `media-plugin`、`media-agents` 数据库；不改变既有环境或 Session 绑定。前置 Control 应已完成 runtime_environments 迁移。

```bash
# 仅在初始化或明确更新部署配置时执行；已部署实例日常启动用上文命令。
# /data/agent-product 需要预先创建并授权当前用户写入。
python3 docker-compose/agent-product-local.py
export COMPOSE_PARALLEL_LIMIT=1
agent_product_env=/data/agent-product/product.env
docker compose --env-file "$agent_product_env" -f docker-compose/docker-compose.agent-product.yml build
docker build -f media_agents/codex/Dockerfile -t media-agent-codex:local media_agents
docker build -f media_agents/claude_code/Dockerfile -t media-agent-claude-code:local media_agents
docker build -f media_agents/goose/Dockerfile -t media-agent-goose:local media_agents
docker compose --env-file "$agent_product_env" -f media_auth/docker-compose.yml up -d --no-deps --build media_auth
docker compose --env-file "$agent_product_env" -f docker-compose/docker-compose.agent-product.yml up -d product-broker media_plugin runtime-controller agent-ui
docker compose --env-file "$agent_product_env" -f media_control/docker-compose.yml up -d --no-deps --build media_control
docker compose --env-file "$agent_product_env" -f media_agent_gateway/docker-compose.yml up -d --no-deps --build media_agent_gateway
```

Edge 路由使用既有动态配置挂载，当前 watch=false，修改后需执行 `docker restart media-agent-gateway` 加载；Oathkeeper rules 修改后需重启既有 Oathkeeper 服务。不要启动旧 Gateway Compose 中的固定 Codex/Claude 服务或其 host Broker；本产品 Broker 在 media-auth 网络中，由创建服务按 Workspace 启动 Agent。

`/data/agent-product/` 和 `media_agent_gateway/.secrets/` 为忽略提交的私有部署材料，product.env/策略/私钥0600，目录0700。Gateway 的 runtime_bootstrap volume 保管加密后的 Agent 凭据。保留这些目录、volume和密钥以支持正常重启；不要输出 env 或 Docker inspect Env 到 review/log。Auth 继续拥有凭据授权事实，Controller 只负责创建，Agent 配置仍来自 Control。

## 使用

以现有企业成员登录，创建带企业/部门的 Workspace。在资源库创建 Prompt、上传/编辑并发布 Skill、由企业管理员登记 MCP；到 Workspace 设置配置连接并明确确认固定版本，随后创建会话。模型 API Key 在 Workspace 表单中填写，只写不回显。模型 OAuth 登录应使用专用 Workspace 的原生运行目录，准备命令不会复制本机开发者登录。

无需认证、Bearer/API Key 和 OAuth 的远端 MCP 均通过 Gateway；OAuth 用户连接由 Auth 持有。外部服务地址需要从容器网络可达。新增企业 MCP 须满足 Gateway 现有上游地址允许规则。

此启动方式只覆盖本机既有部署；数据库地址/网络、Linux UID/GID 和 Docker socket group 由准备命令按当前环境生成。其他主机应使用其部署配置。扩缩容和后台凭据轮换不在当前功能范围。

模型服务需要出网代理时，在私有 product.env 设置 AGENT_HTTP_PROXY、AGENT_HTTPS_PROXY、AGENT_NO_PROXY，重新执行准备命令并重启创建服务后，新建 Agent 容器才会使用。代理地址必须从容器可达；宿主127.0.0.1不是容器宿主地址。NO_PROXY 应包含 Gateway、Broker、数据库及内部域名。不要把带凭据的代理地址提交仓库。

Control、Gateway 与 Agent 必须使用 product.env 中同一 MEDIA_AGENTS_RABBITMQ_URL（product-broker 内网地址）。Compose 允许显式覆盖旧部署的 host Broker 默认值；只检查历史回放不能证明实时 delta 与元数据通知已接通。

个人 Codex 登录可在“模型连接”页上传 auth.json，按用户加密存储并作为可选连接，不自动绑定 Workspace。创建或编辑 Workspace 时选择“个人 Codex 连接”。更新文件使用连接编辑入口；撤销后停止后续 claim。工作台不会读取开发者主目录作为隐式默认账号。

宿主代理仅监听127.0.0.1:7897时，可使用可选的正式本机转发服务（只监听Docker宿主内网地址，不读取凭据）：

```bash
docker compose --env-file /data/agent-product/product.env -f docker-compose/docker-compose.agent-product.yml --profile model-proxy up -d agent-model-proxy
```

在私有 product.env 配置 AGENT_HTTP_PROXY/AGENT_HTTPS_PROXY 为 http://172.27.0.1:17897，AGENT_NO_PROXY 包含内部服务与数据库地址，执行准备命令并重启 Controller。既有 Agent 需在空闲时重建以使用新部署配置；连接 auth.json 的上传和替换无需重建容器。

## Claude Code / Goose

创建或编辑 Workspace，Agent 选择 Claude Code 或 Goose，在模型服务里填写协议、模型名称、API 基础地址，再填写 API key。Claude Code 使用 Anthropic 兼容接口；Goose 支持 OpenAI / Anthropic 兼容接口。密钥加密保存且不回显；模型设置随 Session 固定，更新 Workspace 后新建 Session 使用新设置。

SiliconFlow 示例：模型名 `deepseek-ai/DeepSeek-V4.1-Flash`，Claude Code 使用 Anthropic 兼容协议、基础地址 `https://api.siliconflow.cn`；Goose 使用 OpenAI 兼容协议、基础地址 `https://api.siliconflow.cn/v1`。不要把 `/messages` 或 `/chat/completions` 填入基础地址。其他服务请使用其兼容协议及基础地址，不以运行客户端名称推断模型厂商。

两个 Runtime 使用 Control 已确认的 Prompt、Skill 固定版本，并仅通过 Gateway 发现和调用 Workspace 授权 MCP。工具请求在对话中等待用户选择，选择只允许本次时不会扩大到后续调用。Claude Code 原生用户提问映射为对话输入请求。网络断开不会隐式终止后端执行，刷新可读取持久历史。
