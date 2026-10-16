# Agent 产品功能落地

依据 [功能规格](../spec/agent-product.md)，按已确认产品语义实现。每个切片写 review.md、执行相关功能检查并 commit 后自动继续，不等待逐阶段批准。涉及尚不明确的业务规则先对齐。

## F0 · 产品规则对齐

- [x] 企业 Prompt/Skill 资源库、部门 Grant 与 Workspace 双重授权。
- [x] Prompt 三种来源及显式更新、历史与回滚。
- [x] Skill 上传、Git、在线编辑、草稿发布和历史。
- [x] 企业管理员可登记企业外部 MCP，统一走 Gateway。

## F1 · Prompt 管理与配置

- [x] 企业归属迁移、资源 ACL、可见列表/详情、内容管理。
- [x] 版本历史、读取、回滚与归档。
- [x] Agent Prompt 三种来源确认接口及运行快照。
- [x] 对应功能检查、F1/review.md 和 commit。

## F2 · Skill 完整内容管理

- [x] 企业 ACL、内容产物、上传与 Git 导入/手动同步。
- [x] 文件树与文件增删改、草稿发布、历史查看下载。
- [x] 配置选择与 Agent 加载接通、检查、review.md 和 commit。

## F3 · MCP 企业管理与使用

- [x] F3a：Gateway 企业目录与授权、运行调用企业范围校验。
- [x] F3b：Plugin 绑定管理和固定配置查询、静态凭据与 OAuth 代理。
- [x] 工具发现、连接配置和 Agent 调用链路（真实协议服务验证，正式模型验收属 F4）。
- [x] 功能检查、F3a/F3b review.md 和 commit。

## F4 · 界面与正式使用

切为 F4a 企业资源库与配置界面、浏览器认证/Workspace 创建接通；F4b1 Docker 部署、凭据自动准备与默认配置，F4b2 正式模型对话与真实使用验收。每个切片各自 review 与 commit。

- [x] F4a：企业资源库与管理、Agent 配置和显式更新交互、浏览器身份与 Workspace 企业/部门创建。
- [x] F4b1：Docker 部署、自动凭据、默认配置原子创建、真实登录与 Session 注册。
- [x] F4b2：正式 Agent 对话、流式回复、历史、工具权限确认。
- [x] Docker 实际使用入口、功能验收、review.md 和 commit。

后续可发现并补充阻塞产品使用的功能缺口；不能以当前仅有 API 或 fixture 联通宣布完整产品完成。SRE、隔离扩展和扩缩容不作为本计划阶段。

## F5 · Codex auth.json 与个人模型连接

- [x] 个人连接加密持久化、本人 ACL、版本替换/撤销与回归。
- [x] Workspace 明确绑定、配置历史、内部执行凭据链路与原生文件装配。
- [x] 管理界面、Workspace 选择、正式导入本机 Codex 并完成真实对话验收。
- [x] 清理手工临时文件，review.md 与 commit。

## 旧会话模型认证恢复

- [x] 固定版本提示、明确新建入口、保持旧会话执行与历史语义。
- [x] 浏览器回归、真实 Docker 对话、review.md 与 commit。

## Settings UI 统一

- [x] 对齐agent-platform左侧导航，统一企业资源/个人连接管理与旧链接。
- [x] 修复滚动视口和窄Slot布局，保留权限与原业务功能。
- [x] 回归、部署、review.md与commit。

## F6 · Claude Code 与 Goose

- [x] F6a：Workspace固定模型调用设置、Control/Gateway/Agent链路及界面，相关回归与review/commit。
- [x] F6b：原生Runtime对话/流式/历史/工具配置接通，Docker镜像构建、创建与使用。
- [x] F6c：使用明确的真实模型服务完成验收，记录所需凭据和外部限制；review与commit。

本期优先此项；Kubernetes和外部OAuth验收后排，Docker扩缩容继续不开发。
