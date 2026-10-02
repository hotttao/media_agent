# Workflow 可视化编辑实现规格

## 范围

`ui_workflow` 使用 `@flowgram.ai/free-layout-editor` 渲染 Workflow DAG。FlowGram 是 UI 投影和画布框架；保存接口仍只提交 `WorkflowInput`，后端 Semantic Compiler 仍只读取业务 Workflow DSL。

## 组件边界

- `workflowToFlowDocument`：纯函数，把 `WorkflowStep[] + output_step_key` 转为 FlowGram `WorkflowJSON`；只绘制 `step_output` 执行依赖。`workflow_input` 和 `const` 留在节点绑定详情。
- `WorkflowGraph`：负责 FlowGram Provider、只读画布、节点 Renderer 和视图适配。
- `WorkflowForm`：负责业务草稿与节点 Inspector；不直接查询 Template domain。
- Workflow Create/Edit Page：查询 Template 候选并注入 `WorkflowForm`，保持 L2 domain 之间无依赖。
- Workflow Show Page：组合业务摘要、FlowGram 画布和 Step 摘要。
- Workflow Create/Edit Page 以画布工作台为主视图，占满 App 可用区域；元数据默认折叠，仅创建时引导补全必填信息。Inspector 按“使用的 Template → 输入来源 → 审批/复用”组织，并从 Template Slots 生成待配置绑定。
- 已发布 Workflow 详情提供直接运行入口，Job 创建页接受预选 Workflow ID。

## 数据兼容

API Adapter 同时接受 protobuf JSON 的数字与名称枚举：Workflow `status=1/2/3` 和 Slot Binding `source=1/2/3` 分别映射为领域字符串。未知 Binding Source 保持兼容期的 `workflow_input` 默认行为，但测试必须覆盖三种已知值。

## 非目标

- 不在浏览器执行 Workflow。
- 不把 FlowGram Runtime 用作 Conductor 的替代品。
- 不持久化坐标或从 edges 生成第二份 Binding。
- 本阶段不支持直接拖线修改 Binding；连线由结构化 Slot Binding 即时投影。
