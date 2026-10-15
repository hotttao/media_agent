# F1 · Prompt 企业资源库与 Agent 配置接口

## 完成能力

- Prompt 企业资源库创建、可见列表/详情、owner/editor 内容编辑、owner 部门 ACL/归档；企业内 public 不跨企业。
- 当前 Auth 企业成员及部门状态参与最终授权，支持 viewer/editor Grant；跨企业、失效部门、资源不可读和 Workspace 无 manage 分别拒绝。
- 内容历史查询、固定版本读取、回滚生成新版本；当前 ACL 控制历史内容访问，历史不被覆盖。
- 无归属历史 Prompt 保留为原 owner 私有，可列出并一次性归入本人所属有效企业；已有 Workspace 资源按真实企业迁移，企业资源不再随原 Workspace 级联删除。
- Agent 配置确认支持 none/custom/library，保留原资源引用请求兼容；直接填写和显式清空形成新 Release，库引用选择固定历史内容。
- 浏览器可读取已确认配置，库正文仍校验资源读取权限；配置确认同时要求 Workspace manage 和资源授权。幂等键冲突与 CAS 冲突拒绝，不重复提交通知意图。
- IDL 驱动生成共享模型、客户端、路由和 OpenAPI。

## 验证

- `go test ./media_control/... ./hertz_infra/authn/resourceaccess`：通过；覆盖 owner/editor/viewer、企业边界、部门 Grant、Auth 故障、回滚和归档。
- Control `archcheck`：通过；`git diff --check`：通过。
- 使用本机受限 Docker PostgreSQL，实际迁移全部 Control schema，经 Hertz 路由和实际 HTTP 调用验证完整资源与配置链路，最终 `TestPromptLibraryAndExplicitConfiguration` 通过（17.492s）。Auth 为明确的 HTTP 身份事实夹具；数据库、仓库、事务、handlers、权限及 Release/Outbox 是实际实现。
- 现有 Agent Metadata 集成夹具已更新为真实 Prompt 服务授权；本次只做编译/非 live 检查，未重新启动 RabbitMQ，也不把未运行的 live 分支记作通过。

复现：提供独立 PostgreSQL 管理 DSN，运行 `MEDIA_AGENT_PRODUCT_TEST_DSN=… go test -count=1 ./harness/tools/agent_product_integration`。测试只创建并删除本轮独立数据库；不运行 Kubernetes、不新增运维功能。

## 交付边界

本阶段完成后端 Prompt 管理与配置接口，不宣称完整产品或正式模型可用性已验收。Skill 完整内容管理、企业 MCP/Plugin 实现、管理界面及正式 Agent 使用入口按 F2–F4 继续。Penpot 新项目由独立 subagent 并行设计，设计审查另行记录。
