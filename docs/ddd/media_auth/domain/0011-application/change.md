# 2026-11-08 应用认证与业务访问

新增 Application、受限访问凭证、Team Grant 与 MCP Target。profile 与业务凭证隔离；应用 secret/状态/Grant 变化递增凭证版本。用户映射及托管约束分别由 identity/auth 持有；应用注册不自动赋予成员或资源权限。MCP Target 不授予工具 ACL。SQL 仅在 DAL，跨域写入由 ApplicationUsers Workflow 协调。
