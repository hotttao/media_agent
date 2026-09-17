# Execution Capability 变更记录

## 2026-09-17：建立版本化 Worker 能力契约

- 新增 Execution Capability 聚合，把 Provider 路由、schema、输出类别和 Conductor Task Definition 从 Template 自由字符串中分离。
- published 版本不可变；Template 必须固定引用一个版本。
