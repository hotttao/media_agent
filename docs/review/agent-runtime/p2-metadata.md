# P2 · 用户确认的元数据更新

状态：本阶段代码实现完成，等待用户确认后进入 P3。新增功能默认关闭；仅在隔离环境验证，没有更改已有部署或生产数据库。测试创建的 RabbitMQ/PostgreSQL 容器和临时数据库已删除。

## 已完成的行为

Control 的 Catalog 更新保留固定历史，但不会投递运行更新通知。用户确认完整资源版本向量后，授权和对象校验在事务外完成；同一事务 CAS 配置版本，保存不可变 Release 和 Outbox。重复确认返回已有目标，同幂等键不同内容或并发版本冲突409。选择旧资源产生递增的新选择序号，不把旧消息解释为回滚。

Gateway 每次向 Control 查询，不缓存其配置。稳定 Agent 委托与独立实例凭证共同验证当前配置/副本/代次；Gateway 到 Control 使用专用服务凭证。Platform Prompt 是独立正文，不并入 Configured Prompt。Skill 下载只允许已授权 Release 引用的 checksum，不接受任意来源 URL。

Agent 每个实例有自己的 RabbitMQ 临时队列，接纳提示后按实例串行拉取、准备和再次核对用户选择。完整目录原子发布；应用引用和待上报结果在同一 marker 原子提交。失败保留最后成功目录，上报失败独立重试，不重新安装；启动、重连、60秒对账修复漏通知。未知提示 kind 不影响完整拉取，未知必填资源或不兼容响应明确失败。

Prompt 正文分别落盘；Skill 校验 checksum、ZIP 安全约束及 SKILL.md frontmatter。Memory 写入固定描述文件，保留类型、状态、凭据引用及选定配置引用，不复制任意秘密配置。已有 Session 使用 metadata:config:target 固定引用，queued/running 与原生 Session handle 保持；恢复旧进程时仍加载旧文件。Control 加密保留执行配置历史，未确认的配置编辑不改变已确认的新 Session 版本。

Codex 使用业务 Session 专属 CODEX_HOME，分别加载平台 AGENTS.md、Configured developer_instructions 和固定 Skill；Claude 使用固定 append-system-prompt-file 与本地版本插件。工作 cwd 和共享 HOME 保留。

## 验证证据

| 检查 | 结果 |
|:---|:---|
| 独立 Control/Gateway PostgreSQL 数据库 + RabbitMQ 4.3 + 两个 Python 实例 | 通过：实际迁移、生成路由、HTTP 代理、Outbox 发布、实例各自消费、Skill 下载/版本文件和结果上报 |
| 发布失败/恢复、并发确认、幂等冲突、显式回滚 | 通过：未交付意图保留并沿用事件 ID；同 expected_version 只提交一次；回滚产生新选择 |
| 权限、旧代次与晚到结果 | 通过：错副本凭证/旧代次403；report_id 内容冲突409；晚到失败不撤销成功；历史成功不能完成新选择 |
| 历史执行配置与汇总 | 通过：未确认编辑不改变已确认配置；旧 Session 可读取历史；空/不完整成员集合不宣称成功 |
| Python 全量测试 | 101 passed / 6 skipped；真实 RabbitMQ 事件总线两项另外执行并通过，见下方命令 |
| 文件与 Runtime 场景 | 通过：失败保留旧版本、回报独立恢复、两实例共享产物、损坏文件拒绝、原生 ACP/Claude 子进程恢复固定 Prompt/Skill |
| 已安装原生 CLI | Codex 0.146.0 app-server skills/list 验证旧/新/恢复旧；Claude 2.1.185 本地插件 strict 校验通过；未调用模型 |
| Control/Gateway go test 与 arch-check | 全量通过（GOMAXPROCS=4、go test -p 1） |
| hertz_infra messagebus/metadataartifact 编译检查 | 通过（独立 module，GOWORK=off） |
| Python ruff、格式、跨包层级与新增元数据 mypy | 通过；新增核心及校验脚本9个源文件无类型错误 |
| 22场景业务模型、Compose config、Go 格式和 diff 空白检查 | 通过；模型验证不替代实际集成 |

真实集成入口：`harness/tools/agent_metadata_integration/metadata_test.go`。使用 `AGENT_METADATA_TEST_DSN` 与 `MEDIA_AGENTS_TEST_RABBITMQ_URL` 执行 `cd harness && go test -p 1 -v ./tools/agent_metadata_integration`；测试创建并删除独立随机数据库。Auth/Team 端口使用测试身份，实例事实及凭证摘要预置于测试数据库；没有调用生产注册或工作负载控制器。

在 media_agents 下执行 `uv run python scripts/metadata_native_check.py` 可重现原生 CLI 配置发现/插件校验；需要本地已安装 Codex 和 Claude。`MEDIA_AGENTS_TEST_RABBITMQ_URL=... uv run pytest -q adapter/test_session_event_bus.py` 验证真实 Broker 事件总线。Protocol fixture 测试不需要付费模型调用。

宿主机负载导致一次原生 CLI 初始化超过20秒，单独执行45秒上限检查后通过；没有放宽业务断言。原生本地 CLI 验证不代表已验证 Dockerfile 固定版本镜像，镜像完整部署测试仍在 P6。

## Harness 与阶段边界

Python 全量 mypy 仍为56条既有错误，逐条归一化比较 P1 无新增；import-linter 仍为13 kept / 1 broken，旧 ACP Domain 引用 shared.clock/shared.id 尚未清理。vulture 为106条，P1 为105条，多出的1条是新 Pydantic MetadataConfig.model_config 的动态框架误报，其余相同。完整门禁仍需在 P6 处理，不能把这些检查写成已通过。

P2 读取已验证实例登记并验证凭证；生产注册和凭证签发属于 P3，不提供不核验资源的自注册捷径。Gateway 当前尚无 P5 原生期望成员集合，返回 membership_complete=false，Control 显示 waiting_instances；完整成员算法已用明确的双实例 fixture 验证，生产集合接入后才可显示全部成功。

本阶段交付用户确认/进度/上传 API，没有增加前端设置页面。完整 MCP/Plugin/Memory 服务接入、Platform Prompt 模板管理是 P4 对应工作；P2 的 applied 只表示已选产物准备完成，不代表外部工具服务健康。默认不开启，Control 对象根持久卷已配置；多 Control 部署需共享对象根。实例注册、Session 接管和执行 epoch 未宣称完成。

## 下一阶段

取得用户确认后进入 P3：Session 身份/Token、实例登记及凭证签发、稳定绑定与 CAS、执行 owner/epoch、排空与 queued 接管恢复。本阶段停点不继续修改这些能力。
