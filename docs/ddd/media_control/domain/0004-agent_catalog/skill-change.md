# Skill 变更记录

## 2026-08-22：Skill 首个垂直切片

- 新增 owner-scoped Skill 元数据 CRUD，内容仅保存 Storage 相对路径与 checksum。
- 内容路径拒绝绝对路径和目录穿越；不在 Control 中读取或保存 Skill 明文内容。
- 支持 `active → disabled/archived`、`disabled → active/archived`，归档不可逆；所有转换使用 `expected_version`。
