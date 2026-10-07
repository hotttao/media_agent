# 原生执行边界收敛

依据[spec](../../spec/media_workflow/conductor-native-execution.md)。替代durable-retry的接线计划，保留历史证据但不继续新增执行控制层。

- [x] N1：运行中Conductor 3.32.4 + 实际编译器并行/循环图的原生retry技术门禁。36.13s PASS；保留成功B分支、A原render、恢复失败invoke、新增合流后完成。前两次误将5秒空poll当失败，改为观察同实例的异步JOIN后通过；不作为Conductor故障或补偿依据。
- [ ] N2：撤出未部署自建Retry层，添加原生adapter；更新DDD当前目标并保留变更记录。
- [ ] N3：普通失败恢复API/Step入口、查询投影和Worker交付以引擎身份/状态为准；替换手动DAG重置，保留输入覆盖功能的明确兼容语义。
  - [x] Job/Step普通retry接线，撤出局部影响规划与节点重置；原生retry不增加旧rerun周期。
  - [x] failed缓存可刷新；投影跳过retried旧Task；Personal/Managed活动Task验证替代缓存窗口callback。
  - [x] 旧claim/complete/fail公开路由410退役，main不再启动本地Claim扫描器；接线回归通过。
  - [ ] 旧调度器代码清理、审批/取消/租约全面收敛、Provider错误分类及输入覆盖rerun边界验收。
    - [x] 内部Finalize真实登记/原生身份/稳定结果键；Get/List只投影、不登记。
    - [x] 内部错误分类与真实Conductor自动retry、登记响应丢失恢复门禁；Provider错误分类仍单列待完成。
- [ ] N4：部署HTTP与独立CLI/客户端Worker的一条完整流程（失败继续、审批反馈循环、多候选、结果读取），trace_id与wiki收尾；不涉及UI/全量历史。
  - [x] 部署Workflow API/内部Worker，一条独立CLI Submit + HTTP反馈/重放 + CLI原生retry/确认 + 客户端mock执行 + 四产物读取通过。
  - [x] 恢复既有Vite开发进程，192.168.2.41:5173直接HTTP200；未改UI代码/设计。
  - [ ] async trace_id关联、剩余执行边界整改后复验及最终wiki。

覆盖：原生并行失败恢复external-owner（Conductor）+技术门禁covered；普通retry源代码接线、失败缓存刷新、活动Task验证和旧路由退役covered。完整投影/Finalize/错误分类/审批取消租约/输入覆盖兼容/部署流程missing。旧未部署持久Retry控制层deprecated-with-reason（重复执行权威，用户要求Conductor拥有语义）。有missing不宣称Goal完成。

2026-10-07阶段证据：Workflow全量单测、关键包race、架构与diff检查通过；实际Conductor并行retry38.62s，原生交付复测0.88s；隔离PG审计37.12s、Personal鉴权29.90s、Conductor+隔离PG Managed执行43.66s通过。均未更新业务运行镜像，不代替N4。

2026-10-07第二批证据：真实编译图+内部Runner/Finalize+HTTP登记协议替身，TCP登记响应丢失后原生自动retry恢复同Asset且不调用Job GET，5.17s PASS（不冒充Materials真实存储）。纯投影原子性隔离PG43.19s PASS。API镜像`sha256:a7e8f2b36383ee696beb55d271508800bc66c9fd8cdfb920287cea29eab6b3db`、内部Worker`sha256:9378614cba7bec85b3707c9e567b61be6c077bc0d3f7e8c34eab0010a8465ae1`已部署，库23/dirty=false。CLI仅提交Job `job_eb6c7f61-740b-49b0-9267-df1aabcc090c`；FixtureConfig13.77s、Execution83.82s、产物Range206读取4.13s PASS。实际Materials保存4条独立真实Asset，均有Object Path；Conductor COMPLETED，第二轮model invoke原失败标retried、新invoke retry_count=1，两轮render均只一次。mock客户端已停止，后端/Vite常驻，无真实Provider调用、历史迁移或用户数据删除。
