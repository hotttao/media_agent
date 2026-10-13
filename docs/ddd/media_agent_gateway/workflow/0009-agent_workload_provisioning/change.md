# 变更记录

## 2026-10-11 · 按原设计收窄 Runtime Controller，创建入口归 Gateway

- 撤销本次讨论中扩大为跨平台运行管理器/探活服务的设计。
- Gateway 使用 Kubernetes 客户端提交 manifest，自定义控制器创建/协调 StatefulSet 等承载资源；StatefulSet/kubelet 负责副本与探针，Gateway 直接与 Agent 通信。
- Control 保留业务配置/Placement，Gateway/Agent 协调业务排空和 queued 重绑；Controller 不查询 Turn 或判定 Agent 健康。
- Docker 由有 Docker 权限的创建服务提供单实例容器，本期不做扩缩容。


## 2026-10-13 · P5 工作负载落地

- Control target CAS/请求幂等、CR 排空意图与原生回收确认、代次变化的绑定恢复、缩容到0逐 Session 核查、Docker 文件/容器 UID 一致及私有凭据交付。
