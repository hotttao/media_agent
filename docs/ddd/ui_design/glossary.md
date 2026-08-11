# UI 通用语言

| 术语 | 定义 | 不包含 |
|:---|:---|:---|
| UI Platform | 承载 Shell、App 注册、Slot 布局和跨 App 基础能力的前端宿主 | 后端业务规则 |
| Shell | 始终可用的顶层框架，负责 Workspace 切换、身份操作和响应式导航 | Session 或 Chat 业务内容 |
| App | 可注册到 UI Platform 并由 Slot 承载的功能单元 | 独立部署的后端服务 |
| App Definition | App 的稳定注册描述，包括 ID、标题、组件、单实例策略和可用性 | App 的运行时状态 |
| App Instance | 某个 App 在 Slot 中的运行实例，由稳定 `instanceId` 标识 | 后端 Agent Session |
| Slot | 布局中的应用承载位置，维护已打开实例及活动实例 | React `children` 或视觉留白 |
| App Window | Slot 为活动 App 提供的统一标题栏、切换和关闭边界 | App 自己的业务界面 |
| App Header Portal | App 将标题和业务操作注入 App Window header 的宿主入口 | App Instance 导航 |
| Dock | 位于 Shell 底部、按 Slot 分段展示 App Instance 并提供 App Picker 的导航区 | App 业务工具栏 |
| Layout | 定义 Canvas 中 Slot 数量、位置和尺寸的视口级框架 | Slot 内打开了哪些 App |
| Slot Picker | 在指定 Slot 搜索并打开 App 的命令式选择器 | 原生 select 字段 |
| Workspace Profile | 某个 Workspace 的 Layout、Slot、App Instance 和实例视图配置 | Workspace 名称、Agent 类型或 API Key |
| Agent Session | 用户与 Agent 的连续会话，是后端业务实体 | App Instance |
