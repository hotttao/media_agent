# 2026-08-06

MCP OAuth access/refresh token 改为带 `nonce + key_version` 的密文存储；`media_plugin` 只持有连接引用，凭据所有权和刷新继续归 `media_auth`。
