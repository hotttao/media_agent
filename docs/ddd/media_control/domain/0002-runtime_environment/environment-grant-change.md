# Environment Grant 变更记录

## 2026-08-22：运行环境授权投影

- 新增 Environment Grant 的 active/revoked 生命周期。
- Grant 只记录 subject 与 permission，不在控制面响应中携带连接 Token；Token 签发留待独立 secret 边界切片。
