# Password visibility

登录和注册表单使用同一个 `PasswordInput` UI primitive。组件保持原生 input 契约，并在输入框右侧提供 `type=button` 的眼睛按钮。

## 交互契约

- 初始 `type=password`，操作按钮后切换为 `type=text`，再次操作恢复。
- 隐藏时按钮名称为 `Show password`，显示时为 `Hide password`，并通过 `aria-pressed` 暴露状态。
- 每个密码字段独立控制显示状态；切换不清空值、不提交表单。
- `disabled` 同时作用于输入框和按钮。
- 密码值和显示状态仅存在于 React 内存，不持久化、不记录日志。

## 使用范围

- `/login` 的 Password。
- `/register` 的 Password 与 Confirm Password。

## 验证

- 组件测试覆盖默认隐藏、显示/隐藏切换、值保持、按钮不提交和禁用状态。
- 前端 TypeScript build、Biome 与现有测试保持通过。
