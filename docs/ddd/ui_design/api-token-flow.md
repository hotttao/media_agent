# 个人 API Token

入口为既有 Settings 的个人分组，`/settings/tokens`，不需要 Workspace。L2 `domains/api-token` 管理表单和 Auth API，L3 Settings 只组合分区；不新增 App 或 Slot。

列表显示当前 User 的 Token 名称、scope、状态和到期时间；加载、空列表、失败和刷新分别可见。创建字段为名称、有效期、MCP/API scope 选择；默认仅 MCP，至少选择一项，必须为未来时间。MCP scope 只允许入口访问，具体工具及资源仍按当前业务授权判定。旧 Token 不自动新增 scope。

每次表单尝试使用独立幂等键，失败不自动重试签发；保留同一尝试键并刷新列表以便检查可能已生效的 Token。用户可明确清空并新建尝试。一次性 Secret 仅在当前组件内存中显示，可手动复制或关闭；列表从不回显 Secret。撤销要求用户确认，成功后刷新服务端状态；失败保留现有列表并显示错误。

桌面沿用 Settings 内容区域；窄屏表单及列表纵向排列，Secret 可换行。测试覆盖显式 scope、禁止重试、Secret 关闭和撤销刷新。

局域网 HTTP 入口也必须能创建 Token：签发尝试标识使用 getRandomValues 生成，不能依赖仅安全上下文开放的 randomUUID。剪贴板 API 不可用时显示手动复制提示；明文仍只存组件内存。
