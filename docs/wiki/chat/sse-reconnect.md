---
name: sse-reconnect
description: 前端 buildSSEHandlers 的 onItemStarted/onItemCompleted 语义，以及 cp↔web SSE 重连的 at-most-once 投递与状态自愈机制
type: workflow
domain: agents
related:
  - docs/wiki/workspace/sse.md
  - docs/wiki/workspace/session.md
  - docs/wiki/env_runner/cp_proxy.md
  - web/src/stores/agent-session-store.ts
  - web/src/lib/api/sse.ts
  - control-plane/src/lib/sse.ts
  - control-plane/src/lib/session-recovery.ts
created: 2026-07-14T08:48:27Z
updated: 2026-07-14T08:48:27Z
---

# SSE 重连与 onItemStarted 语义

> 视角：架构师给新人讲解。配套 [sse.md](sse.md) 一起看 -- sse.md 讲"一条 SSE 消息从 agent 到 cp 到前端的正向链路"，本文专门拆"链路断了之后怎么接回来"，并回答三个具体疑问：onItemStarted 为什么只处理 tool_call？tool_call 为什么能直接 append 不判重？重连时 toolA 已发但 ack 丢了怎么办？

> 顶层设计一句话：**cp 的 `ActiveSSEStream` 是"火忘式"广播扇出 + at-most-once 投递；重连只接未来、不补过去。前端靠"内存态 + onItemCompleted 兜底判重"自愈。** 这条红线决定了 onItemStarted 可以无脑 append。

---

## 1. 代码地图

| 文件 | 角色 | 关键点 |
|---|---|---|
| `web/src/stores/agent-session-store.ts` | 前端 session store；`buildSSEHandlers` 工厂构造 SSE 回调 | onItemStarted / onItemCompleted 的 append/dedup 逻辑都在这 |
| `web/src/lib/api/sse.ts` | SSE 消费层；`handlersToPlugin` 把回调适配成 `TurnPlugin`，含 web↔cp 重连工厂 | `buildCPReconnectFactory` / `createCPReconnectStream` |
| `control-plane/src/lib/sse.ts` | cp 侧 SSE 截流；`ActiveSSEStream` 广播 + persist 落库 | `createReconnectSSEResponse`（只接 live，不补播）/ `emitSSE` |
| `control-plane/src/lib/session-recovery.ts` | cp 重启后捡回孤儿 session | `recoverOrphanedSessions`（cp→agent 重连，会 replay） |

> 抓手：先看 **谁广播**（cp 的 `ActiveSSEStream`）、**谁收**（web 的 `runTurn` + handlers）、**重连谁接谁**（web POST `/cp-reconnect` → cp 把 web 加进 `controllers` 集合）。三层对齐后，"会不会判重"就是"广播方补不补播"的题。

---

## 2. onItemStarted 为什么只处理 tool_call

UniversalItem 有四种 kind：`message` | `tool_call` | `tool_result` | `status`。对每种，`item.started` 的语义和前端要不要动作为下表：

| kind | item.started 带什么 | 前端要立刻建块吗 | 为什么 |
|---|---|---|---|
| `message`（assistant 文本） | 空（无文本） | ❌ 不建 | 文本走 `item.delta` 流式增量，首个 delta 到时懒建文本块（`onItemDelta` 里 `last?.type === 'text'` 判断）。started 时没东西可渲染 |
| **`tool_call`** | **携带 `arguments`（输入参数）** | **✅ 必须建** | 见下方三条理由 |
| `tool_result` | 输出（工具产出） | ❌ 不建 | result 是"工具产出的结果"，没有"开始了但还没产出"的中间态；到 `onItemCompleted` 时按 `call_id` 匹配已有 tool 块、贴上 `result` 即可 |
| `status` | label/detail（状态标签） | ❌ 不建 | status 是一次性状态标签（如"Thinking..."），到 `onItemCompleted` 时 append |

tool_call 必须在 `onItemStarted` 建块的三条理由（直接抄自 `agent-session-store.ts:411-419` 的注释）：

1. **渲染器靠 `(name, input)` 解出来的"真实工具名"选定** -- `ToolCallBlock.tsx:50-51` 渲染时调 `resolveRenderer(tool, agentType)`，它第一步是 `unwrapExecuteDispatchName(tool.name, tool.input)`（`registry.ts:186-190`）。这个函数的逻辑是：当工具名是字面量 `execute`（**Codex 把所有 MCP 调用都包成 `execute`**，真实工具名藏在 `input.tool`、配 `input.server`/`input.arguments`）时，**只有 `input` 已被解析成 `{tool:'真实名', server:'...'}`，才会返回 `input.tool`**；否则原样返回 `'execute'`。然后 `toolRenderers[真实名]` 才能命中专用渲染器，命不中就 fallback 到 `agentFallbacks` 或 `null`（→ `<DefaultInput>` 通用渲染器，`ToolCallBlock.tsx:128`）。所以 input 必须在 started 时就 parse 填进去。对比：

   ```
   tool.name = "execute"
   情况 A（started 已 parse input）: input = { server:"git", tool:"create_pr", arguments:"..." }
     unwrapExecuteDispatchName -> "create_pr"
     toolRenderers["create_pr"] 命中 -> 专用 PR 卡 ✅
   情况 B（started 没填 input）: input = {}
     unwrapExecuteDispatchName("execute", {}) -> "execute"（unwrap 失败，record.tool 是 undefined）
     toolRenderers["execute"] 不存在 -> codexFallback / null -> DefaultInput 通用卡 ❌
   ```

2. **有的 tool_call 不发独立的 `item.completed`** -- 某些 agent 的事件序列是 `item.started(tool_call)` 之后**直接跳** `item.completed(tool_result)`，中间没有 `item.completed(tool_call)`。而"重新 parse input 并 update 块"的逻辑在 `onItemCompleted` 的 tool_call 分支（§7.2 ←3-4）。如果把这个 parse 推迟到 completed 才做，对这种"没有 completed 事件"的调用，input 永远是空 `{}`——和理由 1 叠加，`unwrapExecuteDispatchName` 永远 unwrap 不出来，块卡在通用 `DefaultInput` 上永远翻不了身。所以 input 必须在 started 阶段就填，不能指望 completed 兜底。

3. **用户要立刻看到"工具已开始"** -- 尤其审批流，用户需要在工具执行前就看到 input 并决定是否批准，不能等 completed。

所以 `onItemStarted` 里那段 `if (item.kind === 'tool_call')` 不是"只处理 tool_call"，而是"tool_call 是唯一需要在 started 阶段就建块"的 kind。其余 kind 在 started 阶段都是 no-op，留到 delta/completed 再处理。

```ts
// agent-session-store.ts:407-449 简化
onItemStarted: guard((item) => {
  if (item.kind === 'tool_call') {           // 唯一需要在这里动的 kind
    const tc = item.content?.[0]
    if (tc?.type === 'tool_call') {
      let input = {}
      try { input = JSON.parse(tc.arguments || '{}') } catch {}
      store.setState((s) => ({
        messages: s.messages.map((m) => m.id === assistantId
          ? { ...m, blocks: [...m.blocks, { type: 'tool', tool: { id: tc.call_id!, name, input, startedAt: Date.now() } }] }
          : m),
      }))
    }
  }
  // message / tool_result / status：started 阶段 no-op
}),
```

---

## 3. tool_call 为什么可以直接 append 不判重

注意上面 `onItemStarted` 是 **`...m.blocks, { type: 'tool', ... }]` 直接 append**，没有 `findIndex` 检查 call_id 是否已存在。对比 `onItemCompleted`（`agent-session-store.ts:508-548`）对 tool_call 的处理：

```ts
onItemCompleted: guard((item) => {
  if (item.kind === 'tool_call') {
    const tc = item.content?.[0]
    if (tc?.type === 'tool_call') {
      store.setState((s) => ({
        messages: s.messages.map((m) => {
          const idx = m.blocks.findIndex((b) => b.type === 'tool' && b.tool.id === tc.call_id)
          if (idx >= 0) {
            // ✅ 找到了 -> UPDATE（贴 completedAt / 补 input）
            blocks[idx] = { type: 'tool', tool: { ...b.tool, input, completedAt: Date.now() } }
          } else {
            // ❌ 没找到 -> APPEND（兜底）
            blocks.push({ type: 'tool', tool: { id: tc.call_id!, ... completedAt } })
          }
        }),
      }))
    }
  }
})
```

`onItemCompleted` 是 **findIndex 判重**（找到就 update，找不到才 append），而 `onItemStarted` 是 **无脑 append**。为什么 onItemStarted 能这么"放肆"？因为底层投递语义保证了它不会被重复触发。

### 3.1 cp 的 `ActiveSSEStream` 是火忘式广播扇出

cp 侧 `emitSSE`（`control-plane/src/lib/sse.ts:217-228`）：

```ts
function emitSSE(stream: ActiveSSEStream, sseText: string) {
  if (stream.controllers.size === 0) return
  stream.lastEmitAt = Date.now()
  const encoded = encoder.encode(sseText)
  for (const c of stream.controllers) {     // 遍历当前所有已连接的 controller
    try { c.enqueue(encoded) } catch { stream.controllers.delete(c) }
  }
}
```

关键：**一条事件只 `enqueue` 给"此刻在 `controllers` 集合里"的客户端**。enqueue 之后这条事件就从 cp 的视野里消失了 -- cp 不缓存历史、不记 ack、不知道某个客户端到底读没读到。

### 3.2 重连只接未来，不补过去

`createReconnectSSEResponse`（`sse.ts:276-320`）收到 web 的 `/cp-reconnect` 时：

```ts
const clientReadable = new ReadableStream<Uint8Array>({
  start(controller) {
    stream.controllers.add(controller)   // ← 把新客户端塞进集合，仅此而已
  },
  cancel() { stream.controllers.delete(myController) },
})
```

**没有任何回放循环**。注释写得很直白（`sse.ts:262-266`）：

> Attaches as live client to receive **subsequent** events; the caller is expected to have already loaded persisted history from the DB.

以及（`sse.ts:285-287`）：

> Stream already finished - clean up and return null. The completed turn's messages are already persisted to DB; the client will pick them up via **loadHistory, not SSE replay**.

### 3.3 推论：item.started 对一条 web 连接至多投递一次

把 3.1 + 3.2 拼起来：

- 一条 `item.started` 事件在 cp 的 `emitSSE` 里只走一遍 `for (const c of controllers)`。
- 重连进来的新 controller 是在 `add` 之后才加入集合的，之前已经 `enqueue` 过的事件对它不可见。
- 所以**对任意一条 web 连接，`item.started` 最多收到一次**。

web 侧 `runTurn` 级重连（`sse.ts` 的 `buildCPReconnectFactory`）也不会重建 plugin -- `handlersToPlugin` 适配的回调状态在 `runTurn` 内部跨重连保留，store 里的 `messages` 不清空。所以 toolA 的块如果重连前已 append，重连后仍在内存里，且不会再收到第二次 `item.started`。

> 闭环：**"不判重"的安全前提是"不会重发"。** cp 的火忘广播 + 重连只接未来，共同保证 `onItemStarted` 对一次连接至多触发一次，所以 append 不会产生重复块。

### 3.4 兜底：万一 item.started 在途丢失怎么办

"至多一次"意味着可能零次 -- 事件发出去了但 TCP 断在半路，web 没收到。这时 toolA 的块不会在 `onItemStarted` 建出来。兜底是 `onItemCompleted`：

- `onItemCompleted`（tool_call）走的是 **findIndex 判重 + 不存在则 append** 的路径（§3 代码）。
- 所以即使 `item.started` 丢了，`item.completed` 一到，块照样会被建出来（带 `completedAt`，但缺 `startedAt`）。
- 代价：UI 上"工具运行中"那个中间态没了，工具块直接以"已完成"形态出现。可接受。

> 颗粒度：这是经典的 **at-most-once 投递 + 幂等 completion** 组合。`onItemStarted` 享受 at-most-once 的"不判重"红利；`onItemCompleted` 承担"幂等兜底"的责任。两者分工明确，不要把判重逻辑挪到 started -- 那等于否认了 at-most-once 的前提。

### 3.5 对称问题：万一 onItemCompleted 在途丢失怎么办

这是 §3.4 的镜像。先分清"丢在哪一层"：

| 层 | 投递语义 | 丢了能恢复吗 |
|---|---|---|
| agent -> cp | at-least-once（cp 的 `runTurn` 调 `reconnectFactory` 重连 agent，agent replay 缓冲事件）+ persist 幂等（call_id 判重 `ON CONFLICT DO NOTHING`，`sse.ts:696`） | ✅ 能，replay + 幂等落库 |
| **cp -> web** | **at-most-once**（`emitSSE` 火忘，重连不补播，§3.1-3.2） | ❌ 这条连接恢复不了，靠 **DB 刷新兜底** |

本问指 cp -> web 这层（和 §3.4 的 item.started 丢失同层）。**关键认知：persist 插件在 broadcast 之前跑**（`sse.ts:478-480`："persist runs first so its DB enqueues land on the shared queue before broadcast's deferred emits"）。所以 onItemCompleted 事件**即使 web 没收到，DB 也已经落库了** -- DB 是权威，刷新即恢复。这和 item.started 不同（item.started 不落库，§5 不变量 3，只能靠 onItemCompleted 的 append 兜底）。

按 kind 分述（cp -> web 层丢失 onItemCompleted）：

**① message/assistant completed 丢：**
- 流式 delta 已经累积 -> UI 有文本（可能有拼接毛刺，但可读）。
- 权威文本替换（§7.1 ←13）没发生。
- 运行时重连：不补播，UI 保持 delta 文本。
- 刷新：DB 有权威文本（persist 在 completed 落 text，`sse.ts:1011-1019`）-> 恢复。

**② tool_call completed 丢（本问核心，对应 toolA 场景）：**
- 若 onItemStarted 已建块（有 startedAt + input）-> 块存在，显示"运行中"（spinner）。
- 缺 `completedAt`。
- 后续 `tool_result` completed 若到达：findIndex by call_id 命中（块在）-> 贴 result，spinner 消失，绿点出现。时长用 `resultAt - startedAt`（`ToolCallBlock.tsx:31-37` 的兜底分支）-> **功能正常，只缺 inference 时长徽标**。
- 后续 tool_result 也丢：spinner 永驻。刷新：DB 有 tool_call（persist 在 completed 落库，`sse.ts:1023-1051`）-> 恢复。
- **若 onItemStarted 也丢了（started + completed 都丢）**：块不存在，tool_result completed 到了 findIndex=-1 被丢（§7.3 ←4 无 append 兜底）。刷新：DB 有 tool_call + tool_result -> 块完整恢复（缺 startedAt）。

**③ tool_result completed 丢：**
- tool 块在（started/completed 建的），但永远没 result -> spinner 永驻。
- 刷新：DB 有 tool_result（persist 在 completed 落库，`sse.ts:1053-1066`）-> 恢复。
- 副作用：`toolDispatcher.dispatch`（§7.3 ←10）没触发 -> 插件 UI 不刷新。这是 live-event 副作用，刷新不补派发（但插件 UI 读 DB 块会显示 result）。

**④ status completed 丢：**
- 状态标签永不出现。
- **status 不落库**（persist 的 item.completed 没有 status 分支）-> 刷新也不恢复。但 status 是临时标签（"思考中"），丢了是纯装饰，无功能影响。

> 闭环：**item.started 丢靠 onItemCompleted 兜底（append 路径）；item.completed 丢靠 DB 刷新兜底（persist 已落库）。** 两条兜底路径不对称 -- 根源是 started 不落库、completed 落库。理解这条不对称，就理解了为什么 §3.4 强调"onItemCompleted 必须幂等"，而这里强调"刷新可恢复"。

---

## 4. Reconnect 示例：toolA 已发、ack 丢了的完整过程

### 4.1 场景设定

- 一次 turn T，session=`sess-1`，workspace=`ws-1`。
- turn 已开始，cp 已发出 `session.started`、一段 `item.delta`（文本 "Let me check"）、以及 toolA 的 `item.started`（call_id=`call-A`，name=`read_file`，input=`{path:"a.ts"}`）。
- **恰在 toolA 的 `item.started` 发出后、cp 不确定 web 是否读到时，web↔cp 连接断开。** cp 视角：事件已经 `enqueue` 给旧 controller，但旧 controller 的 cancel 触发了，cp 不知道 web 那边到底解析没解析到这条事件。
- toolA 尚未 completed，cp↔agent 的流还活着（cp 还在持续收 agent 事件）。

### 4.2 reconnect 前的状态

**cp 侧（`ActiveSSEStream` for `ws-1:sess-1`）：**

```
activeStreams["ws-1:sess-1"] = {
  workspaceId: "ws-1",
  controllers: Set[],              ← 旧的 web controller 已 cancel 移除，集合空了
  done: false, turnActive: true,   ← turn 还在跑
  queue: SerialQueue (含已落库的 text/tool_call)
}
- 已 emit 给旧 controller（现已断）的事件：session.started, item.delta("Let me check"), item.started(toolA)
- 这些事件 cp 不再持有，也不会重发
```

**web 侧 store（断连瞬间，两种可能）：**

| 子情形 | 内存 messages | 说明 |
|---|---|---|
| **A. web 实际收到了 toolA.started** | `[userMsg, assistant{blocks:[text:"Let me check", tool{call-A, startedAt}]}` | toolA 块已 append，isStreaming=true |
| **B. web 没收到 toolA.started（在途丢失）** | `[userMsg, assistant{blocks:[text:"Let me check"]}]` | 缺 toolA 块 |

cp 无法区分 A/B（火忘，无 ack）。这正是用户疑问的来源。

### 4.3 重连后的 SSE 处理过程

web 侧 `runTurn`（`sse-consumer`）检测到主响应流结束，调用 `buildCPReconnectFactory` 返回的重连工厂（`sse.ts:179-220`）：

```
1. web: 指数退避（500ms→1s→2s→... 上限 8s，最多 5 次）
2. web: POST /_proxy/agent/ws-1/cp-reconnect?session_id=sess-1
3. cp:  createReconnectSSEResponse("ws-1", "sess-1")
        └─ activeStreams.get("ws-1:sess-1") 命中（turnActive=true, done=false）
        └─ new ReadableStream -> stream.controllers.add(newController)   ← 只做这一步
        └─ 返回 200 text/event-stream
4. web: runTurn 把新 Response 当作主源继续消费，handlersToPlugin 状态保留（store 不清空）
```

**关键：cp 不补播任何历史事件。** 从这一刻起，cp 的 `emitSSE` 后续 emit 的事件才会进 newController。

### 4.4 处理完成后的状态

假设重连后，agent 继续产出：toolA 的 `item.completed`（call_id=`call-A`，input=`{path:"a.ts"}`）、`item.delta`（"Done"）、`session.ended`。

**子情形 A（web 已有 toolA 块）的演化：**

```
重连后收到 item.completed(toolA, call-A):
  onItemCompleted: findIndex(call-A) -> idx=1 ✅ 命中
    -> UPDATE blocks[1].tool = { ...tool, input, completedAt }
    （不 append，块不重复）
重连后收到 item.delta("Done"):
  onItemDelta: last block is text -> 追加 "Done"
重连后收到 session.ended:
  onSessionEnded: isStreaming=false, isLoading=false, isBusy=false

最终 web messages:
[userMsg, assistant{blocks:[text:"Let me check Done", tool{call-A, startedAt, completedAt, input}]}]
                                     ↑ 块在重连前就有，重连后只 update，无重复 ✅
```

**子情形 B（web 缺 toolA 块）的演化：**

```
重连后收到 item.completed(toolA, call-A):
  onItemCompleted: findIndex(call-A) -> idx=-1 ❌ 没找到
    -> APPEND { type:'tool', tool:{ id:call-A, name:read_file, input, completedAt } }
    （兜底建块，缺 startedAt）
重连后收到 item.delta("Done"):
  onItemDelta: last block is tool -> push 新 text 块 { text:"Done" }
重连后收到 session.ended:
  onSessionEnded: 收尾

最终 web messages:
[userMsg, assistant{blocks:[text:"Let me check", tool{call-A, completedAt, input}, text:"Done"]}]
                                       ↑ 块在 completed 时兜底建出，无 startedAt，但内容完整 ✅
```

两种子情形最终都收敛到"无重复块、内容完整"的一致状态，差别仅在 toolA 块有没有 `startedAt`（影响"运行中"时长徽标，不影响正确性）。

### 4.5 页面刷新变体（createCPReconnectStream）

如果断连是整页刷新（不是 runTurn 级断流），web store 从零开始：

```
1. switchSession("sess-1")
   ├─ getWorkspaceMessages(ws-1, sess-1) 从 DB 拉历史
   │   注意：DB 里 tool_call 只在 item.completed 时落库（persist 插件 sse.ts:1023-1051）
   │   若 toolA 未 completed -> DB 无 toolA 行 -> 历史里没有 toolA 块
   ├─ getSession -> chat_status === 'agent' -> 触发 startReconnect()
2. startReconnect(): 复用刚拉的 messages（缺 toolA），把最后一条 assistant 标 isStreaming=true
3. createCPReconnectStream -> /cp-reconnect -> 接 live
4. 后续 item.completed(toolA) 到 -> onItemCompleted findIndex=-1 -> 兜底 append
```

> 拉通：runTurn 级重连靠"内存态保留 + at-most-once 不补播"自愈；页面刷新靠"DB 历史是权威 + onItemCompleted 兜底 append"自愈。两条路径殊途同归 -- 都不依赖 cp 补播 item.started，所以 onItemStarted 的无脑 append 在两条路径下都安全。

---

## 5. 关键不变量（不要破坏）

1. **cp 的 `ActiveSSEStream` 是火忘广播** -- `emitSSE` 只 enqueue 给当前 controllers，不缓存、不回放。破了这条（比如加事件缓存），at-most-once 语义变 at-least-once，onItemStarted 的无判重就会产生重复块。
2. **重连只接未来不补过去** -- `createReconnectSSEResponse` 只做 `controllers.add`，不做回放循环。破了这条（比如补播历史 item.started），必须同时给 onItemStarted 加判重。
3. **`item.started` 不落库** -- persist 插件明确注释 "Not persisted on its own; item.completed carries the final state"（`sse.ts:972-975`）。所以 DB 历史里只有 completed 态的 tool_call，页面刷新时 onItemStarted 的"漏块"靠 onItemCompleted 兜底。
4. **`onItemCompleted`（tool_call）必须幂等** -- findIndex 判重 + 不存在则 append。这是 item.started 丢失时的安全网，不能改成"找不到就丢弃"。
5. **web 的 `runTurn` 跨重连保留 plugin 状态** -- handlersToPlugin 的回调闭包跨重连不重建，store 不清空。破了这条（重连时重置 messages），子情形 A 的"已有块"会丢，退化成 B 但带重复风险。
6. **cp→agent 重连（recovery）的 replay 只落库、不重广播给已有客户端** -- `recoverOrphanedSessions` 走的是新 cp pod、新 ActiveSSEStream、还没 web controller；persist 侧靠 call_id 判重（`ON CONFLICT DO NOTHING`，`sse.ts:696`）幂等。若未来在同一 cp pod 内做 cp→agent 中途重连并 replay，须注意 broadcast 会重发 item.started 给已连 web 客户端 -- 这条路径目前不在主流程，改动时要同步给 onItemStarted 加判重。

## 6. 变更前的自检清单

- [ ] 改 `onItemStarted`：是否仍只对 tool_call 建块？若新增 kind 的 started 也要建块，确认该 kind 的事件不会被 cp replay（否则要加判重）。
- [ ] 给 `onItemStarted` 加判重？先确认是否破坏了"at-most-once"前提。若 cp 仍火忘、不补播，加判重是多余开销但无害；若 cp 改成补播，则判重必须加。
- [ ] 改 cp `createReconnectSSEResponse`：是否仍只 `controllers.add`？若引入历史回放，必须同步评估 web 侧 onItemStarted 的判重需求。
- [ ] 改 cp `emitSSE`：是否仍是一次性 `for (controllers)` enqueue？若加事件缓存/ack 机制，at-most-once 语义改变，web 侧 onItemStarted/onItemCompleted 的判重策略要重新对齐。
- [ ] 改 persist 插件 `item.started` 处理：是否仍不落库？若改成 started 也落库，页面刷新时 DB 历史会带 started 态 tool_call，onItemCompleted 的兜底 append 路径需复核。
- [ ] 加新的 reconnect 路径（如 cp→agent 同 pod 中途重连）：broadcast 会不会重发 item.started 给已连 web 客户端？若是，给 web onItemStarted 加 findIndex 判重或让 cp 在重连后过滤已广播事件。

---

## 7. 附录：onItemCompleted 逐行拆解

> 这是 `buildSSEHandlers` 里最长、最绕的一段。它处理 turn 内所有 item 的"完成态"，分四个 `if` 分支按 `kind` 路由。读不懂时先记住一句话：**所有分支都只改 `assistantId` 这一条 assistant 消息** -- turn 的流式产出（文本/工具/状态）全部累加到它身上，user 消息是另一条、不在这里动。

### 7.0 整体骨架

```ts
onItemCompleted: guard((item) => {
  if (item.kind === 'message' && item.role === 'assistant') { ... }   // ① 文本收尾
  if (item.kind === 'tool_call')                                  { ... }   // ② 工具调用完成
  if (item.kind === 'tool_result')                                { ... }   // ③ 工具结果回贴
  if (item.kind === 'status')                                     { ... }   // ④ 状态标签
})
```

- 四个 `if` 是**平级**不是 `else if`，但因为 `kind` 互斥，一次只会命中一个。
- `guard(...)` 包最外层：session 切换时 `switchVersion` 变，直接 return，防止旧流的缓冲事件污染新 session（§1 代码地图讲过）。
- 下面逐分支拆。每段代码用 `// ←N` 标行号，对应下方解释。

### 7.1 ① message + assistant：用权威文本替换流式累积

```ts
if (item.kind === 'message' && item.role === 'assistant') {              // ←1
  const fullText = (item.content || [])                                  // ←2
    .filter((p) => p.type === 'text' && p.text)                         // ←3
    .map((p) => p.text!)                                                // ←4
    .join('')                                                           // ←5
  if (fullText) {                                                       // ←6
    store.setState((s) => ({                                            // ←7
      messages: s.messages.map((m) => {
        if (m.id !== assistantId) return m                               // ←8
        const blocks = [...m.blocks]                                    // ←9
        let lastTextIdx = -1                                            // ←10
        for (let i = blocks.length - 1; i >= 0; i--) {                  // ←11
          if (blocks[i].type === 'text') {
            lastTextIdx = i
            break
          }
        }
        if (lastTextIdx >= 0 && blocks.length - 1 === lastTextIdx) {    // ←12
          blocks[lastTextIdx] = { type: 'text', text: fullText }        // ←13
        } else {
          blocks.push({ type: 'text', text: fullText })                // ←14
        }
        const newContent = blocks                                       // ←15
          .filter((b) => b.type === 'text')
          .map((b) => (b as { type: 'text'; text: string }).text)
          .join('')
        return { ...m, content: newContent, blocks }                    // ←16
      }),
    }))
  }
}
```

| 标号 | 干什么 | 为什么这么做 |
|---|---|---|
| ←1 | 只处理 assistant 的 message item | user message 不走 SSE item 流（它在 dispatch 前已落库）；只收 assistant 产出 |
| ←2-5 | 把 `item.content` 里所有 text part 拼成 `fullText` | 一条 message item 可能含多个 text part（agent 把回答拆几段）；`|| []` 防御 content 为空 |
| ←3 `&& p.text` | 过滤掉空文本 part | 防御性，空文本没意义不参与拼接 |
| ←6 `if (fullText)` | 空文本直接跳过 | 有的 turn 是纯工具调用，message item 的 completed 文本为空，不能 push 空块污染 UI |
| ←7 `store.setState((s) => ...)` | 用函数式更新 | 读最新 state，避免闭包陈旧 |
| ←8 `if (m.id !== assistantId) return m` | 只改本 turn 的 assistant 消息 | 其他消息（user、历史 assistant）保持引用相等，让 React.memo 短路（§7.3 详讲） |
| ←9 `[...m.blocks]` | 浅拷贝 blocks 数组 | 下面要改下标，不能 mutate 原数组（zustand 不可变更新） |
| ←10-11 从后往前找最后一个 text 块 | 找 `lastTextIdx` | 从后找是因为"当前正在流式累积的文本块"一定在尾部；线性扫一遍即可 |
| ←12 `lastTextIdx >= 0 && blocks.length - 1 === lastTextIdx` | **核心判断**：存在 text 块 且 它就是最后一块 | 这是"替换 vs 新增"的分水岭，见下方专门解释 |
| ←13 替换 | 把尾部 text 块整段换成 `fullText` | delta 累积的文本可能有拼接毛刺/不完整；completed 是 agent 给的**权威最终文本**，必须替换不是追加 |
| ←14 push 新 text 块 | 尾部不是 text（是 tool/status）时新增 | 说明这个 message item 的文本是新的一段，不能覆盖前面的工具块 |
| ←15 `newContent` | 把所有 text 块重新拼一遍 | `m.content` 是"消息全文本"的快照视图（用于搜索/预览/侧边栏），要保持与 blocks 一致；每次都重算最稳 |
| ←16 返回新消息对象 | immutable 更新 | 触发 zustand 通知订阅者重渲染 |

**←12 是这段最绕的一行，单独讲。** 一个 turn 可以有**多个** message item，顺序大致是：

```
[text "我先查一下" ] -> [tool_call A] -> [tool_result A] -> [text "结果是..."]
```

每个 message item 的 completed 事件只该动**属于自己**的那个 text 块：

- 如果尾部块是 text（`lastTextIdx === length-1`）：它就是当前这个 message item 在 delta 阶段建出来的流式缓冲块，用权威文本替换。
- 如果尾部块不是 text（`lastTextIdx !== length-1`，比如尾部是 tool）：说明当前 message item 还没建过 text 块（delta 可能没来或为空），completed 时补一个 push。

绝不替换"前面的 text 块" -- 那是上一个 message item 的产物，不属于这次。

### 7.2 ② tool_call：findIndex 判重，找到 update / 找不到 append

这就是 §3 讲的"幂等兜底"。`onItemStarted` 无脑 append 享受 at-most-once 红利，这里承担"万一 started 丢了，completed 兜底建块"的责任。

```ts
if (item.kind === 'tool_call') {                                        // ←1
  const tc = item.content?.[0]                                          // ←2
  if (tc?.type === 'tool_call') {
    let input: Record<string, unknown> = {}                             // ←3
    try {
      input = JSON.parse(tc.arguments || '{}')                         // ←4
    } catch {}
    store.setState((s) => ({
      messages: s.messages.map((m) => {
        if (m.id !== assistantId) return m                              // ←5
        const idx = m.blocks.findIndex(                                // ←6
          (b) => b.type === 'tool' && b.tool.id === tc.call_id)
        if (idx >= 0) {                                                 // ←7 命中 -> update
          const blocks = [...m.blocks]
          const b = blocks[idx] as ContentBlock & { type: 'tool' }
          blocks[idx] = {
            type: 'tool',
            tool: { ...b.tool, input, completedAt: Date.now() },       // ←8
          }
          return { ...m, blocks }
        }
        return {                                                       // ←9 没命中 -> append
          ...m,
          blocks: [
            ...m.blocks,
            {
              type: 'tool' as const,
              tool: {
                id: tc.call_id!,                                       // ←10
                name: tc.name ?? transcriptI18n.t(                      // ←11
                  'components.chat.toolRenderers.labels.unknown'),
                input,
                completedAt: Date.now(),                              // ←12 只有 completedAt
                parentToolUseId: item.parent_tool_use_id ?? null,     // ←13
              },
            },
          ],
        }
      }),
    }))
  }
}
```

| 标号 | 干什么 | 为什么这么做 |
|---|---|---|
| ←1 | 只处理 tool_call 的 completed | 区分于 tool_result（③）和 status（④） |
| ←2 `item.content?.[0]` | 取第一个 content part | tool_call item 的 content 只有一个 part（call 描述）；`?.` 防御 content 缺失 |
| ←3-4 防御性 parse args | 解出 input 对象 | args 是 JSON 字符串；bad args 落回 `{}`（和 onItemStarted 一致） |
| ←5 | 只改 assistantId 消息 | 同 ① |
| ←6 findIndex by `b.tool.id === tc.call_id` | **判重核心** | call_id 是工具调用的唯一标识；找到说明 onItemStarted 已建过块 |
| ←7 命中分支（update） | 已有块 -> 只补字段 | 块已存在（startedAt/name 都在），不能重建 |
| ←8 `{ ...b.tool, input, completedAt }` | 用展开保留旧字段，只覆盖 input + completedAt | **关键**：`...b.tool` 保留了 `startedAt`（onItemStarted 建的）、`name`、`parentToolUseId`、`result`（如果 result 先到了）；只把 input 用 completed 的权威值覆盖一次、加上 completedAt |
| ←9 没命中分支（append） | 块不存在 -> 新建 | started 丢了（§3.4 的子情形 B），completed 兜底建块 |
| ←10 `id: tc.call_id!` | 用 call_id 作 tool 块 id | 后续 tool_result 靠这个 id 匹配（③） |
| ←11 `name ?? unknown` | name 缺失兜底 | 渲染器需要 name 选 renderer，不能 undefined |
| ←12 `completedAt` 但**没有 startedAt** | 只记完成时间 | started 事件丢了，无法重建 startedAt；UI 上"运行时长"徽标会缺开始时间，但内容完整（§3.4 说过这是可接受代价） |
| ←13 `parentToolUseId` | 记录父工具调用 | 用于嵌套工具调用（如 call_agent 里子工具），渲染时能挂到父块下 |

> 闭环：**update 路径不覆盖 startedAt，append 路径不补 startedAt** -- 两条路径合起来保证"startedAt 只能来自 onItemStarted"，体现 at-most-once 语义的诚实性。

### 7.3 ③ tool_result：按 call_id 贴结果 + 派发插件

tool_result 是某次 tool_call 的**产出**，必须贴回对应的 tool 块；同时触发插件结果处理器。

```ts
if (item.kind === 'tool_result') {                                      // ←1
  const tr = item.content?.[0]
  if (tr?.type === 'tool_result') {
    let matchedTool: ToolCall | undefined                               // ←2
    store.setState((s) => {
      const messages = s.messages.map((m) => {
        const idx = m.blocks.findIndex(                                // ←3
          (b) => b.type === 'tool' && b.tool.id === tr.call_id)
        if (idx < 0) return m                                          // ←4 没命中 -> 原样返回
        const blocks = m.blocks.slice()
        const prev = blocks[idx] as ContentBlock & { type: 'tool' }
        blocks[idx] = {                                                // ←5
          ...prev,
          tool: {
            ...prev.tool,                                              // ←6
            result: tr.output,                                        // ←7
            isError: tr.is_error,
            resultAt: Date.now(),
          },
        }
        matchedTool = prev.tool                                       // ←8
        return { ...m, blocks }
      })
      return { messages }
    })
    if (matchedTool) {                                                 // ←9
      toolDispatcher.dispatch({                                        // ←10
        toolName: matchedTool.name,
        toolInput: matchedTool.input,
        toolOutput: tr.output ?? '',
        isError: !!tr.is_error,
        callId: tr.call_id ?? '',
        workspaceId,
      })
    }
  }
}
```

| 标号 | 干什么 | 为什么这么做 |
|---|---|---|
| ←1 | 只处理 tool_result | 区分于 tool_call（②） |
| ←2 `matchedTool` 在闭包外声明 | setState 是异步的，但闭包能捕获 | dispatch 要在 setState 之后做，靠这个变量把"匹配到的 tool"带出来 |
| ←3 findIndex by `b.tool.id === tr.call_id` | 按 call_id 找对应 tool 块 | tool_result 的 `call_id` 与 tool_call 的 `call_id` 一致，是匹配键 |
| ←4 `if (idx < 0) return m` | **没命中就原样返回 m** | 这是 React.memo 优化的关键（见下方专讲），且**不 append 兜底** -- tool_result 没有匹配块时静默丢弃（极端：started/completed 都丢了但 result 到了；DB 里有，刷新可恢复） |
| ←5 `{ ...prev, tool: {...} }` | 浅拷贝 block 再改 tool | 不可变更新；外层 `...prev` 保留 block 的其他属性 |
| ←6 `{ ...prev.tool, result, isError, resultAt }` | 展开保留旧字段 | **关键**：保留 `startedAt`/`completedAt`/`input`/`name`/`parentToolUseId`，只加 result 三件套；不能把已完成的工具"退回"到无 completedAt 状态 |
| ←7 `result / isError / resultAt` | 贴上产出 | isError 让 UI 渲染红色错误态；resultAt 算"产出时间"徽标 |
| ←8 `matchedTool = prev.tool` | **在更新前**捕获旧 tool | dispatch 需要 name + input（input 在这里取）；如果取更新后的 tool 也行，但取 prev 语义更清晰"派发的是这次调用本身" |
| ←9 `if (matchedTool)` | 只在真匹配到时派发 | 没匹配到（result 被丢）就不派发，避免插件收到孤儿事件 |
| ←10 `toolDispatcher.dispatch` 在 setState **外面** | 副作用与状态分离 | dispatch 触发插件 UI（如 code review 插件收到测试结果刷新），是副作用不是状态；放在 setState 外避免渲染阻塞 |

**←4 的 React.memo 优化单独讲。** 注意这一行注释（源码 556-557）：

> Only the message containing the matching tool block needs a new ref; other messages keep reference equality so React.memo can short-circuit.

`s.messages.map((m) => ...)` 会遍历**所有**消息。对于没命中 tool 块的消息，`return m` 返回**同一个对象引用**。这样 React.memo 包裹的消息组件看到 `prevProps === nextProps`，直接短路不重渲染。只有真正被改的那条消息换了新 ref。如果这里写成 `return { ...m }`（哪怕没改也复制一份），所有消息都会换 ref，React.memo 全部失效，长会话会卡。

> 颗粒度：这是"不可变更新 + 引用相等"的经典套路 -- **改谁就克隆谁，没改的保持原引用**。② 和 ① 里 `if (m.id !== assistantId) return m` 也是同一个套路。

### 7.4 ④ status：无脑 append 状态标签

```ts
if (item.kind === 'status') {                                           // ←1
  const part = item.content?.[0]
  if (part?.type === 'status') {
    store.setState((s) => ({
      messages: s.messages.map((m) =>
        m.id === assistantId                                            // ←2
          ? {
              ...m,
              blocks: [
                ...m.blocks,                                           // ←3
                {
                  type: 'status' as const,
                  label: part.label!,                                  // ←4
                  detail: part.detail,                                 // ←5
                  isError: item.status === 'failed',                  // ←6
                },
              ],
            }
          : m,
      ),
    }))
  }
}
```

| 标号 | 干什么 | 为什么这么做 |
|---|---|---|
| ←1 | 只处理 status item | status 是一次性标签事件（"思考中"/"运行工具 X"/"失败"） |
| ←2 `m.id === assistantId` | 用三元而非 if-return | 这段没 findIndex 逻辑，三元更紧凑；其他消息走 `: m` 保持引用相等 |
| ←3 `[...m.blocks, {...}]` | **无脑 append** | status 每条都是独立事件，不更新已有块（不像 tool_call 有 call_id 可配对）；不需要判重 |
| ←4 `label!` | 状态标签文案 | `!` 断言非空（前面 if 已保证 type 是 status，label 必有） |
| ←5 `detail` | 可选详情 | 比 label 更细的说明，可为 undefined |
| ←6 `isError: item.status === 'failed'` | 从 item 顶层 status 推 isError | status item 自带 `status: 'in_progress'|'completed'|'failed'`；failed 时 UI 渲染红色错误态 |

> 拉通：status 是四个分支里最简单的 -- 没有 delta、没有 findIndex、没有 dispatch，纯 append。它和 tool_call 的"无脑 append"（onItemStarted，§3）同源：因为 status 也是 at-most-once 投递（cp 不补播），所以可以不判重。

---

### 7.5 四个分支的对比一图流

| 分支 | 处理对象 | 判重？ | 落点 | 副作用 |
|---|---|---|---|---|
| ① message | 流式文本块 | findLastTextIdx + 位置判断 | 替换尾部 text / push 新 text | 无 |
| ② tool_call | tool 块 | findIndex by call_id | update（保留 startedAt）/ append（无 startedAt 兜底） | 无 |
| ③ tool_result | tool 块 | findIndex by call_id | update（贴 result 三件套），没命中丢弃 | `toolDispatcher.dispatch`（插件 UI） |
| ④ status | status 块 | 不判重 | 无脑 append | 无 |

> 闭环：**① 用"位置"判重（trailing text），②③ 用 call_id 判重，④ 不判重**。判重策略各异，但底层都靠 cp 的 at-most-once 投递兜底 -- 只要事件不重发，判重只是防"completed 兜底建块"和"started 错过"的边界情况，不是防广播重复。理解了这条，四段代码就不绕了。

---

## 8. 附录：message 的 id 是怎么生成、怎么传递的

> 前面七节反复出现"handler 靠 `assistantId` 闭包路由更新"和"call_id 判重"，但没说清 `assistantId` 本身从哪来、跟后端有没有关系。这节专门拆"一条 message 的 id 在前后端怎么流动"。读完你会明白一件事：**前端直播时用的 id 和 DB 里的 id 是两套不同的东西，页面刷新后会被悄悄换掉，而 SSE 链路从头到尾不依赖前端的 id。**

### 8.1 三套 id，各管各的

一条 user/assistant message 在系统里同时背着三个 id，分属三个不同的关注面：

| id | 谁生成 | 长啥样 | 活在哪 | 用途 |
|---|---|---|---|---|
| **前端 ChatMessage.id** | 浏览器 `genId()` | `msg-1720961234567-3` | 仅前端 store / React key | 本地定位"要更新哪条消息"；**从不发往后端** |
| **DB message.id** | cp `generateId()` | `a3f9xq2k`（8 字符 base36） | `messages` 表主键 | 持久化主键；历史回放时回传前端；events/blocks 通过 `message_id` 外键挂上去 |
| **agent item_id** | agent 运行时（env-runner） | 随实现而异 | SSE 事件 `evt.item.item_id` / `evt.item_id` | agent 侧 per-item 标识；**cp 透传不消费，web 也基本不用** |

> 抓手：先分清"谁是本地临时 id（前端 genId）、谁是持久 id（DB generateId）、谁是过路 id（agent item_id）"。三者从不混用 -- SSE 广播的 `item` 带的是 agent item_id，DB 落库用的是 message.id，前端直播路由用的是 genId 的 assistantId。**没有任何一层把这三个 id 互相翻译。**

### 8.2 前端 id：`genId()` 与"闭包路由"

生成点（`web/src/stores/agent-session-store.ts:181-183`）：

```ts
function genId(): string {
  return `msg-${Date.now()}-${++_id}`
}
```

`Date.now()` 毫秒时间戳 + 模块级自增序号 `_id`，保证同一页面内不重复。**纯客户端、无后端参与、不保证跨标签页/跨刷新唯一**。

两个生成时机：

| 时机 | 代码位置 | 生成什么 |
|---|---|---|
| `sendMessage` | `agent-session-store.ts:751,757` | user 消息 id + assistant 占位 id（各调一次 `genId()`） |
| `startReconnect`（重连且没有 assistant 尾消息时） | `agent-session-store.ts:222` | 新建 assistant 占位的 id |

> 关键：`sendMessage` 把 `assistantId` 传进 `buildSSEHandlers(assistantId, ...)`（`:778`），**handler 闭包捕获它**。后续所有 `onItemStarted`/`onItemDelta`/`onItemCompleted` 都靠 `m.id === assistantId` 在 `messages` 数组里找目标消息 -- 这就是"闭包路由"。

**重连时的复用逻辑**（`startReconnect`，`:210-237`）：若最后一条消息是 assistant（`last?.role === 'assistant'`），直接 `assistantId = last.id`（复用现有 id），把 `isStreaming` 翻回 true；否则 `genId()` 新建一条占位。这意味着 **`runTurn` 级重连复用同一个 assistantId**，handlers 闭包重绑后仍能定位同一条消息 -- 这是 §5 不变量 5 成立的前提。

#### "闭包路由"长什么样：`.map` + `m.id === assistantId`

上面反复说"靠 `m.id === assistantId` 找目标消息"，具体写法（以 `onSessionEnded` 翻 `isStreaming` 为例，`agent-session-store.ts:378-384`）：

```ts
store.setState((s) => ({
  // …其他字段…
  messages: s.messages.map((m) =>
    m.id === assistantId ? { ...m, isStreaming: false } : m,
  ),
}))
```

逐层拆：

1. **`s.messages` 是整个会话的所有消息**，不止本轮这条。典型形态：`[userMsg, assistantMsg_上轮, userMsg, assistantMsg_本轮]`。`.map` 遍历每一条。
2. **`assistantId` 是闭包变量**（不是这里现取的）。链路：`sendMessage` 调 `genId()` 拿到 `assistantId` -> 传给 `buildSSEHandlers(assistantId, …)` -> 四个 handler 闭包全部捕获同一个 id。所以 `onSessionEnded` 里能直接用。
3. **三元分两种返回**：

| 命中条件 | 返回 | 为什么 |
|---|---|---|
| `m.id === assistantId`（本轮 assistant 那条） | `{ ...m, isStreaming: false }` -- **新对象** | 展开旧字段 + 把 streaming 关掉 |
| 其他所有消息（user 消息、历史轮 assistant） | `m` -- **原对象引用** | 不是本轮目标，碰都不碰 |

语义一句话：**"在所有消息里，只把本轮 assistant 那条的 `isStreaming` 关掉，其余原样。"**

> 为什么不直接 `messages[idx].isStreaming = false`？三个原因：
> - **不可变更新** -- zustand/React 靠引用变化感知重渲染，直接 mutation 不触发更新，必须返回新对象。
> - **不知道 index** -- handler 闭包里只有 `assistantId`（id），不知道它在数组第几位；`.map` + 比较是"按 id 找"的自然写法。
> - **没命中返回 `m` 而非 `{...m}`** -- 这是 **React.memo 优化**：没改的消息保持旧引用，包了 memo 的消息组件 `prevProps === nextProps` 浅比较短路不重渲染；若写成 `{...m}` 每条都复制，所有消息换新引用，memo 全失效，长会话卡顿。

> 拉通：这条比较是**纯前端对前端**的比对 -- `assistantId` 是 `genId()` 生的 `msg-...`，后端不感知（§8.3），所以不存在"后端还没回传 id"的时序问题。`onItemStarted`/`onItemDelta`/`onItemCompleted` 里的 `m.id !== assistantId ? return m` 守卫、`onItemCompleted` 四个分支里反复出现的 `m.id === assistantId` 都是同一个套路（§7）。理解了这条，§7 的所有 `.map` 就不绕了。

#### FAQ：为什么不直接取 `messages` 的最后一条？

直觉疑问：`sendMessage` 明明把 assistant 占位 append 到末尾（`:770`），`startReconnect` 也会 append（`:226-236`），那"最后一条 = 本轮 assistant"不是总成立吗？干嘛还 `.map` + id 比较，不直接 `messages[messages.length-1]`？

**短答：这个"最后一条 = assistant"是 `sendMessage`/`startReconnect` 靠 append 主动维护的约定，不是结构不变量。有一个真实窗口期里它不成立 -- draining（带排队 follow-up 的 turn 结束）的 history reload。用 id 比较把 handler 和目标消息从"位置耦合"解耦成"身份绑定"，扛住数组整体替换/尾部插入/未来变更。**

**反例：draining 的 history reload 窗口**

场景：用户在 turn 1 进行中又输入了"接着帮我补充"（`isLoading=true` 时输入，走 `updatePendingMessage` 排队，`:739-742`）。turn 1 正常结束后 cp 把排队的消息 drain 成 turn 2。时序如下：

```
时间轴 ─────────────────────────────────────────────────────────────────►

[turn 1 进行中]
  messages = [ userMsg1(msg-1), assistantMsg1(msg-2, isStreaming:true) ]
                                                       ↑ 最后一条 = assistant_1 ✓

onSessionEnded（reason='completed', draining=true）            agent-session-store.ts:373
  ├─ const queued = pendingMessage  // "接着帮我补充"
  ├─ setState: messages.map(m =>
  │     m.id === assistantId_1(msg-2) ? {...m, isStreaming:false} : m
  │  )
  │  └─ 只翻 msg-2 的 isStreaming。此时 last 仍是 assistant_1，"取最后一条"勉强对
  ├─ isLoading=draining=true, isBusy=draining=true   // 保持 busy 等 drain
  └─ void followPendingTurn("接着帮我补充")            :403
        │
        ▼ 轮询 getSession，直到 chat_status==='agent'（cp 已 drain 成 turn 2）
        │
        │  ⚠️ 这中间 cp 已经把 drained 的 user 消息落库了：
        │     DB messages 表 = [..., assistant_1(完成), drainedUserMsg]
        │
        ├─ line 288-290: history = getWorkspaceMessages()
        │                store.setState({ messages: history.map(toChatMessage) })
        │                ↑ 整个数组被 DB 历史替换！
        │
        │   messages = [..., assistantMsg1(DB-id="a3f9xq2k", isStreaming:false),
        │                drainedUserMsg(DB-id="b7g2wp9n", role:"user")]
        │                                                  ↑ 最后一条 = USER 消息 ✗
        │                                                    "取最后一条"会拿到 user 消息！
        │
        └─ line 294: startReconnect()
              ├─ last = drainedUserMsg（role: 'user'）
              ├─ last?.role === 'assistant'?  NO
              ├─ assistantId_2 = genId()  // 新建 msg-T2-1           :222
              └─ setState: messages.push(新 assistant 占位)           :226-236
                 messages = [..., assistantMsg1, drainedUserMsg,
                             assistantMsg2(msg-T2-1, isStreaming:true)]
                                                        ↑ 最后一条 = assistant_2 ✓
                                                           约定被 startReconnect 重新建立

[turn 2 进行中]  handlers_2 绑 assistantId_2 = msg-T2-1
  onItemDelta -> append 文本到 msg-T2-1
```

**窗口期在哪：** line 290 的 `setState` 到 line 294 的 `startReconnect` 之间，`messages` 末尾是 **drained 的 user 消息**（role: 'user'），不是 assistant。今天这个窗口里没有 handler 在跑（turn 1 的 `onSessionEnded` 已返回，turn 2 的 `handlers_2` 还没建），所以不爆。但足以证明 **"最后一条 = assistant" 不是不变量，是 `startReconnect` 主动 append 才修好的瞬时状态**。

**反例的推论 -- 如果改用 `messages[last]` 会怎样：**

| 时刻 | id 比较（现状） | 取最后一条（假设） |
|---|---|---|
| turn 1 onSessionEnded | 翻 `msg-2` 的 isStreaming ✓ | 翻 `msg-2`（最后一条）✓ 勉强对 |
| draining history reload 后 | 旧 handler 已被 `guard()` 的 `switchVersion` 挡掉，不跑 | 若有 handler 漏网，会翻 drainedUserMsg（role:user，没 isStreaming 字段）✗ |
| 未来：谁往尾部插一条 status/error 消息 | id 比较不受影响 ✓ | 静默改错对象 ✗ |

**更深一层：handler 创建 vs 数组读取是两个时刻**

`assistantId` 在 `buildSSEHandlers(assistantId, …)`（`:778`）被闭包**钉死**。`.map((m) => m.id === assistantId …)` 里的 `s.messages` 是**执行时**读的快照。中间数组可能被：

- `loadHistory` / `followPendingTurn` 的 `history.map(toChatMessage)` **整体替换**（`:290`、`:903`）-- genId 占位被 DB 消息替换，位置全变
- 下一个 `sendMessage` 追加（`isLoading` 守卫挡住主路径，但 draining 不走 sendMessage）
- `clearMessages` 清空（`:909`）

id 是 handler 在创建时钉死的身份句柄，不受这些数组变更影响。"最后一条"是读取瞬间的位置属性，数组一变就漂移。**用 id 比较把 handler 和目标消息从"位置耦合"解耦成"身份绑定"** -- 这是 `m.id === assistantId` 比取最后一条的根本理由。

> 闭环：**"最后一条 = assistant"靠 append 约定维护，draining 的 history reload 会暂时打破它。id 比较是身份绑定，扛住数组替换/尾部插入/未来变更；取最后一条是位置耦合，约定一破就静默改错对象。** 四个 handler 统一用 `m.id === assistantId`，不靠"谁是最后"，这就是 §7 所有 `.map` 不绕的根。

### 8.3 POST 请求体：前端 id 从不上车

`createAgentChat` 发出的 POST（`web/src/lib/api/sse.ts:251-256`）：

```ts
body: JSON.stringify({
  message,          // 文本内容
  session_id: sessionId ?? null,  // 会话 id（恢复时传，新建时 null）
  source: 'web',
  ...(images?.length ? { images } : {}),
})
```

**没有 user 消息 id、没有 assistant 消息 id。** 后端 cp 收到的只有"文本 + session_id（可能为空）+ 图片"。这意味着 cp 落库时用的 message.id 是**自己生成**的，跟前端的 `msg-...` 毫无关系。

### 8.4 DB id：cp 懒生成，两路写入

生成点（`control-plane/src/services/db/pool.ts:78-80`）：

```ts
export function generateId(): string {
  return Math.random().toString(36).substring(2, 10)  // 8 字符 base36
}
```

写入入口是 `addMessage()`（`services/db/messages.ts:4-29`），返回带 `msg.id` 的 `Message` 行。**cp 的 persist 插件按"事件类型"分两路调它**：

| 路径 | 触发点 | 角色 | 代码 |
|---|---|---|---|
| user 消息 | `enqueueUserMessage` | 收到首个 assistant text 前插队落库 user 消息（延迟到首响应，确保 user 先于 assistant 入库） | `sse.ts:828-848`，调 `addMessage(..., 'user', text)` |
| assistant 消息（懒） | `enqueueEvent` / `enqueueToolResultUpsert` | 首次落事件时才 `addMessage(..., 'assistant', textContent)`，拿到的 `msg.id` 缓存进闭包 `assistantMessageId`，后续事件复用 | `sse.ts:736-737` / `:759-760` |

> 颗粒度：assistant 消息行是"第一个事件触发时才建"，不是 turn 开始就建。`assistantMessageId` 闭包变量是懒初始化标志，避免为空 turn 创建空行。

**恢复（resume）场景的差异**（`executeChat.ts:155-159`）：若前端传了 `session_id`（恢复已有会话），cp 在 agent 跑起来之前就先把 user 消息落库（`addMessage(..., 'user', ...)` + `insertUserMessageBlocks`），不等首响应。新建场景则等 `session.started` 才有 session_id，user 消息走 §8.4 的 `enqueueUserMessage` 延迟路径。

### 8.5 agent item_id：cp 透传，web 基本忽略

事件 schema（`control-plane/src/openapi/events.schema.ts:40-52`）：

```ts
const UniversalItemSchema = z.object({
  item_id: z.string(),   // ← agent 生成，cp 不改
  kind: z.enum(['message', 'tool_call', 'tool_result', 'status']),
  role: z.enum(['user', 'assistant', 'tool']).nullable(),
  status: z.enum(['in_progress', 'completed', 'failed']),
  content: z.array(ContentPartSchema),
  parent_tool_use_id: z.string().nullable().optional(),
})
```

`item_id` 由 agent 运行时生成，cp 的 `emitSSE` 原样转发给 web（cp 只截流做 persist，不修改 item）。三种事件携带方式：

| 事件 | 携带位置 | web 怎么收 |
|---|---|---|
| `item.started` | `evt.item.item_id` | `onItemStarted?.(evt.item)`（`sse.ts:91`） |
| `item.delta` | `evt.item_id`（顶层字段，不在 item 里） | `onItemDelta?.(evt.item_id, evt.delta)`（`sse.ts:94-95`） |
| `item.completed` | `evt.item.item_id` | `onItemCompleted?.(evt.item)`（`sse.ts:99`） |

**web 怎么用 item_id？几乎不用。** 看 `onItemDelta`（`agent-session-store.ts:452`）：

```ts
onItemDelta: guard((_itemId, delta) => {   // ← 前缀 _ 表示未用
  if (delta.type === 'text') {
    // 靠 assistantId 闭包找消息，append 到最后一个 text block
  }
})
```

为什么能忽略？**因为单个 turn 内只有一条 streaming assistant 消息**，所有 delta 都该贴到 `assistantId` 这条上。item_id 本是用来区分"多个并发 streaming item"的（比如 agent 同时产两条 message），但本平台的 turn 模型保证一次只流一条，所以 assistantId 闭包路由就够了。`onItemStarted`/`onItemCompleted` 同理 -- 全部 `m.id === assistantId` 定位，不看 `item.item_id`。

> 工具块的"判重键"是 `call_id`（tool_call.content[0].call_id），不是 item_id -- 这是另一套命名空间，专门配对 tool_call ↔ tool_result，见 §7.2/§7.3。

### 8.6 session_id：另一条线，不属于 message id

别把 session_id 跟 message id 混。session_id 标识"一次会话"（一个 turn 序列），message.id 标识"会话里的一条消息"。

| 场景 | session_id 谁生成 | 怎么到 web |
|---|---|---|
| 新建会话 | **agent 运行时生成**，在 `session.started` 事件里带出 | cp 截流 → `onSessionStarted(sid)` → 前端存进 `activeSessionId`（`executeChat.ts:95-98`：cp 用 NULL session_id 铸 token，等 `session.started` 才绑定） |
| 恢复会话 | 前端传已有的 session_id（POST body `session_id`） | `createAgentChat` 把它放进 body（`sse.ts:253`） |

> 对齐：session_id 的"生成权"在 agent，不在 cp 也不在 web。web 只在恢复时把旧 id 透传回去。这条线跟 message.id 完全平行 -- 改 message.id 的生成逻辑不会影响 session_id，反之亦然。

### 8.7 页面刷新：id 被悄悄换掉

这是全篇最容易踩坑的点。**同一条 assistant 消息，直播时和刷新后 id 不同**：

```
直播时：assistantMessage.id = "msg-1720961234567-3"   ← genId()
刷新后：assistantMessage.id = "a3f9xq2k"               ← DB message.id
```

刷新走 `loadHistory` → `toChatMessage`（`internal/ui-sdk/src/to-chat-message.ts:48-54`）：

```ts
return {
  id: String(message.id),   // ← DB 的 message.id，覆盖掉 genId 的值
  role: message.role,
  content: message.content,
  blocks,
  created_at: message.created_at,
}
```

**为什么安全？** 因为前端 id 只用于：
1. React key（diff 用）-- 换 id 等于换 key，整个子树重挂，但刷新本来就从零渲染，无 diff 损失
2. `m.id === assistantId` 闭包路由 -- 刷新后 handlers 绑的是新 `assistantId`（从 `startReconnect` 或新一轮 `sendMessage`），跟 DB id 一致

**不安全的情况**：如果哪个组件把 `message.id` 存进外部状态（比如 localStorage 草稿、URL hash），刷新后 id 变了会对不上。当前代码里 `clearDraftFor(workspaceId, sid)` 用的是 session_id 不是 message.id，所以没事 -- 但**新增功能时如果要把 message.id 持久化出去，必须改用 DB id 而非 genId 的值**，否则刷新即失配。

### 8.8 端到端时序：一次 sendMessage 的 id 流动

```
web sendMessage("hi")
  ├─ genId() → userMessage.id = "msg-T1-1"
  ├─ genId() → assistantId  = "msg-T1-2"
  ├─ setState: messages.push(userMessage, assistantMessage)
  └─ POST /chat { message:"hi", session_id:null }     ← 不带任何 message.id
        │
        ▼
cp executeChat
  ├─ sessionId=null → 铸 token 带 NULL session_id
  ├─ runTurn → agent...
  │     ├─ session.started { session_id:"sess-abc" }    ← agent 生成 session_id
  │     │     ├─ cp persist: createSession(ws, "sess-abc", ...)
  │     │     └─ cp broadcast → web onSessionStarted("sess-abc")
  │     │           └─ web: activeSessionId = "sess-abc"
  │     ├─ item.started { item:{ item_id:"i1", kind:"tool_call", ... } }
  │     │     ├─ cp persist: 不落库（仅记 toolCallStartedAt）
  │     │     └─ cp broadcast → web onItemStarted(item)
  │     │           └─ web: 在 assistantId="msg-T1-2" 这条上 append tool block
  │     └─ item.completed { item:{ item_id:"i1", kind:"message", role:"assistant", ... } }
  │           ├─ cp persist: addMessage(ws,"sess-abc","assistant",text)
  │           │             → msg.id="a3f9xq2k"  ← DB generateId()，web 不知道
  │           │             insertEvent({ messageId:"a3f9xq2k", ... })
  │           └─ cp broadcast → web onItemCompleted(item)
  │                 └─ web: 在 assistantId="msg-T1-2" 这条上替换权威文本
        │
        ▼ （用户刷新页面）
web loadHistory → getWorkspaceMessages → GET /messages?session_id=sess-abc
  └─ 返回 [{ id:"a3f9xq2k", role:"assistant", content:"hi 回答", blocks:[...] }, ...]
        └─ toChatMessage: ChatMessage.id = String("a3f9xq2k") = "a3f9xq2k"
           ← 从这一刻起，这条消息的 id 从 "msg-T1-2" 变成 "a3f9xq2k"
```

### 8.9 不变量补充

在前述 §5 不变量之外，id 流动还有两条隐含约束：

7. **前端 `genId()` 的 id 永远不上车 POST** -- 后端从不感知 `msg-...`。若哪天把 message.id 加进请求体想让 cp 复用，会破坏 cp 的懒生成 + DB 主键语义（cp 的 `addMessage` 自己 `generateId`，不接受外部指定 id）。
8. **DB message.id 与 agent item_id 永远不互相翻译** -- cp persist 插件用 `messageId`（DB）+ `ordinal`/`callId` 组织 events 表，不用 `item_id` 作关联键；web 也不用 `item_id` 做 React key。若要让 web 按 item_id 定位（比如支持并发多 streaming message），需要新引入一张 item_id → messageId 的映射，目前没有这个需求。

> 闭环：**前端 id 管本地定位（genId，闭包路由），DB id 管持久化（generateId，主键 + 历史回放），agent item_id 管事件流（透传，web 基本不用）。** 三套 id 各司其职、互不串台，SSE 链路靠"闭包捕获 assistantId"把前端 id 钉在 handler 里，不依赖后端回传。页面刷新是唯一的 id 切换点（genId → DB id），由 `toChatMessage` 静默完成，因为 id 只作本地定位用，换了不影响正确性。


---

### FAQ：cp->web 断连后重连，turn2 的事件会不会串进 turn1 的 msg1？

**问**：前端消费和 agent 产出是经 cp 中转的两条独立流（`runTurn` 的 `SerialQueue.run` 不因 DB 写失败或前端发送失败而停）。有没有可能：cp->web 断连时是 msg_1 的 turn1，前端重连时 turn1 已结束、cp 已自动 drain 进 turn2（`sse.ts:556`），导致 turn2 的 SSE event 被更新进 turn1 的 msg1？

**答：会。这是一个真实的竞态串台窗口。** 根源是 drain 复用同一个 `session_id`（`executeChat.ts:236-242`），而 `runTurn` 级重连复用同一份 handlers（`assistantId_1` 闭包），两者叠加让 turn2 的事件被路由进 msg1。下面拆机制、时序、竞态窗口和修复方向。

#### 两条流确实独立

| 流 | 谁推 | 失败会停吗 | 证据 |
|---|---|---|---|
| agent -> cp | agent 运行时产 SSE，cp 的 intercept 插件 tee 出来 | 不停。`runTurn` 的 persist 走 `SerialQueue.run`，queue 只串行化不中断；DB 写失败只 log 不抛 | `sse.ts` persist 插件 + `SerialQueue` |
| cp -> web | cp 的 `emitSSE` 给 `activeStream.controllers` enqueue | 不停。`emitSSE` 是火忘广播，单个 controller 投递失败不影响 queue | `sse.ts:217` `emitSSE` |

**所以你的理解对：agent->cp 和 cp->web 是两条独立流。** agent 持续产 event，cp 持续落库 + 广播，web 掉了 cp 不停。这是 at-most-once 投递的根源（§3.1），也是本问串台的前提 -- cp 不等 web。

#### 串台机制：drain 复用 session_id + 重连复用 handlers

三个事实叠出串台：

1. **drain 复用 session_id**（`executeChat.ts:236-242`）：`drainPendingMessage` 调 `executeChat({sessionId, ...})` 把同一个 `sessionId` 传进去。turn1/turn2 共享 session_id。
2. **turn2 启动时 `setupActiveStream` 驱逐 turn1 的流**（`sse.ts:151-179`）：新 turn 调 `setupActiveStream(workspaceId, sessionId)`，按 `streamKey = workspaceId:sessionId` 找到 turn1 的 `ActiveSSEStream`，**关闭它所有 controllers、标记 `done=true`、从 `activeStreams` 删除**，再建 turn2 的新 stream 挂进同一个 key。所以 turn1 结束后，`activeStreams.get("ws:sid")` 指向的是 **turn2 的活流**，不是 turn1 的死流。
3. **`runTurn` 级重连复用同一份 handlers**（web `sse.ts:274`、`:316-317`）：`consumeStream(response, handlers_1, ...)` 只调一次，`runTurn` 内部断流后调 `reconnect()` 拿新 Response，**plugin/handlers 不重建**。所以 handlers_1 的 `assistantId_1` 闭包跨重连存活，继续把事件路由进 msg1。

加上 `handlersToPlugin` 的 session_id 过滤（web `sse.ts:74-75` `if (evt.session_id !== active) return`）挡不住 -- turn2 的 `evt.session_id === active`（同一个 id），事件放行。

#### 时序图：串台怎么发生

```
时间轴 ──────────────────────────────────────────────────────────────────►

[turn1 进行中]  web handlers_1(assistantId_1=msg-T1-x) 绑 liveSessionId=sid
  web ←cp  session.started{sid}  onSessionStarted(sid) -> activeSessionId=sid
  web ←cp  item.delta ...        onItemDelta -> append 到 msg-T1-x (msg1)
  web ←cp  item.completed{...}   onItemCompleted -> 替换 msg1 文本

  ⚡ cp->web 断连（session.ended 尚未投递给 web！）
     │
     ├─ web runTurn 检测断流，调 reconnect()（buildCPReconnectFactory）
     │  └─ POST /cp-reconnect?session_id=sid  （attempt 0，无延迟）
     │
     └─ cp 侧 turn1 继续跑完（web 掉了 cp 不停）：
        ├─ agent 产 session.ended{reason:'completed'} -> cp persist 落库
        ├─ cp broadcast session.ended 给 turn1 的 controllers -> web 已不在，丢
        └─ runTurn.finally -> drainPendingMessage(ws, sid)  sse.ts:556
              └─ executeChat({sessionId: sid, ...})  ← 复用同一个 sid！
                  └─ setupActiveStream(ws, sid)  sse.ts:151
                      ├─ 找到 turn1 的 ActiveSSEStream（key=ws:sid）
                      ├─ 关闭其 controllers（无，web 已断）
                      ├─ 标 turn1 stream.done=true，删 key
                      └─ 建 turn2 新 stream 挂进 ws:sid
                  └─ turn2 agent 开始产 event：
                     session.started{sid}  item.delta ...  item.completed ...

[竞态窗口] web 的 reconnect POST 到达 cp
  cp createReconnectSSEResponse(ws, sid)  sse.ts:276
    ├─ activeStreams.get("ws:sid")  -> 找到的是 turn2 的新 stream（turn1 的被驱逐了）
    ├─ stream.done?  false（turn2 活着）
    └─ controllers.add(web)  -> web 挂到 turn2 的流上！返回 200

[串台发生] web 收到 turn2 的事件，handlers_1（未重建）路由进 msg1：
  web ←cp  session.started{sid}   onSessionStarted(sid) -> activeSessionId=sid（同值，no-op）
  web ←cp  item.delta{...}        onItemDelta -> append 到 msg-T1-x (msg1)  ← turn2 文本进 msg1！
  web ←cp  item.completed{...}    onItemCompleted -> 替换 msg1 文本     ← turn2 完成态覆盖 msg1
  ...
  web ←cp  session.ended{...}     onSessionEnded -> 翻 msg1.isStreaming=false

结果：msg1 = turn1 的部分文本 + turn2 的内容，挤在一条消息里。
```

#### 竞态窗口：不是每次都串，取决于谁先到

串台只在 **reconnect POST 到达 cp 时 turn2 已经启动** 才发生。两种结局：

| reconnect 到达时机 | cp 侧状态 | `createReconnectSSEResponse` 返回 | web 行为 | 结果 |
|---|---|---|---|---|
| turn1 已结束、turn2 尚未启动 | `activeStreams.get(ws:sid)` 是 turn1 的 stream，`done=true` | null（`sse.ts:288-296` 清理并返回 null） | 非 2xx -> `onSessionEnded(disconnected)`（web `sse.ts:312-313`） | ✅ 安全。web 认为断连，走 loadHistory 从 DB 捡 turn1 最终态 |
| turn2 已启动（drain 跑完 setupActiveStream） | `activeStreams.get(ws:sid)` 是 turn2 的 stream，`done=false` | 200，挂到 turn2 流 | handlers_1 路由 turn2 事件进 msg1 | ❌ 串台 |

**窗口大小**：turn1 `runTurn.finally` -> `drainPendingMessage` -> `executeChat` -> `setupActiveStream`，这段是异步但很快（毫秒级 DB 查 + 启动）。web 的 reconnect attempt 0 无延迟立即发。两者赛跑。turn1 内容越多、drain 启动越快，越容易落到串台分支。

#### 为什么 session_id 挡不住

`handlersToPlugin` 的过滤（web `sse.ts:74-75`）是 `evt.session_id !== active` 才丢。turn2 的 `evt.session_id === sid === active`（drain 复用 id），事件全放行。**session_id 是"会话"维度，不是"turn"维度** -- 一个 session 内可以有多个 turn，session_id 无法区分它们。

#### 为什么 handlers_1 不重建

`runTurn` 级重连（`buildCPReconnectFactory`）和页面刷新重连（`createCPReconnectStream`）是两条不同路径：

| 路径 | 触发 | handlers | assistantId | 串台？ |
|---|---|---|---|---|
| `runTurn` 级重连 | 断流（`consumeStream` 内 `runTurn` 调 `reconnect()`） | **复用** handlers_1 | assistantId_1（闭包） | ❌ 会串台（本问） |
| 页面刷新重连 | 用户刷新页面 -> `startReconnect` | **重建** handlers_2 | assistantId_2（新 genId 或复用 last.id） | ✅ 不串台。turn2 事件进 msg2（新占位），turn1 从 DB loadHistory |

所以本问的串台只在 `runTurn` 级重连路径发生。页面刷新因为重建 handlers + loadHistory，反而安全。

#### 影响与修复方向

**影响**：msg1 被污染成 turn1 部分 + turn2 全部。用户看到一条内容错乱的消息。DB 侧 turn1/turn2 各自正确落库（persist 用各自的 `assistantMessageId`），所以**刷新可恢复** -- loadHistory 会按 DB 重建两条独立消息。但**不刷新就脏在内存里**。

**修复方向**（任选其一，按侵入性排序）：

1. **reconnect 带 turn/epoch 标识**：cp 的 `ActiveSSEStream` 加一个 `turnEpoch`（每 `setupActiveStream` 递增）。`cp-reconnect` 请求带 `?epoch=N`，cp 检查当前 stream 的 epoch 是否等于 N，不等则返回 non-2xx（"你的 turn 已结束"）。web 走 disconnected -> loadHistory。最干净的方案，但要改 cp 协议。
2. **turn2 的 `session.started` 让 web 认出"新 turn"**：web 的 `onSessionStarted` 检测"我已经在这个 session 里收到过 session.started，又来一个 -> 这是 drain 的新 turn"，主动结束 handlers_1（翻 isStreaming false）并触发 loadHistory/startReconnect 重建。不改 cp 协议，但要给 session.started 加"第几次"的区分（或靠 turn stats）。
3. **drain 不复用 session_id**：drainPendingMessage 开新 session。但这破坏了"一个 session = 一段连续对话"的语义，且历史/通知都按 session 聚合，影响面大。不推荐。

> 闭环：**drain 复用 session_id + `runTurn` 级重连复用 handlers_1 = turn2 事件串进 msg1。** 串台是竞态（reconnect 到达时 turn2 是否已启动），不是必现。session_id 是会话维度挡不住（turn2 同 id）。页面刷新重连因重建 handlers 反而安全。DB 侧两条 turn 各自正确落库，刷新可恢复；不刷新则脏在内存。
