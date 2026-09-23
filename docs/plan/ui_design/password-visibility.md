# Password visibility implementation plan

1. 在 L0 UI primitives 中增加可复用 `PasswordInput`，复用现有 `Input` 样式与 Lucide 图标。
2. 将登录、注册和确认密码字段替换为 `PasswordInput`，不改变表单状态与 API payload。
3. 添加组件测试覆盖显隐状态、可访问名称、禁用和非提交行为。
4. 运行前端测试、构建和静态检查。
