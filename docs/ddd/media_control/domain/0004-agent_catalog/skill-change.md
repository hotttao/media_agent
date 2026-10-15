# Skill 变更记录

## 2026-10-15 · 内容管理实现契约

- 区分资源 CAS 和供 Agent 选择的发布内容版本；草稿只保存候选 ZIP，发布后追加 skill_versions。
- 增加企业 Grant、来源信息、只写 Git 凭据、文件操作与包下载；凭据密文不进入 Catalog 历史。
- 旧企业归属明确的资源自动迁移，无法确定的保留为原 owner 待归属资源；不猜测企业。
- 用户显式选择发布版本时检查当前资源 ACL、同企业和 Workspace manage；发布本身不发送运行更新通知。

## 2026-10-15 · 完整内容管理与发布

- 用户确认支持 ZIP/tar.gz 上传、Git 仓库/分支/子目录导入和手动同步、在线文件浏览与增删改。
- 引入可编辑草稿和不可变发布版本，支持历史版本查看与下载；草稿不作为运行配置版本。
- 发布或同步不自动更新 Agent，运行选择仍需显式确认。
- 原有仅登记内容路径与校验值的 CRUD 是待扩展基线，不能作为功能完成标准。
- 增加 Skill 内容发布 Workflow，分别描述外部文件产物和本地版本提交；没有新增 SRE 阶段。

## 2026-08-22：Skill 首个垂直切片

- 新增 owner-scoped Skill 元数据 CRUD，内容仅保存 Storage 相对路径与 checksum。
- 内容路径拒绝绝对路径和目录穿越；不在 Control 中读取或保存 Skill 明文内容。
- 支持 `active → disabled/archived`、`disabled → active/archived`，归档不可逆；所有转换使用 `expected_version`。
