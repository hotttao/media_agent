# 变更记录

## 2026-10-29 · User MCP 入口 scope

User 签发请求可显式选择 mcp.access，与 api.access 独立，其他 Actor 不接受该 scope。沿用既有一次性 Secret 交付、Talos 管理与清理/撤销流程；新增个人 Settings 签发入口，不自动扩张旧凭证的 scopes。scope 仅授权 MCP 入口，业务工具与资源仍由资源所有服务检查当前权限。
