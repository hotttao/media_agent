# Docker-native Agent pipeline

从仓库根执行：

```bash
harness/tools/agent_ownership_integration/run-docker.sh
```

需要本机 Docker/Compose、Go 1.26，以及已准备的 Agent 依赖镜像 `media-agent-codex:local`。可用 `MEDIA_RUNTIME_DOCKER_PIPELINE_BASE_IMAGE` 指定其他兼容依赖镜像。脚本按锁文件同步 Python 依赖并覆盖当前源码；不重新安装模型 CLI，不使用真实模型密钥。首次准备基础镜像可执行 `docker build -f media_agents/codex/Dockerfile -t media-agent-codex:local media_agents`。

脚本创建独立 Compose project/network，串行启动 PostgreSQL（256Mi/0.5 CPU）、RabbitMQ（384Mi/1 CPU）；创建服务交付一个 Agent（256Mi/0.5 CPU）。Go 并行度2。默认只向 localhost 发布25432/25672，可分别用 `P8_POSTGRES_PORT`、`P8_RABBIT_PORT` 更换。初次启动 Broker 会等待原生 TCP listener；使用轻量探针，不为每次探针启动 Erlang VM。

联调使用真实 Docker Engine、Controller HTTP/生成client、Gateway 原生身份适配、签名 Agent HTTP 路由、数据库所有权/FIFO 与 RabbitMQ。外部 Auth/Control 上下文及模型执行是明确 fixture；不代表生产模型和配置更新部署。Gateway/Controller 在宿主机作为真实 HTTP 服务运行，Agent 在容器运行；测试 Dialer 只把受控 Agent DNS 解析到其原生容器 IP，同网络生产部署直接使用 Docker DNS。

验证稳定创建与 proof、登记与健康、重复 Turn 幂等、FIFO、同一 Gateway 两个 SSE、持久回放、原生重启后的新代次/空闲 Session 重绑与文件保留、旧代次拒绝写、Docker scale 返回501。不会创建或访问 Kubernetes 资源。

正常/失败退出均清理本项目的基础设施。Agent 清理须匹配本轮 bootstrap hash 和资源标签，已有同名容器会拒绝复用；保留预先准备的镜像以便后续运行。测试失败输出该 Agent 的有限日志。所有凭据均为隔离 fixture 的固定值，不能用于生产。
