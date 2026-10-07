# Worker结果直接交付

## 范围

只调整Personal/Managed Capability结果回写；不修改UI、历史迁移、初次启动/发布/审批Outbox或外部Conductor。Task输出/状态由Conductor拥有；对象存储/Materials副作用仍需自身幂等，不承诺跨服务事务或端到端exactly-once。

## 服务端

保留Complete/Fail路由及DTO。验证Worker identity/scope、Dispatch owner/企业/attempt及Job实例映射，读取精确原生Task核对输入身份。已匹配终态只确认，不要求lease仍有效；异结果/异失败分类冲突409。首次提交须实例RUNNING、Task活动未retried、lease有效，旧Task/取消拒绝410，不重定向新Task。

直接POST原TaskResult后GET确认status和完整output JSON（失败核对分类/原生reason）。POST响应丢失也查询恢复；不能仅依HTTP成功确认ignored update。引擎不可查或尚未确认返回503/固定安全错误。确认后仅更新Dispatch交付记账，不写Job/Step/Inbox。记账失败可重放确认。不同内容并发更新遵循原生引擎语义，不承诺跨系统线性化保证。

响应200/submitted，receipt_id为Dispatch/attempt派生的不透明ack而非数据库行。客户端必须核对submitted，不能将旧pending当完成。fail分类空/permanent→FAILED_WITH_TERMINAL_ERROR，transient→FAILED；技术重试由冻结Task Definition决定。

删除生产RunResultSubmitter/SubmitReceipt和新结果SaveResult调用；旧Inbox表/仓储存档不删除。启动检查pending/failed旧回执，存在则拒绝切换。部署须先停止旧版本再核对，避免双版本新增旧回执。

## 客户端

执行函数一次，先持久保存返回的成功/失败结果再发送。state-dir/MEDIA_WORKER_STATE_DIR默认.worker-state；0700目录、0600文件，临时写入+fsync+rename+目录fsync，网关/实例/凭据摘要隔离，不保存Token或重复prompt。同命名空间进程文件锁，结果不可覆盖；坏文件/不安全权限拒绝启动。

注册后先重发待确认结果，再poll；恢复不调用executor。确认后删除自己的pending；409/410保留.rejected，其余错误/退出保留原结果/稳定key。运行中交付持续心跳；离线恢复不创造新lease或attempt。轮换凭据不自动转移旧记录。

生成副作用完成但结果落盘前的窗口仍要求Provider幂等/查询恢复。Managed同次函数执行保留返回值、重试发送并持续心跳；进程退出后的恢复依赖Conductor至少一次任务与下游幂等，不声称拥有Personal文件日志保证。

## 验收

单测/race覆盖响应丢失查询、终态重放0额外POST、异结果冲突、HTTP成功未采纳不确认、身份与旧Task、Inbox零新增；客户端权限/坏文件/锁/凭据隔离及真实子进程落盘退出后恢复交付。真实Conductor验证正常回写与自动retry/永久失败/丢响应；隔离PG验证记账及无Inbox写入。技术门禁不替代部署网关/独立CLI/客户端完整流程。
