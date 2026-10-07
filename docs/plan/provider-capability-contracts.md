# Provider Capability纠偏落地

依据provider-capability-contracts.md，保持检查点eb75d53与旧运行数据；不涉及UI。

实施基线：001_auth.md四类Task；platform+generation_method+generation_type组成核心身份，版本固定schema。原三条种子草案已撤回，改为即梦i2i/豆包i2v各首帧与多图两种；真实Provider适配不在本轮。执行P1时仍只使用隔离数据库，不把代码/种子存在当作上线。

当前状态：P1–P4代码、部署及无人工干预单流程契约验收完成，运行库23/dirty=false。首轮失败/人工恢复证据保留，2026-10-07修复交付窗口后新回归通过；不宣称完整持久Retry及全部Workflow收尾完成。具体边界见[验收记录](../review/media_workflow/workflow/0003-conductor-compilation/provider-capability-acceptance.md)。

2026-10-07后续状态：精确延迟交付修复已部署，新增单回归Job`job_79ba8969-bc99-48af-b8aa-bf9bc781e090`完成HTTP重新生成/重放、独立CLI重试/候选确认及四节点执行，24.46s无人工引擎操作通过；媒体和终态回执也通过。原失败证据保留，窗口修复不等于持久Retry恢复任务全部完成，详见[窗口计划](media_workflow/retry-delivery-window.md)。

- [x] P1：规范身份/版本、技术OutputMediaType、资源媒体/数量约束、migration023四类能力、DAL读取与旧null兼容。domain/Repo race覆盖四类身份、17组不一致拒绝、旧类型不猜测、Managed独立Key/Pool及首帧/多图数量；隔离PG `TestProviderCapabilityMigrationAndRegistry` 38.30s通过：真实22→23前后旧记录完整JSON指纹相等、四类契约有效读取、八路同版插入单赢家、规范Key校验、发布冻结/禁止删除/不能原位转换旧契约、归档不可复活、降级保护。P1隔离证据先于部署，运行接入证据见P4。
- [x] P2：Template业务Asset Kind与技术输出兼容校验、Invocation Mapping参数与IR/Compiler冻结。Template/Publication/语义编译器测试覆盖同一即梦能力复用model_image/style_image、IMAGE/VIDEO与未知媒体拒绝、首帧数量上限、图片顺序、注册元数据缺失/禁用/错误resource_type/不可用、输入schema无引用共享。旧IR规范JSON字节不变，旧provider=client编译不写新契约字段。Workflow全包测试、相关五包race与架构检查通过；代码门禁不单独证明运行时资源MIME或Worker接入，部署证据见P4。
- [x] P3：客户端Provider处理器注册与mock配置、Gateway类型/图片顺序/数量校验、Personal/Managed隔离回归；不实现真实模型HTTP适配。
  - 已完成客户端`provider_fixture`四类契约执行入口、精确Job/Step私有配置、按配置选择注册能力、服务端已渲染prompt/图片数组与数量校验；旧profile明确隔离保留。Worker全包race通过，测试覆盖同能力多节点/多Job隔离、输入顺序、首帧与多图、未知版本/内部任务拒绝、候选拷贝与私有URL不泄漏。
  - Gateway新增Provider参数声明校验，Personal/Managed的新领取和重交付均校验；错误输入不创建Dispatch。Materials解析响应增加存储HeadObject的content_type/size_bytes，IDL重新生成；Gateway校验受控对象企业归属、存储MIME/非空及签名有效期，外部URL拒绝，重交付重验MIME。S3客户端、公共/内部解析响应、下游client与Gateway测试覆盖透传和错误媒体/未知MIME/空对象/签名过期/跨企业/外部URL拒绝。
  - Worker全包race、Workflow/Materials全包、关键Gateway/client/Storage/handler race与两服务架构检查均通过；代码阶段的证据不代替运行联调，实际部署与边界见P4。
- [x] P4：显式能力配置的新样本脚本、部署、公开HTTP与独立CLI的受控单流程契约验收；保留全部旧能力/模板/执行，不重写历史。
  - 先部署兼容的新Materials解析协议，再部署Workflow/migration023，最后配置精确Job/Step启动客户端测试Worker。Materials旧响应缺少媒体元数据时新Provider交付应fail closed，不能静默跳过。实际启动前检查全部待处理队列，只运行一个完整样本。
  - 新脚本显式选择即梦多图/豆包首帧，模板名含契约引用且参数改为images；修正PRODUCT/VIRTUAL_IP聚合图像输入，不错误查询AssetKind。脚本/Publication race和架构通过。Materials/API镜像已部署、库23且4类新Provider契约有效。
  - 260条Job完整审计活跃0，CLI dry-run/单次Submit；唯一样本job_7c51a130-2743-412f-b8cc-c29a69c5ac99、新Workflow wf_2c1231e5-7379-4207-a0de-a9bb5d93b278@1。FixtureConfig 4.18s通过；Execution首轮120.72s失败，已确认原Retry交付窗口。只回收该实例一条已领取未交付的Invoke任务，原Job继续观察14.18s通过，未创建第二个Job/实例，未重新发送regenerate/retry。此成功**不是**无干预重试验收通过。
  - 四节点成功、model retry_count=1/selected_index=1，输出3 PNG+1 MP4 Range206；终态回执重放/反向409通过1.03s。真实IR/Conductor/Inbox核对前三节点复用同一即梦契约，视频豆包；全部prompt/images且mock=true，内部Render服务端执行。测试Worker已停止、后端常驻，UI/历史全量/Agent未改。

原能力处置矩阵：model/style/first_frame业务用途归Template，旧能力为deprecated-with-reason（保留既有契约，退出新样本）；Provider契约/客户端fixture处理器和部署无人工单流程为covered，持久Retry故障/崩溃恢复门禁仍missing并归原可靠Retry任务；Render/Finalize/审批/Gateway身份为covered，不重建；模型原生调用为external-owner（未来客户端Provider适配器，本轮仍mock）。不以本轮契约验收关闭未完成的可靠重试或整体Goal。
