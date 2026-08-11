# 扩展模型

内置 App 与未来 plugin 最终都转换为 `AppDefinition`，再交给同一个 Registry/Slot Provider。布局和 Slot Container 不导入具体 App。

当前只实现静态内置 Catalog：Sessions、Chat、Workspace Settings。暂不实现 plugin 清单发现、远程代码加载、权限声明、版本协商和故障隔离。

未来 plugin loader 只能新增/移除 App Definition，不得直接修改 Slot Provider 内部状态；不可用或被卸载的 App 在 Profile 归一化时被安全移除。
