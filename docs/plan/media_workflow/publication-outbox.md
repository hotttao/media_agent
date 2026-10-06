# Workflow 发布恢复落地计划

依据 `docs/spec/media_workflow/publication-outbox.md`，已有注册保护为前置。

## P1 持久意图核心

- [x] Publication身份/摘要/边界验证及域race测试。
- [x] migration022状态、冻结、FK/唯一性、降级保护；仅隔离库执行，未部署。
- [x] CompilationRepo的caller事务接受、并发去重、claim/lease/CAS、原版本收口与源过期保护。
- [x] 隔离PostgreSQL `TestPublicationOutboxPersistence` race通过5.233s：8路同版去重、接受/完成回滚、错误/过期claim、重建Repo恢复租约、终态/命令不可变、非空降级保护、归档/改版superseded及原版本失败收口。重建Repo不冒充OS进程崩溃测试。

## P2 发布编排

- [ ] Compiler冻结定义和排序完整TaskDefinitions；接受与同步外部注册解耦。
- [ ] 后台只消费冻结意图、事务外注册、原claim收口、安全失败码和退避；冲突不覆盖。
- [ ] 真实Conductor丢响应、独立进程恢复、源修改/归档时不发布新版本。

## P3 公开与部署

- [ ] IDL状态/发布回执、服务权限与并发状态守卫、CLI和标准main装配；不做UI。
- [ ] publish/pending/恢复后published的HTTP与独立CLI门禁，只有注册完成版本可Submit。
- [ ] 全包/race/架构/gofmt/diff、部署和已有执行回归；不以P1证明整个功能完成。

## 当前提交检查点

发布Flow新增AcceptPublication和统一编译冻结辅助函数，TaskDefinitions按名称排序；新增Delivery只消费冻结意图、注册在事务外、完成在caller事务内。代码可编译、架构检查通过，但编排行为/真实Conductor与独立进程恢复测试尚未补齐，标准main和公开发布入口仍使用旧同步路径，migration022未部署。此提交保留明确的未完成P2/P3，不计为发布恢复已上线。
