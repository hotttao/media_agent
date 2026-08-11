# ADR-0002：当前 App 单实例，模型兼容多实例

## 状态

Accepted

## 决策

当前 Sessions、Chat 和 Workspace Settings 在一个 Workspace Profile 内最多各打开一个实例。App Instance 仍拥有独立稳定 ID，Profile 使用实例数组，未来可按 App Definition 开启多实例。

## 结果

当前交互简单且不会重复打开相同 App，同时避免未来演进时迁移 Slot Profile 数据结构。
