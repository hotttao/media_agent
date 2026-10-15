# Docker 功能联调规格

本机功能验证默认使用 Docker，k3s 保持关闭。Kubernetes 双 Ready/RWX 实测延期到独立存储环境，不阻塞 Docker 后续开发；保留已有严格测试，不把延期记作通过。

## Docker 交付边界

Docker 环境只要求创建单个 Agent 容器并可正常使用。扩缩容不实现；不额外建设 Docker 专属的生命周期管理或执行中崩溃接管机制，也不将此类验证作为后续功能开发的前置条件。现有单实例创建、身份校验及可用性链路保留；已完成的重启测试作为既有证据，不继续扩大 Docker 范围。

## 当前切片

使用真实 Docker Engine、Runtime Controller HTTP handler/生成 client、Gateway 实例登记/Session Routing/RunTurn/EventStream，以及容器内 Agent 生产 HTTP routes、所有权服务、数据库迁移和事件总线。验证单 Workspace/单 Agent 创建至会话执行的完整链路，不实现 Docker 扩缩容。

外部用户授权、Control 的固定上下文、付费模型执行使用明确 fixture；不读取本机真实模型密钥。生成容器的 native UID/generation/bootstrap proof 必须经过真实 Docker Controller 校验，不能沿用原 ownership 测试的 nativeFixture。

## 资源与隔离

- PostgreSQL/RabbitMQ 为独立测试容器，设置 CPU/内存上限；测试串行、Go 构建并行度2。
- Agent 由创建服务生成，使用批准镜像与 spec 内的 CPU/内存上限。按需构建镜像，不在每次测试重新拉取工具链。
- Fixture 镜像基于已有 Agent 依赖镜像覆盖当前源码，仅用于协议联调，不声称真实付费模型执行。
- Gateway/Controller 测试入口绑定宿主机可达地址，Agent 在独立 Docker network 中。宿主机 Gateway 的受限 Dialer 仅把当前受控 Agent DNS 映射到实际容器 IP；实际同网络部署使用 Docker DNS。
- 名称带阶段标识；清理仅处理本阶段创建的容器/network/目录。持久目录重启保留验证后随 fixture 清理。

## 验收

真实创建与稳定幂等；native 身份/错误 proof；Agent 登记及健康；签名会话写入、FIFO、双 SSE 本地广播及 durable replay；容器重启后身份代次变化、空闲 Session 重绑与持久文件保留；扩缩容返回501。断言使用业务状态和原生状态，不靠固定 sleep 判断执行完成。

阶段完成后输出 P8/review.md 并 commit。执行记录包含 fixture 范围、实际通过项、资源清理和后续限制。
