# 0007 · Agent Settings 统一导航

状态：采纳。

Settings 统一承载 Workspace 配置、企业 Prompt/Skill/MCP 资源管理与个人模型连接。参考 agent-platform WorkspaceSettingsPanel：固定左侧导航，右侧内容区独立滚动。此前企业库和个人连接位于独立顶层页面，造成导航和滚动边界不一致；本决策替代 agent-product-flow 里将二者作为顶层管理入口的交互安排。

Settings 仍是 Workspace 内置 Slot App；管理功能的服务端所有者和权限范围保持原有模型。没有 Workspace 时，同一 Settings 视图可作为完整页面管理企业资源和个人连接，不能伪造 Workspace 或自动绑定连接。旧管理链接进入相同视图，继续支持直接访问。

右侧滚动视口占满剩余宽高，内部表单可限制阅读宽度；不得把 overflow 放到 max-width 居中容器。窄插槽按照容器宽度切换紧凑分区选择，不能用整屏宽度误判插槽可用空间。固定导航不随右侧表单滚动。
