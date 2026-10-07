# 原生执行边界收敛

2026-10-07用户调整优先级：先完成核心功能与完整联调，部门/角色/资源ACL扩展在核心验收后统一添加，不再插入当前核心执行收尾。保留既有Token验证、任务归属与租户隔离。已写但未部署的ACL改动须与核心部署分离；部门ACL隔离PG门禁按用户转向中止，不记PASS。权限项是后续阶段，不标记已完成。

ACL代码已从当前核心构建撤出，归档于`/tmp/media-workflow-deferred-acl.4pQHJ3/resource-acl.patch`（基于当前HEAD的可恢复补丁，含原新建测试；重新合入时复核核心改动冲突并重跑IDL生成）。下方第一阶段/race检查记录属于撤出前证据，不代表当前构建包含ACL扩展。没有部署权限版本或删除用户数据。

依据[spec](../../spec/media_workflow/conductor-native-execution.md)。替代durable-retry的接线计划，保留历史证据但不继续新增执行控制层。

## 当前状态（最终部署复验）

下方阶段性“未部署/待复验”描述保留为当时记录，以本节最新证据为准。核心源码整改和公开单样本闭环均已部署验证；UI、Agent、权限扩展、全量历史迁移与引擎启动幂等扩展仍明确不纳入本轮。旧计划的自建可靠 Retry/局部 DAG 重置不继续实现。

- [x] N2/N3 核心：原生 retry adapter/公开入口、旧本地调度源码清理、纯查询投影、审批精确 HUMAN、取消/PAUSED 投影、原生 Task 超时权威、输入覆盖 whole rerun、Provider 分类、Finalize 真实登记。
- [x] N4 核心：独立 CLI + 独立客户端 mock 的一条完整四节点流程、异步日志关联、部分技术边界复验与 [wiki](../../wiki/workflow-conductor.md)。各门禁分别证明，不用部分协议测试代替完整客户端流程。
- [ ] 后续阶段：部门/角色/资源 ACL 扩展；按用户要求暂缓，不作为本轮新增开发。
- [ ] 后续阶段：历史迁移/恢复运行与真实厂商接入；不冒充本轮 mock 验收。

### 输入覆盖与真实交付时间边界

公开部分执行固定键 `boundary-probe-20261008-single`、Job `job_63630368-245c-45f3-ace8-7a6c2437835a`、原实例 `7311eede-9a90-48b1-867f-8b7c0be6ca8d`。Submit 19.02s PASS；首轮协议门禁 6.55s 在 input_override 返回400，不记PASS。只读检查发现原生 rerun 已成功、新输入与 Task 已出现，本地确认却失败。原因是平台直接比较 Go 结构体与解码 map 的 JSON 字节，字段顺序不同；不是 Conductor 调度缺陷。以规范化 JSON/UseNumber 修复，新增真实 ProductData/VirtualIpData wire 输入与大整数回归；首轮新增测试类型名拼错的编译失败也不记PASS，修正后通过。

没有再次 Submit 或换键；明确核对原实例后，只在该部分样本通过协议失败当前 Task，再执行授权的 FAILED whole rerun。193.65s PASS：新输入正确确认并原子保存、retry_count=1、原 trace 保留；等待真实本地90秒到期后心跳200，再等下一90秒到期后 Complete 200/submitted。未改数据库租约，未启动模型 Worker、未审批，不形成第二条完整流程。最后 Job/启动记录 cancelled，原实例 TERMINATED；Generated Asset新增0，Inbox仍93条/零新增，User活动队列0。

### 最终镜像与只读回归

API `sha256:01f6b6d3d1e443671646037f5cf6b8ea50ddcfc60c1b699dd6df270afe372ed6`、内部 Worker `sha256:959e75ca3230448b608bae34af5b8dcf5d2863b4b0600bba471bb8f1a07ffbbc` 已部署；Materials沿用已验收镜像。仅更新仓库内这两个服务，未修改/重启 Conductor、未启动可观测性。数据库24/dirty=false，Vite5173 HTTP200。

最终输出契约审查拒绝字符串/小数候选位置，以及显式非字符串 URL/content、非布尔 is_primary，负向证明不向原生 Task 更新坏结果；可缺省/null的兼容零值未变。Workflow全量race、CLI命令/e2e race、Job/Worker关键race与架构/构建通过。部署后已完成原样本 `job_d47d2b73-bd6a-4aaa-9f4f-1ebaf5ab232d` 的四份 Range206及完整SHA256复读3.78s，旧本地任务路由公开410复验0.72s；没有新Submit或Worker执行。

最终上传负向复验首轮0.35s填错实例（提交键不是Worker实例），被403正确拒绝，不记PASS。只读核对原Dispatch绑定为 `artifact-e2e-20261007` 后，原身份无Token/User拒绝、完成Task准备410门禁0.42s PASS；没有签名授权、上传或原生Task更新。Wiki本地链接与diff检查通过。

- [x] N1：运行中Conductor 3.32.4 + 实际编译器并行/循环图的原生retry技术门禁。36.13s PASS；保留成功B分支、A原render、恢复失败invoke、新增合流后完成。前两次误将5秒空poll当失败，改为观察同实例的异步JOIN后通过；不作为Conductor故障或补偿依据。
- [x] N2：撤出未部署自建Retry层，添加原生adapter；更新DDD当前目标并保留变更记录。
- [x] N3 核心：普通失败恢复API/Step入口、查询投影和Worker交付以引擎身份/状态为准；替换手动DAG重置，保留输入覆盖功能的明确兼容语义。下面历史阶段记录以顶部最新状态为准，权限扩展单独暂缓。
  - [x] Job/Step普通retry接线，撤出局部影响规划与节点重置；原生retry不增加旧rerun周期。
  - [x] failed缓存可刷新；投影跳过retried旧Task；Personal/Managed活动Task验证替代缓存窗口callback。
  - [x] 旧claim/complete/fail公开路由410退役，main不再启动本地Claim扫描器；接线回归通过。
    - [ ] 旧调度器代码清理、审批/取消/租约全面收敛、Provider错误分类及输入覆盖rerun边界验收。
    - [x] Worker产物上传与输出契约：见[上传plan](../worker-artifact-upload.md)。任务绑定RPC、客户端result_files、回写前媒体校验、查询候选响应URL已实现并部署；同一四节点样本实际上传并完成审批/反馈/失败后原生retry及四份产物SHA256读取验证。首轮因验收脚本旧缓存误判失败，修正后继续同实例通过，不隐去失败或另Submit；无真实模型调用、历史迁移或UI修改。
    - [ ] Worker超时边界部署复验：移除本地Reaper和过期准入，原生Task决定心跳/结果有效性，同身份重投保留原ID/attempt只刷新记账。Workflow全量、关键race/架构与构建通过；真实Conductor过本地时间仍交付+丢响应确认4.45s、隔离PG绑定/记账刷新/心跳/结果35.83s通过。技术门禁不代替部署业务完整流程，不重绑定不同Worker/凭证/实例。
    - [ ] input_override部署复验：源码改为原生精确终态准入、固定版本输入校验、提交后GET确认及明确的查询缓存失效；不增加执行队列。Finalize取本轮原生商品/人物来源，防止本地审计延迟错误归因。
      - [x] Job/Finalize单测与race通过；隔离PG事务36.55s覆盖缓存/审计回滚、取消、并发周期；真实Conductor+隔离PG丢响应33.70s通过，一次POST、原旧Task删除、重复RUNNING请求不再POST。技术简单HUMAN图不代替完整编译业务图部署验收。
    - [ ] 旧调度源码退役部署验收：本地领取/完成/失败、旁路审批、DAG推进/扫描器及DAL任务池端口已删除；Submit不再无引擎回退；旧IDL地址deprecated+Handler410，旧CLI本地协议测试改网关410。Workflow/CLI全量、关键race、架构、gofmt/diff通过，公开网关旧地址410门禁1.01s通过（不写资源）。最新源码尚未部署，不删除历史数据。
    - [x] 内部Finalize真实登记/原生身份/稳定结果键；Get/List只投影、不登记。
    - [x] 内部错误分类与真实Conductor自动retry、登记响应丢失恢复门禁。
    - [x] Provider失败分类源码：IDL/客户端/Personal与Managed直接映射；真实Conductor自动retry和永久失败门禁通过；直接回写版本已部署并完成公开单样本复验。
    - [x] 用户确认Worker结果撤出Inbox：同步代理精确原Task确认、Personal本地恢复交付、生产发送器删除；部署33.28s完整单样本通过，Inbox零新增，见[直接交付plan](../worker-result-direct-delivery.md)。
    - [x] F12两个校验缺口源码修正：Template ID+Version快照与共享入口required逻辑OR；回归先复现失败再通过，Workflow全量/关键race/架构检查通过。未部署此新增校验；资源权限仍待整改。
    - [ ] 审批原生权威：HUMAN冻结候选、只存决定事务、精确Task送达复核及完成选择投影。源码与技术门禁通过，运行镜像尚未更新，部署复验仍待完成。
    - [ ] 取消/状态投影：native活动实例准入、持久意图、清理实例身份、PAUSED独立投影、TERMINATED取消语义；源码与技术门禁通过，生产迁移24/部署复验仍待完成。
      - [x] Workflow全量单测、Job/Start Delivery/Review Delivery/Handler race、archcheck、gofmt/diff通过。
      - [x] 实际Conductor+隔离PG取消恢复23.13s，启动记录事务22.58s通过；新暂停PG测试首轮因未绑定实例而失败，补正确绑定后40.73s通过，覆盖暂停/恢复/终止投影与有paused时拒绝降级。没有运行数据库迁移或用户数据修改。
      - [x] Workflow全量单测、Job/Review Delivery race、archcheck、gofmt与diff检查通过；覆盖旧候选/状态缓存、接受不推进执行、原生完成选择投影、忽略HTTP更新不误确认。
      - [x] 隔离PG审计34.80s、审批事务59.73s；真实Conductor+隔离PG丢响应31.62s、真实进程重启78.12s（领取退出与提交退出）通过。没有修改用户数据；不是公开Gateway部署验收。
    - [x] 资源权限第一阶段源码：Template Update/Archive/Publish/Delete与公开版本读取身份检查；Template/Workflow Grant改用认证caller，IDL granted_by废弃兼容。service及handler负向/真实owner正向race通过。未部署，不关闭部门viewer/editor共享与组织边界缺口。
- [x] N4：部署HTTP与独立CLI/客户端Worker的一条完整流程（失败继续、审批反馈循环、多候选、结果读取），trace_id与wiki收尾；不涉及UI/全量历史。见顶部最终证据。
  - [x] 部署Workflow API/内部Worker，一条独立CLI Submit + HTTP反馈/重放 + CLI原生retry/确认 + 客户端mock执行 + 四产物读取通过。
  - [x] 恢复既有Vite开发进程，192.168.2.41:5173直接HTTP200；未改UI代码/设计。
  - [ ] async trace_id关联、剩余执行边界整改后复验及最终wiki。
    - [x] 异步关联见[追踪plan](../workflow-trace-correlation.md)：API接受、独立后台启动、独立内部Render、Personal客户端、API确认及Materials上传准备/对象校验实际日志同trace_id。新镜像已部署，受控部分执行在首节点审批时取消，不重跑第二条完整流程；不启动可观测性、不修改UI或扩大权限。rerun/历史恢复关联源码回归不冒充全部剩余门禁。

覆盖：原生并行失败恢复external-owner（Conductor）+技术门禁covered；普通retry源代码接线、失败缓存刷新、活动Task验证和旧路由退役covered。完整投影/Finalize/错误分类/审批取消租约/输入覆盖兼容/部署流程missing。旧未部署持久Retry控制层deprecated-with-reason（重复执行权威，用户要求Conductor拥有语义）。有missing不宣称Goal完成。

2026-10-07阶段证据：Workflow全量单测、关键包race、架构与diff检查通过；实际Conductor并行retry38.62s，原生交付复测0.88s；隔离PG审计37.12s、Personal鉴权29.90s、Conductor+隔离PG Managed执行43.66s通过。均未更新业务运行镜像，不代替N4。

2026-10-07第二批证据：真实编译图+内部Runner/Finalize+HTTP登记协议替身，TCP登记响应丢失后原生自动retry恢复同Asset且不调用Job GET，5.17s PASS（不冒充Materials真实存储）。纯投影原子性隔离PG43.19s PASS。API镜像`sha256:a7e8f2b36383ee696beb55d271508800bc66c9fd8cdfb920287cea29eab6b3db`、内部Worker`sha256:9378614cba7bec85b3707c9e567b61be6c077bc0d3f7e8c34eab0010a8465ae1`已部署，库23/dirty=false。CLI仅提交Job `job_eb6c7f61-740b-49b0-9267-df1aabcc090c`；FixtureConfig13.77s、Execution83.82s、产物Range206读取4.13s PASS。实际Materials保存4条独立真实Asset，均有Object Path；Conductor COMPLETED，第二轮model invoke原失败标retried、新invoke retry_count=1，两轮render均只一次。mock客户端已停止，后端/Vite常驻，无真实Provider调用、历史迁移或用户数据删除。
