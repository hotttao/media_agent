# P5 · 工作负载创建与资源协调

## 完成功能

Control 保存 Workload Target：批准资源快照、用户副本设置、生效副本数、Workspace/目标版本，以及事务内请求幂等记录。公开副本修改通过 Workspace manage 授权与 CAS；Gateway 只按 Control 当前目标执行。内部列表限 system 服务身份，后台对账无需依赖配置消息必达。

Gateway 使用 client-go 提交 AgentWorkload，读取原生 Ready/身份并直接调用 Agent。首次 Session 分配接入创建；扩容保持已有绑定，缩容将 operation/目标版本持久化到 CR，然后施加实例 admission 屏障、等待 running 和本机 flush 结束、转移 queued，再降低副本数。资源回收完成后退休登记并清除 operation；新原生代次可恢复 active，重复启动不清除同代次 draining。缩容到0逐 Session 核查执行事实；未知旧运行不推断完成、不重放。未完成转移继续阻止副本提交。

Kubernetes Controller 独立模块仅协调 StatefulSet Parallel、Headless Service、共享 RWX PVC 和绑定 PV Retain。没有 Pod 读取、Agent HTTP、Control 或业务数据库调用。kubelet/StatefulSet 管探针与副本；PVC 无删除 ownerReference。部署提供 CRD、分离 RBAC、Controller Deployment 和 CR 示例；Agent 无 Kubernetes API 权限，仅获得 Pod-bound 启动 token、公钥和自身稳定 API Token。

Docker Controller 独立 Hertz 服务持有 Unix socket，Gateway 只使用受 creation credential 保护的 RPC。批准镜像、稳定名称、配置摘要、归属、UID/GID、固定网络和受控挂载全部校验；持久目录/private proof 保留。原生 intent 的 start/stop 不明可对账恢复，单实例不开放副本扩缩容。120秒 stop grace 与 RPC 等待期限匹配。原生代次为 CID + startedAt；正面终止证明还检查非 restarting 与结束时间顺序。

自定义 session/metadata exchange 随批准 spec 交付到 Kubernetes/Docker Agent。元数据 membership 包含所有预期序号及缺失登记占位，扩容期间不会只根据已有报告误报全部应用。Control 类型/profile 覆盖 Codex、Claude Code 与 Goose；补充 Goose 镜像构建入口。

## 验证证据

| 检查 | 结果 |
|---|---|
| Control、Gateway、Controller 全量 Go tests | 通过 |
| Controller fake API | 原生三探针、共享 claim、Headless、幂等默认值、零副本保留、PV Retain/归属拒绝、custom exchange 通过 |
| Gateway native adapter | resourceVersion CAS、归属拒绝、保留 operator metadata/status/finalizers、持久 drain 通过 |
| 多 Gateway 排空状态机 / race | running/未知阻止缩容、未完成 transfer 阻止提交、后续目标恢复、退休序号重新登记、缺失 membership 通过 |
| 真实 Control PostgreSQL | 新 migration、目标 CAS 单赢家、请求幂等/不同 hash 拒绝、资源快照保留、分页通过 |
| 真实 Go/Python/Broker/数据库链路 | 两个 Gateway/两个 Agent，running 完成前保持旧数量、queued 保持原ID/顺序并转移、原生目标最后降低、退休登记；P3 的 lost response/签名/旧 epoch/原生 handle 回归通过 |
| 真实 Docker Engine | API 1.41 协商、创建幂等、native healthcheck、private proof、停止/重启代次、共享目录和 proof 保留通过；fixture 为独立 Linux 进程，未冒充模型调用 |
| 真实 Kubernetes API | 临时 CRD/namespace、Controller 创建 StatefulSet/Headless/RWX claim、API 默认值幂等、2→0→2目标、PVC UID 保留通过 |
| Python | 114项 Adapter/三个 Runtime测试 + 2项 MCP Runtime测试，合计116 passed；包含新代次 admission 真实数据库测试 |
| Harness | 三个 Go 模块 architecture check、Controller 两个独立 GOWORK=off 二进制构建、gofmt、git diff whitespace；Python ruff/format、143文件层级检查、启动/实例客户端定向 mypy 通过 |
| 生成与部署 | hz model/client/router、config protobuf、三侧 Swagger 已生成；独立模块加入 go.work，Controller Dockerfile/Makefile/部署示例齐全 |

真实测试只创建有测试标识的独立容器/网络/数据库和 Kubernetes namespace/CRD。清理使用本次创建的 UID/归属，不修改既有业务服务，不创建提交或发布。

## 环境限制与阶段边界

本机 nfs-nap StorageClass 的 provisioner 没有运行，真实 RWX claim 保持 Pending。双 Pod Ready、共享文件和真实已绑定 PV Retain 尚未在该集群完成；它们在 fake API 验证，完整 live 分支保留 `MEDIA_RUNTIME_KUBERNETES_REQUIRE_READY=1`，待 P6 的可用 RWX 环境补验。不将 Pending 解释成 RWO 可替代，也不宣称已通过完整 Kubernetes Agent 部署。

Agent 稳定 API Token 通过已有 Auth 管理流程获取，依部署私有 Secret/policy 交付；创建接口不自行签发委托凭据。已有 Workload 的 image/type/资源 profile 快照保持固定，配置差异返回冲突，需要受控重建；本阶段不隐式滚动打断正在执行的 Pod。Goose 与 Controller 镜像构建入口已提供，完整镜像部署启动属于 P6 验收。

P4 已记录的 Python 全量 mypy/import-linter/vulture 历史门禁仍未在此阶段清理；定向 mypy 通过不等于全项目类型检查通过。完整多服务部署、完整门禁及六能力总审查仍属 P6。本阶段完成后暂停，待用户确认。

## 复现入口

- `media_runtime_controller/kubernetes/{reconcile_test,live_test}.go`；live 默认验证资源/缩放，显式 REQUIRE_READY 验证可用 RWX 环境。
- `media_runtime_controller/docker/service_test.go`；`MEDIA_RUNTIME_DOCKER_TEST=1 MEDIA_RUNTIME_DOCKER_REQUIRE_HEALTH=1`。
- `media_agent_gateway/biz/workflow/workload_provisioning_test.go` 与 `biz/shared/client/kubernetes_workload_test.go`。
- `media_control/biz/dal/repo/runtime_environment/workload_integration_test.go`；`MEDIA_CONTROL_WORKLOAD_TEST_DSN` 指向隔离数据库。
- `harness/tools/agent_ownership_integration/ownership_test.go`；Gateway DSN 与 Python URL 使用同一隔离数据库，分别采用原生迁移 schema；每次 fixture 使用新 incarnation，防止重试错误复用排空状态。
- `media_agents/adapter/biz/dal/repos/runtime_session/test_ownership.py`。

本次日志：`/tmp/p5-go-broker-final.txt`、`/tmp/p5-final-race.txt`、`/tmp/p5-control-postgres3.txt`、`/tmp/p5-cross-language5.txt`、`/tmp/p5-docker-final.txt`、`/tmp/p5-kubernetes-resources.txt`、`/tmp/p5-python-all.txt`、`/tmp/p5-python-mcp-root.txt`。

原生资源契约依据 [Kubernetes StatefulSet](https://kubernetes.io/docs/concepts/workloads/controllers/statefulset/)、[StatefulSet API](https://kubernetes.io/docs/reference/kubernetes-api/apps/stateful-set-v1/)、[Docker Engine API](https://docs.docker.com/reference/api/engine/)。Goose 构建使用已验证的 [CLI 1.52.0](https://github.com/aaif-goose/goose/releases/tag/v1.52.0) 与 [官方安装脚本](https://raw.githubusercontent.com/aaif-goose/goose/v1.52.0/download_cli.sh)，不以可变化的 latest 安装脚本作为版本事实。
