# Workspace Provider API Key 由 Control 加密托管

Workspace 的 Provider API Key 需要供后台 Turn 使用，不能依赖创建请求或浏览器持续在线，因此由
`media_control` 在 PostgreSQL 中使用认证加密密文托管。公开 Workspace API 只返回
`api_key_configured`，不返回明文或密文；只有经过 Workspace owner 授权的 Runtime Context 内部接口
可在调用期间解密，Gateway 仅将明文随单次 Agent `/chat` 请求传入。Agent 将密钥放入子进程环境，
不得写入 `/workspace`、日志、URL、localStorage 或 Universal Event。

采用该方案而不是浏览器保存、Gateway 数据库复制或 Agent PVC 长期保存，可以维持单一秘密所有者并让
Codex 与 Claude Code 使用同一安全契约。当前本地部署从 `CONTROL_ENCRYPTION_KEY` 派生 AES-GCM 密钥；
生产环境必须提供独立高熵密钥，后续迁移 KMS 时需要引入 `key_version` 并滚动重加密。

## 状态元数据

accepted
