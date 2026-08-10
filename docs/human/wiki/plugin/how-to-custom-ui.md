---
name: how-to-custom-ui
description: 给后端同学的"通过 MCP 自定义平台 UI"最小可跑示例；用 Python 类比解释每个前端概念；含一个端到端可复制的 plugin 代码
created: 2026-07-02T07:49:06Z
updated: 2026-07-02T07:49:06Z
---

# 通过 MCP 自定义平台 UI —— 给 Python 程序员的入门示例

> 你的痛点：我会写 backend，我会写 MCP tool，但 UI 那一层全是 React / TypeScript / 浏览器，看一眼就劝退。
> 这篇文档：**只讲一个最朴素的场景**（加一个面板，按钮点了把 prompt 发给 Agent），**每个前端概念都用 Python 类比**，**代码可以直接复制粘贴**。

---

## 1. 先讲故事，再讲代码

你要做的事，翻译成自然语言就三句：

1. 在 workspace 视图里新增一个 tab（叫"README Explainer"）
2. 这个 tab 里有一个按钮（叫"帮我解释 README"）
3. 点按钮后，平台右侧的聊天面板自动出现一条新的用户消息："请解释 README.md"，然后 agent 跑起来

为什么这三步能搞定？因为你调用的"按钮 → 弹 prompt"函数（`sendMessage`）就是平台聊天框在用的同一个函数，**整个链路都跑在平台自己的 runtime 里，没出域**。

下面开始拆。

---

## 2. 前端概念 Python 类比速查表

| 前端概念 | Python 后端类比 | 一句话解释 |
|---|---|---|
| **React 组件** | 一个返回 HTML 的 **render 函数** | 你给它"props"（相当于 kwargs），它返回一段 HTML。对应到一个文件 `MyPanel.tsx`，里面 `export function MyPanel(props: {...}) { return <div>...</div> }` |
| **TypeScript** | Python 的 **type hints** | `: string`、`interface {...}` 就是 `def foo(x: str) -> None`。运行时会擦除，写错了编译期报错 |
| **JSX**（`<div>...</div>`）| **f-string 拼 HTML** | `const el = <div>Hello {name}</div>` 实际上等价于 `html = f"<div>Hello {name}</div>"` |
| **Hook**（`useState`、`useEffect`）| **装饰器 + 类成员变量** | 在函数组件里调用 `useState(0)`，你拿到 `[count, setCount]`。`useEffect(fn, deps)` 相当于 `@after_render` |
| **`window.tos`** | **全局单例服务** | 类似 `app.services.xxx`。`window` 是浏览器全局对象 = Python 的 `globals()` |
| **`registerPanel` / `registerPlugin`** | **app startup 注册 handler** | 跟 Django `AppConfig.ready()` 里 `from xxx import signals` 一模一样 |
| **`sendMessage(content)`** | **走平台自己的服务调用** | 你不用自己 POST `/api/chat`，平台封装好了；你调一下它就跑 |
| **IIFE**（`(function() {...})()`）| **if __name__ == '__main__'** | 一段"加载完立刻跑"的代码块 |
| **`<script>` 标签注入** | **`exec(open('plugin.js').read())`** | 浏览器通过 `<script src="...">` 把 JS 文件当字符串执行 |
| **`window.prompt(question, default)`**| **`input(提示语)` / `tkinter.simpledialog.askstring()`** | 浏览器原生的"弹个小输入框"。返回：`string`（用户敲了 + 确定）/`null`（用户取消）/`''`（空串） |
| **`if (!q) return`** | **`if not q: return`** | JS 的 truthy/falsy 简写。`!null === true`、`!'' === true`、`!'abc' === false`，一个 `!` 同时挡掉"取消"和"空输入"两种情况 |

> 只要你写过 Django Flask FastAPI 任意一个，这套就懂了——只是 naming convention 怪。

---

## 3. 端到端示例：加一个 "README Explainer" 面板

下面这段代码，**是一段可以被浏览器执行的完整 IIFE**。结构按平台规定来（注释里都写了），名字你可以换。

### 3.1 文件：`explain-panel.js`（放你 plugin bundle 目录下）

```javascript
/**
 * README Explainer —— 一个最小的 "自定义 UI 通过 MCP 触发 prompt" 示例。
 *
 * 它做了两件事：
 *   1. 注册一个 workspace 面板（id = 'readme_explainer'，会出现在右侧 tab 里）
 *   2. 面板里有一个按钮，点一下把 "请解释 README.md" 这个 prompt 发给当前 session
 *
 * 怎么加载：见 §4
 */

(function () {
  // 平台暴露给所有 plugin 的 "全局服务台"。
  // 类似 app.services（一个对象，方法全是已绑定好上下文的）。
  const tos = window.tos
  if (!tos) {
    console.error('[readme-explain] platform host (window.tos) is missing — load order wrong?')
    return
  }

  // 用到的东西先取出来（避免在 React 函数里反复访问，看起来也清爽）
  const React = tos.React                                              // 渲染库
  const { useState } = React                                           // React 的状态 hook
  const actions = tos.workspace.useAgentSessionActions()                // 平台 session 操作集
  const nav = tos.workspace.useSessionNavigation()                      // 切换 active session 的函数（这个示例用不到）

  // ── 面板组件 ────────────────────────────────────────────────
  // 一个函数返回一段 HTML。React 等价于 render(view, context)。
  // 平台约定 props = { workspaceId, instanceId }（看 PluginPanelProps）。
  function ExplainPanel({ workspaceId }) {
    const [busy, setBusy] = useState(false)
    const [lastQuestion, setLastQuestion] = useState('')

    async function onAsk() {
      // browser 原生 prompt：弹一个小输入框，返回 用户输入的字符串 / null（取消）/ ''（空）
      // Python 对照：q = input('...')  或 q = tkinter.simpledialog.askstring('...')
      const q = window.prompt('想问 AI 关于 README 的什么？', '整体讲一下这个仓库的结构')
      // !q 同时挡掉两种无效输入：null（取消） 和 ''（空字符串）。
      // 相当于 Python 的 if q in (None, ''): return
      if (!q) return
      setLastQuestion(q)
      setBusy(true)
      try {
        // 这是核心 —— 平台聊天框在用的同一个 sendMessage
        // 调完之后聊天面板会出现一条 user message，agent 接着跑
        actions.sendMessage(`请帮我用中文解释 README.md，重点回答：${q}`)
      } finally {
        setBusy(false)
      }
    }

    return React.createElement(
      'div',
      { style: { padding: 16, fontFamily: 'sans-serif' } },
      React.createElement('h3', null, 'README Explainer'),
      React.createElement(
        'p',
        { style: { color: '#666' } },
        '这是一个通过 MCP 自定义的面板。点击下面的按钮，会把 prompt 发给当前 Agent session。'
      ),
      React.createElement(
        'button',
        {
          onClick: onAsk,
          disabled: busy,
          style: { padding: '8px 12px', fontSize: 14 },
        },
        busy ? '已发送…' : '请 AI 解释 README'
      ),
      lastQuestion
        ? React.createElement(
            'p',
            { style: { marginTop: 16, fontSize: 12, color: '#999' } },
            '上一次问题：' + lastQuestion
          )
        : null
    )
  }

  // ── 把面板注册到平台 ────────────────────────────────────────
  // 平台根据 'readme_explainer' 这个 id 找到你的面板；这个 id 必须在
  // mcp_catalog 里某条 server 行的 ui_panel 字段里写出来才会显示。
  tos.registerPanel({
    id: 'readme_explainer',
    label: () => 'README Explainer',
    component: ExplainPanel,
  })

  console.log('[readme-explain] panel registered')
})()
```

**关于 `React.createElement` 而不是 JSX**：
JSX 需要 build step（`npm run build` → 转成 `React.createElement`）。一个完整的 IIFE bundle 通常由构建工具产出，源代码里写 JSX 即可。这里为了"零构建、直接在浏览器里看"，所以用 `React.createElement`，**你理解就好，写源码时用 JSX 写就行**。

### 3.2 改成 JSX 写源码（推荐写法，需要 build）

如果你有前端构建工具，源码可以这样写（更清晰、更主流）：

```tsx
// src/ExplainPanel.tsx
import { useState } from 'react'

export function ExplainPanel({ workspaceId }: { workspaceId: string }) {
  const [busy, setBusy] = useState(false)
  const [lastQuestion, setLastQuestion] = useState('')

  async function onAsk() {
    const q = window.prompt('想问 AI 关于 README 的什么？')
    if (!q) return
    setLastQuestion(q)
    setBusy(true)
    try {
      const actions = window.tos.workspace.useAgentSessionActions()
      await actions.sendMessage(`请帮我用中文解释 README.md，重点回答：${q}`)
    } finally {
      setBusy(false)
    }
  }

  return (
    <div style={{ padding: 16, fontFamily: 'sans-serif' }}>
      <h3>README Explainer</h3>
      <p>通过 MCP 自定义的面板</p>
      <button onClick={onAsk} disabled={busy}>
        {busy ? '已发送…' : '请 AI 解释 README'}
      </button>
      {lastQuestion && <p style={{ marginTop: 16 }}>上一次问题：{lastQuestion}</p>}
    </div>
  )
}
```

构建后产出一个 IIFE bundle（`webpack` / `vite` / `esbuild` 都行），平台照样通过 `<script>` 加载它。

---

## 4. 怎么让平台知道加载这个 plugin？

平台有两种加载模式：

### 4.1 Dev 模式（最简单，本地调试用）

平台启动时会从 `/dev-plugins/manifest.json` 读清单。把你的 JS 加上即可：

```jsonc
// web/dist/dev-plugins/manifest.json
{
  "plugins": [
    {
      "id": "readme-explain",
      "label": "README Explainer (dev)",
      "entry": "/dev-plugins/readme-explain.js",
      "ui_panel": "readme_explainer"
    }
  ]
}
```

把你的 `explain-panel.js` 放到 `web/dist/dev-plugins/readme-explain.js`，刷新页面就能看到 tab。

### 4.2 Eager 模式（生产路径）

平台启动时按 `/api/plugins` 拉清单，依次 `<script src>` 注入你的 IIFE bundle。**plugin 服务端要返回 entry URL**，前端会按 URL 加载并执行。

加载流程图：

```
浏览器启动 → 前端 index.html
         → main.ts 调 /api/plugins（拉清单）
         → 按清单依次插入 <script src="https://your-cdn/readme-explain.js">
         → 你的 IIFE 跑起来 → window.tos.registerPanel(...)
         → 主 App 渲染时 useWsApps 检测到面板，挂到 workspace UI
```

> 关键不变量：**平台只负责加载脚本 + 提供 `window.tos`**；**你的 IIFE 脚本负责调 `registerXxx`**。和 Django app config 在 startup 注册 handler 是一样的分工。

---

## 5. 为什么点按钮会自动在右侧聊天里出现新消息？

把链路展开看：

```
+---------------------------+
| 你的面板（浏览器）         |
| <button onClick>          |
+-----------------+---------+
                  | 调 actions.sendMessage("请解释 README.md")
                  v
+---------------------------------------------------+
| 平台：AgentSessionStore                          |
|   sendMessage() {                              　  |
|     ① 拼装 user message + 流式占位 assistant msg |
|     ② POST /api/workspaces/:id/chat            　 |
|     ③ 把 SSE 的 text_delta / tool_call ...      　|
|        一片片塞进 store.messages                　 |
|   }                                              |
+---------------------------------------------------+
                  | 同一个 store 也被聊天面板订阅
                  v
+---------------------------+
| 右侧聊天 Transcript 组件   |
| 自动出现一条 user 消息     |
| 然后 assistant 开始流式回答 |
+---------------------------+
```

**没有"我给你发事件""你通知我刷新"这种桥**——因为聊天面板和你按的按钮看见的是**同一个 store**。这就是引言里说的"UI 和 prompt 都被平台管理，被彻底打通"。

### 5.1 如果你想发到一个**不是当前**激活的 session

`sendMessageToSession(sessionId, content, images?)` —— 文档注释（`stores/agent-session-store.ts:138-148`）很直白：

> 用于 plugin UI 需要唤醒某个特定 session 的场景（例如代码 review plugin 把请求提交给发起 review 的那个 chat，不管用户现在在看哪个 chat）。

签名：

```typescript
actions.sendMessageToSession('session-uuid-here', '请帮我 review MR #42')
```

实现是：HTTP /chat 请求从本地发出去，**SSE 流由服务端 cp 的 persist 插件自己接管消费**，本地 store 不动；用户下次切到那个 session 时就能看到新 exchange。**不抢当前流、不打架**。

---

## 6. 验证清单（不要交付没验证的东西）

按顺序跑一遍，每条都要看到预期输出：

- [ ] **6.1** `window.tos` 存在：在浏览器 console 执行 `console.log(window.tos)`，能看到 version / registerPanel / workspace 等字段。
- [ ] **6.2** Panel 注册成功：你的 IIFE 里 `console.log('[readme-explain] panel registered')` 被打印，**且没有 `[plugins] duplicate panel id` warning**（重复 id 会被静默拒收）。
- [ ] **6.3** Tab 出现：打开任一 workspace，右侧 launcher 里能看到 "README Explainer" 标签。**如果没看到**：检查 `mcp_catalog.ui_panel` 里是否写了 `readme_explainer`（id 必须严格匹配）。
- [ ] **6.4** 按钮响应：点 "请 AI 解释 README"，prompt 弹出 → 输入 → 确认。**预期**：右侧聊天出现一条 user message（"请帮我用中文解释 README.md..."），**接着 assistant 开始打字**。
- [ ] **6.5** Developer Tools → Network：能看到 `POST /api/workspaces/{id}/chat` 请求状态 200，响应是 SSE stream（`Content-Type: text/event-stream`）。
- [ ] **6.6** Reload 后还在：刷新页面 → 面板仍然在 → session 自动加载 → 上一次的对话历史也还在（持久化是平台做的，不是你的事）。

> 其中任何一步没通过，回去看 §4 的加载路径；99% 的"按钮不响应"是 plugin bundle 没真的加载进浏览器。

---

## 7. 进阶（同一个机制的不同形态）

平台的其他"UI 定制面"用完全相同的 `window.tos` 暴露，只是接口不同。**Python 类比**：同一个 `app.services` 提供的不同 service class。

### 7.1 改 MCP tool 在聊天里的卡片样式

```js
tos.registerToolRenderer('my_tool_name', {
  defaultExpanded: true,        // 卡片默认展开
  renderInput: (tool) => /* React 组件，<div>{JSON.stringify(tool.input)}</div> */,
  renderResult: (tool) => /* React 组件，结果怎么显示你说了算 */,
})
```

注册后，所有名为 `my_tool_name` 的 tool call 在 transcript 里都走你的渲染器。Builder Mode 的 Approve/Reject 卡片就是这个机制（见 `plugins/index.ts:47`）。

### 7.2 监听 tool_result 自动做副作用

```js
tos.registerPlugin({
  id: 'my-plugin',
  toolResultHandlers: [
    {
      id: 'my-plugin.refresh-on-tool',
      match: ({ toolName }) => toolName.includes('something_changed'),
      debounceMs: 200,
      onMatch: () => /* 你自己 fetch / setState / 重渲染某面板 */,
    },
  ],
})
```

> Python 类比：Django signal —— 收到 `post_save` 自动跑回调。`(toolName, toolInput, toolOutput, isError, callId, workspaceId)` 是 SSE 流里 tool_result 事件推上来的 payload。

### 7.3 跨插件事件总线 / 主题 / i18n

| 能力 | 入口 | 用途 |
|---|---|---|
| 平台事件总线 | `tos.events.on('lang.change', cb)` | 监听语言切换，通知你的 i18n 实例 |
| 当前主题 | `tos.ui.useResolvedTheme()` | 跟平台主题切换（CSS 变量已经自动级联） |
| 自定义持久化 state | `tos.workspace.useInstanceState()` / `useInstancePersistentState()` | 面板级 localStorage（key 会带 instanceId，自动 namespace） |
| Header 按钮 | `tos.windows.useAppHeaderSlot(...)` | 弹窗顶栏挂按钮 |

---

## 8. 常见误区（踩坑提前看）

| 症状 | 根因 | 修法 |
|---|---|---|
| 我 `console.log` 跑通了，但 UI 上没有 panel | 忘了在 `mcp_catalog.ui_panel` 里写你的 id | 那个 id 必须**和 panel id 严格相等**，且必须挂在某个 enabled MCP server 上 |
| 按钮 click 没反应 | IIFE 跑早了，`window.tos` 还是 undefined | 在 `index.html` 后面或者用 `DOMContentLoaded` 后再 `<script>` |
| `registerPanel` 后看到 `duplicate panel id` warning | 同 id 重复注册（lazy bundle 又被 eager 加载了一份） | 检查 manifest，避免把同一个 bundle 列在 eager + dev 双份 |
| `sendMessage` 后没出现 user message | 你在**非 AgentSessionProvider** 下层里调的（比如 SharePage 公开会话） | 这种场景下改用 `api.getWorkspaceMessages` 之类手工处理（store 不存在） |
| 调试时希望 click 后立刻刷新视图 | 直接调 `sendMessage` 不会自动刷新右侧 chat | 不需要手动刷——聊天组件订阅同一个 store，你自己 refresh 反而错乱 |
| TypeScript 报错 `Cannot find name 'React'` | bundle 构建里漏了 React 的 alias | dev 环境使用 `window.tos.React`，别自己 import 'react' |

---

## 9. 它在代码里到底藏哪儿了？（给 review 时查）

> 这一节可选，初读跳过；以后 review 设计时回来定位用。

| 你用到的 API | 平台暴露位置 |
|---|---|
| `window.tos`（全局宿主） | `web/src/lib/host.ts:81 installHost()` |
| `tos.registerPanel` | `web/src/lib/panel-registry.ts:30` |
| `tos.registerPlugin` | `web/src/plugins/registry.ts:17` |
| `tos.registerToolRenderer` | `internal/ui-sdk/src/tool-renderers/plugin-registry.ts:17` |
| `tos.workspace.useAgentSessionActions` / `useSessionNavigation` | `web/src/stores/AgentSessionContext.tsx:182` + `web/src/stores/active-session-store.ts:57` |
| `actions.sendMessage` / `sendMessageToSession` | `web/src/stores/agent-session-store.ts:714` / `:763` |
| 4 级 renderer 优先级解析（builder/built-in/plugin/fallback） | `internal/ui-sdk/src/tool-renderers/registry.ts:154 resolveRenderer` |
| Dev plugin manifest 路径 | `web/src/lib/plugin-lazy-registry.ts` + `/dev-plugins/manifest.json` |

读源码是反着查这套设计的好起点（**先看本节查到的具体行号 → 顺着上下文 50 行读 → 比抽象描述快**）。

---

## 10. 小结

- **能自定义什么**：工具卡片样式 / workspace 面板（tab）/ 完整 plugin（面板 + 副作用）。三档分别对应 `registerToolRenderer` / `registerPanel` / `registerPlugin`。
- **UI 触发 prompt 的桥梁**：`tos.workspace.useAgentSessionActions()` 拿到 `sendMessage` / `sendMessageToSession`。这两个函数**就是聊天框在用的同一个**——所以"按钮 → 右侧新消息"是天然闭环，不是事件桥接。
- **怎么加载**：Dev 模式一份 manifest；Eager 模式 `/api/plugins` 给你 entry URL，平台按 URL 插 `<script>` 跑你的 IIFE。
- **验证**：6 项 checklist（§6）跑完才算"做完"。空口说"做完了"在阿里这里叫 3.25。
