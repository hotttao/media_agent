# how_css.md - CSS 工具链是怎么协同工作的

> 面向前端初学者。用项目里真实的 `<Button>` 例子，把 **Tailwind / PostCSS / shadcn / Vite** 四层关系讲清楚，并追踪 `vite build` 时 CSS 是怎么被编译出来的。

---

## 总览：四层各干什么

先不要管术语，记住一张图：

```
你写的代码                工具                  产物
──────────────────────────────────────────────────────────
variables.css      ->    （地基，手工写的）   ->  CSS 变量定义 :root { --primary: ... }
tailwind.config.js ->    Tailwind（翻译字典） ->  "bg-primary 这个 class 等于 oklch(var(--primary))"
button.tsx         ->    shadcn（用 class 的组件源码） -> className="bg-primary text-primary-foreground"
index.css          ->    PostCSS（流水线）    ->  把上面三者拼成浏览器能跑的最终 CSS
                   ->    Vite build（总调度） ->  dist/assets/index-xxxx.css
```

四层一句话定位：

| 层 | 是什么 | 类比 |
|---|---|---|
| **CSS 变量** | `:root` 里定义颜色值 | 颜料盒 |
| **Tailwind** | 把变量名翻译成 class 的工厂 | 字典：`bg-primary` = `background-color: oklch(var(--primary))` |
| **shadcn** | 拷进项目的组件源码，用 class 拼出样式 | 现成的菜谱，照着用 |
| **PostCSS** | 流水线，串联 Tailwind + autoprefixer | 厨房流水线 |
| **Vite** | 总调度，dev server + build | 餐厅经理 |

下面用一个 Button 从头追到尾。

---

## 第 1 步：CSS 变量（地基）

文件：`internal/theme/src/variables.css`，被 `src/index.css` 第一行 `@import` 进来。

```css
:root {
  --primary: 0.567 0.237 267.896;          /* 紫色 */
  --primary-foreground: 1 0 180;           /* 白 */
  --background: 1 0 180;
  /* ... 一堆变量 */
}
```

**关键点**：变量存的不是完整颜色，是 `oklch()` 的三个分量（亮度 色度 色相），用空格分隔。为什么要这样存？因为颜色包装函数 `oklch()` 放在 Tailwind 配置里（下一步），这样能动态拼 alpha 透明度。先记住：`--primary` = "0.567 0.237 267.896" 这串数字。

> 暗色主题怎么来？同一个文件里有 `.dark { --primary: ...; }` 重写变量值。切主题只改变量值，class 一行不用动。

---

## 第 2 步：Tailwind 配置（翻译字典）

文件：`tailwind.config.js`。

```js
theme: {
  extend: {
    colors: {
      primary: {
        DEFAULT: 'oklch(var(--primary) / <alpha-value>)',
        foreground: 'oklch(var(--primary-foreground) / <alpha-value>)',
      },
      // background / destructive / muted / border ... 都是这套写法
    },
  },
}
```

这行配置告诉 Tailwind："以后凡是 `bg-primary`，就生成 `background-color: oklch(var(--primary) / <alpha-value>)`"。`<alpha-value>` 是占位符：写 `bg-primary` 替换成 1，写 `bg-primary/90` 替换成 0.9。

### class 命名规则：前缀定属性，后缀定值

Tailwind 的 class 不是随便起的，规则是 `<属性前缀>-<值名>`。**前缀锁定 CSS 属性领域，后缀决定具体值**。跨领域对比看清分工：

| class | 前缀管什么 | 后缀 | 生成的 CSS 属性 |
|---|---|---|---|
| `bg-primary` | `bg-` = 背景 | 颜色名 | `background-color` |
| `w-10` | `w-` = 宽度 | 数字 | `width` |
| `h-10` | `h-` = 高度 | 数字 | `height` |
| `p-4` / `m-4` | 内/外边距 | 数字 | `padding` / `margin` |

`bg-` 永远管背景、`w-` 永远管宽度，互不越界。所以 `bg-primary` 不可能是宽度--宽度归 `w-` 管，且 `primary` 是颜色名（注册在 `colors` 下）不是数字，双重确认是背景色。

### 同一个 bg- 前缀，后缀类型决定子属性 + 配置位置

`bg-` 跟颜色名是 background-color，跟 CSS 关键词是其他子属性。关键区别：颜色需自己配置，关键词 Tailwind 内置：

| class | 后缀类型 | 生成的 CSS | 走哪个 theme 命名空间 | 是否内置 |
|---|---|---|---|---|
| `bg-primary` | 颜色名 | `background-color` | `colors` | 否，需注册 |
| `bg-cover` | 尺寸关键词 | `background-size` | `backgroundSize` | 是（cover/contain/auto） |
| `bg-center` | 位置关键词 | `background-position` | `backgroundPosition` | 是 |
| `bg-fixed` | 附件关键词 | `background-attachment` | `backgroundAttachment` | 是 |

所以 `bg-cover` 不用配置开箱即用，`bg-primary` 必须在 `theme.extend.colors` 注册（颜色是项目特定的，Tailwind 不知道你要什么色）。想加自定义尺寸才扩展：`theme.extend.backgroundSize = { '4xl': '4rem' }` -> `bg-4xl` => `background-size: 4rem`。

> 规律：Tailwind 内置"通用 CSS 关键词"（cover/center/auto），留"项目特定值"（你的主题色）给你在 `theme.extend` 注册。

### 颜色族：一个 colors key 生成一整族 class

配置里 `colors.primary = { DEFAULT, foreground }` 展开成两条 key path：`primary`（取 DEFAULT）、`primary-foreground`（取 foreground）。自动生成一整族颜色 class，配不同前缀管不同色属性：

| 你写的 class | 查到的配置 | 生成的 CSS |
|---|---|---|
| `bg-primary` | `colors.primary.DEFAULT` | `.bg-primary { background-color: oklch(var(--primary) / 1) }` |
| `bg-primary/90` | 同上 + alpha | `background-color: oklch(var(--primary) / 0.9)` |
| `text-primary-foreground` | `colors.primary.foreground` | `color: oklch(var(--primary-foreground) / 1)` |
| `border-border` | `colors.border`（非嵌套，字符串） | `border-color: oklch(var(--border) / 1)` |

`border` 这种非嵌套写法（值是字符串不是对象）不需要 `DEFAULT`。同一个 `primary` 配合 `bg-`/`text-`/`border-`/`ring-` 前缀，自动生成背景色/文字色/边框色/聚焦环色一整族。

> 反直觉点：CSS 里文字色属性就叫 `color`，没有 `text-color`。Tailwind 用 `text-` 当文字前缀。且 `text-` 多功能：跟颜色名是 `color`（`text-red-500`）、跟尺寸是 `font-size`（`text-sm`）、跟对齐词是 `text-align`（`text-center`）。所以"前缀 + 后缀类型"共同决定属性。

> 没有这步配置，`bg-primary` 就是个不存在的 class，浏览器不认识。

---

## 第 3 步：shadcn 组件（用 class 的源码）

文件：`src/components/ui/button.tsx`（shadcn CLI 拷进来的）。

```tsx
const buttonVariants = cva(
  "inline-flex items-center justify-center ...",  // 基础 class
  {
    variants: {
      variant: {
        default: "bg-primary text-primary-foreground hover:bg-primary/90",
        destructive: "bg-destructive text-destructive-foreground ...",
        // outline / secondary / ghost / link ...
      },
      size: { default: "h-10 px-4 py-2", sm: "h-9 ...", ... },
    },
  },
)
```

**这一步在做什么**：组件源码里**全是 Tailwind class 名**，没有一行真正的 CSS。`bg-primary` 这个 class 名，靠第 2 步的 Tailwind 配置才有意义。

`cva` 是个库，作用是"按 variant 拼出 class 字符串"。`<Button variant="default">` 最终渲染出 `className="... bg-primary text-primary-foreground ..."`。

> shadcn 不是运行时依赖，它就是把这段源码拷给你。你完全可以手改 button.tsx，改完就是你的了。

---

## 第 4 步：PostCSS（编译流水线）

文件：`postcss.config.js`。

```js
export default {
  plugins: {
    tailwindcss: {},   // 第 1 个插件：跑 Tailwind
    autoprefixer: {},  // 第 2 个插件：加浏览器前缀
  },
}
```

PostCSS 是 CSS 的"流水线调度员"。它读 `src/index.css`，按顺序让两个插件处理。Tailwind 插件干三件事：

### (a) 扫描源码，提取用到的 class

Tailwind 读 `tailwind.config.js` 里 `content` 配置的所有文件：

```js
content: [
  './index.html',
  './src/**/*.{js,ts,jsx,tsx}',
  '../internal/ui-sdk/src/**/*.{js,ts,jsx,tsx}',
  // ...
]
```

它发现 `button.tsx` 里用了 `bg-primary`、`text-primary-foreground`、`bg-primary/90`、`h-10`、`px-4`……于是只生成这些 class 的 CSS（没用到的根本不生成，这是 Tailwind 体积小的原因，叫 purge）。

### (b) 展开 @tailwind 三指令

`src/index.css` 里有这三行：

```css
@tailwind base;
@tailwind components;
@tailwind utilities;
```

Tailwind 把它们替换成真正的 CSS：

| 指令 | 展开成 |
|---|---|
| `@tailwind base` | Tailwind 的 reset 样式（Preflight，统一各浏览器默认） |
| `@tailwind components` | 组件类（本项目基本没用） |
| `@tailwind utilities` | 扫描出来的所有原子 class：`.bg-primary { ... }` `.h-10 { ... }` 等 |

### (c) 处理 @apply

`src/index.css` 里还有：

```css
@layer base {
  * { @apply border-border; }
  body { @apply bg-background text-foreground; }
}
```

`@apply border-border` 的意思是"把 `.border-border` 这个 class 的样式搬到这里"。Tailwind 把它编译成：

```css
/* @apply 没带 /N 修饰符，<alpha-value> 被填成 1 */
* { border-color: oklch(var(--border) / 1); }
body { background-color: oklch(var(--background) / 1); color: oklch(var(--foreground) / 1); }
```

#### `<alpha-value>` 占位符是怎么填的

配置里颜色值写成 `'oklch(var(--primary) / <alpha-value>)'`，`<alpha-value>` 是个占位符。Tailwind 编译时根据你**有没有写 alpha 修饰符 `/N`** 来填：

| 写法 | 在哪写的 | alpha 修饰符 | `<alpha-value>` 填成 | 最终 CSS |
|---|---|---|---|---|
| `bg-primary` | HTML class | 没写 | `1` | `oklch(var(--primary) / 1)` |
| `bg-primary/90` | HTML class | `/90` | `0.9` | `oklch(var(--primary) / 0.9)` |
| `@apply border-border` | CSS 里 | 没写 | `1` | `oklch(var(--border) / 1)` |
| `@apply border-border/50` | CSS 里 | `/50` | `0.5` | `oklch(var(--border) / 0.5)` |

所以 @apply 跟 HTML class 是同一套机制：没带 `/N` 填 1（不透明），带了填对应小数。本项目 index.css 里 `@apply border-border` 都没带修饰符，所以 alpha 全是 1。

#### `@layer base` 是什么意思

`@layer` 是 **CSS 原生语法**（层叠层 cascade layer），不是 Tailwind 发明的。它给样式分组并控制优先级。

看 `src/index.css` 开头这一行：

```css
@layer plugin-utilities, base, components, utilities;
```

这行**声明层的优先级顺序**：越靠后优先级越高：

```
plugin-utilities（最低）< base < components < utilities（最高）
```

`@layer base { * { @apply border-border } }` 就是"把基础重置放进 base 层"。为什么要分层？因为 **utilities 层优先级更高**，组件用的 `bg-primary`（在 utilities 层）能覆盖 body 的 `bg-background`（在 base 层）。

**具体例子**，看优先级怎么生效：

```html
<body>  <!-- body 在 base 层被设成 bg-background（浅色） -->
  <div class="bg-primary">我是紫色</div>  <!-- bg-primary 在 utilities 层，优先级更高 -->
</body>
```

```css
/* 编译后，分层放 */
@layer base {
  body { background-color: oklch(var(--background) / 1); }      /* 浅色，base 层 */
}
@layer utilities {
  .bg-primary { background-color: oklch(var(--primary) / 1); } /* 紫色，utilities 层 */
}
```

body 整体是浅色背景，但 div 用了 `bg-primary`，因为 utilities 层 > base 层，div 显示紫色，不会被 body 的背景"压住"。

> 不分层会怎样：CSS 默认靠选择器特异性（specificity）和源码顺序定优先级，`body` 和 `.bg-primary` 特异性不同、谁后写谁赢，容易乱。用 `@layer` 显式声明优先级，跟书写顺序无关，更可控。
>
> 项目里特意把 `plugin-utilities` 放最前（最低优先级），是为了让插件注入的 class 永远覆盖不了 host 的 utilities（见 index.css 开头注释），避免插件样式污染主应用。

之后 autoprefixer 给所有需要前缀的属性加 `-webkit-` / `-moz-` 前缀。

---

## 第 5 步：Vite build 时 CSS 怎么被编译

现在把整条链路串起来。跑 `pnpm build`（=`tsc -b && vite build`）时：

```
1. Vite 读 index.html
   └─ <script src="/src/main.tsx">

2. Vite 从 main.tsx 追踪依赖
   └─ main.tsx 里有 import './index.css'（或间接 import）
      -> Vite 把 index.css 标记为 CSS 入口

3. Vite 把 index.css 交给 PostCSS 处理（读 postcss.config.js）
   ├─ tailwindcss 插件：
   │   ① 扫描 content 里的所有 .tsx，收集用到的 class
   │   ② 展开 @tailwind base/components/utilities
   │   ③ 编译 @apply border-border 等
   │   ④ 输出：一大坨真正的 CSS，含
   │      .bg-primary { background-color: oklch(var(--primary) / <alpha-value>) }
   │      .h-10 { height: 2.5rem }
   │      ...
   └─ autoprefixer 插件：
       ⑤ 给需要前缀的属性加 -webkit- / -moz-

4. Vite 收集所有 CSS（含从 JS 里 import 的 .css），
   合并、压缩、抽成单独文件：
   dist/assets/index-a3b2c1d9.css   ← 带 hash 的文件名

5. Vite 生成 dist/index.html，注入：
   <link rel="stylesheet" href="/assets/index-a3b2c1d9.css">

6. 浏览器加载这个 CSS 文件
   └─ :root { --primary: 0.567 0.237 267.896; }   ← 来自 variables.css
   └─ .bg-primary { background-color: oklch(var(--primary) / <alpha-value>) }
       渲染时 var(--primary) 解析成 "0.567 0.237 267.896"
       -> 最终 oklch(0.567 0.237 267.896) -> 紫色
```

> dev 模式（`pnpm dev`）原理一样，但 Vite 不落地文件，而是把 CSS 通过 `<style>` 注入到内存，热更新更快。

---

## 最终生成的 CSS 长什么样（简化版）

为了让你看清 build 后的 CSS 结构，假设整个项目只有 `button.tsx` 用到这些 class，最终 `dist/assets/index-xxxx.css` 大概长这样（已简化，真实文件更大）：

```css
/* ============ 1. @import 的变量（来自 variables.css）============ */
:root {
  --radius: 0.5rem;
  --background: 1 0 180;
  --foreground: 0.126 0.026 268.939;
  --primary: 0.567 0.237 267.896;        /* 紫色 */
  --primary-foreground: 1 0 180;        /* 白 */
  --border: 0.928 0.006 264.531;
}
.dark { --primary: 0.5 0.2 270; }       /* 暗色主题重写变量值 */

/* ============ 2. @tailwind base 展开的 Preflight（reset）============ */
*, ::before, ::after {
  box-sizing: border-box;
  border-style: solid;
  border-color: oklch(var(--border) / 1);   /* <- @apply border-border 编译来，alpha 填 1 */
}
body {
  margin: 0;
  background-color: oklch(var(--background) / 1);  /* <- @apply bg-background */
  color: oklch(var(--foreground) / 1);            /* <- @apply text-foreground */
}

/* ============ 3. @tailwind components（本项目基本空）============ */

/* ============ 4. @tailwind utilities：扫描出的原子 class =========== */
@layer utilities {
  .inline-flex { display: inline-flex; }
  .items-center { align-items: center; }
  .gap-2 { gap: 0.5rem; }
  .rounded-md { border-radius: calc(var(--radius) - 2px); }
  .text-sm { font-size: 0.875rem; line-height: 1.25rem; }
  .h-10 { height: 2.5rem; }
  .px-4 { padding-left: 1rem; padding-right: 1rem; }

  /* 颜色 class：套 oklch(var(--xxx) / 1)，alpha 已填 */
  .bg-primary { background-color: oklch(var(--primary) / 1); }
  .text-primary-foreground { color: oklch(var(--primary-foreground) / 1); }

  /* 带 /90 修饰符的，alpha 填 0.9，class 名里 / 被转义成 \/ */
  .bg-primary\/90 { background-color: oklch(var(--primary) / 0.9); }

  /* hover 变体：前面加 :hover，特殊字符都转义 */
  .hover\:bg-primary\/90:hover { background-color: oklch(var(--primary) / 0.9); }
}

/* ============ 5. 自定义 utilities（index.css 里手写的）============ */
@layer utilities {
  .scrollbar-none { scrollbar-width: none; }
  .field-sizing-content { field-sizing: content; }
}
```

**看懂这个结构，记住 6 个关键点**：

1. **变量在最前**（来自 `@import`）：颜色值是分量（`0.567 0.237 267.896`），不是完整 oklch，包装函数放在 class 里。
2. **`@tailwind` 三指令被替换**：`base` 变成 Preflight reset，`components` 基本空，`utilities` 变成扫描出的原子 class。
3. **`@apply` 被内联**：`@apply border-border` 变成 `*` 选择器里的 `border-color: oklch(var(--border) / 1)`，alpha 填 1。
4. **颜色 class 全套 `oklch(var(--xxx) / 1)`**：`<alpha-value>` 占位符已被填成实际数字。
5. **带修饰符的单独成条**：`bg-primary/90` 生成 `.bg-primary\/90`（`/` 转义成 `\/`），alpha 是 0.9；`hover:bg-primary/90` 生成 `.hover\:bg-primary\/90:hover`。
6. **原子 class 按需生成**：只生成 button.tsx 实际用到的，没用到的 class 不在这文件里（这就是 purge，体积小的原因）。

> 真实项目的 CSS 文件会更大（Preflight 本身几百行，加上所有用到的 class），但结构跟这个简化版一致。浏览器加载这个文件后，`:root` 变量生效，`.bg-primary` 的 `var(--primary)` 解析成 `0.567 0.237 267.896`，最终 `oklch(0.567 0.237 267.896)` 渲染成紫色。

---

## 完整例子追踪：一个紫色按钮怎么变成紫色

你在代码里写：

```tsx
<Button variant="default">保存</Button>
```

它最终在浏览器里是紫色的，全链路如下：

| 步骤 | 发生了什么 | 谁干的 |
|---|---|---|
| 1 | `cva` 按 `variant="default"` 拼出 `className="... bg-primary text-primary-foreground hover:bg-primary/90 ..."` | React 运行时 |
| 2 | Tailwind 扫描到 `button.tsx` 里有 `bg-primary`，生成 `.bg-primary{background-color:oklch(var(--primary)/<alpha-value>)}` | Tailwind 插件（build 时） |
| 3 | PostCSS 把生成的 CSS + autoprefixer 前缀交给 Vite | PostCSS |
| 4 | Vite 落地成 `dist/assets/index-xxxx.css`，HTML 里 `<link>` 引用它 | Vite build |
| 5 | 浏览器加载 CSS，`:root{--primary:0.567 0.237 267.896}` 生效 | variables.css |
| 6 | 渲染时 `.bg-primary` 的 `var(--primary)` 解析成 `0.567 0.237 267.896`，最终 `oklch(0.567 0.237 267.896)` = 紫色 | 浏览器 |

**改色怎么做**：只改 `variables.css` 里 `--primary` 的值，全站所有 `bg-primary` 自动变色，class 一行不用动。这就是 CSS 变量 + Tailwind 配合的威力。

**改组件怎么做**：直接改 `button.tsx`，比如把 `default` variant 的 class 换成 `bg-destructive`。下次 build 时 Tailwind 重新扫描，生成新 class 的 CSS。

---

## 一句话收口

- **CSS 变量**存颜料值（`--primary: 数字`）
- **Tailwind 配置**把变量名注册成 class（`bg-primary` = `oklch(var(--primary))`）
- **shadcn 组件**用这些 class 拼出样式（button.tsx 里全是 class 名）
- **PostCSS** 在 build 时把 class 编译成真正的 CSS（扫源码->展开指令->@apply->加前缀）
- **Vite** 调度整条链，把最终 CSS 落地成带 hash 的文件，在 HTML 里 `<link>` 引用

四层各管一段，名字对齐（`--primary` 变量 / `primary` 色板 key / `bg-primary` class）就能跑起来。
