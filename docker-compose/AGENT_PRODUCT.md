# 本机 Docker Agent 产品

用于当前仓库已有 Auth/Control/Gateway、Ory Edge 和 PostgreSQL 的本机环境。Agent 创建为单容器；不要启动 k3s。原 Workflow 入口保留，新界面为 `http://192.168.2.41:18080/agent/`。端口5178仅提供界面，API 和登录请走18080。

## 准备与启动

从仓库根执行，需 Docker 权限、Python3、OpenSSL。准备命令沿用现有部署的服务凭据，生成持久 runtime 签名/Session 密钥、Plugin 加密凭据、创建服务策略与单个 Docker 环境，并创建 `media-plugin`、`media-agents` 数据库；不改变既有环境或 Session 绑定。前置 Control 应已完成 runtime_environments 迁移。

```bash
python3 docker-compose/agent-product-local.py
export COMPOSE_PARALLEL_LIMIT=1
agent_product_env=docker-compose/.agent-product/product.env
docker compose --env-file "$agent_product_env" -f docker-compose/docker-compose.agent-product.yml build
docker build -f media_agents/codex/Dockerfile -t media-agent-codex:local media_agents
docker compose --env-file "$agent_product_env" -f media_auth/docker-compose.yml up -d --no-deps --build media_auth
docker compose --env-file "$agent_product_env" -f docker-compose/docker-compose.agent-product.yml up -d product-broker media_plugin runtime-controller agent-ui
docker compose --env-file "$agent_product_env" -f media_control/docker-compose.yml up -d --no-deps --build media_control
docker compose --env-file "$agent_product_env" -f media_agent_gateway/docker-compose.yml up -d --no-deps --build media_agent_gateway
```

Edge 路由通过既有动态配置挂载加载；Oathkeeper rules 修改后需重启既有 Oathkeeper 服务。不要启动旧 Gateway Compose 中的固定 Codex/Claude 服务或其 host Broker；本产品 Broker 在 media-auth 网络中，由创建服务按 Workspace 启动 Agent。

`.agent-product/` 和 `media_agent_gateway/.secrets/` 为忽略提交的私有部署材料，product.env/策略/私钥0600，目录0700。Gateway 的 runtime_bootstrap volume 保管加密后的 Agent 凭据。保留这些目录、volume和密钥以支持正常重启；不要输出 env 或 Docker inspect Env 到 review/log。Auth 继续拥有凭据授权事实，Controller 只负责创建，Agent 配置仍来自 Control。

## 使用

以现有企业成员登录，创建带企业/部门的 Workspace。在资源库创建 Prompt、上传/编辑并发布 Skill、由企业管理员登记 MCP；到 Workspace 设置配置连接并明确确认固定版本，随后创建会话。模型 API Key 在 Workspace 表单中填写，只写不回显。模型 OAuth 登录应使用专用 Workspace 的原生运行目录，准备命令不会复制本机开发者登录。

无需认证、Bearer/API Key 和 OAuth 的远端 MCP 均通过 Gateway；OAuth 用户连接由 Auth 持有。外部服务地址需要从容器网络可达。新增企业 MCP 须满足 Gateway 现有上游地址允许规则。

此启动方式只覆盖本机既有部署；数据库地址/网络、Linux UID/GID 和 Docker socket group 由准备命令按当前环境生成。其他主机应使用其部署配置。扩缩容和后台凭据轮换不在当前功能范围。

模型服务需要出网代理时，在私有 product.env 设置 AGENT_HTTP_PROXY、AGENT_HTTPS_PROXY、AGENT_NO_PROXY，重新执行准备命令并重启创建服务后，新建 Agent 容器才会使用。代理地址必须从容器可达；宿主127.0.0.1不是容器宿主地址。NO_PROXY 应包含 Gateway、Broker、数据库及内部域名。不要把带凭据的代理地址提交仓库。
