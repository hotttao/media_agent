# Provider Capability纠正验收（2026-10-06）

## 范围与状态

本轮只纠正底层Capability身份、输入输出契约、模板/IR/Conductor编译和客户端fixture。UI、全量历史迁移、真实模型适配、可靠Retry重构不在本轮。

契约接入及部署完成，单个Job在受控恢复后完成四节点。**首次无干预重试门禁失败**，不能将本报告作为可靠Retry已完成或无人工恢复执行已全面通过的证据。

## 部署与定义

- Materials镜像：`sha256:4a723d2ee1d062507e081472a023888c35872527a2d32e2bee437b9ade0856a7`。
- Workflow API镜像：`sha256:be85cf95a3913eb4ec0e2270c52f46d8c685f96b3ccb263a0dd37ecff3ec3878`；数据库23，dirty=false。
- 保留旧Capability、模板版本、Job和Compilation；新增四类规范Provider契约，没有重写旧定义或迁移全量历史。
- 新Workflow：`wf_2c1231e5-7379-4207-a0de-a9bb5d93b278@1`。
- 唯一测试Job：`job_7c51a130-2743-412f-b8cc-c29a69c5ac99`。
- 原Conductor实例：`ae3af148-8681-45fb-ab60-3548361d7e4e`；全过程未创建第二个Job或替代实例。

冻结IR及实际成功Invoke核对：

| 业务Step | Capability | 输入图片数 | 业务输出 |
|:---|:---|:---|:---|
| model_image | jimeng.i2i.multi_image_prompt@1 | 2 | model_image |
| style_image | jimeng.i2i.multi_image_prompt@1 | 2 | style_image |
| first_frame | jimeng.i2i.multi_image_prompt@1 | 2 | first_frame |
| video | doubao.i2v.first_frame_prompt@1 | 1 | ai_video |

全部Invoke使用prompt/images，没有旧resources参数；结果带mock=true/profile=provider_fixture。5个Render任务由服务端执行。即梦首帧/豆包多图契约另有domain/PG/Worker输入数量门禁，本次完整示例不使用它们，不宣称验证了Provider原生API。

## 测试时间线

1. 完整分页审计260条Job、活跃0。新模板名称含Provider引用，原旧模板不更新。
2. 独立CLI先dry-run，再以稳定键只提交一次；输入选择Product、Virtual IP、pose、scene及纯文本movement。
3. `TestProviderCapabilityFixtureConfig`真实网关预检通过4.18s：精确Job/Step、模板引用、仅当前样本活跃、Materials受控对象MIME/大小/有效期；创建0600私有配置，不启动Worker。
4. 独立客户端Worker使用Worker API Token经网关注册两类契约。首轮模特图返回两候选；HTTP请求重新生成并同键重放。第2个model Dispatch显式mock失败；独立CLI执行Step Retry。
5. `TestProviderCapabilityExecution`首轮120.72s超时失败。Conductor重试产生新Invoke并已被poll，但当时本地Job尚为failed，Gateway拒绝交付；该任务没有创建Dispatch，后续poll返回204。原实例保持running；此时不能伪造成功或重发Submit。
6. 核对该Job的User/企业、原引擎实例、精确任务`708bac93-ccb5-4021-b037-662b37d7f41e`为IN_PROGRESS、业务Step=model_image、底层Task=jimeng_i2i_multi_image_v1且无输出。仅对这一个测试任务进行管理侧IN_PROGRESS/callbackAfterSeconds=1重排队，不修改Job/IR，不写完成结果，不修改外部Conductor源码。
7. 重新解析fixture结果URL、重新启动独立Worker；继续观察同一Job，不再请求regenerate/retry。独立CLI确认候选数组位置1，后续三个节点执行完成；恢复门禁14.18s通过。
8. 全部输出独立无认证Storage Range读取返回206：3 PNG、1 MP4。终态HTTP确认原键返回applied原回执，反向决定409。fixture进程已停止，后端常驻；没有启动观测后端。

## 持久事实

Job=succeeded/retry_count=1；model_image=succeeded/retry_count=1/selected_index=1，其余3个Step=succeeded/retry_count=0。四个Step各有Generated Asset：

- model：`gen_be487512-509a-45e2-8bd8-c2ef466f29c9`。
- style：`gen_32c9f694-ba2e-4435-84aa-69d1f459e1e3`。
- first_frame：`gen_d2d37eb8-72b4-4acd-a2bd-ee80f0cb1acb`。
- video：`gen_1e288fd4-4c27-43a0-94bd-79d7bd8d3266`。

真实结果Inbox共5份complete（即梦4、豆包1）和1份即梦fail，均submitted；与实际Provider Task结果匹配。人工重排队没有生成结果Inbox，结果仍由真实客户端经网关回写。

## 未解决限制

可靠Retry的“引擎已重跑、本地仍终态”交付窗口仍存在，归原Workflow收尾，不借本轮扩大实现。自动执行门禁保留失败记录；受控恢复后的成功只能证明新Capability参数转换、身份/媒体校验、客户端回写、审批、Finalize与媒体读取链路接通。`MEDIA_E2E_PROVIDER_CONTINUE_AFTER_RETRY=1`仅用于已确认原决定/Retry的观察，不自动修复任务，不保证任意失败可恢复。

## 2026-10-07 修复后的无人工干预回归

上述2026-10-06失败/管理回收证据保留。随后仅针对交付窗口增加精确原Task的自动延迟回调，设计/实现及剩余边界见[计划](../../../../plan/media_workflow/retry-delivery-window.md)。此修复不是持久Retry意图，不关闭进程崩溃/不明确重跑等其他可靠Retry任务。

- API已部署`sha256:e081f5c63dd160040662fc6a5967074608e29be4571d6b881f5c64f607395b63`，schema23/dirty=false；Materials不变。
- 261条Job全分页审计活跃0，独立CLI dry-run后仅提交一次新的回归Job：`job_79ba8969-bc99-48af-b8aa-bf9bc781e090`。复用原已发布四节点Workflow，不修改Template/Compilation。
- 原引擎实例：`e4a10828-a1f4-4047-a2c3-9c7384b49ba7`。Worker两契约经网关执行，显式mock第2个model dispatch失败；HTTP原键regenerate/replay、独立CLI Retry/confirm/query直到成功。未设置继续观察模式，未向Conductor手工更新/重排队，未重发Submit。
- FixtureConfig 4.61s通过；Execution 24.46s通过，媒体读取子门禁0.87s；终态回执重放/反向409另通过0.87s。
- Job=succeeded/retry_count=1；model=succeeded/retry_count=1/selected_index=1，其他三节点succeeded/retry_count=0。产物分别为`gen_d591f658-e921-4e96-9f29-e153c492db2f`、`gen_a1358da4-d64b-4773-9f1f-1311a4bd5573`、`gen_437cf7f0-4110-4bcd-85db-8c5dfb192711`、`gen_7111b89d-7294-42d6-9f37-e51cc7eb9959`；3 PNG/1 MP4独立Range206。
- 引擎COMPLETED，五条成功Provider Invoke都明确mock=true/profile=provider_fixture，前三业务节点复用即梦，视频豆包；5个Render仍服务端执行。Inbox为即梦4 complete+1 fail、豆包1 complete，全部submitted。
- 重试后的新Invoke ID=`9220cb4e-7928-441d-8529-875750714b94`、pollCount=2；旧失败ID=`f99e6dea-18ea-498c-9a10-3eb3d7f7b449`不同。24秒内没有600秒任务超时/90秒lease回收，没有管理重排队，与生产代码的2秒精确回调再领取一致；正常回写完成同一新Task。

Capability纠正现在已有部署后的无人工干预单流程证据。完整可靠Retry（持久命令、提交失败/进程退出恢复）、并行DAG恢复、Worker上传与完整异步trace等仍按原未完成任务维护，不能因本例成功宣称整体Goal完成。测试Worker已停止，后端继续运行，未启动观测后端、未改UI或迁移全量数据。
