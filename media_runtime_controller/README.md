# Runtime Controller

Kubernetes 控制器只协调 AgentWorkload 关联的 StatefulSet、Headless Service、共享 RWX PVC 和 PV Retain。它不读取 Pod，不连接 Agent，不连接 Control 或业务数据库。Gateway 使用 Kubernetes SDK 提交 CR、读原生身份及 Ready，再直接调用 Agent。

Docker 创建服务通过 Unix socket 创建单实例容器；Gateway 只连接其受认证 HTTP API。容器启动/停止目标和排空进度保存在私有资源目录，原生 start/stop 的结果不明可从该目录对账。没有 Docker 副本扩缩容入口。

## Kubernetes 部署

1. 在 Control 配置 `agent_workload` profiles、driver、namespace、Gateway URL、RWX StorageClass；Gateway 配置相同环境的 `agent_runtime`，两侧启用 `AGENT_WORKLOAD_ENABLED`。
2. 应用 `deploy/crd.yaml`、`deploy/rbac.yaml`、`deploy/controller.yaml`。Controller 的批准镜像必须与 Control profile 一致。
3. 在 runtime namespace 配置 `media-agent-runtime` Secret，包含 `database-url`、`rabbitmq-url`、`runtime-public-keys.json`；为每个 Workspace 提供稳定名 `agent-<环境/Workspace hash>-identity` Secret，包含 `agent-api-token`。Token 通过已有 Agent Token 管理流程取得，Auth 继续拥有凭据事实；不要把模型 Provider Key 填入此处。
4. Agent ServiceAccount 没有 API 授权，关闭自动挂载，只有 Pod-bound 启动 token 的投影。Gateway/Controller 使用各自的 ServiceAccount。修改 RBAC 的 namespace 时同时修改对应 subject namespace。
5. 共享 PVC 没有删除 ownerReference；资源删除及缩容保留数据。RWX provisioner 必须真实可用，Pending 不会退回 RWO。Controller 会将绑定的独占 PV reclaim 设置为 Retain。

副本更新经 Control `PUT /api/v1/workspaces/{id}/agent-workload`，提交 replicas、expected_version、idempotency_key。首次目标可由 Gateway Session 分配或内部 Ensure 建立。Gateway 后台扫描 Control 的 durable target，不依赖一条配置通知必达。Gateway Scale 只执行匹配的 Control 目标，不能单独创造期望配置。

## Docker 部署

先构建/准备批准镜像，建立 Gateway 与容器可互通的 private Docker network。复制 `deploy/docker-policy.example.json` 到私有配置文件，填写数据库、Broker、批准镜像与 `AgentTokens[configuration_id]`；文件限制为创建服务可读。设置创建服务 credential，通过 `deploy/docker-compose.yaml` 启动。

`Root`、`PublicKeysFile` 在创建服务与 Docker host 必须采用相同绝对路径挂载。Root 为创建服务私有目录；容器 UID/GID 与创建服务创建文件的 UID/GID 一致，保证禁用额外 capabilities 后自身文件可读写。部署期间保持该 UID/GID 与目录所有权稳定。Gateway 不挂 Docker socket；creation credential 与 Agent bootstrap proof 是不同凭据。

默认 Docker 原生 healthcheck 通过镜像内的 Python 读取 `/health`；restart policy 为 unless-stopped。创建服务只读 Docker 原生状态，不调用 Agent HTTP。Engine API 在 1.41–1.45 范围内按服务端版本协商；低于 1.41 或要求高于 1.45 的服务端明确拒绝。

同名容器须通过 image、UID、归属标签、配置摘要、网络、持久目录及只读 proof/public-key 挂载校验。变化冲突返回409，不替换已有容器。停止保留文件和容器；再次运行使用新的 startedAt 作为原生执行代次。自动重启丢失旧代次的终止事实时返回未知，不推断旧执行结束。

## 验证

`go test ./...` 运行 fake Engine、client-go 资源协调与归属测试。显式设置 `MEDIA_RUNTIME_DOCKER_TEST=1 MEDIA_RUNTIME_DOCKER_REQUIRE_HEALTH=1` 验证独立真实容器与 healthcheck。Kubernetes 设置 `MEDIA_RUNTIME_KUBERNETES_TEST=1` 创建临时 namespace/CRD（已存在 CRD 不覆盖），验证真实资源及缩放目标；可用 RWX provisioner 另设 `MEDIA_RUNTIME_KUBERNETES_REQUIRE_READY=1` 验证双 Ready、PV Retain、逐 Pod DNS。全部资源清理限定本次创建的 UID/labels。

Runtime 镜像以 media_agents 为 build context，可分别构建 codex/Dockerfile、claude_code/Dockerfile、goose/Dockerfile；默认都保留 `/workspace/.home`。Goose 镜像固定 CLI 1.52.0，安装依据 [官方版本](https://github.com/aaif-goose/goose/releases/tag/v1.52.0) 与 [官方安装脚本](https://raw.githubusercontent.com/aaif-goose/goose/v1.52.0/download_cli.sh)。真实付费模型及生产镜像部署需按实际凭据与 RWX 存储环境验收；协议/创建 fixture 验证不能替代完整生产部署。

Docker 创建服务只接受单实例目标；扩缩容和 drain/pending 请求返回 HTTP 501 `not_implemented`。重复同一版本/配置保持容器、bootstrap proof、目录与 revision 不变，启动失败可重复 Ensure 恢复。服务不运行副本协调循环；普通运行失败交给 Docker restart policy。创建服务从不发出停止容器的业务指令，原生身份查询仍可验证外部终止的代次。
