# Execution Capability 变更记录

## 2026-09-17：建立版本化 Worker 能力契约

- 新增 Execution Capability 聚合，把 Provider 路由、schema、输出类别和 Conductor Task Definition 从 Template 自由字符串中分离。
- published 版本不可变；Template 必须固定引用一个版本。

## 2026-09-25：明确 Capability 与 Invocation Mapping 边界

- Capability 只声明 Worker 固定输入 schema 与 Task Definition，不保存业务 Slot 映射。
- Template Version 的 Invocation Mapping 负责生成 Capability 参数；Semantic Compiler 在发布期完成覆盖性与类型检查。
