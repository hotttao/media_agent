
## 2026-10-15 · 复用 Auth 连接事实

连接状态作为 Auth 当前连接与批准 Definition 的读模型，取消重复持久化连接事实。Auth 既有 user/server_origin 连接负责授权、callback、断开与刷新；Plugin 仅保存非秘密绑定并在每次代理时校验固定 Release。避免不必要的连接缓存和后台同步。
