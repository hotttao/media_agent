# Workflow 可视化定义流程

## 目标

Workflow App 使用 FlowGram 展示业务 Workflow 的节点与依赖，并用结构化表单编辑 Step、Template、审批策略、复用策略与 Slot Binding。业务用户不读取或编写 Steps JSON，也不接触 Conductor Definition 或 Executable Workflow IR。

## 状态所有权

| 状态 | 所有者 | UI 行为 |
|:---|:---|:---|
| Workflow、Step、Slot Binding、output_step_key | `media_workflow` | 服务端业务事实；编辑表单维护未保存草稿 |
| FlowGram nodes、edges 与自动布局位置 | Workflow UI Adapter | 从业务 Workflow 投影；不是第二份事实源 |
| Template 候选与版本 | `media_workflow` Template | 页面层查询后以只读 Option 传入 Workflow 编辑器 |
| Executable Workflow IR、Conductor Definition | 编译链路 | 不进入编辑器，不从前端反向覆盖 Workflow |

## 投影规则

1. 每个业务 Step 投影为一个 `generation_step` 节点；节点展示 Step Key、Template 引用、输入绑定数和审批标记。
2. 画布连线只表示 `SlotBinding.source=step_output` 的节点间执行依赖；同一对节点的多个 Binding 合并为一条边。`workflow_input` 与 `const` 是节点参数，放在选中节点的绑定详情，不生成输入汇聚节点或边。
3. `output_step_key` 只标记最终输出节点，不额外创建业务 Step。
4. 服务端 protobuf JSON 可能返回数字或名称枚举；UI Adapter 必须先归一化，再生成拓扑，禁止把未知值静默解释为另一种依赖。

当前版本不持久化 FlowGram 坐标。画布位置由 DAG 层级确定性生成，用户可以在当前页面拖动节点整理视图；刷新后重新按 DAG 自动布局。拖拽位置不是业务数据，不能写入 Slot Binding。后续若需要跨会话保存布局，应增加独立的 UI 文档字段或 Profile 状态。

## 编辑旅程

1. 创建时先填写必要的 Workflow 名称和业务范围；进入编辑时直接打开占满 App 内容区的画布工作台。名称、说明、可见范围等元数据折叠在“工作流设置”，默认不挤占画布。
2. 用户通过“添加节点”创建 Step，点击画布节点或使用右侧选择器定位 Step。右侧先选择 Template，再按 Template 输入槽位配置来源，最后设置审批与复用策略；不能把底层字段平铺成没有任务顺序的表单。
3. Template 变更时按槽位名保留兼容的 Slot Binding，为新增槽位补默认的工作流输入引用；用户可改为上游 Step 输出或固定值。每项绑定都明确显示目标槽位和来源。
4. 编辑器同步更新 FlowGram 投影；删除 Step 时同时删除引用该 Step 的失效 Binding，并重新选择最终输出节点。
5. 用户先执行结构校验，再保存；发布动作仍由 Workflow 详情页发起。

## 页面状态

- 空 Workflow 显示画布空态与“添加节点”，不显示空 JSON 数组。
- Template 未加载或当前 Scope 无候选时保留 Template ID 输入，避免阻断已有数据修复。
- 校验失败在画布下方按 Step 展示错误；不能以保存失败 toast 代替结构反馈。
- 详情页以 FlowGram 画布为主体，不再平铺渲染所有 Step 卡片；点击节点后在右侧详情面板查看 Template、输入绑定、审批策略和输出标记。不渲染原始 Steps JSON。
- 编辑页以画布选中节点作为 Step 配置入口；节点配置面板不再维护一组与画布重复的 Step 导航列表。
- 节点配置面板提供可用键盘操作的节点选择器；画布点击与选择器共同修改当前节点，所选节点有明确高亮。流程输入在选中节点的 Binding 中查看和编辑。
- 画布节点支持拖动，节点点击和节点拖动是两种独立交互：点击更新当前选中节点，拖动只改变本次页面的视觉布局。
- 窄屏时画布与节点配置上下排列；宽屏时画布在左、配置在右。
- 详情页已发布 Workflow 提供“运行工作流”入口，直接进入预选该 Workflow 的输入表单；执行结果回到 Job 详情，避免用户先切到任务列表再重新找 Workflow。

## 验证

- 单元测试验证 Slot Binding 到 FlowGram edges 的确定性投影，以及 protobuf 数字枚举归一化。
- Playwright 验证真实已发布 Workflow 显示 FlowGram 节点、没有 JSON `<pre>`，并能进入结构化编辑器。
- Job 创建 E2E 继续验证 UI 改造没有破坏 Workflow Input Schema 到资源选择器的链路。
