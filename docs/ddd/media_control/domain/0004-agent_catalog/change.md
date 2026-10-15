# Agent Catalog 变更记录

## 2026-10-15 · Prompt 配置与历史

- 用户确认沿用 agent-platform 的不设置、直接填写和库引用三种配置方式；库引用只选择一份固定版本。
- 资源库支持历史查询与回滚，回滚形成新的内容版本并保留历史。
- 本项目继续采用用户确认后运行更新，未照搬 agent-platform 的库编辑自动通知关联 Workspace reload。

## 2026-10-15 · 企业资源库与授权边界

- 用户确认 Prompt、Skill 属于企业资源库，可跨同企业 Workspace 复用，不强制绑定单个 Workspace。
- 默认私有，支持企业内部门 viewer/editor Grant；public 仅限所属企业内读取。
- 资源内容编辑与 Workspace 配置分别授权；选择时同时要求资源可读、同企业和 Workspace manage。
- 保留显式确认更新规则。当前 owner-scoped CRUD 与可空 workspace_id 不满足目标，需要后续迁移及接口实现。
- Skill 内容来源、编辑和发布接口待下一项对齐；本次不据旧表字段推定这些规则。

## 2026-10-11 · 用户确认后通知 Agent，经 Gateway 拉取配置

- 配置通知不限定 Platform Prompt，覆盖选定 Prompt/Skill/Memory/MCP 等元数据。
- 仅用户明确确认运行更新后发布通知，资源目录编辑不自动推进已选配置。
- Agent 消费通知并经 Gateway 拉取 Control 的已确认目标，Gateway 不缓存控制配置，仅在返回链路生成独立平台正文。
- 固定旧 Session/Turn 版本；要求版本化文件避免共享目录原地覆盖。消息协议与未完成实现契约记录于跨服务 spec 设计稿。

## 2026-10-11 · 区分 Platform Prompt 与 Configured Prompt

- Gateway 迁入的是平台模板管理与 Platform Prompt 正文生成，不是全部 Prompt 的所有权或最终提示词合成。
- Configured Prompt 的内容与版本继续归 Control，两类内容分别交付；Agent 负责 Runtime 特有转换、落盘与加载。
- 生成职责与生效时点分离，不改变 Session 固定版本，也不隐含热更新。

## 2026-08-22：Prompt 聚合首个垂直切片

- 新增 Prompt 的 owner-scoped Create/List/Get/Update/Archive 能力。
- Prompt 状态仅允许 `active` 与 `archived`；归档不可逆。
- 更新和归档使用 `expected_version` 乐观并发控制，版本冲突拒绝写入。
- workspace-scoped Prompt 创建时校验 Workspace 所有权；平台级 Prompt 使用 `workspace_id = NULL`。
- API 响应不包含任何凭据字段；后续 Agent Template、Skill、Provider、Credential Reference、Memory Store 分别实现。
