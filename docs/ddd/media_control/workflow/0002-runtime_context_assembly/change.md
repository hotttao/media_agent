# Runtime Context Assembly 变更

## 2026-10-15 · 解析三种 Prompt 来源

- 组装只使用已确认配置中的不设置、直接填写或固定库版本，不把历史引用解析成 latest。
- 直接填写不写回资源库；不设置仅清空 Configured Prompt，不停用 Gateway 的 Platform Prompt。

## 2026-10-11 · 用户确认后通知 Agent，经 Gateway 拉取配置

- 配置通知不限定 Platform Prompt，覆盖选定 Prompt/Skill/Memory/MCP 等元数据。
- 仅用户明确确认运行更新后发布通知，资源目录编辑不自动推进已选配置。
- Agent 消费通知并经 Gateway 拉取 Control 的已确认目标，Gateway 不缓存控制配置，仅在返回链路生成独立平台正文。
- 固定旧 Session/Turn 版本；要求版本化文件避免共享目录原地覆盖。消息协议与未完成实现契约记录于跨服务 spec 设计稿。

## 2026-10-11 · 区分 Platform Prompt 与 Configured Prompt

- Gateway 迁入的是平台模板管理与 Platform Prompt 正文生成，不是全部 Prompt 的所有权或最终提示词合成。
- Configured Prompt 的内容与版本继续归 Control，两类内容分别交付；Agent 负责 Runtime 特有转换、落盘与加载。
- 生成职责与生效时点分离，不改变 Session 固定版本，也不隐含热更新。

## 2026-08-22：双 Agent 路由与 Provider API Key 安全下发

- Runtime Context 明确携带受控的 `agent_type`，Gateway 据此选择 Codex 或 Claude Code Runtime。
- Provider API Key 只在同时通过 Workspace owner Cookie 和 Control 服务令牌的同步内部请求中解密。
- Gateway 不持久化 Key，只随单次 Agent `/chat` 请求传入；Agent 仅注入子进程环境。
- 公开调用 Runtime Context 被拒绝，避免浏览器利用 owner Cookie 回显秘密。

## 2026-08-22：异步 Worker 服务身份

- Runtime Context 支持内部服务令牌与 `X-Control-Acting-User-ID` 组合建立代表用户身份，供不持有用户 Cookie 的 River Worker 使用。
- acting user 只在内部令牌恒定时间校验通过后生效，Assembler 仍按该用户校验 Workspace owner。
- Task、Outbox、River Job 均不保存 Cookie、Session Token 或 Provider API Key。
