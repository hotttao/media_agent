# Workflow 发布恢复落地计划

依据 `docs/spec/media_workflow/publication-outbox.md`，已有注册保护为前置。

## P1 持久意图核心

- [x] Publication身份/摘要/边界验证及域race测试。
- [x] migration022状态、冻结、FK/唯一性、降级保护；仅隔离库执行，未部署。
- [x] CompilationRepo的caller事务接受、并发去重、claim/lease/CAS、原版本收口与源过期保护。
- [x] 隔离PostgreSQL `TestPublicationOutboxPersistence` race通过5.233s：8路同版去重、接受/完成回滚、错误/过期claim、重建Repo恢复租约、终态/命令不可变、非空降级保护、归档/改版superseded及原版本失败收口。重建Repo不冒充OS进程崩溃测试。

## P2 发布编排

- [x] Compiler冻结定义和排序完整TaskDefinitions；AcceptPublication与同步注册解耦。单位/race覆盖权威源编译（非调用方同版本对象）、完整Capability retry/timeout、确定性排序、模板离线时原意图重放且仍查当前owner。
- [x] Delivery只消费冻结意图、事务外注册、caller Tx/原claim收口、安全失败码和退避；严格冻结契约解码/hash检查，永久冲突不重试、不忽略未知字段。单位/race覆盖暂时失败/重新构建Delivery、无模板/能力查询端口、源归档/改版/删除及注册中变更、陈旧claim拒绝。
- [x] 真实Conductor丢响应、独立进程恢复、源修改/归档时不发布新版本。真实技术元数据+隔离PostgreSQL两场景门禁5.780s：实际TaskDefinition/Workflow注册成功后代理503仍GET收口；本地完成写后回滚，新Repo/Delivery重取过期claim仅GET恢复，写入各1次。独立子进程 `TestPublicationDeliveryRealProcessRestart` 6.676s：注册成功后os.Exit(77)、另一子进程恢复，代理禁止恢复阶段任何非GET，0重写。只创建并清理唯一技术元数据，没有业务实例、Worker或模型执行；源变更保护仍由隔离库/单位证明，不混作公开API验收。

## P3 公开与部署

- [x] IDL状态/发布回执、服务权限与并发状态守卫、CLI和标准main装配；不做UI。移除同步Publisher路径，main启动Delivery；Get授权后投影不透明回执。单位/race权限与重放、隔离PG `TestPublicationEditingVersionGuard` 4.034s：8路编辑仅一次成功、新版本draft、旧意图superseded、owner/归档守卫。
- [x] publish/pending/published的HTTP与独立CLI门禁，只有注册完成版本可Submit。真实网关 `TestWorkflowPublicationGateway` 1.10s，8路发布单回执、202 pending、后台收敛/完成重放/归档409；独立CLI发布及查询0.97s。只创建并归档两条唯一测试定义，无业务Job或模型调用。Submit单位测试覆盖draft/publishing/publish_failed/archived拒绝且无Job/Step/Start副作用，published缺registered也拒绝；故障恢复仍采用P2真实Conductor/隔离库/独立进程证据，不冒充公开HTTP故障注入。
- [x] Workflow/CLI全包、相关service/domain/编排race、架构及gofmt/diff通过；migration022已部署，公开HTTP/独立CLI与已有执行产物/审批回执回归通过。迁移工具只兼容异步发布协议（单次Publish后查询原ID至registered，超时不创建替代源），没有执行历史迁移；单位验证202等待时仅一次创建/发布。

## 当前提交检查点

提交 `5dd6501` 保存P1/P2代码，`928c491`保存P2测试。P3标准main/公开入口/CLI已接线；API镜像8d7244970b23d9185c2fae7baa380089a9e7b73d7cdb45dac87bd5ca1385609b运行，schema=22/dirty=false。公开HTTP及独立CLI通过；已有Job job_d4c5a619-ba10-4e83-b284-950c47fd1d96产物读取0.16s、审批终态回执重放0.37s通过。工具异步协议兼容单位通过0.114s，Vite入口5173为HTTP200。本计划完成不代表启动幂等、可靠Retry、并行分支恢复、Worker上传或最终wiki等其他收口任务完成。
