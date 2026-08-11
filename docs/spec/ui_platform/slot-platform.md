# UI Platform Shell 与 Slot P0 实现规格

## 范围

按 agent-platform 的核心交互实现 Menubar、Workspace Switcher/Dialog、Workspace/Session URL、1/3 栏 Layout、Segmented Dock、Command Slot Picker、Slot Empty State、AppWindow Header Portal 和 fill/close。保留 Sessions、Chat、Workspace Settings 三个内置 App。

不实现 plugin loader、Dock 拖拽/overflow、Saved Layout、popout/fullscreen、服务端 Profile、高级 Session/Chat 和复杂 Workspace 配置。

## 路由

- `/`：Workspace 选择空态；没有 Workspace 时提供创建 CTA；
- `/w/:workspaceId`：Workspace Desktop；
- `/w/:workspaceId?session=:sessionId`：指定活动 Session；
- 无效 Workspace 导航回 `/`；无效 Session 回退第一个 Session并 replace query。

## Shell

- Menubar：品牌、Workspace Switcher、用户菜单；
- Workspace Switcher：搜索、选择、新建、当前 Workspace 设置；
- 创建/更新通过 Dialog，字段为名称、Agent 类型和 API Key；
- Canvas：按 Layout 渲染 Slot；
- Dock：Layout 切换、按 Slot 分段的实例 pill 与 Picker。

## Slot API

`getState`、`openApp`、`ensureInstance`、`activateInstance`、`closeInstance`、`setFilledSlot`。单实例查找覆盖全部 Slot；目标 App 已存在时激活其所在 Slot。关闭活动实例按前一实例、下一实例顺序回退。

Profile v2：

```ts
{
  version: 2,
  layoutId: "1col" | "3col",
  slots: Record<string, { opened: AppInstance[]; activeInstanceId: string | null }>,
  instances: Record<string, Record<string, unknown>>
}
```

通过 `ProfileBackend` 读写；P0 backend 为 localStorage。v1 Profile 可安全迁移或回退默认值。

## Layout

- 3 栏：slot-a Sessions、slot-b Settings、slot-c Chat；左右栏可调整宽度；
- 1 栏：slot-a，Dock 可将任意内置 App 激活到该 Slot；
- filledSlot 绕过 Layout，仅渲染目标 Slot；
- 移动端使用单栏 Canvas，Dock 作为唯一 App 导航。

## AppWindow

- header 左侧提供 portal target；App 注入标题与业务操作；
- header 右侧提供 fill/unfill 与 close；
- header 不渲染 App Instance tabs；
- Slot 无实例时渲染带 Picker CTA 的 Empty State。

## 验收

- Workspace 和 Session 深链刷新可恢复；
- 快速切 Workspace 不发生 Session 结果串写；
- Dock 与 Canvas 活动实例一致；
- 单实例跨 Slot 唯一；
- 关闭最后 App 后可从空态 Picker 恢复；
- 1/3 栏切换保留隐藏 Slot Profile；
- 移动端一次显示一个 App 并通过 Dock 切换；
- 创建/更新 Workspace 不常驻占屏且不回显 API Key；
- TypeScript、单元、依赖检查和 Playwright E2E 通过。
