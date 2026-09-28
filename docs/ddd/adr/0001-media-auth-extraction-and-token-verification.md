# media_auth 抽离边界与 control_plane token 校验契约

> 本 ADR 的本地 Cookie JWT 与 Service Token 验证方案已被 ADR 0012 和 ADR 0018 取代，仅保留为历史决策。

把 control_plane 的认证鉴权能力抽成独立 media_auth 微服务后，control_plane 作为网关继续承接非认证路由，其自有路由仍需校验 `Cookie: token=<jwt>` 与 `Authorization: Bearer <service_token|jwt>`。当时敲定：JWT 沿用 HS256 + 共享 `JWT_SECRET` 由 control_plane 本地验（零 RPC），opaque Service Token 走 media_auth 新增的 mTLS 内部 `VerifyToken` RPC 解析，4 个认证专属前缀由网关反向代理到 media_auth。该方案现已退出目标架构。

## 状态元数据
accepted

## 备选方案考量
- **非对称签名（RS256/ES256）+ VerifyToken RPC**：media_auth 私钥签、control_plane 公钥本地验，零共享密钥。被否决：需把 control-plane 现有 HS256 全量迁移、轮换所有已签发 token，迁移成本高；本阶段优先保持与现状兼容。
- **HS256 共享密钥 + sidecar 直读 DB**：control_plane 直读 `users`/`service_tokens` 表。被否决：schema 耦合、双读一致性风险，且与"media_auth 自治 schema"目标冲突。
- **网关层 OAuth2 introspect**：media_auth 暴露 RFC 7662 introspect 端点。被否决：control_plane 改造量大，且 cookie JWT 仍需本地验，引入两套校验路径。

## 衍生影响
- media_auth 本批次 IDL 需新增 internal `token` service（`VerifyToken`），仅 mTLS / 内网暴露，不挂网关路由、不出 OpenAPI。
- control_plane 不再直读 media_auth 任何表 → sidecar 双读过渡态取消，收敛路径 P3/P4 提前。
- `JWT_SECRET` 在 media_auth 与 control_plane 间共享，轮换需双方同步；远期可再迁移到非对称签名（见 ADR-0001 备选）。
- control_plane 中间件解析顺序：Bearer service_token（调 VerifyToken）→ Bearer JWT（本地验）→ cookie JWT（本地验 + 滑动续期）。
