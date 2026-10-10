# Workflow 单条 UI 联调记录

2026-10-09。本文记录实际样本，不代替全部测试完成声明。整体设计取舍见 [编译与 Conductor 边界](workflow-conductor-design.md)，剩余验收见 [计划](../plan/workflow-ui-query-and-integration.md)。

## 样本与边界

测试账号1556824234@qq.com；企业8afeb26b9df643952f4f816576eb2504；Workspace为“历史工作流联调”。旧兼容定义缺少企业/Workspace，因此按冻结模板版本创建隔离企业资源，不修改旧表或旧定义的归属。

Workflow：wf_4e7e7f21-efc2-4eba-a8de-d37c20cafb53。唯一Job：job_e8a850ec-20dd-445f-b7cd-e89fe1b35592。所有节点配置always审批与force_new；前三节点引用jimeng.i2i.multi_image_prompt@1，video引用doubao.i2v.first_frame_prompt@1。能力是Provider契约，不是业务节点名。

输入由真实UI按照Input Schema选择；通过普通用户会话经Vite代理/统一网关提交。客户端media_worker是独立进程，使用已有Worker API Token，经网关访问Worker Gateway；不直接访问Conductor、不承担渲染。Renderer/Finalize仍由media_workflow内部Worker执行。

## 实际过程

| 业务节点 | 实际客户端执行 | 用户决定 | 最终结果 |
| --- | --- | --- | --- |
| model_image | 两次，每次两候选 | 首轮反馈再生成，次轮选择第2候选 | succeeded |
| style_image | 一次，单候选 | always要求显式确认 | succeeded |
| first_frame | 一次，单候选 | always要求显式确认 | succeeded |
| video | 一次，单候选 | always要求显式确认 | succeeded |

生成循环与HUMAN等待由编译后的Conductor执行，UI提交决定，不在浏览器调度下游。再生成回执review_3097089c-4469-40d8-abd8-085a1ac13f07已applied；测试等待同一回执与真实need_confirm状态，不使用retry_count猜测轮次。单候选在always策略下仍须审批，单输出不等于只允许生成一张图片。

外部客户端完成五个Dispatch，日志同一trace_id为20edfebd1c7f96ad00466c9263640a06。mock返回已存在素材的确定媒体，未调用真实厂商，不能据此评价生成质量。

最终Job succeeded，四个StepRun succeeded。独立查询Generated Asset并解析底层存储URL，媒体Range读取全部206：

| 节点 | 独立产物ID | 媒体 |
| --- | --- | --- |
| model_image | gen_8a6ffdf1-d092-4ac5-854a-20568f46adde | PNG，1486762字节 |
| style_image | gen_8cde9694-75c0-4296-aae5-39c3be427500 | PNG，1486762字节 |
| first_frame | gen_340b8b1d-b282-4a1c-ba6d-f4818ab28d3d | PNG，1875292字节 |
| video | gen_9435930f-05bd-48c5-8f4c-332a11b5d8c9 | MP4，4381181字节 |

同一底层mock图片可被不同节点接受，但业务产物ID独立。表中ID不是签名URL，不记录Token、完整输入或提示词。

## 意图保护与测试纠错

测试分submit/review两个门禁，以忽略的0600私有账本保存Workflow、Job和决定回执。Worker由操作员在两段之间启动，测试内没有fake Worker。

提交确已成功，初版测试却把IDL回执job_id误读成id，导致断言失败；实际UI已进入执行详情。只读查询新隔离Workflow确认恰好一个Job并恢复账本，没有再次Submit。脚本字段已修正，但本轮没有为了取得绿色submit测试而重跑创建。这是测试脚本错误，不是Conductor启动幂等保证；上游启动幂等扩展仍按原决定暂不实现。

review浏览器门禁通过。仅归档本次四模板/定义，保留成功Job和产物供用户查看；已停止mock客户端，业务服务保持运行。119个历史待继续Job没有启动或批量迁移。

仍须另行完成原生失败继续、输入覆盖整次重跑、历史补缺Resume的隔离验收及完整UI回归。这条成功样本不证明这些分支，也不证明真实厂商或media_platform接入。

## 隔离分支测试补充

同日三项后端门禁通过：真实 Conductor 编译并行流程的原生 retry；隔离 PG +真实 Conductor 输入覆盖 rerun 响应丢失后的恢复（不再次 POST）；隔离 PG 历史 Resume 命令事务与重放。未重跑上述成功 Job，未启动119个原历史待补缺 Job。测试代码分别为 `TestConductorNativeRetryCompiledParallelWorkflow`、`TestNativeInputRerunRecoversLostResponseWithoutSecondPost`、`TestResumeOutboxTransactionAndReplay`。最后一项属于既有历史恢复命令机制，不是 Worker 结果 outbox。

UI 全量单测34文件/116项通过；新增冻结输入表单7项组件测试另行通过，包含缺失冻结证据、旧版本输入、只补缺、原 Job 整次重跑，以及 Resume 同请求键重放/Rerun 不盲目再次 POST。类型检查通过。这些仍不代替真实 UI 的失败继续/重跑/历史补缺端到端门禁。

浏览器第一轮10项中7项通过：编辑器、模板映射与实例草稿、素材及生成产物窄屏、商品准备运行、standalone切换和打包plugin测试宿主。多语言、虚拟形象及商品详情3项在初始导航点击阶段超时，未进入业务断言。测试补充显式认证 Shell 启动等待，复用既有启动预算；复跑结果另记，不把这些超时隐瞒为全绿。

复跑多语言及商品详情通过；虚拟形象仍未通过启动门禁。读取本地 trace 的脱敏摘要发现 `Desktop.tsx` 动态 import 失败，该模块HTTP响应为200；进一步只运行虚拟形象并记录失败请求，多个模块同时出现 `net::ERR_NETWORK_CHANGED`。该次未进入虚拟形象详情业务断言，不能宣称页面验收通过；不靠继续增加超时或绕过认证掩盖。测试请求日志只输出路径/状态/脱敏网络错误，不输出响应正文、URL查询或签名。8个涉及测试文件的定向 lint 通过。未改变其他服务、网络配置或生产加载逻辑。

## 后续恢复验收

后续单独复跑虚拟形象详情通过，含图片和窄屏、无写入；原10项现在均有通过结果，不删除此前网络失败记录。

失败继续专用Workflow `wf_476ce74e-3a82-41e9-8a0b-0c12158c96dd`、Job `job_f0db5d4c-c8e4-48bb-98f9-be80cf16ec56`。UI提交通过，独立客户端首个model_image Dispatch永久失败；UI以无input_override的原生retry继续同Job，下一Dispatch返回两候选，UI取消。下游三个节点均未执行。浏览器门禁通过，trace_id `93540f7dab3cd70b0de576713ca57a29`。只归档该定义/四模板，保留cancelled Job。

从该cancelled Job尝试输入覆盖重跑被400拒绝，暴露前端错误提供入口。服务端保持FAILED/TIMED_OUT准入；UI限于非历史failed（含超时投影），不在成功/取消记录上提供该入口。设计文档同步，相关组件2文件12测试通过。这不是扩大Conductor能力，也不是取消成功样本后强行重跑。

另创建输入重跑专用Workflow `wf_75a2a258-92ce-4fc6-857f-a8e99f6d0e1e`、唯一Job `job_b057f058-3cff-494a-beb4-9545c2f55892`。UI提交通过，独立客户端首节点永久失败后停止；UI读取版本1、选择至少一个不同的真实资源、对原Job提交input_override，原Job进入running后UI取消，定义/四模板归档。该浏览器门禁通过，不新建重跑Job，未审批执行到下游；初始失败trace_id `fd37efa197d55702c407bcb94309fe10`。两个mock客户端均已停止，服务保持运行。

历史样本的冻结查询已只读核对200，但实际隔离Resume运行尚未完成。导入入口的迁移Token当前禁用，不自动开启该权限或修改原119条待继续Job；已有事务/编译单测不是实际Resume成功证明。

历史补缺表单的真实浏览器只读门禁已通过：冻结版本、只展示缺失槽位、兼容资源可选、提交按钮准入，读取前后原输入、119任务之一的状态和已完成输出均不变。浏览器拦截所有业务写请求，无Resume发出。当前270条Job为144 succeeded、7 cancelled、119 awaiting_resume、active为空。实际隔离Resume仍待临时导入入口授权，不能据此关闭该验收项。

最终静态/组件门禁：相关2文件12测试、类型检查、定向lint、依赖边界212模块/708依赖、多语言键、standalone与plugin双构建、diff-check通过。只修正现有UI执行准入与过期错误文案，未新增引擎语义、真实Provider、平台集成或性能优化。设计技能约束继续保留Slot、Header Portal与服务器状态权威；未修改用户DESIGN.md。

## 隔离历史Resume最终结果

用户授权临时导入入口后创建独立Workspace `91b0ddc9-1a86-45d5-a1fc-acdb296ec224`、Workflow `wf_cef4d150-3e05-4b06-85f4-604d072d0cb3`，复制一条已验证的三图片谱系，生成唯一Job `job_359bbae4-ab14-49f9-bed1-a2c77b4c9be8`。临时Token仅存在进程环境，导入后立即关闭入口，最终部署后仍确认关闭；未写Token到文档、浏览器或仓库。新Workspace保留供review，只归档这次四模板/定义。

该冻结历史版本的视频能力仍为 `media.video.generate@1`；显式使用既有历史兼容mock，独立客户端只注册这一能力，不能执行前三生图或Renderer。此证据不等于旧历史定义已迁移为新Provider契约；新Provider契约由前述独立流程验收，不改写历史版本。

联调暴露Resume表单把全局输入缺失误认为待执行输入缺失，错误要求pose/scene。UI按冻结绑定和服务端保留的legacy成功StepRun，仅要求视频使用的movement；模板版本/前缀证据缺失禁止提交，后端仍校验全部事实。企业和显示契约加入候选缓存键，避免Resume子集污染Submit/Rerun。修复及新增单测后全量36文件127项通过。

实际流程：UI补movement→一次Resume回执202→内部恢复三图片→客户端仅一次video Dispatch→一候选明确选中/确认→Job succeeded。trace_id `6e1523a772b47616b271f1d895e5af1f`。三个图片产物ID与来源摘要不变；新视频 `gen_7fbf8f33-d485-4aa0-ba29-e19f035b518c` 可解析及Range读取。初版审批脚本漏选radio而失败，修复后继续观察/审批原Job，没重发Resume。

旧兼容候选原签名后来返回403而登记产物的新链接可读：查询响应现在对已配置origin内的同企业/Scope旧链接重签名，保留原候选快照、不写新的签名，不解析外部URL，同origin越界拒绝。单测/race、完整backend测试通过，服务已部署；只读媒体浏览器门禁证明候选HTTP206、videoWidth大于0，而不仅是后台可访问。

Resume完整浏览器门禁逐条确认原119任务仍待继续；最终全队列271=145成功+7取消+119待继续，active为空。所有mock客户端已停止，业务服务保持启动，不启用观测后端；原成功完整样本未重跑、取消或替换。静态门禁、双构建、依赖边界213模块/712依赖和多语言检查通过。此目标仅查询补齐及standalone联调，未接平台、真实Provider或展开全量迁移/性能/身份工作。
