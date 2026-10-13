# Agent Configuration 变更

## 2026-10-12 · DDD 收拢与核心接口模型验证

- 核心六能力与旧入口纳入覆盖矩阵，保持完整目标与 Plugin/Auth 外部所有者。
- 实例注册、执行所有权、配置发布/应用分开建模；迁移持久意图与 Agent epoch 双侧协调。
- queued 接管重取凭据，浏览器 Cookie 与稳定 Session Token 分离；原Session配置产物保持。
- 接口伪代码22场景通过，完整spec与plan已形成；模型验证不替代实际数据库/Broker/环境集成测试。

## 2026-10-11 · 按原设计收窄 Runtime Controller，创建入口归 Gateway

- 撤销本次讨论中扩大为跨平台运行管理器/探活服务的设计。
- Gateway 使用 Kubernetes 客户端提交 manifest，自定义控制器创建/协调 StatefulSet 等承载资源；StatefulSet/kubelet 负责副本与探针，Gateway 直接与 Agent 通信。
- Control 保留业务配置/Placement，Gateway/Agent 协调业务排空和 queued 重绑；Controller 不查询 Turn 或判定 Agent 健康。
- Docker 由有 Docker 权限的创建服务提供单实例容器，本期不做扩缩容。

## 2026-10-11 · 版本化是系统契约，不是 Agent 可选能力

- 撤销 Skill 版本化、旧 Session 隔离的能力登记设计，不新增 metadata_capabilities/required_capabilities。
- Control 管理版本选择和历史；Agent 侧平台代码管理版本化产物与发布，Runtime adapter 负责具体转换和加载。
- 保留消息/配置响应 schema 的版本兼容与部署验证，所有实现必须满足同一业务不变量。

## 2026-10-11 · 元数据应用结果经 Gateway API 上报 Control

- 结果反馈采用受控 API，不增加独立结果消息；Gateway 代理，Control 保存实例应用投影。
- 上报关联选择序号、配置版本、副本及运行代次，防止旧实例/旧选择晚到结果覆盖当前视图。
- 重复结果幂等，成功后的晚到失败不撤销成功，上报故障独立重试，不重新应用配置。
- 实例成员计数、超时及固定 IDL 待定稿。

## 2026-10-11 · 元数据通知扩展性审查修订

- 分离通用封套与具体消息的数据主版本；当前更新消息生产权仍归 Control，不扩大用户确认边界。
- changes 改为可选解释信息，支持来源命名空间和未知提示前向兼容；实际配置必需能力独立校验。
- 以 selection_revision 表示选择顺序，允许用户明确回滚资源，旧消息不得降低已应用选择序号。
- 增加应用前目标核对、实例启动/重连/周期对账，以及结果观测的实例代次边界。
- 多副本结果传输、能力登记与部署参数仍待定稿。

## 2026-10-11 · 用户确认后通知 Agent，经 Gateway 拉取配置

- 配置通知不限定 Platform Prompt，覆盖选定 Prompt/Skill/Memory/MCP 等元数据。
- 仅用户明确确认运行更新后发布通知，资源目录编辑不自动推进已选配置。
- Agent 消费通知并经 Gateway 拉取 Control 的已确认目标，Gateway 不缓存控制配置，仅在返回链路生成独立平台正文。
- 固定旧 Session/Turn 版本；要求版本化文件避免共享目录原地覆盖。消息协议与未完成实现契约记录于跨服务 spec 设计稿。

## 2026-08-22：启用 Codex 与 Claude Code 双 Agent 配置

- 将 Agent Type 明确为 `codex | claude-code` 受控枚举。
- Provider API Key 继续由 Control PostgreSQL 加密托管，公开接口仅投影是否已配置。
- 规定更新留空保留、切换 Agent 共用 Workspace Runtime Directory、现有 Session 不变。
- Runtime Context 按 Agent Type 路由并仅在单次运行调用链传递明文密钥。

### 能力覆盖矩阵

| 原能力 | 目标上下文 | Domain | 聚合/读模型 | Workflow | 状态 |
|:---|:---|:---|:---|:---|:---|
| 创建 Workspace/Codex 配置 | media_control | agent_configuration | AgentConfiguration | workspace_lifecycle | covered |
| 更新配置且留空保留 API Key | media_control | agent_configuration | AgentConfiguration | runtime_context_assembly | covered |
| 选择 Claude Code | media_control | agent_configuration | AgentConfiguration | runtime_context_assembly | covered |
| 公开查询配置状态 | media_control | agent_configuration | WorkspaceResponse | 无：纯查询 | covered |
| 解密并下发运行上下文 | media_control | agent_configuration | RuntimeContext | runtime_context_assembly | covered |
| Codex/Claude Turn 执行 | media_agent_gateway/media_agents | agent_session/runtime adapter | Session/Agent process | agent turn | external-owner |
## 2026-10-03 · 稳定委托身份解析

区分类型标签 agent_id 与配置记录的稳定 ID，以 AgentConfiguration ID 形成 Agent Actor。增加内部只读身份投影，回查 Workspace 归属和当前 owner 的部门权限，不返回 Provider 秘密。只读能力不创建 Application Workflow；Token 签发/撤销仍由既有跨服务 Workflow 协调。没有把运行环境 stopped 隐式改成身份停用。
## 2026-10-04 · 失效事实与查询不确定性

- 明确不存在/已删除返回404；不完整或未知身份事实返回503，防止后台误撤销。


### 2026-10-13 · P4 固定平台模板版本

Control 发布记录固定 Gateway 不可变模板引用，正文继续由 Gateway 生成；缺失引用的历史 release 使用 P2 版本1，用户新确认发布选择完整版本2。此引用避免 Gateway 升级悄悄重新解释既有 Session，并且没有新增 Gateway 控制配置缓存。

## 2026-10-13 · P5 工作负载落地

- 工作负载创建配置覆盖已接入的 Goose Runtime，Agent 类型统一允许 codex、claude-code、goose；镜像与资源 profile 由部署控制。
