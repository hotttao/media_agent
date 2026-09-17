# Workspace 配置旅程

## 字段

| 字段 | 创建 | 更新 | 规则 |
|:---|:---|:---|:---|
| 名称 | 必填 | 可修改 | 去除首尾空白后非空 |
| Agent 类型 | 必填 | 可修改 | `Codex` 或 `Claude Code`；默认 Codex |
| API Key | 可选 | 可替换 | 密码输入，不回显；更新时留空表示保持原值 |

## 创建

用户从 Menubar Workspace Switcher 或根路由空态打开创建 Dialog，提交成功后经 Navigation Port 导航到 `/o/:organizationId/t/:teamId/w/:workspaceId`，并加载它的默认 Slot Profile。提交失败时保留非秘密字段；API Key 是否保留在 Dialog 生命周期内由表单决定，但不得持久化。

## 更新

Workspace Settings App 或 Workspace Switcher 的设置入口打开更新 Dialog。Dialog 显示名称、Agent 类型和“已配置/未配置”状态，不显示 API Key。提交成功后刷新 Menubar 名称和 Agent 标签。若响应的配置版本改变，页面立即创建并选中一个使用新版本的新 Session；仅名称变化不创建 Session。

Agent 选择在 Dialog 内存中维护；打开编辑时取服务端当前值。保存失败保留选择和本次输入的 Key 供用户
修正，关闭 Dialog 即清空 Key。切换 Agent 后已有 Session 仍显示其创建时 Agent Type 和历史；当其版本与
Workspace 当前配置不同，Chat 输入区替换为“该 Session 使用旧配置”的只读提示与“Start new session”动作。
历史查询、事件订阅和运行中 Turn 的停止动作继续可用。自动创建失败时提示“配置已保存，但新 Session
启动失败”，并保留手动重试入口。加载、失败和无权限沿用 Workspace Dialog 状态；窄屏选择框和密码框
保持单列全宽。
