# 应用Agent的幂等创建

触发：已验证应用Bearer请求PUT /apps/v1/agents/:name。Auth验证当前Application/User/Grant/Team，Control创建者始终取验证后的UserID。

```mermaid
sequenceDiagram
 participant B as 应用后端
 participant C as Control应用Agent工作流
 participant A as Auth权威
 participant D as Control数据库
 B->>C: token + name + agent configuration
 C->>A: VerifyBusiness(workspace.ensure)
 A-->>C: User / Application / Team / Organization
 C->>A: 当前create_workspace授权（既有TeamAccess，事务外）
 C->>D: BEGIN; advisory lock(app,user,team,name)
 C->>D: 查询绑定
 alt 已存在
 C->>C: 比较配置digest
 else 首次
 C->>D: 保存Workspace+AgentConfiguration
 C->>D: 确认初始Metadata release
 C->>D: 保存不可变ApplicationAgentBinding
 end
 C->>D: COMMIT
 C->>A: 重复读取时校验Workspace当前view权限（事务外）
 C-->>B: 不含Secret的Workspace信息
```

唯一键是幂等键；advisory事务锁串行同键，无Workflow事实表。所有本地写入使用同一事务context，不调用另一个开启事务的创建Flow。metadata失败全部回滚；网络断开后重放返回同一Workspace。第一次配置参数形成摘要，后续冲突409，不返回或记录明文API key。Auth远程失败则不进入写事务，已提交Workspace不因响应丢失删除。既有绑定不会在Workspace不可用时自动重建或改绑。

GET按app/user/team/name查询；Gateway检查Session时按app/user/team/workspace查询绑定，再执行当前view权限。外部ID不直接作为Workspace ID，浏览器/应用提交的User/Org/Team不能覆盖验证主体。无额外领域事件；运行资源创建继续由既有Gateway/Controller流程管理。
