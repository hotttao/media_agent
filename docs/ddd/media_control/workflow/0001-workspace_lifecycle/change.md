# Workspace Lifecycle 变更记录

## 2026-10-11 · 按原设计收窄 Runtime Controller，创建入口归 Gateway

- 撤销本次讨论中扩大为跨平台运行管理器/探活服务的设计。
- Gateway 使用 Kubernetes 客户端提交 manifest，自定义控制器创建/协调 StatefulSet 等承载资源；StatefulSet/kubelet 负责副本与探针，Gateway 直接与 Agent 通信。
- Control 保留业务配置/Placement，Gateway/Agent 协调业务排空和 queued 重绑；Controller 不查询 Turn 或判定 Agent 健康。
- Docker 由有 Docker 权限的创建服务提供单实例容器，本期不做扩缩容。

## 2026-10-11 · 区分 Controller 健康观测与 Agent 执行状态

- Runtime Controller Port 管理资源与观测 Pod/容器的存活、Ready、运行代次、终止，不查询 Turn 或决定队列排空。
- Control Workflow 经 Gateway 向 Agent 发起排空命令/查询，保持 Agent 业务 API 的统一入口。
- 容器终止与此前工具调用的业务结果分别判断，不以探针失败直接写 Turn 终态。

## 2026-10-11 · 缩容时 queued Turn 随 Session 重绑继续

- draining 副本完成当前 running Turn 后停止领取下一条，无需清空 queued 队列。
- 已接受 queued Turn 保留原 ID/FIFO/配置版本，由后台主动协调重绑，新副本获得所有权后继续，不等待新用户输入。
- 无可用候选时保持等待；原失败/中断暂停不隐式恢复，执行凭据在新副本按固定引用重新协调，不迁移秘密。

## 2026-10-11 · Runtime Controller Port 职责确认

- Control 持有运行目标，通过 Port 协调资源创建/更新、存储、观测、扩缩容、停止/删除与持续状态协调。
- Kubernetes Operator 与 Docker Controller 实现环境适配；Gateway 保存 Session 绑定并路由，Agent 拥有执行事实。
- 缩容由 Control Workflow 协调排空，查询不可达不视为排空；Controller 不决定 Session 迁移或 Turn 终态。

## 2026-10-11 · 参考 agent-platform 的副本路由与共享存储

- Session 绑定副本持久化；Ready 且非 draining 时沿用，扩容不改变有效绑定。
- 支持缩容及不可用/draining 副本上的 Session 重绑，替代本次讨论中较早的禁止缩容设想。
- 多副本共享 Workspace RWX 卷及 HOME；执行互斥在 Session 级，共享文件采用独立的原子发布规则。
- 保留本项目 Agent-owned FIFO；恢复扫描限定负责范围，重绑不得自动重复未知工具副作用。
- 运行所有权与排空准入的实现细节待 spec 明确，本次仅更新目标设计。

## 2026-08-22 · 在启动 Agent 前协调持久化运行目录

- Runtime Controller 先幂等取得 Workspace Runtime Directory，再以 `/workspace` 挂载启动 Agent。
- Workspace ID 同时作为目录归属与 mount identity；冲突不可自动覆盖。
- 普通 stop/restart 不清理文件，rebuild/delete 的清理必须是显式生命周期语义。

## 2026-10-16 · 新建即具备默认运行配置

创建授权在事务外校验，Workspace、稳定 Agent 配置和初始 none/空选择 Release 在同一事务提交，失败整体回滚；用户不需要先保存空配置。资源升级仍要求显式确认。
