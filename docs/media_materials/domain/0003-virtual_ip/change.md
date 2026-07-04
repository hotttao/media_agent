# virtual_ip domain 变更记录

## 2026-07-03 · 初始建模（media_agent v1）
- 来自 `media_ai` 的 `VirtualIp` 设计迁入并扩展：
  - 把 IP 基础图片（avatar / full_body / three_view / nine_view）作为 URL 字段写在 `virtual_ips` 上，**不**走 asset_bindings——理由：IP 的形象是身份信息，不是从素材库里挑选的；要改 IP 形象直接 URL 改写。
  - 加 `virtual_ip_grants` 表，应用 [Resource Grant](../../media_auth/CONTEXT.md#resource-grant资源共享授权) 统一模型。
  - 加 `ResolveVirtualIpForReachability` 跨域 RPC：解决"通过不同 vip 看到的 asset 集合不同"问题。
- 用户反馈"template / workflow 应跟 skills 和 virtual_ip 一样属于资源"——这里落到 virtual_ip 域即镜像同一权限模型，作为该模型的源头之一。

## 来自业务规则的强约束
1. scope 不可改。理由同 template/workflow/asset。
2. soft_delete 不级联删 asset_bindings——只打 `bindings_end_at`，让历史追溯可见。
3. 不开放 editor grant 之外的 `viewer` 之外档位（用 `public` 替代"全平台可见"）。
