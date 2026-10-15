# 003 Agent 架构落地计划

依据：docs/spec/agent-runtime.md。目标保持完整六项能力，不以通过局部测试缩减范围。

执行规则：每阶段完成后输出独立 review.md，记录能力、验证及遗留问题并 commit，然后自动进入下一阶段；全部完成后由用户统一 review。

## P0 · 设计与接口门禁

- [x] 核对当前代码、用户确定的职责与 Harness。
- [x] 建立能力覆盖矩阵，保留 Plugin/Auth 外部所有者。
- [x] 明确 Controller 仅创建资源，Docker 本期单实例。
- [x] 定义核心接口、身份/版本/错误、Broker 与创建资源契约。
- [x] 执行伪代码模型22场景，补齐迁移恢复和发布顺序。
- [x] 形成完整 spec。
- [x] DDD 全部新增/修改文档的引用、聚合/Workflow 门禁复核。

## P1 · RabbitMQ 替代 Redis

依赖 P0；验收：真实 Go/Python 发布消费、每 Gateway 一个队列、多本机 SSE、重连及回放。

- [x] 实现 Go RabbitMQ Source/本机广播与活跃绑定引用计数。
- [x] 实现 Python RabbitMQ SessionEventBus。
- [x] 修复 Source 不可用/断线时 SSE 订阅重建与回放降级。
- [x] 更新 typed config、依赖、Docker Compose，移除 Agent 事件链路的 Redis。
- [x] 执行跨语言 Broker 集成、Gateway/Agent Harness 检查；历史门禁问题如实记录。
- [x] 用户确认 P1 总结后进入 P2（用户已明确同意继续）。

阶段功能完成，验证及历史门禁问题见 [P1 审查](../review/agent-runtime/p1-rabbitmq.md)。P2/P3 已完成；用户已确认 P3，P4 已完成且用户确认继续；P5 的功能和阶段验收已完成，用户已授权自动进入 P6，并要求每阶段 review.md 与 commit。

## P2 · 用户确认的元数据更新

依赖 P1；验收：Catalog 修改不自动加载，用户确认才发布；旧 Session 文件保持。

- [x] 定义并生成 Control/Gateway 元数据 IDL。
- [x] 迁移 Catalog 历史、releases、Outbox、结果投影；本地事务无外部调用。
- [x] Gateway 查询代理与 artifact 下载，无控制配置缓存。
- [x] Agent 通知、串行应用、原子版本产物/待上报 marker、60秒对账。
- [x] 结果 API 与代次隔离、明确资源回滚、失败留旧版本。
- [x] 真实配置更新与原生 Runtime 固定版本加载验证。

- [x] 用户确认 P2 总结后进入 P3（用户明确继续）。

阶段实现与验证见 [P2 审查](../review/agent-runtime/p2-metadata.md)。实例登记/凭证签发由 P3 接入，工作负载完整成员集合由 P5 接入；当前未完整获取成员时不宣称全部成功。

## P3 · Session 身份、实例与接管

依赖 P2 的固定配置/凭据查询；验收：两个 Gateway 竞争不双执行，queued 接管保持 FIFO。

- [x] Session Token 与浏览器 Cookie 分离，密钥版本/Hash 迁移。
- [x] 实例登记、受控身份、绑定 CAS/transfer 意图。
- [x] Agent owner/epoch/admission 迁移及全部执行写入校验。
- [x] Scoped startup recovery、BeginDrain/GetDrain/Transfer/Adopt/Resume API。
- [x] queued 重取凭据，旧暂停保持；跨库结果不明查询恢复。
- [x] 故障/晚到事件/数据库并发与旧数据迁移验证。

阶段实现、真实测试及保留门禁见 [P3 审查](../review/agent-runtime/p3-ownership.md)。

- [x] 用户确认 P3 总结后进入 P4（用户明确继续）。

## P4 · MCP 与 Platform Prompt

依赖 P2/P3；验收：Agent只配置 Gateway，一致授权发现/调用，平台与用户 Prompt 分离。

- [x] MCP 目录迁移、IDL/管理入口、平台授权。
- [x] 标准 MCP 入口及上游 adapter，tools/prompts/resources 来源映射。
- [x] 发现分页/命名冲突、每次调用重新授权、结果不明不重试写。
- [x] Platform Prompt 模板/生成迁到 Gateway；Agent 只落盘加载。
- [x] 更新所有 Runtime 的 Gateway 客户端与配置。
- [x] 真协议服务器/授权撤销/多 Gateway/旧 Session 隔离验证。

阶段实现与验证见 [P4 审查](../review/agent-runtime/p4-mcp.md)。Plugin/Auth 仍为外部事实所有者；后续按用户授权自动推进。

- [x] 用户确认 P4 总结后进入 P5（用户明确继续）。

## P5 · 创建资源

依赖 P3/P4；验收：Controller只创建资源，Gateway提交manifest并直接与Agent通信。

- [x] AgentWorkload CRD、RBAC、StatefulSet/Headless/RWX/PVC Retain manifests。
- [x] 新 Controller 模块与 reconcile tests，禁止 Agent 探活/业务调用。
- [x] Gateway Kubernetes客户端、稳定资源名/冲突归属、Ready读取。
- [x] Gateway扩缩容/drain/queued重绑与CR目标更新。
- [x] Docker创建服务IDL/容器权限隔离/稳定幂等/单实例。
- [x] fake API与可用本地K8s/Docker真实资源测试，记录真实环境证据。

阶段实现和证据见 [P5 审查](../review/agent-runtime/p5-workloads.md)。本机 RWX provisioner 缺失，真实双 Pod Ready/共享文件待可用环境补验；已保留严格 live 分支，未把 Pending 当作通过。

- [x] 用户授权自动进入 P6；阶段完成后生成 review.md 并 commit。

## P6 · Kubernetes 与 Agent 主链路验收

- [x] Go各模块go test/arch-check/gofmt；Python全部Harness检查。
- [x] IDL/router/client/swagger/schema生成一致。
- [x] 集成夹具启动两个真实 HTTP Gateway 和两个 Agent，验证核心交互链（外部身份/模型使用明确 fixture）。
- [x] 六能力逐项设计实现review，覆盖矩阵无缺失。
- [x] 整理修改、验证证据与环境限制；用户接受当前结果，Kubernetes 实测另行延期。

阶段证据见 [P6 审查](../review/agent-runtime/p6/review.md)。Kubernetes 双 Ready/RWX 实测尚未完成；用户明确后续先使用 Docker，本机关闭 k3s，该项延期至具备可用 RWX 的独立环境，不阻塞后续 Docker 功能开发。

## P7 · Docker 最小创建服务收尾

用户要求 Docker Runtime Controller 放到最后，保持单实例创建/查询/身份校验，保留扩缩容接口但返回明确未实现；不实现 Docker 扩缩容及对应复杂协调。P5 已提交的 Docker 基线在此阶段审查并精简。

- [x] 审查并精简 Docker Controller 与 Gateway 适配。
- [x] 受控单实例创建、稳定幂等、身份与必要持久目录验证。
- [x] 扩缩容保留接口并明确返回未实现。
- [x] 最终总审查与 P7/review.md、commit。

阶段证据见 [P7 审查](../review/agent-runtime/p7/review.md)。核心开发完成，Kubernetes 存储环境验收保留为部署前门禁。

## P8 · Docker 完整链路功能联调

依据 [Docker 联调规格](../spec/agent-runtime-docker-validation.md)，串行、受限资源执行，保留每阶段 review.md 与 commit。

- [x] 停止本机 k3s 及其工作负载，保留持久数据。
- [x] 增加可复用 Docker Agent 协议镜像和单 Workspace 原生联调夹具。
- [x] 真实创建服务/Gateway/Agent 路由、FIFO/SSE/replay/重启持久性验证。
- [x] 必要缺陷修复及相关检查；P8/review.md 与 commit。

阶段审查见 [P8/review.md](../review/agent-runtime/p8/review.md)，复现命令见 `harness/tools/agent_ownership_integration/README.md`。

## 风险与对策

- 原生 Runtime 读取全局 Prompt/Skill：使用固定版本配置目录/参数并以真实 fixture 验证，不能用可选能力声明绕过隔离。
- 跨库迁移：persist transfer intent + Agent epoch +查询恢复，保留 queued原ID/顺序。
- Broker故障：事件replay、配置Outbox/对账、独立结果重试。
- 环境状态未知：不把NotReady当旧执行停止，现有unknown outcome不重放原则保留；Controller不承接业务判断。
- 外部Plugin服务当前未落地：保持受控契约和最终授权，Gateway不复制其Binding/OAuth事实；实际协议测试覆盖代理边界。
