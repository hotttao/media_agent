# Workflow 执行创建流程

## 目标

Workflow App 根据已发布 Workflow 的 Input Schema 生成 Job 输入表单。业务用户选择 Product、Virtual IP、Material 或 Generated Asset 后提交；UI 不要求用户理解或手写 JobInput JSON。

## 状态所有权

| 状态 | 所有者 | 生命周期 |
|:---|:---|:---|
| Workflow、版本与 Input Schema | `media_workflow` | 服务端事实；React Query 查询 |
| Product、Virtual IP、Material、Generated Asset 候选 | `media_materials` | 服务端事实；按 Organization、Scope 与 Asset Kind 查询 |
| 当前 Workflow 与每个输入的选中 ID | Job 创建表单 | 当前视图内存；Workflow 切换时重置不兼容选择 |
| JobInput 快照与执行状态 | `media_workflow` Execution | 服务端持久化；提交成功后跳转到 Job 详情 |

## 用户旅程

1. UI 仅列出已发布、当前用户可执行的 Workflow；选择后读取其 `scope`、`version` 和 `input_schema.entries`。
2. `PRODUCT`、`VIRTUAL_IP` 分别显示当前 Organization 与 Workflow Scope 下的资源选择器。
3. `MATERIAL`、`ASSET` 使用 Input Schema 冻结的 `asset_kind` 查询兼容资源；同一 Workflow 可以出现两个相同 kind 的独立输入，例如 `movement_a`、`movement_b`。
4. required 输入未选择时禁用提交，并在对应字段显示原因；可选输入允许留空。
5. UI 把选中对象映射为自包含 JobInput bundle，并携带 Workflow 当前版本提交。成功后打开 Job 详情。
6. Workflow 切换会清空旧选择并加载新候选，不能把前一个 Workflow 的资源静默带入。

## 状态与失败

- Workflow、公司上下文或 Input Schema 加载中显示加载态，不渲染可提交的空表单。
- 没有兼容资源时显示空态和资源类别，不使用任意类型资源兜底。
- 候选查询失败保留 Workflow 选择，显示可重试错误；不降级为 JSON 编辑器。
- 服务端返回版本冲突时重新加载 Workflow，要求用户复核输入后再提交。
- 桌面表单最多两列；窄屏单列。标签展示业务 input name 与资源类别，选择项展示资源名称，不展示内部 Object Path。
