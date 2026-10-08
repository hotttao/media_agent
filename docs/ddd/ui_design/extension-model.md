# 扩展模型

内置 App 与未来 plugin 最终都转换为 `AppDefinition`，再交给同一个 Registry/Slot Provider。布局和 Slot Container 不导入具体 App。

当前静态 Catalog 包含 Sessions、Chat、Workspace Settings，并新增 Materials、Workflow。Materials/Workflow 通过同一 App Definition 在 standalone 与 Agent Platform plugin adapter 中注册。暂不实现任意第三方远程代码加载、权限声明、版本协商和故障隔离。

未来 plugin loader 只能新增/移除 App Definition，不得直接修改 Slot Provider 内部状态；不可用或被卸载的 App 在 Profile 归一化时被安全移除。

Host Adapter 只实现 Shell Context、Navigation Port、App Header Portal 和必要生命周期，不向业务 App 泄漏 `window.tos` 或宿主 Router 类型。新增宿主能力必须先扩展稳定端口，再分别实现 standalone/plugin adapter。

Workflow v1.2 使用应用局部视觉边界，不覆盖宿主主题。plugin 构建对本插件 CSS（包含重置、FlowGram 与动画名称）限定作用域，字体随包交付。Radix 弹层由 UiBoundary 注入的 Portal Container 承载；全屏时容器进入实际全屏元素。Header Portal 仍由宿主提供，只包裹应用注入的内容；应用不得控制窗口 chrome。列表筛选和运行来源继续使用 host-shim 的 instance state，不用模块全局状态替代 Slot 状态。
