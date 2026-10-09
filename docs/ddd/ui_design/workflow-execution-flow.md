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
   选择后显示实际参考媒体或文字摘要，纯文本不伪造图片；Product 使用主图、Virtual IP 优先全身参考。无扩展名对象的播放类型使用 Storage 解析响应的 content_type，不从业务分类猜测媒体。
5. UI 把选中对象映射为自包含 JobInput bundle；Product 输入包含业务 ID 与主图（无主图则首图）的可访问 URL。携带 Workflow 当前版本提交，成功后打开 Job 详情。
6. Workflow 切换会清空旧选择并加载新候选，不能把前一个 Workflow 的资源静默带入。
7. Job 详情以执行进度、输入对象摘要和按顺序排列的 Step Run 展示服务端状态；图片与视频 Candidate 直接预览。提示词按需展开，内部输入 JSON 和临时签名 URL 不作为页面正文展示。
8. Step Run 进入 `need_confirm` 后，可选中任一 Candidate 并确认；即使只有一张图，配置要求审批时也必须能确认。用户可在审批时选择重新生成。已完成的 Step 保留候选预览与最终产物编号。
9. Job 列表先显示 Workflow 名称、执行状态和当前进度；进入 Job 后，使用与定义页一致的节点拓扑标出运行中、等待、完成和待审批节点。点击节点查看其候选、产物与可用操作，不在画布下方无差别展开全部步骤。

## 状态与失败

- 执行工具栏注入宿主 Header Portal，选中节点结果在 App 内右侧面板；窄 Slot 可覆盖并关闭。输入/提交时间放按需弹窗，图中只标真实 StepRun 状态。
- 执行图直接铺满详情主体，不必先打开“查看执行图”弹窗。只有版本匹配才使用定义拓扑；版本不匹配时主体为真实 StepRun 列表，并说明冻结拓扑不可查。点击无 StepRun 的节点显示尚未开始，不借用其他节点结果。
- 结果面板只由用户选择打开，关闭后保留选中节点；轮询更新状态不自动打开面板或抢焦点。待确认执行提供定位待审批节点的入口。桌面停靠右侧，窄容器覆盖画布；关闭或 Escape 返回画布选择位置，输入快照和提交信息仅按需打开。
- 父 Job 进入取消/失败/成功等终态后，即使旧 StepRun 保留 need_confirm，候选仅供查看，不再显示审批命令。可运行定义必须有 registered 发布回执，published 字样或缺失注册信息不能当作已就绪。
- 任务列表只针对当前可见的活动记录核对快照，终态停止轮询；定义详情的执行列表按需加载，不在关闭面板时持续读取每个 Job。前端轮询不能承担服务端调度或状态权威职责。
- 用户选中的是当前候选数组位置，Candidate.index 只作来源信息。多候选必须显式选择；审批回执 pending 禁止再发另一决定，applied 不表示步骤已经完成。
- 未确认的网络结果保留原意图键及内容，重试发送完全相同命令。面板/应用切换不丢失该内存意图；媒体签名更新不代表新生成轮。页面仅维护选择状态，不推导引擎轮次或权限。
- 未确认意图的重放入口不能依赖 Mutation Hook 的临时 error：关闭 Inspector 再打开后该错误会消失，但实例内意图仍在。当前仍可审批且没有请求中/pending 回执时提供显式同键重放；终态不提供该命令。
- 公开接口没有 expected-generation 前置条件，UI不能承诺跨窗口并发的原子轮次锁定；以服务端当前 HUMAN 与回执冲突为权威。不能用 retry_count 充当生成轮次。

- Workflow、公司上下文或 Input Schema 加载中显示加载态，不渲染可提交的空表单。
- Workflow 索引只作选择目录，不含发布回执时仍可显示已发布候选。选择后读取该定义详情，以真实 registered 回执、版本与 Input Schema 判定可提交状态；不能用缺少字段的索引把所有选项过滤为空，也不对整个目录批量读取详情。
- 恢复/输入覆盖重跑等待原 Job 读取成功后再初始化表单，取消回到原执行。提交要求 Workspace 属于实际公司候选，选择对象仍在相应输入候选内；失效的实例内选择不能绕过提交禁用。
- 执行信息弹窗展示结构化输入快照及参考媒体，窄 Slot 不依赖被隐藏的右侧摘要，也不退回 JSON 查看器。
- 快照中的 `object_path:<path>` 是冻结引用，预览适配只移除确切前缀再调用企业对象解析；不修改持久快照，不重写路径内 Organization，也不把已签名 URL 写回输入。
- 没有兼容资源时显示空态和资源类别，不使用任意类型资源兜底。
- 候选查询失败保留 Workflow 选择，显示可重试错误；不降级为 JSON 编辑器。
- 服务端返回版本冲突时重新加载 Workflow，要求用户复核输入后再提交。
- 桌面表单最多两列；窄屏单列。标签展示业务 input name 与资源类别，选择项展示资源名称，不展示内部 Object Path。
- Job 详情加载失败提供重试；Candidate 媒体地址失效时展示明确占位与打开媒体入口，不把加载失败误判为 Job 失败。步骤卡在窄屏单列，宽屏可并排展示候选。
