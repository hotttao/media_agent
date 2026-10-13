# P1 · RabbitMQ 事件总线

2026-10-12。阶段功能完成；按用户要求暂停于阶段停点，未进入 P2。生产部署未替换，本次仅使用隔离测试容器。

## 已完成能力

- Agent 与 Gateway 的 Redis Session Event Bus 替换为 RabbitMQ；旧实现和运行依赖移除，统一 exchange/JSON/十六进制 UTF-8 路由键。
- 每 Gateway 一个 server-named exclusive auto-delete 实例队列，本机广播至 SSE；第一订阅建立绑定，最后订阅释放绑定。订阅在 binding ACK 后返回，随后开始回放。
- Broker 断线重建实例队列并恢复活跃绑定，本机订阅继续存活；首次订阅失败和本机通道关闭后，EventStream 周期重订阅。
- 保持 delta 的 sequence=0、不落库、不回放；持久事件的连续序号/去重/缺口回放和周期补查保留。慢订阅只关闭自己的接收者。
- Python 发布采用 confirms 和有界超时；无活跃接收者时允许 unroutable，生产 Agent 仅发布时不创建消费队列。
- typed Protobuf/YAML 配置及生成代码更新；Compose 使用 RabbitMQ 4.3，Broker 健康检查和 Agent/Gateway 连接变量更新。

## 验证证据

| 检查 | 结果 |
|:---|:---|
| 真实 RabbitMQ 4.3 + Python 发布 + 两 Go Gateway | 通过：四个本机订阅仅两个 Broker 队列；隔离 Session、delta、关闭单接收者、断线恢复、释放最后绑定 |
| Python 真实 Broker 集成 | 2 项通过：实例间广播、本机多订阅、robust 恢复绑定、无接收者发布 |
| Gateway 相关包 race 检查 | 通过：本机广播、连续回放、首次订阅失败/关闭后的恢复 |
| Gateway 全量 go test + arch-check | 通过 |
| hertz_infra 全量 GOWORK=off go test | 通过；此 module 不在根 go.work，按独立 module 检查 |
| Python 全量 pytest | 92 passed / 6 skipped；Broker 两项另在真实集成中运行，其余外部环境检查仍跳过 |
| Python ruff check/format + layer checker | 通过 |
| 本阶段新增 Python adapter/test 的 mypy | 通过 |
| Compose 配置、gofmt、git diff --check、修改 Markdown 引用 | 通过 |

测试容器使用随机本机端口，不替换已有部署。首次跨语言测试遇到管理接口超时，管理接口就绪后重新执行通过；未放宽生产协议或删去队列数量断言。

## 仓库既有门禁问题

在隔离目录提取 Git HEAD 的 media_agents，使用与当前代码相同的工具版本重新检查；不能把当前全量失败记成通过。

| 检查 | HEAD 基线 | 当前 | 差异 |
|:---|:---|:---|:---|
| mypy | 57 条错误 | 56 条错误 | 无新增；旧 Redis adapter 的无效 ignore 随替换消失 |
| vulture | 106 条报告 | 105 条报告 | 无新增；多数涉及动态 SDK/FastAPI/Pydantic 入口，仍需逐项判定 |
| import-linter | 13 kept / 1 broken | 13 kept / 1 broken | 相同：acp_adapter Domain 导入 shared.clock/shared.id |

基线错误集中在现有 ACP/Goose 类型、动态 SDK 接口与旧层级依赖。本阶段不扩大到这些模块的整体重构；P6 仍保留全量 Harness 门禁，不能据此宣布整个目标完成。

## 下一阶段

待用户确认后进入 P2：Control 确认元数据选择、不可变版本与 Outbox；Gateway 查询/结果代理；Agent 版本产物、通知应用和对账。P1 的完成不代表元数据通知链路已经实现。
