# build.md - 前端构建与工具链配置

> 介绍 `web/` 根目录下每个配置文件：谁消费它、何时触发、关键配置项、如何初始化生成。面向不熟悉前端工具链的同学。

---

## 配置文件全景

| 文件 | 归属工具 | 一句话作用 |
|---|---|---|
| `package.json` | npm/pnpm | 依赖清单 + npm 脚本入口，所有工具的调度中心 |
| `tsconfig.json` | TypeScript (`tsc`) | TS 编译选项 + 路径别名 + 类型检查规则 |
| `vite.config.ts` | Vite | dev server + 生产打包，插件/代理/别名 |
| `vitest.config.ts` | Vitest | 单元测试运行配置 |
| `biome.json` | Biome | 代码检查（lint）+ 格式化（format）统一规则 |
| `tailwind.config.js` | TailwindCSS | 原子化 CSS 的内容扫描 + 主题扩展 + 插件 |
| `postcss.config.js` | PostCSS | CSS 后处理管线，串联 tailwind + autoprefixer |
| `components.json` | shadcn/ui CLI | 组件代码生成器的路径与风格约定 |
| `index.html` | Vite | 应用入口 HTML 模板，挂载点 + 脚本入口 |

### 触发关系链

```
npm run dev   -> predev(脚本) -> vite -> 读 vite.config.ts
                                       -> 读 postcss.config.js -> 读 tailwind.config.js
                                       -> 读 tsconfig.json (类型)
                                       -> 以 index.html 为入口
npm run build -> prebuild -> tsc -b (读 tsconfig.json) -> vite build (同上)
npm run test  -> vitest run -> 读 vitest.config.ts
npm run lint  -> biome check . -> 读 biome.json
npx shadcn add -> 读 components.json -> 按别名生成组件文件
```

---

## 1. package.json

**谁使用**：npm/pnpm 包管理器；所有 npm 脚本调用的工具（vite/biome/vitest/tsc）也从此文件读取依赖版本。

**触发时机**：`npm install`（装依赖）、`npm run <script>`（跑脚本）、任何工具启动时检查依赖树。

**主要配置项**：

| 配置项 | 作用 |
|---|---|
| `name` / `version` / `private` | 包名 `@neutree-ai/web`，私有包不发布 npm |
| `type: "module"` | 整个项目用 ESM（`import/export`），影响所有 `.js` 配置文件的解析方式 |
| `scripts` | 命令入口表，见下表 |
| `dependencies` | 运行时依赖（React/Radix/zustand/i18next 等会打进 bundle） |
| `devDependencies` | 开发期依赖（vite/biome/tsc/tailwind 等只在开发时用） |

**scripts 关键项**：

| 脚本 | 命令 | 说明 |
|---|---|---|
| `predev` / `prebuild` | `node scripts/copy-excalidraw-assets.mjs && node scripts/prebundle-excalidraw.mjs` | 生命周期钩子，在 dev/build 前自动跑，预拷贝 Excalidraw 资源 |
| `dev` | `vite` | 启动 dev server |
| `build` | `tsc -b && vite build` | 先 TS 类型检查（不产出），再 Vite 打包 |
| `lint` / `lint:fix` | `biome check .` / `biome check --write .` | 检查 / 自动修复 |
| `format` | `biome format --write .` | 格式化 |
| `test` / `test:watch` | `vitest run` / `vitest` | 跑一次 / watch 模式 |
| `knip` | `knip` | 检测未使用的导出/依赖 |
| `i18n:*` | `node tools/i18n-tracker.cjs ...` | i18n key 扫描/更新/校验 |

**如何初始化生成**：

```bash
# 全新项目
npm init -y                      # 生成基础 package.json
pnpm add react react-dom ...     # 加运行时依赖
pnpm add -D vite typescript ...  # 加开发依赖

# 本项目是 monorepo 子包，name 用了 scope (@neutree-ai/web)
# 依赖随后续开发逐步累积，无需手写
```

---

## 2. tsconfig.json

**谁使用**：TypeScript 编译器 `tsc`；IDE（VS Code）做实时类型检查也读它。

**触发时机**：`tsc -b`（build 脚本调用）、IDE 打开 `.ts/.tsx` 时实时检查、`vite build` 前的类型校验步骤。

**主要配置项**：

| 配置项 | 值 | 作用 |
|---|---|---|
| `target` | `ES2020` | 编译目标 JS 版本 |
| `lib` | `["ES2020","DOM","DOM.Iterable"]` | 可用的类型库（DOM 提供 `document`/`window` 等类型） |
| `module` / `moduleResolution` | `ESNext` / `bundler` | ESM 输出 + bundler 解析策略（适配 Vite） |
| `jsx` | `react-jsx` | JSX 转换方式（React 18 新版无需 `import React`） |
| `strict` | `true` | 开启所有严格类型检查 |
| `noUnusedLocals` / `noUnusedParameters` | `true` | 未使用的局部变量/参数报错 |
| `noFallthroughCasesInSwitch` | `true` | switch 防穿透 |
| `noUncheckedSideEffectImports` | `true` | 副作用导入必须显式 |
| `isolatedModules` / `moduleDetection: force` | - | 单文件隔离编译，强制 ESM 语义（Vite 要求） |
| `allowImportingTsExtensions` | `true` | 允许 `import './foo.ts'` 带扩展名 |
| `noEmit` | `true` | 只做类型检查不产出 JS（产物交给 Vite） |
| `skipLibCheck` | `true` | 跳过 `.d.ts` 内部检查，提速 |
| `paths` | 见下 | 路径别名，把 `@/*` 映射到 `./src/*`，monorepo 内部包指向源码 |
| `include` / `exclude` | `["src"]` / test 文件 | 只检查 src，测试文件交给 vitest 配置 |

**paths 关键别名**（与 `vite.config.ts` 的 `resolve.alias` 必须对齐）：

| 别名 | 指向 |
|---|---|
| `@/*` | `./src/*` |
| `@neutree-ai/types` | `../internal/types` |
| `@neutree-ai/sse-consumer` | `../internal/sse-consumer/src` |
| `@neutree-ai/theme` | `../internal/theme/index.d.ts` |
| `@neutree-ai/ui-sdk` | `../internal/ui-sdk/src/index.ts` |

**如何初始化生成**：

```bash
npx tsc --init            # 生成默认 tsconfig.json
# React + Vite 推荐用官方模板，已配好 jsx/target/moduleResolution：
npm create vite@latest -- --template react-ts
# 之后按需调 strict 选项、加 paths 别名
```

---

## 3. vite.config.ts

**谁使用**：Vite（dev server + 生产打包器）。

**触发时机**：`vite`（dev）、`vite build`（build）、`vite preview`（预览产物）。

**主要配置项**：

| 配置项 | 作用 |
|---|---|
| `worker.format: 'es'` | Web Worker 以 ES 模块输出（`@extend-ai/react-xlsx` 的 worker 需要 code-splitting，默认 iife 会被 Rollup 拒绝） |
| `plugins` | `mdx`（MDX 编译，pre 阶段）、`react`（Fast Refresh）、`devPluginsMiddleware`（dev-only 插件加载中间件） |
| `resolve.alias` | 运行时路径别名，与 tsconfig 的 `paths` 对齐：`@` -> `./src`，`@neutree-ai/*` -> `../internal/*` 源码 |
| `resolve.dedupe` | 强制 react/react-dom/sonner 等单实例，避免 ui-sdk 自带副本导致 Radix context 失效（典型坑：Tooltip must be used within TooltipProvider） |
| `server.host: '0.0.0.0'` | 监听所有网卡，容器/局域网可访问 |
| `server.port: 15173` | dev 端口 |
| `server.proxy` | 把 `/api` `/_cp` `/_cg` `/_proxy` `/_saas` 代理到 `VITE_BACKEND_URL`（默认 `http://localhost:3000`），支持 ws |

**devPluginsMiddleware**（自定义插件）：见 `plugins.md` 第 7 节，dev-only 从 `TOS_DEV_PLUGINS_DIR` 扫盘生成 `/dev-plugins/manifest.json`。

**如何初始化生成**：

```bash
npm create vite@latest -- --template react-ts   # 模板自带 vite.config.ts
# 然后手动加：mdx 插件、alias、dedupe、proxy、自定义 middleware
```

---

## 4. vitest.config.ts

**谁使用**：Vitest 测试框架。

**触发时机**：`vitest run`（test 脚本）、`vitest`（test:watch）。

**主要配置项**：

| 配置项 | 作用 |
|---|---|
| `resolve.alias` | 测试环境路径别名，与 vite/tsconfig 对齐（但只列了 `@`、`sse-consumer`、`ui-sdk` 三个测试实际需要的） |
| `test.include` | 测试文件 glob：`src/**/*.test.ts` + `../internal/sse-consumer/src/**/*.test.ts`（跨包测试 sse-consumer） |

> Vitest 默认会复用 `vite.config.ts`，这里单独建一份是因为测试只需要子集别名，且要包含跨包的 sse-consumer 测试文件。

**如何初始化生成**：

```bash
pnpm add -D vitest
# 方式一：在 vite.config.ts 里加 test 字段（共用配置）
# 方式二：单独建 vitest.config.ts（本项目做法，配置隔离更清晰）
# 官方向导：
npm create vitest@latest
```

---

## 5. biome.json

**谁使用**：Biome（统一的 linter + formatter，替代 ESLint + Prettier）。

**触发时机**：`npm run lint` / `lint:fix` / `format`，以及 IDE 的 Biome 插件实时检查。

**主要配置项**：

| 配置项 | 作用 |
|---|---|
| `extends` | `["../biome.base.json"]` - monorepo 共享的根级基础规则，各子包继承后局部覆盖 |
| `files.ignore` | 忽略 `node_modules`/`dist`/`public/excalidraw-assets`/`file-preview/extend` |
| `linter.rules.correctness.useExhaustiveDependencies` | `warn` - useEffect 依赖完整性（React hooks 常见坑） |
| `linter.rules.suspicious.noArrayIndexKey` | `off` - 允许用数组 index 作 key |
| `linter.rules.a11y.noLabelWithoutControl` | `off` - 放宽 label 必须关联 control |
| `overrides[0]` | 对 `src/components/ui/**`（shadcn 生成的组件）特殊处理：关掉 import 排序、用双引号+分号、关掉 exhaustive-deps |

**如何初始化生成**：

```bash
pnpm add -D @biomejs/biome
npx @biomejs/biome init      # 生成 biome.json
# monorepo 场景：在仓库根建 biome.base.json，子包 extends 它
```

---

## 6. tailwind.config.js

**谁使用**：TailwindCSS 编译器，被 PostCSS 调用。

**触发时机**：Vite 处理 CSS 时，PostCSS 管线触发 Tailwind，Tailwind 读此配置决定扫描哪些文件提取 class、生成什么 CSS。

**主要配置项**：

| 配置项 | 作用 |
|---|---|
| `darkMode: ['class']` | 用 `.dark` class 切换暗色主题（而非 media query） |
| `content` | **扫描范围**：`index.html`、`src/**`、`@tremor/**`、`../internal/ui-sdk/src/**`（ui-sdk 源码里有 Tailwind class，漏扫会被 purge 掉导致样式丢失） |
| `safelist` | Tremor 图表运行时拼 class（`fill-blue-500`），扫描器检测不到，强制保留这些颜色×明度组合 |
| `theme.extend.colors` | 扩展色板：`primary`/`secondary`/`destructive`/`muted`/`accent`/`popover`/`card`/`success`/`warning`/`info`/`chart[1-9]`/`tag.*`/`sidebar.*`，全部用 `oklch(var(--xxx) / <alpha-value>)` 引用 CSS 变量 |
| `theme.extend.keyframes` / `animation` | 自定义动画：`pulse-text`、`voice-eq`、`breathing-halo` |
| `theme.extend.borderRadius` | 圆角用 `--radius` 变量驱动 |
| `theme.extend.fontSize` | `micro`/`mini`/`tiny` 三档超小字号 |
| `theme.extend.fontFamily.sans` | 覆盖默认 sans 栈，插入 CJK 字体（PingFang/YaHei/Noto），修复 Chrome 打印 PDF 丢中文字形问题 |
| `theme.extend.typography.sm` | `@tailwindcss/typography` 的 prose 样式微调（紧凑间距，适配聊天消息） |
| `plugins` | `tailwindcss-animate`、`@tailwindcss/typography`、`@tailwindcss/container-queries` |

**如何初始化生成**：

```bash
pnpm add -D tailwindcss postcss autoprefixer
npx tailwindcss init -p       # 生成 tailwind.config.js + postcss.config.js
# content 扫描范围、theme.extend 按项目逐步加
```

---

## 7. postcss.config.js

**谁使用**：PostCSS（CSS 后处理器），Vite 内置 PostCSS 集成会自动加载此文件。

**触发时机**：Vite 处理任何 CSS/PostCSS 文件时（dev 热更新、build 打包）。

**主要配置项**：

| 配置项 | 作用 |
|---|---|
| `plugins.tailwindcss` | 先跑 Tailwind，按 `tailwind.config.js` 扫描提取的 class 生成原子 CSS |
| `plugins.autoprefixer` | 后跑 autoprefixer，为生成的 CSS 补浏览器前缀 |

> 顺序很重要：Tailwind 先生成 class，autoprefixer 再加前缀。本项目配置极简，就这两步。

**如何初始化生成**：

```bash
npx tailwindcss init -p      # -p 同时生成 postcss.config.js
# 或手动创建：
cat > postcss.config.js <<'EOF'
export default {
  plugins: { tailwindcss: {}, autoprefixer: {} },
}
EOF
```

---

## 8. components.json

**谁使用**：shadcn/ui CLI（`npx shadcn@latest add <component>`）。

**触发时机**：执行 `shadcn add` 拉取组件源码时，CLI 读此文件决定文件放哪、用什么样式约定。

**主要配置项**：

| 配置项 | 值 | 作用 |
|---|---|---|
| `$schema` | shadcn 官方 schema | IDE 校验 |
| `style` | `default` | shadcn 默认视觉风格 |
| `rsc` | `false` | 不按 React Server Components 生成（本项目是纯 SPA） |
| `tsx` | `true` | 生成 `.tsx` |
| `tailwind.config` | `tailwind.config.js` | CLI 解析此文件，确认是否启用 cssVariables、有无 prefix、darkMode 怎么配 |
| `tailwind.css` | `src/index.css` | **入口 CSS 路径**，CLI 在 `init` 时往里注入 `:root`/`.dark` 的 CSS 变量定义与 `@tailwind` 三指令 |
| `tailwind.baseColor` | `slate` | shadcn 预设色板（slate/gray/zinc/neutral/stone 之一），决定注入的 CSS 变量具体值 |
| `tailwind.cssVariables` | `true` | **决定组件源码写法**：`true` 用语义 class（`bg-primary`），靠 CSS 变量驱动可切主题；`false` 硬编码颜色 |
| `tailwind.prefix` | `""` | 若 tailwind 有 prefix（如 `tw-`），组件生成的 class 须同步加前缀，否则被 purge |
| `aliases` | 见下 | 组件生成的目标路径映射 |

**aliases**：

| 别名 | 指向 | 用途 |
|---|---|---|
| `components` | `@/components` | 通用组件目录 |
| `ui` | `@/components/ui` | shadcn 生成的 UI 原子组件（Button/Dialog 等） |
| `utils` | `@/lib/utils` | `cn()` 等工具函数 |
| `lib` | `@/lib` | 业务库目录 |
| `hooks` | `@/hooks` | React hooks 目录 |

> 别名与 tsconfig 的 `paths` 必须一致，否则生成的组件 import 路径无法解析。`biome.json` 里对 `src/components/ui/**` 的 override 就是为了不破坏 shadcn 生成代码的格式。

**如何初始化生成**：

```bash
npx shadcn@latest init        # 交互式生成 components.json，并写入 baseColor/cssVariables
# 前提：已装好 tailwind + 配好 tsconfig paths
```

### 为什么 components.json 必须带 tailwind 配置

shadcn/ui 的工作模式与普通 npm 包不同：它**不是运行时依赖，而是把组件源码直接拷进项目**（落到 `@/components/ui/`）。组件代码一落地就脱离 shadcn 默认环境，渲染完全靠项目自己的 Tailwind。所以 CLI 必须在 `add` 时就读懂 Tailwind 配置，保证三者自洽：**生成的组件源码 + 注入的 CSS 变量 + 已有的 tailwind.config.js**。

对照本项目的实际链路：

```
components.json: cssVariables=true + baseColor=slate
        │
        ├── shadcn init 往 src/index.css 注入：
        │     :root { --primary: <slate 色板值>; --background: ...; ... }
        │     .dark { --primary: <暗色值>; ... }
        │
        ├── shadcn add button 生成 src/components/ui/button.tsx：
        │     className="bg-primary text-primary-foreground"   ← 语义 class
        │
        └── tailwind.config.js 的 theme.extend.colors：
              primary: 'oklch(var(--primary) / <alpha-value>)'  ← 引用同名变量
```

三者名字必须对得上：CSS 变量名（`--primary`）、tailwind.config.js 的色板 key（`primary`）、组件源码的 class（`bg-primary`）。CLI 读 tailwind 配置，就是为了让这套名字在生成时就对齐，而不是生成完再手动改。

> 一句话：shadcn 是"源码拷贝"而非"运行时依赖"，CLI 必须在生成时让 Tailwind 环境喂饱它，否则拷进来的组件是裸的、没颜色的。

---

## 9. index.html

**谁使用**：Vite（作为应用入口模板）；浏览器（最终渲染）。

**触发时机**：`vite` dev 时 Vite 以此为 SPA 入口提供；`vite build` 时 Vite 处理此模板生成 `dist/index.html`（注入打包后的 JS/CSS hash 引用）。

**主要配置项 / 结构**：

| 元素 | 作用 |
|---|---|
| `<meta charset="UTF-8">` | 字符编码 |
| `<link rel="icon" href="/favicon.svg">` | 站点图标，指向 `public/favicon.svg` |
| `<meta name="viewport">` | 响应式视口 |
| `<title>` | `NAP - Neutree Agent Platform` |
| `<div id="root">` | React 挂载点，`main.tsx` 里 `createRoot(document.getElementById('root'))` |
| `<div id="portal">` | Glide Data Grid 的 cell overlay 挂载点（Extend UI csv-viewer 用），固定定位 z-index 9999 |
| `<script type="module" src="/src/main.tsx">` | ES 模块入口，Vite 从此文件开始构建依赖图 |

**如何初始化生成**：

```bash
npm create vite@latest -- --template react-ts   # 模板自带 index.html
# 之后按需加 portal div、改 title、加 favicon
```

---

## 10. .i18n-tracker.lock（i18n 工具状态文件，非配置文件）

> 这个文件不是构建配置，而是 `web/tools/i18n-tracker.cjs`（`package.json` 的 `i18n:scan` / `i18n:update` / `i18n:update-all` 脚本调用的自研工具）的运行时状态产物。放在这里是因为常被误认为配置文件。

**谁使用**：`i18n-tracker.cjs`（Node 脚本）。

**触发时机**：`pnpm i18n:scan` / `pnpm i18n:update <file>` / `pnpm i18n:update-all` 时读写。

**它是什么**（源码 `i18n-tracker.cjs:15`）：

| 属性 | 值 |
|---|---|
| 位置 | 仓库根（实际是 `process.cwd()`，即跑命令时的 `web/` 下） |
| 内容 | JSON，结构 `{ "相对路径": "md5哈希" }` |
| 跟踪范围 | 每个 `.ts`/`.tsx` 源码文件的内容指纹，排除 `.d.ts` 与 `.test.` 文件 |

**解决的问题**：i18n key 增量同步的"脏标记"。源码里硬编码 `t('chat.send')` 这种 key，翻译文件要跟着同步；全量扫描慢、噪声大，所以用锁文件做基线--只列出改过的文件让人工补翻译。

**三种模式的读写规则**（`i18n-tracker.cjs:5-8`）：

| 模式 | npm 脚本 | 读锁文件 | 写锁文件 | 行为 |
|---|---|---|---|---|
| scan | `i18n:scan` / `i18n:scan:soft` | 读 | 不写 | 列出 pending 文件；`--soft` 永远 exit 0（CI 用） |
| update | `i18n:update <file>` | 读 | 写该文件 hash | 标记单个文件已同步 |
| update-all | `i18n:update-all` | 读 | 写所有 pending hash | 一次性标记全部已同步 |

scan 模式还会清理锁文件里"文件已删除"的陈旧条目（`i18n-tracker.cjs:157-169`），但只清这一类，不碰其他文件 hash。

**如何生成**：

不会自动生成。只有显式跑 update 命令才创建/更新：

```bash
# 首次（文件不存在时跑 update 会创建）
pnpm i18n:update src/components/chat/ChatPanel.tsx
# -> 落地 web/.i18n-tracker.lock：{"src/components/chat/ChatPanel.tsx": "<md5>"}

# 批量建立基线
pnpm i18n:update-all

# 只看不动（不创建/不修改锁文件）
pnpm i18n:scan
```

`loadHashes()`（`i18n-tracker.cjs:19`）在文件不存在时返回 `{}`，所以 scan 对"无锁文件"也能跑--会把所有文件都当 pending 列出，但绝不创建文件。只有 update/update-all 调 `saveHashes()`（`:29`）才落盘。

> 为什么叫 `.lock`：借用锁的语义，但不是 yarn.lock/package-lock.json 那种"锁定版本"，而是锁定"i18n 同步状态基线"--谁改了源码谁就"破了锁"，需重新走同步流程。本质是内容指纹库，叫 `.i18n-hashes.json` 其实更准确，命名是历史习惯。

---

## 初始化总览（从零搭一个同构项目）

```bash
# 1. 脚手架（一次性生成 package.json / vite.config.ts / tsconfig.json / index.html）
npm create vite@latest web -- --template react-ts
cd web

# 2. Tailwind + PostCSS
pnpm add -D tailwindcss postcss autoprefixer
npx tailwindcss init -p          # -> tailwind.config.js + postcss.config.js

# 3. shadcn/ui（会要求确认 tailwind 配置与别名）
npx shadcn@latest init           # -> components.json

# 4. Biome（替代 ESLint + Prettier）
pnpm add -D @biomejs/biome
npx @biomejs/biome init          # -> biome.json

# 5. Vitest
pnpm add -D vitest
# 建 vitest.config.ts（或复用 vite.config.ts 的 test 字段）

# 6. 装运行时依赖
pnpm add react-router-dom zustand @tanstack/react-query i18next react-i18next sonner ...

# 7. 按需加 monorepo 别名（tsconfig paths + vite resolve.alias 两处都要改）
```

> 关键对齐点：`tsconfig.json` 的 `paths`、`vite.config.ts` 的 `resolve.alias`、`components.json` 的 `aliases` 三者必须一致，否则别名在类型检查/打包/组件生成三个阶段表现不一。
