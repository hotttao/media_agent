# dropdown-menu.md - DropdownMenu 内部：数据流 + 定位机制

> 讲清楚 `<DropdownMenu>` / `<DropdownMenuTrigger>` / `<DropdownMenuContent>` / `<DropdownMenuItem>` 这 4 个组件之间怎么协作：数据怎么流（React Context）、菜单怎么定位到按钮下方（Popper + floating-ui）。配 `web/src/components/workspace/WorkspaceChatPanel.tsx:715` 的实战代码。
>
> 关于 `<DropdownMenuTrigger asChild>` 里的 `asChild` 是什么，看 `radix.md`。

---

## 实战代码

`web/src/components/workspace/WorkspaceChatPanel.tsx:715`：

```tsx
<DropdownMenu>
  <DropdownMenuTrigger asChild>
    <AppHeaderButton icon={MoreHorizontal} />
  </DropdownMenuTrigger>
  <DropdownMenuContent align="end" className="w-44">
    <DropdownMenuItem onSelect={...}>搜索</DropdownMenuItem>
    <DropdownMenuItem onSelect={...}>重命名</DropdownMenuItem>
  </DropdownMenuContent>
</DropdownMenu>
```

这一节回答两个问题：

1. **数据如何在 `DropdownMenu` / `Trigger` / `Content` / `Item` 这些组件之间传递？**（用 React Context）
2. **菜单是怎么定位到按钮下方的？**（用 Popper + floating-ui）

---

## 数据流：用 React Context 跨组件传递

5 个组件之间的通信不是靠层层传 props，而是用一个叫 `DropdownMenuContext` 的 React Context 当"广播站"。看 `web/node_modules/@radix-ui/react-dropdown-menu/dist/index.mjs:21-53`：

```js
var DropdownMenu = (props) => {
  const { __scopeDropdownMenu, children, dir, open: openProp,
          defaultOpen, onOpenChange, modal = true } = props;
  const triggerRef = React.useRef(null);                    // ← ① 在 Root 创建 ref
  const [open, setOpen] = useControllableState({...});      // ← ② 在 Root 创建 open 状态

  return /* @__PURE__ */ jsx(DropdownMenuProvider, {
    scope: __scopeDropdownMenu,
    triggerId: useId(),                                     // ← ③ 生成 id
    triggerRef,                                             // ← 把 ref 放进 Context
    contentId: useId(),                                     // ← ④ 生成 id
    open,                                                   // ← 把 open 放进 Context
    onOpenChange: setOpen,                                  // ← 把 setter 放进 Context
    onOpenToggle: React.useCallback(
      () => setOpen((prevOpen) => !prevOpen), [setOpen]    // ← 把 toggle 函数放进 Context
    ),
    modal,
    children: /* @__PURE__ */ jsx(MenuPrimitive.Root, { ... }) // ← 还套了一层 Menu Context
  });
};
```

**核心思路**：所有共享状态都在 `DropdownMenu` 这一层创建，放进 Context Provider 的 `value`。下面的子组件用 `useDropdownMenuContext()` 这个 hook 读出来用。

### 5 个变量谁定义、谁写、谁读

| 变量 | 在哪定义 | 谁写 | 谁读 |
|---|---|---|---|
| `open` | `DropdownMenu` 里 `useState` | Trigger 调 `context.onOpenToggle()` 写 | Trigger 读（决定 data-state）、Content 读（决定渲染不渲染） |
| `triggerRef` | `DropdownMenu` 里 `useRef(null)` | Trigger 内部 `composeRefs(...)` 在 DOM 挂载时写 | Content 内部 Popper 读位置用 |
| `contentId` | `DropdownMenu` 里 `useId()` | （只读，不写） | Trigger 写 `aria-controls`、Content 写 `id` |
| `triggerId` | `DropdownMenu` 里 `useId()` | （只读，不写） | Trigger 写 `id`、Content 写 `aria-labelledby` |
| `onOpenChange` | `DropdownMenu` 透传自外部 prop | Menu 内部点击 Item 时调 | 外部业务消费 |

### Trigger 怎么"读 + 写" Context

看 `dropdown-menu.mjs:56-89`：

```js
var DropdownMenuTrigger = React.forwardRef((props, forwardedRef) => {
  const context = useDropdownMenuContext(TRIGGER_NAME, __scopeDropdownMenu);
  //        ↑↑↑↑↑  从 Context 把共享状态全读出来

  return jsx(Primitive.button, {
    id: context.triggerId,                                  // ← 读：triggerId
    "aria-haspopup": "menu",
    "aria-expanded": context.open,                          // ← 读：open
    "aria-controls": context.open ? context.contentId : void 0,  // ← 读：contentId
    "data-state": context.open ? "open" : "closed",         // ← 读：open

    ref: composeRefs(forwardedRef, context.triggerRef),     // ← 写：DOM 挂载时
                                                            //   composeRefs 会被 React 调用
                                                            //   把 <button> 节点塞进 context.triggerRef.current

    onPointerDown: composeEventHandlers(props.onPointerDown, (event) => {
      if (!disabled && event.button === 0 && event.ctrlKey === false) {
        context.onOpenToggle();                             // ← 写：调 setOpen 切换 open
      }
    }),
    onKeyDown: composeEventHandlers(props.onKeyDown, (event) => {
      if (["Enter", " "].includes(event.key)) context.onOpenToggle();  // ← 写：键盘也切
    }),
  });
});
```

读写都在这一个组件里发生，全部通过 `context` 这一个对象。**没有 props 透传地狱**。

### Content 怎么"读" Context

```js
var DropdownMenuContent = React.forwardRef((props, forwardedRef) => {
  const context = useDropdownMenuContext(CONTENT_NAME, __scopeDropdownMenu);
  // 读 context.contentId、context.triggerId 等

  return jsx(MenuPrimitive.Content, {
    id: context.contentId,                                  // ← 写到 DOM 上
    "aria-labelledby": context.triggerId,                  // ← 关联 trigger
    // ... 透传给 MenuPrimitive.Content
  });
});
```

### 完整数据流时序（从点击到菜单出现）

```
T0  用户点击 <button>
T1  React 触发 onPointerDown
T2  DropdownMenuTrigger 内部：
       读 context.onOpenToggle
       调用 -> setOpen(prev => !prev)
T3  DropdownMenu 内部 useState 更新：open = true
T4  React 重渲染 DropdownMenuProvider，value.open = true
T5  整棵子树重渲染：
       DropdownMenuTrigger 重渲染 -> data-state="open", aria-expanded="true"
       DropdownMenuContent 重渲染 -> MenuPrimitive.Root 看到 open=true，
         开始 Portal 渲染 Content 到 fullscreenContainer
T6  MenuPrimitive.Content 挂载到 DOM
T7  内部 Popper 的 useLayoutEffect 触发（见下面定位机制）
T8  Popper 算好位置，写到 Content 的 style 上
T9  CSS 动画播放（data-[state=open]:animate-in）
T10 用户点击 <DropdownMenuItem>：
       MenuPrimitive.Item 触发 onSelect 回调
       Menu 内部调 context.onOpenChange(false)
       -> DropdownMenu 的 setOpen(false)
       -> open = false -> React 重渲染 -> Content 卸载
       -> onCloseAutoFocus：把焦点还给 triggerRef.current
```

---

## 定位机制：Popper + floating-ui

`DropdownMenuContent` 自己**不算位置**。它把 `sideOffset`/`align` 等参数透传给 `MenuPrimitive.Content`，后者再交给 `@radix-ui/react-popper`，Popper 又用 `@floating-ui/react-dom` 的 `useFloating` hook 真正算位置。

### 组件树（4 层）

```
<DropdownMenuContent sideOffset={4} align="end">           ← 你写的
  ↓ 透传
<MenuPrimitive.Content>                                    ← @radix-ui/react-menu
  ↓ 内部渲染
<Popper>                                                   ← @radix-ui/react-popper
  <PopperAnchor>    ← 把 trigger ref 注册为 anchor
  <PopperContent>    ← 真正算位置的组件
  <PopperArrow/>
</Popper>
  ↓ 内部用
useFloating({ strategy: 'fixed', placement: 'bottom-end', ... })  ← @floating-ui/react-dom
```

### Popper 怎么拿到 anchor（trigger）的 DOM

`web/node_modules/@radix-ui/react-popper/dist/index.mjs:29-52`：

```js
var Popper = (props) => {
  const [anchor, setAnchor] = React.useState(null);       // ← anchor 状态
  return jsx(PopperProvider, {
    anchor, onAnchorChange: setAnchor,                    // ← 通过 Context 暴露 setAnchor
    children
  });
};

var PopperAnchor = React.forwardRef((props, forwardedRef) => {
  const context = usePopperContext(ANCHOR_NAME, __scopePopper);
  const ref = React.useRef(null);
  React.useEffect(() => {
    if (ref.current !== previousAnchor) {
      context.onAnchorChange(ref.current);                // ← 把自己的 DOM 写进 anchor
    }
  });
  return jsx(Primitive.div, { ...anchorProps, ref: composedRefs });
});
```

**关键链路**：`MenuPrimitive.Anchor` 包了 `PopperAnchor`，而 `MenuPrimitive.Anchor` 又被 `DropdownMenuTrigger` 包了（看 `dropdown-menu.mjs:61` 那行 `<MenuPrimitive.Anchor asChild>`）。所以最终 `Popper` 拿到的 anchor 就是你的 `<button>` DOM 节点。

> 这就解释了 `triggerRef` 为什么必须用 `composeRefs` 合并：Radix 的 Context 里有个 `triggerRef`，Popper 内部又有自己的 `anchor` 状态，两者都要指向同一个 `<button>`。composeRefs 把 DOM 节点同时灌进去。

### PopperContent 算位置（核心）

`react-popper/dist/index.mjs:91-128` 是关键：

```js
const {
  refs,
  floatingStyles,        // ← 算好的 CSS，直接用
  placement,             // ← 最终放哪边（可能因为 flip 翻面）
  isPositioned,          // ← 第一次定位完成了吗
  middlewareData         // ← 中间件产物（arrow 位置、transform-origin 等）
} = useFloating({
  strategy: "fixed",                              // ← 用 position: fixed
  placement: "bottom-end",                        // ← 期望放右下，"end" 来自 align="end"
  whileElementsMounted: autoUpdate,               // ← 滚动/resize 时自动重算
  elements: {
    reference: context.anchor                     // ← anchor 就是 <button> DOM
  },
  middleware: [
    offset({ mainAxis: sideOffset + arrowHeight }),  // ← ① 间距 sideOffset=4
    shift({ ... }),                                   // ← ② 横向溢出时推回来
    flip({ ... }),                                    // ← ③ 下面放不下翻到上面
    size({                                            // ← ④ 暴露 CSS 变量给业务
      apply: ({ elements, rects, availableWidth, availableHeight }) => {
        elements.floating.style.setProperty(
          "--radix-popper-available-width", `${availableWidth}px`
        );
        // ...还有 anchor-width / anchor-height / available-height
      }
    }),
    arrow && floatingUIarrow({ element: arrow }),     // ← ⑤ 箭头位置
    transformOrigin({ ... }),                         // ← ⑥ 动画起点
    hideWhenDetached && hide({ ... })                 // ← ⑦ trigger 被滚走了就隐藏
  ]
});
```

floating-ui 的 `useFloating` 是个 hook，内部用 `useLayoutEffect` 在 DOM 挂载后立即测量并计算。返回的 `floatingStyles` 直接套到 Content 的外层 wrapper：

```js
return jsx("div", {
  ref: refs.setFloating,
  "data-radix-popper-content-wrapper": "",
  style: {
    ...floatingStyles,                              // ← position:fixed; top:..; left:..;
    transform: isPositioned ? floatingStyles.transform : "translate(0, -200%)",
    // 第一次定位前先扔到屏幕外，避免动画从 0,0 飞过来
    minWidth: "max-content",
    zIndex: contentZIndex,
    "--radix-popper-transform-origin": [            // ← 给动画用
      middlewareData.transformOrigin?.x,
      middlewareData.transformOrigin?.y
    ].join(" "),
  },
  children: jsx(Primitive.div, {
    "data-side": placedSide,                        // ← top/bottom/left/right
    "data-align": placedAlign,                      // ← start/center/end
    ...contentProps,
  })
});
```

### 定位算法步骤（简化版）

floating-ui 内部大致流程（真实算法更复杂，包含 clipping 之类的边界检测）：

```
1. 测量 anchor（<button>）的屏幕位置
   const anchorRect = anchor.getBoundingClientRect()
   // = { top: 100, left: 200, bottom: 128, right: 228, width: 28, height: 28 }

2. 测量 Content 期望尺寸
   const contentRect = contentEl.getBoundingClientRect()

3. 算期望位置（placement = "bottom"，align = "end"）
   期望 top  = anchorRect.bottom + sideOffset(4) = 132
   期望 left = anchorRect.right - contentRect.width   // align=end 右对齐

4. 跑 middleware 链：
   ① offset:  期望 top += sideOffset
   ② shift:   如果 contentRect 超出 viewport 右边界，向左推
   ③ flip:    如果期望 top + contentRect.height > viewport.height
              把 placement 翻到 "top"
              期望 top = anchorRect.top - sideOffset - contentRect.height
   ④ size:    写 CSS 变量
   ⑤ arrow:   算箭头位置
   ⑥ transformOrigin: 算动画起点

5. 把结果写到 floatingStyles:
   { position: 'fixed', top: '132px', left: '180px', transform: '' }

6. 应用到 DOM
   Content 的外层 wrapper style 就变成上面那个

7. 设置 isPositioned = true
   transform 从 'translate(0, -200%)' 变成 ''（实际位置）
   触发 CSS 动画（slide-in-from-top-2 等）

8. autoUpdate 监听 window.resize / scroll / ResizeObserver(content) / ResizeObserver(anchor)
   任何变动都重跑 1-7
```

### 暴露给业务的 CSS 变量

`dropdown-menu.mjs:124-133` 里这堆看不懂的代码就是在重新命名 Popper 暴露的 CSS 变量：

```js
style: {
  ...props.style,
  ...{
    "--radix-dropdown-menu-content-transform-origin": "var(--radix-popper-transform-origin)",
    "--radix-dropdown-menu-content-available-width":  "var(--radix-popper-available-width)",
    "--radix-dropdown-menu-content-available-height": "var(--radix-popper-available-height)",
    "--radix-dropdown-menu-trigger-width":  "var(--radix-popper-anchor-width)",
    "--radix-dropdown-menu-trigger-height": "var(--radix-popper-anchor-height)",
  }
}
```

业务能在自己的 Content 上写 CSS：

```css
.my-menu-content {
  width: var(--radix-dropdown-menu-trigger-width);    /* 跟 trigger 等宽 */
  transform-origin: var(--radix-dropdown-menu-content-transform-origin);  /* 动画从 trigger 边缘展开 */
}
```

`shadcn` 封装的 `DropdownMenuContent`（`web/src/components/ui/dropdown-menu.tsx:59`）就用 `sideOffset={4}` 让菜单离按钮 4px。

### 为什么用 Portal

Content 用 `<DropdownMenuPortal>` 渲染到 `fullscreenContainer`（看 shadcn 的 `dropdown-menu.tsx:60` 的 `useFullscreenContainer`），不是渲染在 `<button>` 旁边。原因：

- **避免被父级 overflow: hidden / transform / filter 裁剪**：父级有 `overflow: hidden` 的话，菜单会被切掉
- **避免被父级 stacking context 困住**：z-index 失效，菜单被其他元素盖住
- **定位简单**：fixed 相对 viewport，不用算祖先的 transform offset

Portal 把 Content 挪到 body 末尾或 fullscreen container 里，父级再有什么怪样式都不影响菜单定位。

---

## 数据流 + 定位 的完整闭环图

```
┌─ DropdownMenu (Root) ─────────────────────────────────────────────┐
│  useState(open)          useRef(triggerRef)        useId()*2     │
│      │                       │                       │            │
│      └──-> Provider value ────┴───────────────────────┘            │
│            │     │              │              │                   │
└────────────┼─────┼──────────────┼──────────────┼─────────────────┘
             │     │              │              │
   ┌─────────▼─────▼──────┐       │              │
   │ DropdownMenuTrigger  │       │              │
   │  读: open, ids       │       │              │
   │  写: onOpenToggle    │       │              │
   │       (setOpen)      │       │              │
   │  写: composeRefs     │       │              │
   │       -> triggerRef  │───────┘              │
   │       .current=<btn> │                      │
   └─────────┬───────────┘                      │
             │ asChild + Slot                   │
             ▼                                  │
        <button> DOM                            │
             │                                  │
             │   用户点击                        │
             ▼                                  ▼
   ┌─────────────────────────────────────────────────────────┐
   │ DropdownMenuContent (在 Portal 里，open=true 才渲染)    │
   │  读: contentId, triggerId                                │
   │  透传: sideOffset, align                                │
   └─────────┬───────────────────────────────────────────────┘
             │
             ▼
   ┌─ MenuPrimitive.Content ───────────────────────────────┐
   │  内部用 <Popper>                                       │
   │    <PopperAnchor ref={triggerRef}>  ← 拿到 <button>   │
   │    <PopperContent sideOffset={4}>                     │
   │      ↓                                                │
   │      useFloating({                                    │
   │        reference: <button>,                           │
   │        placement: 'bottom-end',                       │
   │        middleware: [offset, shift, flip, size, ...]   │
   │      })                                               │
   │      ↓                                                │
   │      floatingStyles = { position:'fixed',             │
   │                         top: '132px', left: '180px' } │
   │      ↓                                                │
   │      <div style={floatingStyles}>                     │
   │        <div data-side="bottom" data-align="end">      │
   │          <DropdownMenuItem>搜索</DropdownMenuItem>   │
   │          <DropdownMenuItem>重命名</DropdownMenuItem>  │
   │        </div>                                         │
   │      </div>                                           │
   │    </PopperContent>                                   │
   │  </Popper>                                            │
   └───────────────────────────────────────────────────────┘
```

---

## 一句话总结

> 数据流：`DropdownMenu` 用 React Context 当广播站，Trigger 写 `open` 状态和 `triggerRef.current`，Content 读这些做渲染和定位。
>
> 定位：Content 透传 `sideOffset`/`align` 给底层 `Popper`，Popper 用 floating-ui 的 `useFloating` 测量 anchor（你的 `<button>`）+ viewport，跑 offset/shift/flip/size 四个中间件，算出 `position:fixed; top; left` 套到 wrapper 上。
