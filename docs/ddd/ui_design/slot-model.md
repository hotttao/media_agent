# Slot 模型

## 核心契约

```ts
interface AppDefinition {
  id: string
  label: string
  Component: ComponentType<{ instanceId: string }>
  allowMultiple?: boolean
  hidden?: boolean
}

interface AppInstance {
  id: string
  appId: string
}

interface SlotState {
  opened: AppInstance[]
  activeInstanceId: string | null
}
```

当前所有内置 App 的 `allowMultiple` 为 `false`。单实例约束覆盖同一 Workspace Profile 的全部 Slot：打开已存在 App 时移动焦点到其所在 Slot，不得在另一 Slot 重复创建。未来允许多实例时只改变创建策略，不改变 Profile 格式。

## 生命周期

- `openApp`：创建或复用实例，并设为活动实例；
- `activateInstance`：只激活当前 Slot 已打开实例；
- `closeInstance`：关闭实例，活动实例按相邻顺序回退；
- `moveInstance`：本阶段数据契约保留，界面暂不提供拖拽；
- `ensureInstance`：跨 Slot 查找已有单实例，未找到时按 Layout 默认位置或 fallback Slot 打开；
- `filledSlot`：临时让指定 Slot 填满 Canvas，其他 Slot 状态保留；
- 未注册的 App 或损坏实例在 Profile 恢复时丢弃；
- Slot 无活动 App 时显示复用同一 Slot Picker 的空态。

## Layout 与 Dock

- 3 栏默认：`slot-a = sessions`、`slot-b = workspace-settings`、`slot-c = chat`；
- 1 栏默认：`slot-a = chat`，用于聚焦和窄屏；
- Dock 按当前 Layout 的 Slot 分段展示实例 pill，并为每段提供 Slot Picker；
- AppWindow header 不展示实例 pill。左侧是 App Header Portal，右侧由 Shell 提供 fill 和 close；
- P0 不实现 Dock 拖拽、overflow、popout、fullscreen 和 Saved Layout。
