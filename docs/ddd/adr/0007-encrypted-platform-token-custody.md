# Platform Token 采用加密托管

Platform Token 需要支持离线任务和少数旧式服务的长期委托，因此 `media_auth` 必须能够取回明文，但数据库不再保存裸明文。采用独立凭据记录保存 `token_hash + ciphertext + nonce + key_version`，由 Secret/KMS 提供密钥；仅 `media_agent_gateway` 与 `media_control` Schedule Worker 可通过 mTLS 身份和固定 purpose 获取。该决策替代 ADR-0002，避免 Agent、Plugin、自定义 MCP 和通用业务服务接触 Platform Token。

## 状态元数据

accepted；supersedes ADR-0002
