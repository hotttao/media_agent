# 2026-08-06

将 Platform Token 从 `service_token` 聚合拆出；本 domain 只保留用户创建的 CLI/API Service Token。内部验证术语统一为 `IntrospectServiceToken`，返回 Principal。
