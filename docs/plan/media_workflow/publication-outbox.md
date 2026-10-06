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

- [ ] IDL状态/发布回执、服务权限与并发状态守卫、CLI和标准main装配；不做UI。
- [ ] publish/pending/恢复后published的HTTP与独立CLI门禁，只有注册完成版本可Submit。
- [ ] 全包/race/架构/gofmt/diff、部署和已有执行回归；不以P1证明整个功能完成。

## 当前提交检查点

提交 `5dd6501` 保存P1与P2编排代码检查点。后续已补齐P2单位/race、真实Conductor/隔离PostgreSQL及独立进程恢复测试；Workflow全包、架构、gofmt/diff通过。标准main和公开发布入口仍使用旧同步路径，migration022未部署；P3接线和公开HTTP/CLI验收尚未完成，不计为发布恢复已上线。
