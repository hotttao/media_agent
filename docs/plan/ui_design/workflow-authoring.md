# Workflow 可视化编辑落地计划

- [x] 固化 FlowGram 文档、业务 Workflow DSL、Executable Workflow IR 与 Conductor Definition 的边界。
- [x] 引入 FlowGram Free Layout Editor，并实现 Slot Binding 到 nodes/edges 的确定性适配器。
- [x] Workflow 详情移除 Steps JSON，改为画布、摘要和结构化 Step 卡片。
- [x] Workflow 创建/编辑移除 Steps JSON，改为画布、节点 Inspector、Template 选择和 Slot Binding 编辑。
- [x] 优化 standalone Shell、Workflow 列表、详情及暗色/窄屏布局。
- [x] 修复 protobuf 数字状态与 Binding Source 枚举归一化。
- [x] 通过 TypeScript、Vitest、dependency-cruiser、Vite build 和真实登录 Playwright 回归。
- [x] 画布只显示 Step 输出依赖，输入绑定进入节点详情；编辑器提供可键盘操作的节点选择器。
- [x] Job 列表显示 Workflow 名称与执行位置；详情在相同拓扑上标示节点状态，点击节点查看产物或审批。
- [x] 编辑页改为全屏画布工作台，元数据移入可展开设置；Template 槽位驱动节点输入配置，并从 Workflow 详情直接运行。
