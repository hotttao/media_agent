# F5 · Codex auth.json 与个人模型连接

## 完成功能

个人模型连接由 Control 持有，明确区别于外部 Secret Manager 的 CredentialReference。提供本人列表、导入、名称编辑、认证文件替换、撤销；其他用户不可读取或绑定。auth.json 支持原生 ChatGPT tokens 或 OPENAI_API_KEY，限制64KiB；以绑定连接ID和owner的 AES-GCM envelope 加密保存，公开响应不回显认证。

Workspace 创建/编辑明确选择 API Key 或个人 Codex 连接。当前和历史配置保存连接ID，与 API Key 互斥；选择变更同事务保存引用、历史及沿用旧 Prompt/Skill/MCP 内容的新 Release/Outbox，不意外升级资源。旧 Session 保持原连接身份。仅替换认证时改变私有认证 revision；公开 CAS version 的重命名变化不会覆盖原生已刷新的认证。

Gateway 在已有 Session/Turn/owner/epoch 校验后，经内部上下文将选中认证交给 Agent。Agent 在单 Session 私有 CODEX_HOME 原子写600 auth.json；相同认证 revision 保留 Codex 自行刷新结果，替换时更新，切换到 API Key 时移除旧文件。认证不进入元数据 Release、产物或浏览器存储。撤销连接阻止后续 claim，明确分类 model_authentication_required，避免无限当作网络故障重试。

工作台、资源库增加“模型连接”入口；管理页支持上传、替换、重命名及确认撤销。Workspace 表单不自动选择候选，必须明确选择，并展示授权 Workspace 使用者可通过 Agent 使用模型的说明。原500问题及暂停显式恢复见上一切片 session-recovery/review.md（ed7b5eb）。

## 真实使用结果

按照用户授权，经真实浏览器登录1556824234@qq.com，在正式页面导入原始本机 Codex auth.json，建立“本机 Codex”个人连接（9e535415-0e3c-4377-9f28-950a5d652c05）。没有修改原始登录文件，也未通过手工复制作为正式配置来源。

在 Agent 产品验收 · Docker（d41d11cf-dba2-48d8-94c2-74f81a65a9fb）明确选择该连接，配置版本3，界面自动创建 Session 2（df472509-e2d2-43dc-bae6-886c5011771a）。真实模型成功返回唯一标记，刷新后历史可恢复，脚本错误0。最终部署后重新登录，验证精确的认证源与连接选择、保留第一轮历史，并显式恢复此前失败的测试轮次；第二轮真实模型回复及刷新后的历史再次通过，公开连接列表不含认证正文。Agent 使用当前镜像，私有认证文件600，来源标记对应连接与认证revision1。原失败Session保留其旧配置和历史，不隐式复用新连接。

连接现在是用户正式持有的可复用配置，按本次要求保留加密记录和原生运行状态。其他 Workspace 未自动绑定。可在管理页替换或撤销，而非验收结束删除整个连接。见 [真实验收结果](live-acceptance.json)、[连接管理](model-connections.png)、[对话界面](conversation.png)。

## 验证

- Control 和 Gateway 全模块 Go 测试通过；两服务架构检查通过。
- 全产品真实 PostgreSQL/HTTP/MCP 集成测试通过（82.731秒）。新增连接覆盖密文存储、owner隔离、错误runtime拒绝、输入限制、密文移植拒绝、固定历史引用、资源不升级、替换/CAS/撤销；补充重命名不改变认证revision的集成复测通过（10.487秒）。
- Python14项相关回归通过，7文件 mypy 通过；覆盖私有文件权限、Session隔离、原生刷新保留、明确替换与 API Key 切换清理，兼容既有 Codex/Claude 元数据加载。
- 前端14文件41项测试及 TypeScript 通过；个人连接必须明确选择，切换认证源清除 API Key 草稿。依赖边界、翻译键与修改文件格式检查通过。
- 浏览器7场景通过：原资源管理、首启、权限请求、暂停恢复，以及个人 auth.json 导入、改名、撤销和写入式凭据。正式模型使用单独真实浏览器及服务，无 API fixture、无预置登录Cookie。

## 部署与边界

Control migration26 已应用，Gateway/Control/Codex/界面已构建部署。Edge 当前 watch=false，已重启加载模型连接与既有 Resume 路由；启动文档同步说明。最后静态界面构建遇镜像源 node:22-alpine 的502，使用本机 Node22执行相同 tsc/Vite生产构建，以本机缓存的相同 nginx:1.27-alpine 与 nginx.conf 打包部署；未修改其他应用。

Docker 本机模型出网通过可选的正式 agent-model-proxy Compose 服务连接宿主127.0.0.1:7897，只监听Docker宿主内网地址。私有策略配置 HTTP_PROXY/HTTPS_PROXY 及内部 NO_PROXY，未设置 ALL_PROXY。该服务不读取模型凭据；不同主机按启动说明调整。机器重启后发现 k3s 自动启动，已停止并禁用开机自启；临时测试 PostgreSQL 已删除。

本期正式 auth.json 模型验收覆盖 Codex。原生刷新结果属于该 Session 私有状态，不回写个人连接库；远端认证失效时上传新的 auth.json 替换连接。既有执行中 Turn 不因连接撤销强行终止；后续 claim 被拒绝。本期不增加隔离或扩缩容功能。

最终复验期间容器代理偶有 TLS EOF，原生 Codex 重试后完成真实回复。当前代理存在瞬时网络波动；未通过修改模型响应或模拟接口掩盖该现象。
