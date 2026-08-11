# Workspace 配置旅程

## 字段

| 字段 | 创建 | 更新 | 规则 |
|:---|:---|:---|:---|
| 名称 | 必填 | 可修改 | 去除首尾空白后非空 |
| Agent 类型 | 必填 | 可修改 | 必须来自前端支持的 Agent Catalog；当前仅 Codex |
| API Key | 可选 | 可替换 | 密码输入，不回显；更新时留空表示保持原值 |

## 创建

用户从 Menubar Workspace Switcher 或根路由空态打开创建 Dialog，提交成功后导航到 `/w/:workspaceId`，并加载它的默认 Slot Profile。提交失败时保留非秘密字段；API Key 是否保留在 Dialog 生命周期内由表单决定，但不得持久化。

## 更新

Workspace Settings App 或 Workspace Switcher 的设置入口打开更新 Dialog。Dialog 显示名称、Agent 类型和“已配置/未配置”状态，不显示 API Key。提交成功后刷新 Menubar 名称和 Agent 标签。若 Agent 类型或 API Key 改变，后端递增配置版本，后续新 turn 使用新配置；已有 Session 身份不变。
