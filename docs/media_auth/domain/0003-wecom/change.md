# wecom domain 变更记录

## 0001 — 从 control_plane 抽离初始建立
- 来源：`control-plane/src/routes/wecom-auth.ts` + `src/services/wecom.ts`。
- 独立成 domain（与 proto 1:1），承载企微 corp OAuth 通道；身份读写经 identity 内部能力，session 签发经 auth 内部能力，wecom 自身不直读 users/user_identities 表。
- 无持久化表；corp access_token 内存缓存语义不变（2h TTL，1min 安全余量）。
- `state` 参数沿用 control_plane 的 mode 字符串（login/bind）语义。
