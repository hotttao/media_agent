# Workspace Metadata Tag 变更记录

## 2026-08-22：Tag 与 ResourceTag 首个垂直切片

- 新增 Workspace 内唯一 Tag 及资源关联记录。
- Tag 和关联写操作均校验 Workspace owner；Tag 更新使用 `expected_version`。
- Share Metadata、Public Export 及对象存储交互留待后续切片。
