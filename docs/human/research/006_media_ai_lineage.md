# media_ai 生成链与迁移关系核对

本页是源系统事实调查，不把同名 `workflows` 表误认为旧业务流程的唯一事实。依据：`media_ai/prisma/schema.prisma`、`scripts/workflow-cli/commands/start.ts`、`services/state-reconstructor.ts`，以及 2026-10-01 源 MySQL 只读查询。目标领域设计与导入 API 尚需据此审查。

## 业务主线

```text
Product + VirtualIp
    └─ ModelImage (product_id, ip_id, outfit)
         └─ StyleImage (model_image_id, pose_id, makeup_id, accessory_id)
              └─ FirstFrame (style_image_id, scene_id, composition, generation_path)
                   └─ Video[] (first_frame_id, movement_id; 一种动作一条分支)
                        ├─ VideoTask (video.task_id；单个视频任务，不是全链根)
                        └─ VideoPush (video_id 为逗号分隔的 Video ID 集合)
                             └─ 剪辑结果、文案、发布状态；CLI 可继续推送飞书
```

`WorkflowTemplate` 属于 VirtualIp，指定 `pose_id`、`scene_id` 和多个 Movement；Product 在启动 CLI 时另选。CLI 不从 `workflows.config` 读取这条链，而是通过查询现有 ModelImage、StyleImage、FirstFrame 和 Video 判断可复用结果与待生成动作分支。`VideoPush` 按所选 Video ID 集合形成剪辑/文案/发布层；它的 `video_id` 是 CSV，不是数据库外键。`VideoTask.params.source=manual_upload` 说明该视频任务的入口，不证明这条视频不属于上述业务流程。独立的 `workflows` 表当前唯一一条为 `Manual Upload` 且 `config=NULL`，不能据此重建生成链。

按 `VirtualIp + StyleImage.pose_id + FirstFrame.scene_id + Movement` 四项同时匹配，127 个 Video 中仅 97 个可归入当前唯一的 WorkflowTemplate；其余 30 个不得凭名称或默认模板强行归类。

## 源库实测与迁移边界

| 对象 | 记录数 | 关系及注意事项 |
|:---|---:|:---|
| Product / VirtualIp | 99 / 1 | 生成输入；目标库已另有素材迁移工具，必须对照旧 ID 映射。 |
| ModelImage | 97 | `(product_id, ip_id, outfit)` 去重；可被多个 StyleImage 复用。 |
| StyleImage | 179 | `model_image_id` 指向 ModelImage；98 条当前没有下游 FirstFrame。 |
| FirstFrame | 83 | `style_image_id` 指向 StyleImage；23 条当前没有下游 Video。 |
| Video / VideoTask | 127 / 153 | 所有 Video 有 `model_image_id`、`style_image_id`、`first_frame_id`、`movement_id` 字段；26 个 VideoTask 无 Video。153 个 VideoTask 的 `params.source` 均为 `manual_upload`。 |
| VideoPush | 75 | 61 条引用多个 Video；CSV 共 124 个引用、121 个不同 Video ID，其中 10 个引用当前找不到对应 Video。状态包含 pending/completed、已发布和未发布。 |
| WorkflowTemplate / Movement Link | 1 / 2 | 模板只给定候选输入与动作集合，不能把每条 Video 都无条件归到该模板。 |

完整性核对：127 条 Video 中有 2 条的 FirstFrame、StyleImage、ModelImage 引用在当前源表中均无法解析。Video 的 `pose_id` 在大多数记录中为空，正确的姿势与场景应沿 `StyleImage.pose_id` 和 `FirstFrame.scene_id` 读取，不应只信 Video 的冗余列。只有当 VirtualIp、姿势、场景和 Movement 全部吻合时才能关联到某个 WorkflowTemplate；否则保留为无模板谱系或异常项，不静默猜测。

迁移排除规则（用户确认）：**断链记录不导入目标**，输出旧 ID 与排除原因供计数核对；不直接物理删除源库。断链指 Video 所需的 ModelImage/StyleImage/FirstFrame 缺失、VideoPush 所引 Video 不存在，或无 Video 的 VideoTask。上游存在而尚无下游的 StyleImage/FirstFrame 属于**未完成的 Workflow Execution**，必须导入以测试继续执行；未匹配当前 WorkflowTemplate 也不是断链。当前只读分组核对：53 个完整 VideoPush 分组、8 个有 Video 但无有效 Push 的分组、98 个停在 StyleImage 的分组、23 个停在 FirstFrame 的分组；另有 50 条源记录被排除。此清单只恢复关系，不代表目标端已导入。

## 迁移单位与待定映射

源侧的可重建单位是“一条有来源的产物谱系”，不一定是一条 VideoTask：相同 ModelImage、StyleImage 或 FirstFrame 可以供多个视频分支复用；一个 VideoPush 又可能合并多个视频。重建程序要先从 Video 和 VideoPush 反向连接，再去重上游节点，输出拓扑、来源 ID、时间、所属用户/Team、提示词/媒体路径以及缺失引用。不能把每张表一行一行直接转换为新 StepRun，也不能把未完成的中间产物丢掉。

目标端尚无公开的“历史执行导入/继续”契约；当前 Generated Asset 内部 upsert 要求真实 Job/StepRun 归属。下一步按已确定的语义，将停在 ModelImage/StyleImage/FirstFrame 的完整谱系导入为 `awaiting_resume` Job 与带历史来源标记的已完成 StepRun；用户显式补齐后续输入后，启动**新的** Conductor 实例，仅执行后继节点。导入和继续执行必须有旧 ID 幂等键、对象搬运、异常隔离及已完成节点的恢复分支，不能调用普通 SubmitJob 伪造历史。

## 能力覆盖检查

| 原能力/数据 | 目标归属候选 | 状态 |
|:---|:---|:---|
| Product、VirtualIp、Material、Movement | `media_materials` 对应聚合 | covered（已有独立迁移工具，需复核旧 ID 对照） |
| ModelImage、StyleImage、FirstFrame、Video 媒体与生成依赖 | Generated Asset + 历史 Execution | missing（目标导入契约未定义） |
| 无下游 Video 的完整图片谱系继续执行 | media_workflow Execution/StepRun + Resume Workflow | missing（目标 Resume API 与 Conductor 恢复分支未实现） |
| VideoTask 单任务状态、输入/输出、失败与时间 | 历史执行读模型 | missing（不能伪装为新 Job） |
| VideoPush 剪辑分组、文案、发布状态 | 待确定外部所有者/新 Domain | missing（本轮不迁移发布动作，但须保留历史） |
| WorkflowTemplate 的姿势/场景/动作选择规则 | `media_workflow` Workflow Input/Binding | partial：四节点联调样本可表达，多动作分支和历史版本未完整覆盖 |
| 旧 CLI 编排与失败恢复 | 新 Workflow/Execution | partial：已有新 DSL 与 Worker 联调，旧历史不可直接重放 |

`missing` 项解决前不能宣称“所有数据已迁移”，也不能把当前只读清单脚本当作完成的迁移程序。
