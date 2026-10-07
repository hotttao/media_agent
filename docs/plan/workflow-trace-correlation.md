# Workflow异步关联落地

依据[spec](../spec/workflow-trace-correlation.md)。依赖既有本地非采样追踪，不新增可观测性进程。

- [x] T1源码：冻结启动traceparent、后台恢复与Conductor HTTP注入；接受/重放同一关联、独立后台Context的queued/uncertain恢复、无效兼容、无采样/导出、header注入/隐私及关键race测试通过。真实跨进程/跨服务运行日志证据仍属于T4，不以Context单测冒充。
- [x] T2源码：Internal/Managed/Personal实例关联恢复与安全日志；客户端传输/本地结果恢复；不改Provider参数。读取精确实例只读元数据，2秒上限，失败只失去关联、不改变Task资格。Personal关联只在响应副本；客户端提取并移除保留字段，独立Task字段随原私有结果记录恢复。Internal/Managed恢复本地span；Managed剔除元数据且保留JSON数字精度，不把大整数转浮点。日志用固定字段/状态，不打印正文；运行证据仍待T4。
- [x] T3 当前核心：rerun元数据保留规则、兼容回归与公开运行门禁；历史Resume运行门禁按用户要求暂缓，不记PASS。
  - [x] 源码：rerun从原生Input保留有效原traceparent，不使用触发请求的新trace；Resume在首次启动JSON捕获当前请求。Job Retry实际用例回归、同键Submit新trace不改变原清单及无效字段测试通过；未执行新历史恢复/迁移。
- [x] T4：更新运行镜像并通过公开请求、异步执行及Materials同trace_id日志实测；保留测试失败证据，不冒充完整Goal验收。证据如下。

2026-10-07：domain-modeling先固化追踪仅为传输元数据，不进入执行/授权/提交去重语义；通用W3C术语未加入业务词典。不修改已发布Definition/Compilation，不增加数据库表。Workflow全量race、关键race及架构检查通过；轮询错误不再保留Conductor正文，防止固定日志打印输入/签名。增加rerun保留调用时首轮漏import导致编译失败，补正确import后全量race通过，失败轮未算PASS。T2未实现，最新追踪源码未部署，原上传验收镜像继续运行；5173 HTTP200。

T2阶段：Personal响应关联不写原Dispatch/native输入、坏关联不阻断；Internal Runner恢复原实例root，业务输入不变，原Task更新header继承root；Managed Runner恢复root并剔除元数据，大整数9007199254740993精确保留；Managed HTTP不依赖全局exporter初始化；固定结构日志从Context派生trace并转义ID，不接受调用方伪造trace_id。客户端poll提取/清除业务meta，心跳/Complete/Fail header继承；真实私有结果文件重启恢复关联，Storage PUT无追踪/Token、成功/412/丢响应路径通过。客户端全量race、Workflow全量race及后续关键race、构建/架构/gofmt/diff通过。没有更新运行镜像，不宣称T4完成。

## 2026-10-07 T4运行证据

Workflow API镜像`sha256:308387764e6faaeb787fa22332f08a121520aa19d97b4f51dc8af9ac9d1cd621`，内部Worker`sha256:c7354030bbd6eee8ff497d482d60eb1cbc39d0298c823e228c198324358b08e8`已部署，Materials保持既有已验收镜像。没有启动可观测性或重启/修改Conductor，最新全量race与架构/构建通过；CLI新增门禁全量/关键race通过。

队列审计无其他该User活动Job，从已完成样本仅复用授权的素材快照，公开Submit带W3C父上下文，以固定键`trace-probe-20261007-single`接受唯一**部分执行** `job_1e3cd6a1-42be-4946-a577-6b6a2c8d9054`，原实例`59dd854a-d4be-46a9-bef4-3297a6720b4b`，18.70s通过。没有重跑第二条完整模型流程，也没有修改已完成样本。

原生暂停/恢复技术操作只针对这个已核对归属的实例，公开API PAUSED=8/RUNNING=2投影3.24s通过，不增加平台暂停调度器。首次配置准备与暂停并行，1.06s安全拒绝PAUSED样本且没有生成配置文件；恢复后按顺序准备20.30s通过，未绕过状态校验。独立客户端只注册jimeng图像能力，从私有本地文件上传首节点两候选，不自动审批。

实际日志逐项匹配 `trace_id=6f47731a6d414c85958022531982a167`：API workflow_submission_accepted、独立后台workflow_start_confirmed、独立内部进程internal_task_started/updated（Render COMPLETED）、Personal worker_task_delivered、独立客户端worker_task_completed、API worker_result_confirmed，以及Materials prepareUpload四次200（签名/HEAD确认）和resolve两次200。Materials各span_id不同，但trace_id相同；日志无请求输入/输出或凭据/签名。只用既有本地非采样关联，不靠collector/exporter。

首节点到HUMAN后，公开取消及重复取消、原精确实例TERMINATED确认3.96s通过；原Job和启动记录均cancelled。负向0.51s通过：无Token/User不能准备Worker上传，取消后原Worker准备410。Generated Asset新增0（没有审批/Finalize），Inbox仍93/submitted零新增。只留下本测试的未引用上传对象，不自动删除用户数据。客户端已停止，后端常驻、5173 HTTP200，UI/权限扩展/历史迁移未改。

本门禁证明所要求的跨进程/客户端/Materials日志关联与暂停/取消投影，不冒充Managed真实Provider或历史恢复运行验证；T3剩余门禁仍须明确核对。

## T3 当前核心收口

部分技术样本 `job_63630368-245c-45f3-ace8-7a6c2437835a` 的公开 input_override 整次原生 rerun、后续 Worker 交付保留首次启动 traceparent，193.65s PASS。原生提交确认时结构体与 map 的 JSON 字段顺序导致首轮误判，修复后只继续已核对的原实例；没有再次 Submit，也不把第一次失败记PASS。详细边界与清理证据见[总计划](media_workflow/conductor-native-execution.md)。历史 Resume 仅有已列出的源码回归，不启动历史迁移补运行证据。
