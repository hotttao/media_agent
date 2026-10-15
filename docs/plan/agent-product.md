# Agent 产品功能落地

依据 [功能规格](../spec/agent-product.md)，按已确认产品语义实现。每个切片写 review.md、执行相关功能检查并 commit 后自动继续，不等待逐阶段批准。涉及尚不明确的业务规则先对齐。

## F0 · 产品规则对齐

- [x] 企业 Prompt/Skill 资源库、部门 Grant 与 Workspace 双重授权。
- [x] Prompt 三种来源及显式更新、历史与回滚。
- [x] Skill 上传、Git、在线编辑、草稿发布和历史。
- [x] 企业管理员可登记企业外部 MCP，统一走 Gateway。

## F1 · Prompt 管理与配置

- [ ] 企业归属迁移、资源 ACL、可见列表/详情、内容管理。
- [ ] 版本历史、读取、回滚与归档。
- [ ] Agent Prompt 三种来源确认接口及运行快照。
- [ ] 对应功能检查、F1/review.md 和 commit。

## F2 · Skill 完整内容管理

- [ ] 企业 ACL、内容产物、上传与 Git 导入/手动同步。
- [ ] 文件树与文件增删改、草稿发布、历史查看下载。
- [ ] 配置选择与 Agent 加载接通、检查、review.md 和 commit。

## F3 · MCP 企业管理与使用

- [ ] Gateway 企业目录与授权、Plugin 绑定管理和固定配置查询。
- [ ] 工具发现、连接配置和 Agent 调用链路。
- [ ] 功能检查、review.md 和 commit。

## F4 · 界面与正式使用

- [ ] 企业资源库与管理、Agent 配置和显式更新交互。
- [ ] 正式 Agent 对话、流式回复、历史、工具权限确认。
- [ ] Docker 实际使用入口、功能验收、review.md 和 commit。

后续可发现并补充阻塞产品使用的功能缺口；不能以当前仅有 API 或 fixture 联通宣布完整产品完成。SRE、隔离扩展和扩缩容不作为本计划阶段。
