# F3a · 企业 MCP 目录

## 完成范围

Gateway Definition 增加不可变企业归属，保留平台定义与全局稳定 ID。企业管理员按 Auth 当前企业角色管理所属定义；企业成员读取本企业与平台目录。平台管理员身份不能替代企业管理员事实。更新采用 CAS，历史只追加，退役不可恢复。数据库迁移覆盖当前与历史定义，并为企业定义禁止 platform/session_context。

Auth 内部企业成员接口保留 organization_ids，增加当前有效企业的 role/status 事实。共享事实客户端兼容旧响应，缺少角色时不授予企业管理权。Gateway 公开目录采用现有已验证管理身份，内部接口仍只读并要求服务凭据。

Gateway MCP 调用根据已验证 Agent 身份对应的真实企业归属检查当前与固定历史定义，即使 Plugin 错误返回跨企业授权也拒绝调用。

认证类型已扩充 none/oauth/bearer/api_key，凭据管理及代理接通属于 F3b。本阶段不表示外部 MCP 已可完整配置使用。

## 验证

- Gateway、Auth 全模块 Go 测试及共享 resourceaccess 测试通过（/tmp/agent-product-f3-catalog-checks.txt）。
- Gateway、Auth archcheck 均通过。
- 独立 PostgreSQL 实际执行 Gateway 全量迁移，真实 HTTP 测试通过（7.931s，/tmp/agent-product-f3-catalog.txt）。覆盖企业管理员登记、普通成员读取、跨企业拒绝、平台角色不越权、企业不可迁移、认证约束、版本冲突、退役、成员撤销、不可变历史及事件原子保留。
- MCP 调用测试覆盖外企业、缺失企业及历史企业不匹配均拒绝，同企业接受。
- IDL 通过 hz 重新生成模型、客户端及路由，并生成 OpenAPI。

## 尚未完成

Workspace Plugin 绑定、固定配置选择、静态凭据及 OAuth 可用凭据端口、Plugin 代理、界面和正式 Agent 使用仍需后续切片；未以目录接口完成宣称 F3 或产品完成。继续功能开发，无新增隔离、扩缩容或 SRE 建设。
