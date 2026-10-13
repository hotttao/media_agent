# Agent Session Startup 变更记录

## 2026-10-11 · 按原设计收窄 Runtime Controller，创建入口归 Gateway

- 撤销本次讨论中扩大为跨平台运行管理器/探活服务的设计。
- Gateway 使用 Kubernetes 客户端提交 manifest，自定义控制器创建/协调 StatefulSet 等承载资源；StatefulSet/kubelet 负责副本与探针，Gateway 直接与 Agent 通信。
- Control 保留业务配置/Placement，Gateway/Agent 协调业务排空和 queued 重绑；Controller 不查询 Turn 或判定 Agent 健康。
- Docker 由有 Docker 权限的创建服务提供单实例容器，本期不做扩缩容。

## 2026-10-11 · 区分 Platform Prompt 与 Configured Prompt

- Gateway 迁入的是平台模板管理与 Platform Prompt 正文生成，不是全部 Prompt 的所有权或最终提示词合成。
- Configured Prompt 的内容与版本继续归 Control，两类内容分别交付；Agent 负责 Runtime 特有转换、落盘与加载。
- 生成职责与生效时点分离，不改变 Session 固定版本，也不隐含热更新。

## 2026-10-11 · 参考 agent-platform 的副本路由与共享存储

- Session 绑定副本持久化；Ready 且非 draining 时沿用，扩容不改变有效绑定。
- 支持缩容及不可用/draining 副本上的 Session 重绑，替代本次讨论中较早的禁止缩容设想。
- 多副本共享 Workspace RWX 卷及 HOME；执行互斥在 Session 级，共享文件采用独立的原子发布规则。
- 保留本项目 Agent-owned FIFO；恢复扫描限定负责范围，重绑不得自动重复未知工具副作用。
- 运行所有权与排空准入的实现细节待 spec 明确，本次仅更新目标设计。

## 2026-10-11 · 统一 MCP 入口与 Prompt 职责

- Agent 统一连接 Gateway MCP 入口；Gateway 负责工具发现、授权过滤和调用代理，后端负责具体实现和资源授权。
- Gateway 组装 Platform Prompt；Agent 保留 Runtime 特有的转换、落盘与加载；Control 保留期望配置。
- 目录归属、发现刷新与协议兼容细节尚未定稿，本次不宣称实现完成。

## 2026-08-18 · 拆分 Gateway Registration 与 Agent Runtime Session

- Gateway 继续拥有 Session 身份、访问控制与 Token Hash；Agent 新增同 ID 的 Runtime Session。
- Agent Runtime Session 注册成功后 Gateway Registration 才进入 active。
- 两侧通过幂等接口和稳定 session_id 协调，不共享数据库或跨库事务。
