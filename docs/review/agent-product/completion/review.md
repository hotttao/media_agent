# Agent 当前范围完成审查

日期：2026-10-16。范围：003 Agent核心能力，加上用户后续确认的Prompt/Skill/MCP产品功能、Docker单实例、个人Codex连接及统一Settings。依据最新用户要求优先可用产品，隔离、扩缩容和SRE建设不作为本期完成条件。

结论：当前Docker可用产品范围的开发已完成，代码已提交并部署。不能据此宣称原003全部Kubernetes部署门禁、所有原生Runtime和所有外部OAuth提供商均已实际验收。

## 2026-10-19 状态更新

Claude Code / Goose 已在 F6a/F6b/F6c 完成真实 SiliconFlow 模型验收与部署，见 [F6c](../f6c/review.md)。外部 MCP OAuth 实际提供商接入按用户本日指示延期，不继续要求用户提供服务或凭据。Kubernetes 双 Pod / RWX 仍延期，本机 k3s 保持关闭。

F7 修复聊天长内容在窄 Slot 的显示裁切并完成界面验收，见 [F7](../f7/review.md)。当前功能清单没有新增待开发项；这不表示已完成所有环境验收或全部历史 CLI 门禁。以下 2026-10-16 检查记录保留，不作为本日重新执行的检查。

## 能力覆盖

| 要求 | 当前实现及阶段 | 证据与状态 |
|---|---|---|
| RabbitMQ替代会话Redis，delta不落库，每Gateway一个临时队列、本机SSE广播/补查 | SessionEventBus、Gateway事件Source/Broadcaster；P1/P6/P8 | 核心代码完成；跨语言Broker及Docker联调见Runtime阶段review |
| Control明确确认才通知，配置/资源固定版本，Agent原子落盘，结果上报 | Control metadata_update、Gateway agent_metadata、Agent版本产物；P2/P3 | 完成；目录编辑不隐式升级旧Session |
| 稳定身份/实例分离、Session FIFO与owner/epoch、暂停显式恢复 | Runtime Session、实例路由/执行流程；P3/P6 | 完成；真实模型与旧配置恢复另有产品证据 |
| Gateway统一MCP入口与权限发现/调用，Plugin具体绑定，Auth OAuth | P4、F3a/F3b | 完成；真实HTTP/协议测试覆盖none/Bearer/API Key/OAuth，真实模型调用MCP见F4b2 |
| Gateway仅生成Platform Prompt，Control持有用户配置，Agent转换加载 | P2/P4、F1/F4b2 | 完成；真实固定Skill与模型输出已验证 |
| Docker Controller单实例创建并直接通信 | P7/P8、F4b1 | 完成；扩缩容明确501，符合已确认范围 |
| Prompt管理、企业/部门授权、历史/回滚、none/custom/library | F1/F4a | 完成；Go/数据库HTTP/浏览器回归 |
| Skill上传/Git/在线编辑、草稿发布、固定历史/下载 | F2/F4a | 完成；真实Git/产物/数据库测试，真实模型加载见F4b2 |
| Workspace MCP绑定、凭据只写、明确选版、OAuth连接 | F3b/F4a/F4b2 | 完成；后端真实协议、UI及模型调用分层验收 |
| 登录、创建会话、流式输出、历史、工具确认、暂停恢复 | F4b1/F4b2及登录/恢复修复 | 完成；真实账号、Codex、多轮SSE与历史证据 |
| auth.json个人模型连接、多个Workspace明确复用、加密/替换/撤销 | F5 | 完成；正式“本机Codex”保留；真实两轮对话及恢复新会话验证 |
| Settings左导航、资源/模型连接归一、滚动边界/窄Slot/旧链接 | Settings UI | 完成并部署；真实服务截图、几何断言与10场景回归 |
| Claude Code / Goose 模型设置、流式、多轮、Skill、工具审批与 Gateway MCP | F6a/F6b/F6c | 完成并部署；正式 SiliconFlow 实际模型、界面和持久历史验证 |

能力状态来自实现和阶段证据，而非只看plan勾选。抽查IDL、路由、Workflow、原生产物装配及未实现分支。源码中的Docker scaling NotImplemented为已批准范围；Codex旧非版本化Hooks.load_skills的no-op不承担当前正式元数据加载，当前路径通过main中的codex_home/版本产物装配，已有真实Skill验收，不作为当前功能未实现的依据。

## 2026-10-16 检查记录

- 当前Control、Gateway、Plugin、Runtime Controller、Auth五模块 `go test` 全部通过。
- 五模块archcheck全部通过。
- Python认证、元数据、执行、实例客户端与事件总线相关回归：21 passed、2 skipped（未提供专用RabbitMQ测试URL的真实Broker分支；未记为通过）。此前真实Broker联调保留原P1/P6/P8证据。
- 当前UI提交已有14文件41项测试、10项完整浏览器场景、生产构建、依赖/翻译/格式检查通过；最后手机样式调整另复测2项关键浏览器场景。真实账号验证桌面、填满Slot和手机视图，脚本错误0。
- 本轮不重新发模型对话或修改连接；沿用F4b2、F5、pinned-auth-recovery保存的真实模型结果。当前Agent/Controller/Control/Gateway/界面容器在运行，k3s inactive。

## 计划收口

Runtime计划P9原有四项仍未勾选，但实际由F4/F5及后续修复提交完成。本次补齐勾选和P9审查索引；没有把早期fixture冒充正式模型结果。旧Session配置切换计划保留历史记录，并明确当前固定版本规则，避免历史只读条目误导后续开发。

核心代码提交：F1 790e841、F2 7f7b223、F3a 6f59173、F3b 0ddac58、F4a f3969c8、F4b1 b4b6585、F4b2 ab87a89、F5 59b3b68；实际使用修复16247a2/ed7b5eb/75372ad，统一Settings db0c824。详细阶段记录均在同级目录。

## 未完成验收与已排除范围

- Kubernetes真实双Pod Ready、RWX及跨Pod共享文件实测尚未完成；原存储镜像环境受阻，已按用户指示延期，不启动本机k3s补跑。
- Claude/Goose 原未验收项已由 F6 完成；auth.json 个人连接仍明确用于 Codex，不扩展为其他 Runtime 的 OAuth 登录。
- 外部MCP三类认证能力已有真实HTTP协议验证；第三方OAuth提供商正式接入按用户2026-10-19指示延期，未继续本轮OAuth开发。互联网私有Git服务各自实际环境仍需部署验收。
- 代理曾有间歇TLS EOF，正式模型通过重试后成功；当前功能交付不等于长期可用性/SRE保证。
- Docker扩缩容、隔离增强、故障接管/SRE专项按用户要求不属于本期，不将501接口当成功实现。

以上是明确的验证和范围边界，没有发现当前已确认Docker功能范围内的新增缺失项。代码完成结论不替代用户产品review。
