# 扩展模型

## Materials → Workflow 启动意图

取消运行按来源返回：Workflow 入口返回原 Workflow；Product 入口通过可选 `openProduct` Navigation Port 传递企业及商品 ID，Materials App 消费内存意图后定位商品详情，不能依赖素材 App 上次选中的 Tab。旧 plugin 宿主未提供 `openProduct` 时安全返回原 Workflow，不关闭 Slot 或猜测邻居 App。该导航不提交 Job，目标仍执行企业与资源授权校验。

App 只消费 L1 Navigation Port，不直接调用 SlotProvider、Router 或全局事件。`openWorkflowRun` 传递工作流 ID、商品 ID、企业 ID 和唯一界面请求 ID，不携带商品正文、凭证或执行命令。standalone adapter 使用现有 Slot `ensureInstance/activate` 并向目标 instance 写入内存启动意图；Workflow App 消费一次，创建独立运行草稿，避免覆盖该实例的旧编辑/提交草稿。

plugin 的 `window.tos.navigation.openWorkflowRun` 是可选宿主扩展契约，不假定旧宿主已经实现。不支持时仍可查看兼容定义，但不提供可点击的跨 App 启动，提示用户从 Workflow 手动运行；不嵌入第二个 Shell。目标页重新读取企业、注册回执、输入 Schema 与候选资源，预填商品只作用于该工作流的 PRODUCT 入口，用户换定义/清空选择后不自动重新填入。提交仍须明确合法 Workspace；导航不代表授权或执行成功。

内置 App 与未来 plugin 最终都转换为 `AppDefinition`，再交给同一个 Registry/Slot Provider。布局和 Slot Container 不导入具体 App。

当前静态 Catalog 包含 Sessions、Chat、Workspace Settings，并新增 Materials、Workflow。Materials/Workflow 通过同一 App Definition 在 standalone 与 Agent Platform plugin adapter 中注册。暂不实现任意第三方远程代码加载、权限声明、版本协商和故障隔离。

未来 plugin loader 只能新增/移除 App Definition，不得直接修改 Slot Provider 内部状态；不可用或被卸载的 App 在 Profile 归一化时被安全移除。

Host Adapter 只实现 Shell Context、Navigation Port、App Header Portal 和必要生命周期，不向业务 App 泄漏 `window.tos` 或宿主 Router 类型。新增宿主能力必须先扩展稳定端口，再分别实现 standalone/plugin adapter。

Workflow v1.2 使用应用局部视觉边界，不覆盖宿主主题。plugin 构建对本插件 CSS（包含重置、FlowGram 与动画名称）限定作用域，字体随包交付。Radix 弹层由 UiBoundary 注入的 Portal Container 承载；全屏时容器进入实际全屏元素。Header Portal 仍由宿主提供，只包裹应用注入的内容；应用不得控制窗口 chrome。列表筛选和运行来源继续使用 host-shim 的 instance state，不用模块全局状态替代 Slot 状态。
