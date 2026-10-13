# P7 · Docker 最小创建服务与最终审查

状态：Docker 收尾按用户要求在 P6 提交后最后实施。核心代码开发完成，Kubernetes 双 Ready/RWX 环境验收仍保留为未通过项，详见 P6。

## 实现结果

- Docker Controller 仅提供受控单实例 Ensure/查询/原生身份校验。创建状态只保留批准 spec、target version 和 revision；移除 pending/drain 状态及后台 start/stop 协调循环。
- 同一目标版本/配置重复请求保持容器 ID、proof、持久目录和 revision；启动失败通过重复 Ensure 恢复，不创建第二容器。
- Gateway 和 Docker 创建服务拒绝扩缩容；replicas 非1以及 drain/pending 操作返回 HTTP 501 `not_implemented`。Gateway 在读取/写入原生资源或进行 Agent 排空前拒绝这些操作。旧 IDL 字段保留并标记 deprecated，不宣称实现。
- Docker native healthcheck/restart policy 承担原生运行机制；创建服务不探测 Agent 业务。身份接口仍可核验外部停止的精确 container generation，不能把 NotReady 当作终止证明。
- 修复生成 SDK 在错误时返回空 rawResponse 导致状态映射失效的问题：通过 SDK ResponseResultDecider 捕获 HTTP status，保留 404/409/501 语义，不解析错误正文或错误字符串。

## 验证证据

| 检查 | 结果 |
|---|---|
| 实际 Docker Engine | 通过；`/tmp/p7-docker-live.txt`，包括 health=true、唯一创建、重复请求、proof、外部停止与代次变化、目录与文件保留 |
| Hertz IDL/handler 501 | 五类不支持操作在没有 Engine 的测试中返回501，证明未接触原生创建/停止 |
| Gateway Docker adapter / workflow | 原地拒绝多副本/缩容/drain；原生服务404/409/501映射通过 |
| Go 全量 Gateway/Controller | 通过；`/tmp/p7-go-final.txt` |
| race | client、workflow、Docker service、HTTP handler 通过；`/tmp/p7-race3.txt` |
| 两模块 architecture check | 通过；`/tmp/p7-arch-{gateway,controller}.txt` |
| 独立 Docker Controller build | `GOWORK=off go build ./cmd/docker` 通过；`/tmp/media-p7-controller-docker` |
| 真实 Control/Gateway/Agent 元数据流水线 | 通过；`/tmp/p7-metadata-live.txt` |
| 生成一致性 | 用 hz/protoc 重生成 Container IDL 的 model/client/router/swagger，未手改生成文件 |

Docker 实测运行独立 Alpine 协议/健康 fixture，未调用真实付费模型。测试创建的容器及 network 已按标签/ID 清理。P6 的 PostgreSQL/RabbitMQ 测试容器在最终核查后清理，未操作原有服务。

## 最终设计一致性

稳定 Agent 身份与执行代次分离；Session 固定版本、单 Session FIFO；双 Gateway 使用独立临时 Broker 队列与服务层广播；Gateway 统一 MCP 入口、每次重查权限、只生成 Platform Prompt 正文；Control 仍为配置事实源，明确用户确认才通知；Agent 保留 Runtime 转换/文件/加载及执行事实。Kubernetes Controller 只协调资源，Gateway/Agent 决定业务排空；Docker 保持单实例创建服务。上述能力的设计、IDL、实现与验证映射见阶段索引及覆盖矩阵。

未将 NFS 镜像下载失败或 Pending Pod 算作通过；实际双 Pod Ready/RWX 跨 Pod 文件验收仍须在可用存储环境完成。当前全部开发提交可统一 review，但不能宣称生产部署门禁全部满足。
