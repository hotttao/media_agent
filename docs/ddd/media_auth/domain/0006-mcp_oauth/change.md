# 2026-08-06

MCP OAuth access/refresh token 改为带 `nonce + key_version` 的密文存储；`media_plugin` 只持有连接引用，凭据所有权和刷新继续归 `media_auth`。

## 2026-10-15 · Plugin 内部可用凭据

新增专用 Plugin 身份的可用 access token 端口，复用既有刷新流程，不暴露 refresh token；连接缺失与重新授权明确返回 needs_reauth。
