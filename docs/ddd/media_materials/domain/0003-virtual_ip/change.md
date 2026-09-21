# virtual_ip domain 变更记录

## 2026-09-21 · 固化对象上传入口

- Virtual IP 身份图片先经 media_materials 的 Library Object Upload Workflow 上传，CRUD 只保存返回的 Object Path。
- 浏览器不直接获得 media_storage 的调用权限；Storage 不参与 Virtual IP 的 Organization、owner 或 visibility 判断。

## 2026-09-21 · Organization 级资源共享

- Virtual IP 归属和唯一键由 Workspace 修正为 Organization；Organization 成员可按资源 ACL 在企业内共享使用。
- Team Grant 只能引用同一 Organization 的 Team，Workspace 不拥有或复制 Virtual IP。

## 2026-09-21 · media_ai Virtual IP 能力复核

- 补齐 `workspace_id` 和完整 Persona Profile 值对象，覆盖年龄、体型、教育职业、家庭背景、性格、爱好与表达习惯。
- avatar/full-body/three-view/nine-view 从完整 URL 改为 Storage `object_path`。
- 第一阶段只实现 Virtual IP CRUD，不实现 Workflow 或 Binding；图片由调用方先上传至 media_storage，聚合只保存 object path。
- 明确旧 `IpMaterial` 不在本轮迁移范围。

## 2026-07-03 · 初始建模（media_agent v1）
- 来自 `media_ai` 的 `VirtualIp` 设计迁入并扩展：
  - 把 IP 基础图片（avatar / full_body / three_view / nine_view）作为 URL 字段写在 `virtual_ips` 上，**不**走 asset_bindings——理由：IP 的形象是身份信息，不是从素材库里挑选的；要改 IP 形象直接 URL 改写。
  - 加 `virtual_ip_grants` 表，应用 [Resource Grant](../../../media_auth/CONTEXT.md#resource-grant资源共享授权) 统一模型。
  - 加 `ResolveVirtualIpForReachability` 跨域 RPC：解决"通过不同 vip 看到的 asset 集合不同"问题。
- 用户反馈"template / workflow 应跟 skills 和 virtual_ip 一样属于资源"——这里落到 virtual_ip 域即镜像同一权限模型，作为该模型的源头之一。

## 修改记录

### 2026-07-21 · 跨域 RPC 上提 + 依赖修正
- `CheckVirtualIPAccessible` / `ResolveVirtualIpForReachability` 从本 domain 能力清单移除，上提到 [reachability workflow](../../workflow/0001-reachability/reachability.md)（见 [ADR 0006](../../../adr/0006-reachability-layering.md)）。本 domain 仅暴露 vip 可见性只读查询供其编排。
- `dependencies` 去掉 `scope`（已降为 shared）与 `docs/research/permission.md`（非 domain）；`related_workflow` 补 reachability + media_workflow.validation。
- 校验细则 3 的 `bindings_end_at` 置位规则对齐 [asset](../0002-asset/asset.md) §material_bindings（该列现由 asset domain 声明）。
- 校验细则 2 措辞修正：vip 状态为 active/archived 两态，无 published（原"与 template/workflow 一致"仅对 archived 终态成立）。

## 来自业务规则的强约束
1. scope 不可改。理由同 template/workflow/asset。
2. soft_delete 不级联删 asset_bindings——只打 `bindings_end_at`，让历史追溯可见。
3. 不开放 editor grant 之外的 `viewer` 之外档位（用 `public` 替代"全平台可见"）。
