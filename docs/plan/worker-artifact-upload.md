# Worker产物上传落地

依据[spec](../spec/worker-artifact-upload.md)。依赖当前Worker直接交付和原生执行边界，不扩大为权限体系或新调度器。

- [x] U1：Storage签名PUT、摘要和条件写原语；单测/race通过，现用SeaweedFS真实协议0.66s通过（错误内容拒绝/无对象、PUT成功、重复412、HEAD重放/冲突、GET字节一致）。只写并清理随机e2e对象，未触碰用户文件。
- [x] U2准备器源码：Materials企业/Scope/任务键/内容清单路径、格式与16/256MiB边界，单测/race及架构通过。
- [x] U2端口源码：Materials内部准备RPC、Workflow绑定任务RPC及共享Managed入口已实现，IDL/router/model/swagger由生成器更新。真实路由内部凭证缺失/错误401、任务身份/凭证/attempt/native失效、类型/大小/格式、下游路径/URL/Header及内部client安全错误映射测试通过。尚未更新运行镜像，不等于公开网关验收。
- [x] U3源码与关键race：Worker本地文件helper与mock result_files（URL互斥）、结果媒体元数据校验、Get/GetStepRun响应副本短期URL解析已实现。客户端覆盖PUT成功/已上传/412/提交后丢响应恢复、禁止重定向与Token转发、错误类型；服务端覆盖企业/Scope/类型/大小/文本及未知存储拒绝、不更新native、已确认回写重放不重新查询Storage；查询不持久化签名/不登记Asset。全量最终回归与实际部署完整流程仍在U4，不以HTTP替身测试冒充真实存储完整验收。
- [x] U4：更新运行镜像，公开网关+独立Worker单条四节点完整流程及文件读取；不调用真实模型，不批量迁移，不触碰UI。证据如下。

风险：签名约束或条件写在Provider不受支持时不得声称不可覆盖；上传和Task完成非跨系统原子事务，未知先HEAD，不重发生成。上传在本地结果日志落盘前，进程崩溃窗口仍需Provider幂等/固定结果恢复。旧已签URL无法随Task取消即时撤销，晚到对象只能未引用；不自动删除用户对象。这条路径已验收，不代表真实模型Provider接入、Managed真实模型执行或整个Goal已完成。

本轮修正：合法Capability测试先暴露IMAGE/VIDEO领域枚举与小写MIME前缀比较错误，修复后通过。丢响应模拟首次race发现测试HTTP端口共享计数/状态未同步，添加锁后客户端全量race通过；未将失败轮记为PASS。Workflow全量race第一轮与新源码编辑重叠导致main vet读取旧包快照而失败，源码稳定后必须完整重跑，不沿用该轮成功包输出作为全量证据。

## 2026-10-07部署验收

源码稳定后Workflow、Materials、客户端全量race通过；两后端架构检查及API/内部/Managed构建通过；CLI全量与关键race、diff检查通过。Materials镜像`sha256:e2930e59dfd7f6401fb74ea22231d9aa2beb8d00fd6bdee0ffb075a61635396a`，Workflow API`sha256:21cb51acf57f45901badd2763c823377172a3ea470ff32da3b3ddff5ab6d1caa`，内部Worker`sha256:ce3a947b7509fd9016972c1468d282809884e47821354a0d7e0a9710704727c0`；仅更新这三项服务，数据库迁移24/dirty=false，无可观测性或外部Conductor变更。

队列审计测试User无其他活动Job。独立CLI预检后，以固定键`worker-artifact-20261007-single`仅Submit一次：`job_d47d2b73-bd6a-4aaa-9f4f-1ebaf5ab232d`，原生实例`f9f1023d-e29c-465f-9439-66e4a569a9b6`。配置门禁21.55s下载既有真实图片/视频到新的私有0600文件，`result_files`配置；独立客户端使用Worker API Token经公开网关准备、直传真实SeaweedFS、HEAD确认后仅回写Object Path。

首轮执行门禁3.30s因测试把接受regenerate后的旧缓存当作新轮提示词而失败；原生HUMAN已完成regenerate且第二轮Render包含反馈，模型第二轮按计划失败。修正测试等待新投影后，明确标记已有regenerate决定，继续同一实例，不新Submit、不再次regenerate、不手工改数据库/回收Task。恢复门禁26.25s通过：独立CLI原生retry一次，第二轮原invoke标retried、新invoke retry_count=1；选择候选1，四节点完成并登记4条独立Generated Asset。所有invoke媒体结果只含Object Path；审批/反馈与同键重放通过。首轮失败保留，不宣称首次一气完成。

完成后公开只读字节核对3.97s通过：三个PNG和一个MP4 Range206，全部位于新worker_generated命名空间，完整下载SHA256与对应客户端文件逐一一致。公开负向门禁0.77s通过：无Token/User身份不能准备Worker上传，已完成Task的原Worker准备请求410。Inbox仍93/submitted，零新增；测试Worker已停止，三后端服务保持运行，5173 HTTP200，未修改UI/权限扩展/历史数据。
