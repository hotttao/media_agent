# P3 · Session 身份、实例登记与执行接管审查

日期：2026-10-13。阶段范围依据 [落地计划](../../plan/agent-runtime.md) 和 [执行归属规格](../../spec/agent-session-ownership.md)。本阶段完成后暂停，P4/P5 尚未开始。

## 完成的功能

- Session 凭证与浏览器 Cookie 分离：持久 MAC 密钥环、key_id、摘要保存和确定性重建；并发首次分配通过数据库 CAS 收敛。密钥轮换保留旧 key_id，可重建历史会话凭证。
- 两步实例登记：稳定 Agent 委托、Control 环境授权、原生资源身份核验；固定首次解析的 UID/generation，登记 CAS 与历史同事务保存，同 generation 不同 incarnation 拒绝。登记响应丢失可重建同一实例凭证，重注册保留 draining。
- Gateway 托管路由：有效已有绑定保持；首次 rendezvous 选择与持久化 CAS；transferring 意图、稳定 transfer_id、事实查询恢复、目标失效重新选择。未创建 Agent 行的登记窗口通过唯一主键和 epoch+1 创建协调，不从超时猜测不存在。
- Agent Session 执行归属：Workspace/config/replica/incarnation 与 execution_epoch，所有执行写入和 Turn claim 校验；PostgreSQL 屏障拒绝旧代码和错误 owner/epoch，固定上下文不能被改写。启动恢复只扫描当前 scope，不终结其他副本或旧代次。
- Session Quiesce、实例 BeginDrain/GetDrain、Transfer、Adopt、显式 Resume：单会话移交只阻止该 Session 领取下一条，其他 Session 继续；转移保留 FIFO、Turn ID、固定配置和 Runtime handle。running 仅在明确原生终止证据后 failed/outcome_unknown + paused，原 queued 保留，显式恢复不重放 failed Turn；重复恢复操作不重新打开后来失败的队列；运行中的恢复请求也记录幂等结果，避免旧重试在后续失败时意外生效。
- 托管请求使用短期 Ed25519 签名，绑定正文、method/path、实例与 epoch；独立事实查询使用 epoch 0，执行写入需要匹配执行代次。托管模式关闭旧 ACP 执行入口。live delta 携带 owner/epoch，Gateway 按已确认绑定过滤。
- 每次 Turn claim 经 Gateway 重取固定版本 Provider 凭据和稳定 Session Token；网络故障延迟重取、授权拒绝失败暂停，等待期间可中断，秘密不进入 Turn input。Chat 与 Agent Task 入口均接入托管路由。

## 验证证据

| 检查 | 结果 |
|:---|:---|
| Gateway 全模块 Go 测试、arch-check | 通过 |
| MAC 密钥轮换、作用域和签名 wire contract | 通过；Go 签名由 Python 直接验证 |
| 两个独立 Gateway 数据库连接 | 真实 PostgreSQL：转移 CAS 单一赢家、过期完成拒绝、并发凭证一致 |
| Agent 所有权数据库测试 | 5 项真实 PostgreSQL：FIFO/并发目标、旧 owner 和原 SQL 拒绝、未知运行暂停/恢复幂等、Session 屏障不排空副本、初始迟到登记、旧空闲接入与旧 running 拒绝 |
| 跨语言完整链路 | Gateway Hertz API + 两个 Python HTTP 实例 + 真实 PostgreSQL/RabbitMQ：受控登记、签名、live 通知、queued 重取凭据、丢转移响应后第二个 Gateway 恢复、目标代次失效/缺失登记恢复、原生 handle/上下文固定、旧回调拒绝 |
| 原生身份 adapter | client-go SDK fixture：Pod-bound TokenReview 受众、Pod 身份、StatefulSet/AgentWorkload 归属链；NotReady/NotFound 不证明旧运行结束，只有匹配旧容器 ID 的终止记录有效 |
| Python 全量测试 | `adapter codex goose claude_code`，真实 PostgreSQL/RabbitMQ 环境：119 passed，无跳过 |
| Python ruff、格式、跨包层级、新客户端/签名/联调脚本类型检查 | 通过 |
| IDL 与生成代码 | hz 生成 model/client/router，protoc 生成 Swagger；Python YAML/Pydantic 同步 |

跨语言联调使用确定性的 Native Runtime 与上游身份/环境端口 fixture，调用生产路由、领域服务、数据库迁移、客户端及 RabbitMQ adapter；它没有冒充真实模型调用、Kubernetes 集群资源创建或 Docker 创建服务部署。

复现入口：Gateway 的 `biz/dal/repo/agent_session/ownership_integration_test.go`、Agent 的 `adapter/biz/dal/repos/runtime_session/test_ownership.py`、Harness 的 `tools/agent_ownership_integration/ownership_test.go`。独立测试环境通过 `MEDIA_GATEWAY_OWNERSHIP_TEST_DSN`、`MEDIA_AGENTS_TEST_DB_URL` 和 `MEDIA_AGENTS_TEST_RABBITMQ_URL` 提供；Harness 启动脚本 `scripts/ownership_pipeline_server.py`。

## 阶段边界与已有门禁

P3 托管模式由 `AGENT_RUNTIME_ENABLED` / `AGENT_INSTANCE_ENABLED` 显式启用，默认关闭；需要已配置的稳定身份、持久 MAC 密钥、私有 Ed25519 签名密钥、Agent 公钥与原生启动身份。只给 Agent 交付公钥和自身启动证明。P3 已定义 Docker 原生身份客户端及创建服务 IDL；Kubernetes CRD/StatefulSet/Headless/RWX、Docker 创建服务、扩缩容调度仍属于 P5。MCP 会话凭证解析与完整 Platform Prompt 迁移属于 P4。

仅在新 MAC 密钥配置生效时签发独立 Session 凭证；未启用时兼容执行接口，不继续用浏览器 Cookie 填充 Session Token。没有修改已部署服务、运行生产迁移或创建提交。

Python 全量 mypy 仍为56条既有问题，逐项归一化与 P2 比较无新增。import-linter 仍为13 kept / 1 broken（既有 ACP Domain 引用 shared.clock/shared.id）。vulture 当前113项；与 P2 的106项相比新增9项动态框架误报（4个已被 HTTP 联调调用的路由函数和5个 Pydantic model_config），2项旧发现不再出现。完整历史门禁清理仍属于 P6，不能宣称全项目所有门禁通过。
