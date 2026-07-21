# radix.md - Radix UI asChild 是什么

> 用最短篇幅讲清楚 `<DropdownMenuTrigger asChild><AppHeaderButton/></DropdownMenuTrigger>` 为什么能让一个普通按钮长出"开/关菜单"的能力。
>
> 关于 DropdownMenu 内部的数据流（Context）和定位机制（Popper + floating-ui），看 `dropdown-menu.md`。

---

## 一句话讲清 asChild

**`asChild` 的意思：把 Radix 组件的 props 全部"灌"到你传入的子组件上，DOM 上只有一个节点，不包一层 div。**

打个比方：Radix 的 `DropdownMenuTrigger` 本来是个"开关按钮"，会渲染自己的 `<button>`。但你想用自己的 `AppHeaderButton`（带项目样式），又想要 Radix 的"点一下打开菜单 / 键盘 Enter 也能开 / 菜单打开时按钮变蓝"这些行为。

`asChild` 就是让 Radix 把它准备好的所有 props（点击事件、`data-state`、`aria-*` 等）**合并到你的 `AppHeaderButton` 上**，最后渲染出来还是你那个 `<button>`，但行为已经是 Radix 的了。

---

## 为什么需要它

不用 `asChild` 的两种替代方案都有问题：

| 方案 | 问题 |
|---|---|
| Radix 自己渲染 `<button>` | 样式得重写，跟项目的 `AppHeaderButton` 风格对不上 |
| 用 `<div onClick>` 包一层 | 多了一层 DOM，破坏 flex 布局；a11y 属性也得自己写 |

`asChild` 让你"既要又要"：DOM 是你的，行为是 Radix 的。

---

## 它怎么工作的（3 步）

```
步骤 1：DropdownMenuTrigger 准备好一堆 props
        { onPointerDown, data-state, aria-expanded, onKeyDown, ref, ... }

步骤 2：asChild=true 时，把这些 props 交给一个叫 Slot 的组件
        Slot 的唯一职责：把父级 props 合并到唯一子元素上

步骤 3：Slot 用 React.cloneElement 克隆你的 <AppHeaderButton/>
        合并规则：
          - 事件 handler：两个都调用（你的先，Radix 的后）
          - className：两个拼接（"你的 Radix的"）
          - 其他 props（data-*、aria-*、disabled）：Radix 的会被你的覆盖
        最后只产生一个 <button> 节点，DropdownMenuTrigger 在 DOM 树里"消失"了
```

关键认知：**`asChild` 不是渲染两个组件然后隐藏一个**，它真的只产生一个 DOM 节点。Radix 组件名在最终 React 树里"消失"了，它的所有 props 都被吸进了你的子组件。

---

## 关键概念速通：ref / data-state / onPointerDown

下面 demo 里你会看到这几个词，先讲清楚它们是什么。

### ref 是什么

React 里"拿到 DOM 节点"的方式。两种写法：

```tsx
// 写法 1：函数 ref - 挂载时 React 调用一次，参数就是 <button> DOM 节点
<button ref={(node) => { console.log(node) }} />

// 写法 2：对象 ref - 挂载后 ref.current 就是 DOM 节点
const r = useRef<HTMLButtonElement>(null)
<button ref={r} />
// 之后：r.current 就是那个 <button>
```

### 为什么 ref 要"合并"

`DropdownMenuTrigger` 内部要拿 DOM（测位置好让菜单对齐按钮顶下），同时业务也想拿 DOM（比如 focus 它）。一个 DOM 节点要喂给两个 ref：

```tsx
// Radix 的 ref           业务的 ref
ref={radixTriggerRef}     ref={businessRef}
// 怎么让一个 <button> 同时给两个 ref？包一层函数：
const mergedRef = (node) => {
  radixTriggerRef.current = node   // Radix 拿到
  businessRef.current = node       // 业务也拿到
}
<button ref={mergedRef} />
```

这就是 `composeRefs` 干的事，把多个 ref 串成一个函数。

### data-state 是什么

**HTML 的自定义属性**，前缀 `data-`，浏览器不认识它，**专门给 CSS 和 JS 用**。可以写任何值：

```html
<button data-state="closed">更多</button>
<!-- 菜单打开后变成 -->
<button data-state="open">更多</button>
```

CSS 用"属性选择器"响应它：

```css
[data-state="open"] { background: blue; }   /* 菜单开时按钮变蓝 */
```

Tailwind 写法：`data-[state=open]:bg-foreground/[0.06]`。这就是 Radix 跟样式系统的协作方式：Radix 管"什么时候改 `data-state`"，CSS 管"`data-state` 是什么颜色"。看 `AppHeaderButton.tsx:43` 末尾的 `data-[state=open]:bg-foreground/[0.06] data-[state=open]:text-foreground`，就是消费 Radix 注入的 `data-state`。

### onPointerDown 是什么

**React 的"指针按下"事件**，鼠标按下或手指触屏按下都会触发。比 `onClick` 更早，且能拿到更多细节：

```tsx
<button onPointerDown={(event) => {
  console.log(event.button)    // 0=左键, 2=右键
  console.log(event.ctrlKey)  // 是否按住 Ctrl
}} />
```

Radix 用 `onPointerDown` 而不是 `onClick` 是因为：

- `onClick` 在鼠标抬起时才触发，太慢（用户按下就想看到菜单弹出）
- 要区分左键/右键（右键不该开菜单）
- 键盘的 Enter/Space 走 `onKeyDown`，是另一个事件

---

## 极简 demo（带详细注释）

从零实现一个最小版 `asChild`，重点看 `merged.ref` 怎么工作：

```tsx
import * as React from 'react'

function Slot({ children, ...slotProps }: any) {
  if (!React.isValidElement(children)) throw new Error('asChild 要唯一子元素')

  // 第一步：合并 props（先拷贝一份，再按规则覆盖）
  const merged: any = { ...slotProps, ...children.props }

  // 1.1 className：两个拼接，不要互相覆盖
  if (slotProps.className && children.props.className) {
    merged.className = `${slotProps.className} ${children.props.className}`
  }

  // 1.2 事件 handler：两个都跑！child 先 slot 后
  for (const k in children.props) {
    if (/^on[A-Z]/.test(k) && slotProps[k] && children.props[k]) {
      const cv = children.props[k]   // child 的 handler
      const sv = slotProps[k]         // slot 的 handler
      merged[k] = (...args: any[]) => { cv(...args); sv(...args) }
    }
  }

  // 1.3 ref 合并：把两个 ref 包成一个函数
  //     slotProps.ref       = Radix 内部的 ref（拿 DOM 测位置）
  //     children.ref        = 业务传给 AppHeaderButton 的 ref（如果传了）
  //     合并后：<button> 挂载时，把 DOM 节点喂给这两个 ref
  merged.ref = (node: any) => {
    // 如果 slotProps.ref 是函数，调用它
    if (typeof slotProps.ref === 'function') slotProps.ref(node)
    // 如果是对象（useRef 创建的），赋给 current
    else if (slotProps.ref) slotProps.ref.current = node
    // 同样处理 child 的 ref
    const childRef = (children as any).ref
    if (typeof childRef === 'function') childRef(node)
    else if (childRef) childRef.current = node
  }

  // 第二步：cloneElement 把合并后的 props 灌到子元素上
  //         关键：cloneElement 不是重新渲染组件，是把已有元素复制一份并替换 props
  //         所以 <AppHeaderButton> 内部 forwardRef 的 ref 会被这里赋值
  return React.cloneElement(children, merged)
}

// asChild 入口：开关
const Button = React.forwardRef<any, any>(({ asChild, ...props }, ref) => {
  const Comp = asChild ? Slot : 'button'
  return <Comp {...props} ref={ref} />
})
```

> 真实的 Radix `Slot` 还多处理了 `Slottable`（嵌套场景）、lazy 组件、ref cleanup，但核心就是这么简单。

### merged.ref 执行流程

以这个例子追踪：

```tsx
const businessRef = useRef(null)

<DropdownMenuTrigger asChild>
  <AppHeaderButton ref={businessRef} icon={MoreHorizontal} />
</DropdownMenuTrigger>
```

```
T0  React 准备渲染 DropdownMenuTrigger
T1  DropdownMenuTrigger 内部：
       ref = composeRefs(forwardedRef /* undefined */, context.triggerRef)
       然后把 ref 放到 props 里交给 Primitive.button
T2  Primitive.button 看到 asChild=true，调用 Slot
T3  Slot 收到：
       children = <AppHeaderButton ref={businessRef} icon={...} />
       slotProps.ref = T1 那个 composeRefs 出来的函数
T4  Slot 跑 merged.ref = (node) => { slotProps.ref(node); childRef(node) }
T5  React 渲染，<button> DOM 节点挂载
T6  React 调用 merged.ref(<button DOM 节点>)：
       -> slotProps.ref(node)
            -> context.triggerRef.current = node   ✅ Radix 拿到了
       -> childRef(node)                          ✅ 业务的 businessRef 也拿到了
            -> businessRef.current = node
```

最后：`context.triggerRef.current` 和 `businessRef.current` 指向**同一个 `<button>` DOM 节点**。

---

## 一句话总结

> `asChild` = "把 Radix 的 props 灌到你的组件上，DOM 上只留一个节点"。
>
> 实现关键：`Slot` 用 `React.cloneElement` 把父级 props 合并到唯一子元素上，事件 handler 两个都跑、className 拼接、ref 用 `composeRefs` 合并成一个函数喂给两个 ref。
