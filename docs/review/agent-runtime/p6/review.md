# P6 · Kubernetes 与 Agent 主链路审查

状态：主链路代码与严格检查通过；Kubernetes 双 Ready/RWX 环境验收未通过，原因与范围单列。Docker 最小化收尾移到 P7，按用户要求最后处理。

## 完成内容

- ACP SDK 回调和 MCP/content union 类型对齐；会话依赖结构契约，Clock/draft ID 移入 ACP 自有包。单队列快照保持 FIFO，重连测试保持通过。
- Python pre-commit 覆盖整个 adapter；死代码检测统一到 60% 阈值，动态回调/字段/兼容 API 使用显式逐项白名单。删除无调用的旧错误类型和 stub 响应类，修正测试替身不可达代码。
-  ownership 联调夹具使用两个真实 Hertz HTTP 监听、两套路由流程/数据库连接及独立 Rabbit 队列；两个 Python Agent 分别连接 Gateway。验证双 SSE 实时 delta、断线后 durable 回放、queued 迁移、排空后原生副本缩减、旧 owner 拒绝写入与丢失 transfer 响应恢复。
- 从 IDL 重新生成 model/client/router/swagger/config；修正生成 DTO 描述中旧的 instances:identity/register 路由，接口实现规格与实际 ensure/scale 路由对齐。

## 验证

| 检查 | 结果与证据 |
|---|---|
| Control/Gateway/Controller 全量 Go test | 通过；`/tmp/p6-go-{control,gateway,controller}.txt` |
| 三模块 architecture check | 通过；`/tmp/p6-arch-{control,gateway,controller}.txt` |
| Python ruff/format/mypy/lint-imports/check_layers/vulture | 143 源文件类型通过；14 contracts kept；显式动态表面白名单见 `media_agents/scripts/vulture_whitelist.py` |
| Python 全量真实 PostgreSQL/RabbitMQ | 116 passed、零跳过；`/tmp/p6-python-live.txt` |
| 两 Gateway HTTP / 两 Agent | 通过，包含 drain/queued 子场景；`/tmp/p6-two-gateways2.txt` |
| Control 目标 CAS 与幂等 | 独立 Control 数据库通过；`/tmp/p6-control-pg2.txt` |
| MCP Catalog 实际事务/Outbox Broker 与跨语言事件广播 | 通过；`/tmp/p6-gateway-live2.txt` |
| 抽象流程回归 | 22 场景通过；`python3 harness/tools/agent_runtime_model/verify.py` |

所有数据库/消息基础设施都是此次创建的隔离 fixture。Control 与 Gateway 使用独立数据库，避免共用 migration version 表；一次错误共库运行已废弃，以独立数据库复验结果为准。

## 六能力覆盖核对

| 能力 | 实现位置/既有审查 | 结论 |
|---|---|---|
| RabbitMQ 与恢复流 | P1 + EventStream + 本阶段双 HTTP SSE | covered |
| 明确确认后的元数据更新 | P2 + Runtime 原生版本固定加载 + 本阶段全部 Python 验证 | covered |
| Session 身份/实例/接管 | P3 + 本阶段真实双 Gateway/Agent | covered |
| Gateway MCP 入口与发现 | P4 + 实际 Catalog/Broker/HTTP SDK 测试 | covered；Plugin/Auth 事实 external-owner |
| Platform Prompt 正文与 Runtime 文件 | P4 + 三 Runtime 原生加载/隔离测试 | covered |
| 原生工作负载创建/扩缩容 | P5 + Controller/SDK tests | covered；Docker 单实例精简在 P7 |

详细迁移矩阵仍以 `../capability-coverage.md` 为准。模型 fixture 不代表真实付费模型执行，外部 Auth/Plugin 授权和原生资源身份在 ownership 联调中使用明确的协议 fixture。

## Kubernetes 环境门禁

现有 `nfs-nap` 的 NFS Service 地址已不存在。尝试独立 Namespace/StorageClass/RBAC 的官方 NFS Ganesha provisioner，使用 EmptyDir export，未修改现有服务或 PV。镜像 `registry.k8s.io/sig-storage/nfs-provisioner:v6.5` 被镜像代理拒绝（403）；改用 v4.0.8 后下载连接 reset，最终 ImagePullBackOff。因此本阶段未获得 RWX Bound、两个 Pod Ready 与真实共享文件证据；P5 的真实资源创建/2→0→2 intent/PVC UID 测试仍有效，但不能替代此项。

失败事件保存在 `/tmp/p6-kubernetes-nfs-blocker.yaml`；该 fixture 的 Namespace、StorageClass、ClusterRole/Binding 已删除。严格 live 分支保留，需具备可拉取镜像与可用 RWX provisioner 的部署环境执行。此环境限制保留为未完成验收项，不标记总体门禁全通过。
