---
name: sse-flow
description: SSE 消息从 agent 到 cp 持久化再到前端的完整传递链路（架构师视角，给新人讲解，以"发一条 query"为例）
type: workflow
domain: agents
related:
  - docs/wiki/workspace/session.md
  - docs/wiki/workspace/builder_explan.md
  - internal/sse-consumer/src/read-sse.ts
  - internal/sse-consumer/src/run-turn.ts
  - control-plane/src/lib/sse.ts
  - control-plane/src/services/chat/executeChat.ts
  - control-plane/src/services/chat/dispatchChatTurn.ts
  - control-plane/src/services/db/messages.ts
created: 2026-07-08T07:47:13Z
updated: 2026-07-08T07:47:13Z
---

# SSE 消息流全链路

> 视角：架构师给新人讲解。配套 [session.md](session.md) 一起看 -- session.md 讲的是"一次对话的生命周期"，本文专门拆"一条 SSE 消息从 agent 嘴里吐出来，到落 DB、再到前端渲染"这条纵向链路。读完后你应该能回答：SSE 协议长什么样？cp 为什么中间要"截一刀"？数据落到哪几张表？前端怎么不死等？

---

## 0. 先把 SSE 协议对齐

新人如果不清楚 SSE 协议，先把这个底层逻辑钉死再往下看，否则后面所有代码都会显得很神秘。

### 0.1 SSE 是什么

**SSE（Server-Sent Events）** 是 HTTP 之上的单向长连接协议：服务端推、客户端收。和 WebSocket 的区别就一句话--**SSE 是单向的，且复用 HTTP/1.1 的 chunked transfer**，所以它能穿过任何不坑代理，部署成本几乎为零。在我们这个平台里，agent → cp → 前端 这条链路就是 SSE。

### 0.2 SSE 帧长什么样

SSE 帧是**纯文本**，按行解析，结构极度简单（见 `internal/sse-consumer/src/read-sse.ts:1` 的注释）：

```
data: {"type":"session.started","session_id":"abc","timestamp":1700000000}\n
\n
```

关键约定：

| 元素 | 含义 |
|------|------|
| `data: ` | 字段名，固定以 `data:` 开头。值是这一行 `:` 后面（可选去一个前导空格）的内容 |
| `event: ` | 可选字段，标记事件类型。我们平台不用，统一走 `data` 里的 JSON `type` 字段 |
| `\n\n`（空行） | **一条事件的边界**。两条空行之间的所有 `data:` 行会被 `\n` 拼起来作为完整 payload |
| `: comment` | 注释行，SSE 读流器忽略。我们用它做心跳（见 §2.3） |
| 多行 `data:` | 多行会被读流器用 `\n` 拼接成一条 payload |

我们平台**只用 `data:` 一种字段**，且每条事件只用一行 `data:`。所以一条完整 SSE 帧的实际形状就是：

```
data: <一段 JSON>\n
\n
```

每条事件就是 `<JSON 一行>\n\n`。UniversalEvent 协议就活在这层 JSON 里，type 字段决定事件种类（`session.started` / `item.delta` / `item.completed` / `session.ended` / `question.requested` / `error`）。

### 0.3 为什么选 SSE 而不是 WebSocket / HTTP 轮询

| 选项 | 不选的原因 |
|------|----------|
| HTTP 轮询 | LLM 推理一次几十秒到几分钟，轮询既浪费 token 又拿不到流式 delta |
| WebSocket | 双向能力用不上（agent → 前端单向就够），代理穿透复杂 |
| SSE | 复用 HTTP，能穿 kube-proxy / nginx，天然支持 chunked，浏览器 `EventSource` 或 `fetch` 都能读 |

> 顶层设计一句话：**SSE 是 LLM 推理流的最便宜载体**。后面所有代码都在围绕"怎么把这条 SSE 流截住、落库、再吐给前端"做文章。

---

## 1. 代码地图（顶层设计）

先把参与 SSE 处理的每个角色摊开看清。一次"发 query"的链路会穿过下面所有文件：

### 1.1 后端（control-plane）

| 角色 | 路径 | 作用 |
|------|------|------|
| HTTP 入口 | `control-plane/src/routes/workspaces/chat.ts` | `POST /api/workspaces/:id/chat` 路由定义 |
| 模式分发 | `control-plane/src/services/chat/dispatchChatTurn.ts` | stream / async / sync 三种响应模式选择 |
| Turn 执行 | `control-plane/src/services/chat/executeChat.ts` | auto-start workspace、mint token、POST 到 agent、包装 SSE |
| SSE 拦截器 | `control-plane/src/lib/sse.ts` | **整个 SSE 处理的核心 1249 行** -- 持久化插件 + 广播插件 + 心跳 + 重连 |
| SSE 读流器 | `internal/sse-consumer/src/read-sse.ts` | 协议级 SSE 解析（按行切割、字段提取、空行 flush） |
| Turn 调度器 | `internal/sse-consumer/src/run-turn.ts` | 在读流器之上加：UniversalEvent JSON 解析、生命周期跟踪、插件分发、重连 |
| 消息持久化 | `control-plane/src/services/db/messages.ts` | `messages` + `session_events` 两张表的 CRUD |
| Session 持久化 | `control-plane/src/services/db/sessions.ts` | `sessions` 表的 chat_status 状态机 |
| 协议类型 | `internal/types/events.ts` | `UniversalEvent` / `UniversalItem` / `ContentPart` / `TurnStats` 类型定义 |

### 1.2 前端（web）

| 角色 | 路径 | 作用 |
|------|------|------|
| SSE 发起 | `web/src/lib/api/sse.ts:224 createAgentChat` | `fetch` POST 到 `/api/workspaces/:id/chat`，拿到 SSE Response |
| 流消费器 | `web/src/lib/api/sse.ts:124 consumeStream` | 复用 cp 端同一个 `@neutree-ai/sse-consumer` 包的 `runTurn`，把 SSE 解析成事件 |
| 事件分发 | `web/src/lib/api/sse.ts:60 handlersToPlugin` | 把每个事件翻译成 `onSessionStarted` / `onItemDelta` / `onItemCompleted` 等回调 |
| 重连工厂 | `web/src/lib/api/sse.ts:179 buildCPReconnectFactory` | 断线后 POST `/_proxy/agent/:wid/cp-reconnect` 重接活跃流 |

### 1.3 数据库（`control-plane/migrations/001_init.sql`）

三张表，三件事：

```sql
-- sessions: 一个对话
CREATE TABLE public.sessions (
    id text PRIMARY KEY,
    workspace_id text NOT NULL,
    chat_status text DEFAULT 'idle',  -- idle | agent | human 状态机
    last_active_at timestamptz,
    last_turn_stats jsonb,            -- 上一个 turn 的上下文 gauge
    pending_message jsonb,            -- 用户在 turn 中排队发的下一条
    ...
);

-- messages: 一次说话（user 的话或 assistant 的话）
CREATE TABLE public.messages (
    id text PRIMARY KEY,
    workspace_id text NOT NULL,
    session_id text,
    role text NOT NULL,               -- user | assistant
    content text DEFAULT '',          -- 纯文本累积（每条 item.completed 追加）
    created_at timestamptz DEFAULT now()
);

-- session_events: 一条 SSE 事件的原子行（粒度细到 tool_call / tool_result / text）
CREATE TABLE public.session_events (
    id text PRIMARY KEY,              -- <message_id>-<5位序号>，自然有序
    message_id text NOT NULL,
    session_id text NOT NULL,
    kind text NOT NULL,               -- text | tool_call | tool_result | ...
    call_id text,                     -- tool 调用的 call_id（可空）
    payload jsonb NOT NULL,           -- 这条事件的完整原始数据
    created_at timestamptz DEFAULT now()
);
```

> **三张表的颗粒度差异要拉通看清楚**：sessions 是"对话"，messages 是"一句话"，session_events 是"一句话里的一个事件"。一个 turn = 1 条 user message + 1 条 assistant message + N 条 session_events（text / tool_call / tool_result 任意组合）。这个三层结构是后面所有 SSE 持久化逻辑的骨架。

---

## 2. 抓手拆解（底层逻辑）

骨架看完，下面拆三个抓手。这三个抓手对应 SSE 流处理的三个核心问题：**怎么进、怎么读、怎么落库+广播**。

### 抓手 1：入口与模式分发 -- "用户发 query 进来后走哪条路"

**位置**：`routes/workspaces/chat.ts:120` → `dispatchChatTurn.ts:38` → `executeChat.ts:59`

用户在前端输入"帮我查一下今天天气"（一条 query）点发送，前端 `createAgentChat` 走 `fetch POST /api/workspaces/:id/chat`，body 里带：

```json
{ "message": "帮我查一下今天天气", "session_id": null, "source": "web" }
```

后端入口三步：

```typescript
// chat.ts:120
chat.openapi(chatRoute, async (c) => {
  const { id } = c.req.valid('param')
  const user = c.get('user')
  const workspace = await getWorkspace(id)
  if (!workspace || !canManage(workspace, user)) return c.json({ error: '...' }, 404)
  return dispatchChatTurn({ workspace, body, acceptHeader, callerUserId: user.sub })
})
```

`dispatchChatTurn` 的关键决策是**响应模式选择**（`dispatchChatTurn.ts:38`）：

| mode | 行为 | 适用场景 |
|------|------|--------|
| `stream`（默认） | 直接把 SSE Response 透传给前端 | 前端 chat 界面（看 delta 流） |
| `async` | 一旦拿到 session_id 就返回 202，turn 在后端继续跑 | 集成场景（不阻塞 HTTP） |
| `sync` | 阻塞到 turn 结束，返回聚合 JSON | 兼容老接口 |

我们以前端最常见的 `stream` 模式为主线讲。`executeChat` 干的事是：

```typescript
// executeChat.ts:59 简化版
async function executeChat(opts) {
  await ensureWorkspaceRunning(workspace)         // 冷启动 ~90s
  const sessionToken = sessionId
    ? await ensureTokenForSession(workspaceId, sessionId)
    : await mintToken({ workspaceId })             // 给 MCP 反查用
  const response = await fetch(`${address}/chat`, {
    method: 'POST', body: JSON.stringify({ message, session_id, sessionToken, ... }),
    signal: AbortSignal.timeout(24 * 60 * 60 * 1000)  // 24h 硬上限
  })
  // ↓ 关键一步：把 agent 的 SSE Response 包成 cp 的 SSE Response
  return createInterceptedSSEResponse(response, { workspaceId, userMessageText, ... })
}
```

> 顶层设计一句话：**cp 不只是转发，它在中间"截一刀"做拦截**。这一刀干两件事 -- 落库（持久化插件）+ 广播给前端（广播插件）。下文两个抓手就是这一刀的两个面。

### 抓手 2：读流与事件解析 -- "SSE 字节流怎么变成 UniversalEvent"

**位置**：`internal/sse-consumer/src/read-sse.ts` + `run-turn.ts`

这是协议层最纯粹的一段代码。分两层：

**第一层 `read-sse.ts`** -- 协议级 SSE 解析器。它**不关心 JSON，不关心 NAP**，只做一件事：把字节流切成"一条一条 SSE 事件"。

核心算法（`read-sse.ts:88-114`）：

```typescript
const reader = response.body.getReader()
const decoder = new TextDecoder('utf-8')
let lineBuffer = ''             // 跨 chunk 的行缓冲
let currentEvent = 'message'
let currentData: string[] = []

while (true) {
  const { done, value } = await reader.read()
  if (done) break
  lineBuffer += decoder.decode(value, { stream: true })
  const normalized = lineBuffer.replace(/\r\n?/g, '\n')   // 规范换行
  const lines = normalized.split('\n')
  lineBuffer = lines.pop() ?? ''                           // 最后一段可能是半行，留到下次

  for (const line of lines) {
    if (line === '') {                                      // 空行 = 事件边界
      const evt = flush()                                   // 把累积的 data 拼成一条事件
      if (evt) yield evt
    } else {
      handleField(line)                                     // 解析 "data: xxx" / "event: xxx"
    }
  }
}
```

`handleField` 的协议级处理（`read-sse.ts:71`）：

```typescript
function handleField(line: string): void {
  const colon = line.indexOf(':')
  if (colon === -1) return                       // 没 ':' 的行被忽略（spec 规定）
  const field = line.slice(0, colon)
  let value = line.slice(colon + 1)
  if (value.startsWith(' ')) value = value.slice(1)  // 去掉一个前导空格

  if (field === 'data') currentData.push(value)
  else if (field === 'event') currentEvent = value || 'message'
  // id、retry、未知字段一律忽略 -- 我们用不到
}
```

> 颗粒度对齐：`readSSE` 是 **WHATWG EventSource spec 的最小子集**。它故意不实现 `id:` / `retry:` / 注释行 -- 上层用不到的特性不实现，是底层逻辑保持简单的关键。

**第二层 `run-turn.ts`** -- 在 `readSSE` 之上加 NAP 语义。它干四件事：

1. **JSON 解析**：`readSSE` 吐出的每条 `data` 字符串，用 `JSON.parse` 转成 `UniversalEvent`（`run-turn.ts:385 parseEvent`）。解析失败时合成一条 `{type:'error', code:'parse_error'}` 喂给插件，**不让单条坏事件搞死整个 turn**。
2. **生命周期跟踪**：识别 `session.started` 拿 `sessionId`，识别 `session.ended` 拿 `reason` 和 `stats`。
3. **插件分发**：对每条事件调用所有插件的 `onEvent(evt, rawData)`（同步、错误隔离）。
4. **重连**：主流结束但没看到 `session.ended` 时，调 `reconnect` 源拿一个新 Response 继续消费，**最多重连一次**（`run-turn.ts:280`）。

```typescript
// run-turn.ts:203 consume 函数核心
async function consume(response: Response): Promise<void> {
  for await (const raw of readSSE(response, { signal: internalCtrl.signal })) {
    resetIdleTimer()                                // 每条事件重置 idle 计时
    const evt = parseEvent(raw.data)                // JSON -> UniversalEvent
    if (evt.type === 'session.started') sessionId = evt.session_id
    else if (evt.type === 'session.ended') sessionEndedReason = ...
    for (const p of plugins) {
      try { p.onEvent(evt, raw.data) }              // 同步分发给所有插件
      catch (e) { console.error(`plugin ${p.name} onEvent error:`, e) }  // 错误隔离
    }
  }
}
```

> 这一层的底层逻辑：**协议解析和业务逻辑彻底解耦**。`readSSE` 只懂 SSE；`runTurn` 只懂事件分发；真正干活的"持久化"和"广播"是 cp 注入的两个插件。三层各管一摊，颗粒度清晰。

### 抓手 3：双插件 -- "一条事件如何同时落库 + 广播"

**位置**：`control-plane/src/lib/sse.ts:672 createPersistMainTurnPlugin` + `sse.ts:1120 createBroadcastPlugin`

这是整个 SSE 处理的"业务核心"。`createInterceptedSSEResponse` 给 `runTurn` 喂了**两个插件**，按顺序：

```typescript
// sse.ts:508
runTurn(
  { stream: async () => response, reconnect, idleTimeoutMs: 24*60*60*1000 },
  [persistPlugin, broadcastPlugin],   // ← 持久化在前，广播在后
)
```

两个插件**共享一个 SerialQueue** 和 **共享一个 MainTurnSharedState**（`sse.ts:416-421`）：

```typescript
const queue = new SerialQueue()                        // 串行队列：DB 写 + 延迟广播按同一顺序
const state: MainTurnSharedState = {
  sessionId: existingSessionId,
  sessionEndedSeen: false,
  endReason: null,
}
```

为什么必须共享队列？-- **保证"前端永远不会在 DB 落地之前看到状态变化"**。比如 `session.ended` 这条事件：

1. persist 插件先收到，把 `chat_status: 'agent' → 'human'` 入队
2. broadcast 插件随后收到，把 `session.ended` 原文的广播**入同一个队列**
3. 队列串行执行：先 UPDATE sessions，再 emitSSE 给前端

前端拿到的永远是"DB 已经反映过的状态"。

#### 3.1 持久化插件 -- 怎么把一条 SSE 事件落成 DB 行

`createPersistMainTurnPlugin` 的 `onEvent` 是一个 switch，按事件类型决定落哪张表。**以一条"用户发 query"的完整 turn 为例**：

| 事件 | 触发时机 | 落库动作 |
|------|--------|---------|
| `session.started` | agent 创建了 session | `createSession` + `transitionSessionStatus(sid, 'agent')` + `bindSessionToken` + 排队 `enqueueUserMessage()`（把用户 query 写 messages 表） |
| `item.completed` (kind=message, role=assistant) | agent 流式吐出一段文字 | `addMessage(role='assistant')`（首次）+ `insertEvent(kind='text', payload)` + `updateMessageContent`（累积到 messages.content） |
| `item.started` (kind=tool_call) | 工具调用开始 | 不落库，只记 `toolCallStartedAt`（给 completed 时用） |
| `item.completed` (kind=tool_call) | 工具调用结束 | `insertEvent(kind='tool_call', payload={call_id, name, arguments, started_at, completed_at})` |
| `item.completed` (kind=tool_result) | 工具结果返回 | `upsertEvent(kind='tool_result', payload)` -- 同 call_id 复用 ordinal，多次更新覆盖同一行 |
| `item.delta` | 流式 delta | **不落库** -- delta 是临时的，前端边来边渲染，最终态在 `item.completed` |
| `session.ended` | turn 结束 | `flushPendingToolResults` + `updateSessionStats` + `transitionSessionStatus(sid, 'human')` |
| `question.requested` | agent 要问用户 | `transitionSessionStatus(sid, 'human')` |
| `error` | agent 报错 | 只记日志，状态由 onEnd 兜底 |

关键代码片段（`sse.ts:731 enqueueEvent`）：

```typescript
function enqueueEvent(kind, callId, payload, ordinal) {
  queue.run(async () => {                              // 入串行队列
    if (!state.sessionId) return
    if (!assistantMessageId) {
      const msg = await addMessage(workspaceId, state.sessionId, 'assistant', textContent)
      assistantMessageId = msg.id                      // 懒创建 assistant message 行
    }
    await insertEvent({
      messageId: assistantMessageId,
      sessionId: state.sessionId,
      ordinal,                                          // <messageId>-00001 这种自然序号
      kind, callId, payload,
    })
    if (kind === 'text') {
      await updateMessageContent(assistantMessageId, textContent)  // 累积纯文本
    }
  })
}
```

`session_events.id` 的格式（`messages.ts:85`）：

```typescript
function eventId(messageId: string, ordinal: number): string {
  return `${messageId}-${padOrdinal(ordinal)}`   // "msg-abc-00042"
}
function padOrdinal(n: number): string {
  return n.toString().padStart(5, '0')           // 5 位前导零，自然字典序 = 时间序
}
```

> 这个 ID 设计是个小心机：用 `<messageId>-<5位序号>` 当主键，自然有序，`ORDER BY id ASC` 就等于按事件到达顺序读回。前端拉历史消息时不用再排序。

`insertEvent` 用 `ON CONFLICT (id) DO NOTHING`（`messages.ts:102`）-- 同一事件重放（比如 cp 重启后重连 agent）不会产生重复行；`upsertEvent` 用 `ON CONFLICT (id) DO UPDATE SET payload = EXCLUDED.payload`（`messages.ts:134`）-- tool_result 的增量更新会覆盖而不是新增。

#### 3.2 tool_result 的合并优化 -- "为什么不是 N 条而是 1 条"

Codex 流式工具调用时，stdout 边累积边吐 `tool_call_update` 事件。如果每次都 INSERT 一行，一个 tool 调用可能产生几十行 session_events，前端列表会爆炸。

解决（`sse.ts:704 toolResultOrdinal` + `sse.ts:787 handleToolResult`）：

```typescript
// 同一个 call_id 复用一个 ordinal → event id 不变
const toolResultOrdinal = new Map<string, number>()

function handleToolResult(callId, payload) {
  let ord = toolResultOrdinal.get(callId)
  if (ord === undefined) {
    ord = eventOrdinal++                               // 首次：分配新序号
    toolResultOrdinal.set(callId, ord)
  }
  // ... 合并到 pendingToolResults，定时器 300ms 合并一次
  scheduleToolResultFlush(callId)
}
```

合并在 `TOOL_RESULT_COALESCE_MS = 300` 毫秒窗口内，多次 update 只产生一次 DB 写入（用 `upsertEvent` 覆盖）。首条立即写，避免恢复路径看到空行。

#### 3.3 广播插件 -- "怎么把 SSE 原文转发给前端"

广播插件比持久化插件简单得多（`sse.ts:1120`）。它的策略是：

| 事件 | 处理 |
|------|------|
| `session.started` | 立即 `emitNow(rawData)` + 标记 `turnActive=true` + 把 activeStream 的 key 从 `wid:new-xxx` 改成 `wid:<sessionId>` |
| `session.ended` / `question.requested` | **入队延迟发**（`enqueueDeferredEmit`）-- 等 persist 的 DB 写完再发，保证前端看到状态时 DB 已更新 |
| 其他事件 | 立即 `emitNow(rawData)` + `turnActive=true` |

`emitNow` 的实现（`sse.ts:1133`）：

```typescript
function emitNow(rawData: string) {
  emitSSE(activeStream, `data: ${rawData}\n\n`)     // 拼回 SSE 帧格式
}

function emitSSE(stream, sseText) {
  if (stream.controllers.size === 0) return          // 没人监听就不发
  stream.lastEmitAt = Date.now()
  const encoded = encoder.encode(sseText)
  for (const c of stream.controllers) {              // 扇出给所有连接的客户端
    try { c.enqueue(encoded) } catch { stream.controllers.delete(c) }
  }
}
```

`activeStream.controllers` 是一个 `Set<ReadableStreamDefaultController>` -- 每个元素代表一个连到 cp 的前端客户端。一个 turn 可以同时被多个客户端"看"（比如用户刷新页面后，旧连接和新连接都在 controllers 里），扇出给所有。

#### 3.4 心跳 -- "怎么防止长空闲被代理掐断"

LLM 推理可能几十秒不出一个字，kube-proxy / conntrack / undici 这种中间件会以为连接死了把 TCP 回收。修复（`sse.ts:36`）：

```typescript
const HEARTBEAT_MS = 15_000
const HEARTBEAT_FRAME = encoder.encode(': \n\n')   // SSE 注释帧，读流器忽略

function startHeartbeat(stream) {
  stream.lastEmitAt = Date.now()
  const timer = setInterval(() => {
    if (stream.done) return
    if (stream.controllers.size === 0) return
    if (Date.now() - stream.lastEmitAt < HEARTBEAT_MS) return   // 有真实事件就重置
    for (const c of stream.controllers) c.enqueue(HEARTBEAT_FRAME)
  }, HEARTBEAT_MS)
  timer.unref?.()                                  // 不阻塞进程退出
}
```

`:` 开头是 SSE 协议规定的注释行，读流器看到会忽略，但 TCP 层看到了字节流，idle timer 被重置。

#### 3.5 重连与 drain -- "cp 重启 / 前端刷新怎么办"

两个独立的恢复路径：

**前端断线重连**（`sse.ts:276 createReconnectSSEResponse` + `web/src/lib/api/sse.ts:179`）：
- 前端 `fetch` 失败 → `buildCPReconnectFactory` 重试 5 次（指数退避 500ms → 8s）
- 前端 POST `/_proxy/agent/:wid/cp-reconnect?session_id=xxx`
- cp 在 `activeStreams` map 里按 `wid:sid` 找活跃流，找到就把新 controller 加进 `controllers` Set，**后续事件自动扇出到新连接**
- 找不到（turn 已结束）→ 返回 null，前端走 onSessionEnded

**cp 进程退出 / 滚动更新**（`sse.ts:105 drainActiveStreams`）：
- 设 `draining = true`，**停止发起新的 agent `/reconnect`**（避免和接替 pod 抢）
- 给每个活跃流的 `SerialQueue.flush()` 5 秒预算，**把已入队但未落库的 DB 写完**
- 超时后强制 close 所有 controller
- 接替 pod 的 `recoverOrphanedSessions` 启动时扫 `chat_status='agent'` 的 session，重连 agent `/sessions/:id/reconnect` 拿 buffered 事件继续落库

> 这两段是"为什么 cp 不只是转发，而要中间截一刀"的真正答案 -- **为了把短期不可靠的 agent 流和前端连接，变成一个可恢复、可持久化、可多客户端共享的广播源**。底层逻辑是：把 agent 和前端的耦合拆开，cp 在中间当状态机的中枢。

---

## 3. 端到端时序（拉通）

下面把"用户发一条 query：帮我查一下今天天气"的完整链路串起来。假设 workspace 已经 running、session 是新建的。

### Step 1. 前端发起

```
用户输入 "帮我查一下今天天气" 点发送
  ↓ web/src/lib/api/sse.ts:224 createAgentChat
  ↓ fetch POST /api/workspaces/<wid>/chat
  ↓   body: { message: "帮我查一下今天天气", session_id: null, source: "web" }
  ↓   credentials: include
  ↓
  ← response: 200, Content-Type: text/event-stream   ← 拿到 SSE 流
  ↓
  ↓ consumeStream(response, handlers)
  ↓   runTurn({ stream: async () => response, reconnect: buildCPReconnectFactory(...) },
  ↓            [handlersToPlugin(handlers)])           ← 前端也用 runTurn，但只装一个 UI 插件
```

### Step 2. cp 入口与模式分发

```
POST /api/workspaces/<wid>/chat
  ↓ routes/workspaces/chat.ts:120
  ↓   getWorkspace + canManage ACL
  ↓   ↓ dispatchChatTurn({ workspace, body, ... })
  ↓   ↓   resolveChatMode(body, acceptHeader) → mode='stream'
  ↓   ↓   executeChat({ workspace, message, sessionId: null, ... })
```

### Step 3. executeChat 准备 agent 调用

```
executeChat.ts:59
  ├── ensureWorkspaceRunning(workspace)              ← 冷启动预算 ~90s
  ├── resolveAgentAddress(workspaceId, { sessionId: null })
  ├── sessionToken = await mintToken({ workspaceId }) ← 给 MCP 反查用
  ├── agentBody = JSON.stringify({ message, session_id: null, source, sessionToken })
  ├── response = await fetch(`${address}/chat`, {
  │     method: 'POST', body: agentBody,
  │     signal: AbortSignal.timeout(24*60*60*1000)    ← 24h 硬上限
  │   })
  └── return createInterceptedSSEResponse(response, { workspaceId, userMessageText, ... })
                                                ↓
                                                这是 cp 中间那一刀
```

### Step 4. cp 拦截器装配双插件

```
sse.ts:382 createInterceptedSSEResponse
  ├── queue = new SerialQueue()                       ← DB 写 + 延迟广播共用
  ├── state = { sessionId: null, sessionEndedSeen: false, endReason: null }
  ├── setupActiveStream(workspaceId, null, queue)
  │     ├── 在 activeStreams map 里登记 'wid:new-<uuid>'
  │     └── startHeartbeat(stream)                    ← 15s 心跳启动
  ├── clientReadable = new ReadableStream(...)        ← 给前端的出口
  │     start(controller) { activeStream.controllers.add(controller) }
  ├── persistPlugin = createPersistMainTurnPlugin({...})
  ├── broadcastPlugin = createBroadcastPlugin({...})
  └── runTurn(
        { stream: async () => tapActivity(response), reconnect, idleTimeoutMs: 24h },
        [persistPlugin, broadcastPlugin]
      )                                                ← fire-and-forget，立即返回 clientReadable
  ← Response(clientReadable, { Content-Type: text/event-stream })
```

前端这一刻就拿到了 Response，连接保持打开，开始等事件。

### Step 5. agent 流式吐事件（每个 SSE 帧的形状）

agent（比如 claude-code）按 UniversalEvent 协议吐字节流。原始 SSE 帧长这样：

```
data: {"type":"session.started","session_id":"sess-abc","timestamp":1700000000}\n\n
data: {"type":"item.started","item":{"item_id":"i1","kind":"message","role":"assistant","status":"in_progress","content":[]},"timestamp":1700000001}\n\n
data: {"type":"item.delta","item_id":"i1","delta":{"type":"text","text":"好"},"timestamp":1700000002}\n\n
data: {"type":"item.delta","item_id":"i1","delta":{"type":"text","text":"的"},"timestamp":1700000003}\n\n
data: {"type":"item.completed","item":{"item_id":"i1","kind":"message","role":"assistant","status":"completed","content":[{"type":"text","text":"好的，我来查"}]},"timestamp":1700000004}\n\n
data: {"type":"item.started","item":{"item_id":"i2","kind":"tool_call","role":"assistant","status":"in_progress","content":[{"type":"tool_call","call_id":"c1","name":"weather_lookup","arguments":"{\"q\":\"今天天气\"}"}]},"timestamp":1700000005}\n\n
data: {"type":"item.completed","item":{"item_id":"i2","kind":"tool_call","role":"assistant","status":"completed","content":[{"type":"tool_call","call_id":"c1","name":"weather_lookup","arguments":"{\"q\":\"今天天气\"}"}]},"timestamp":1700000006}\n\n
data: {"type":"item.completed","item":{"item_id":"i3","kind":"tool_result","role":"tool","status":"completed","content":[{"type":"tool_result","call_id":"c1","output":"北京 26℃ 晴"}]},"timestamp":1700000007}\n\n
data: {"type":"item.completed","item":{"item_id":"i4","kind":"message","role":"assistant","status":"completed","content":[{"type":"text","text":"今天北京 26℃，晴天。"}]},"timestamp":1700000008}\n\n
data: {"type":"session.ended","session_id":"sess-abc","timestamp":1700000009,"reason":"completed","stats":{"numTurns":1,"contextTokens":1234,"contextWindow":200000,"costUsd":0.01,"durationMs":9000,"inputTokens":1000,"outputTokens":234}}\n\n
```

### Step 6. cp 端逐帧处理（持久化 + 广播）

`runTurn` 的 `consume` 函数读 `readSSE`，每条事件喂给两个插件。下表是每条事件经过双插件后的副作用：

| # | 事件 | persist 插件落库 | broadcast 插件广播 | 前端可见 |
|---|------|-----------------|-------------------|--------|
| 1 | `session.started` (sid=sess-abc) | `createSession(ws, sess-abc, '', userId, 'web')` + `transitionSessionStatus(sess-abc, 'agent')` + `bindSessionToken(token, sess-abc)` + `enqueueUserMessage()`（写 messages: user "帮我查一下今天天气"） | 立即 `emitNow` + 重设 activeStream key 为 `wid:sess-abc` + `turnActive=true` | 收到 `session.started` → 前端 `onSessionStarted(sess-abc)` |
| 2 | `item.started` (message) | 不落库，记 `toolCallStartedAt`（不是 tool_call 所以这条没副作用） | 立即 `emitNow` | UI 显示"agent 正在输入..." |
| 3 | `item.delta` (text="好") | **不落库** | 立即 `emitNow` | UI 流式显示"好" |
| 4 | `item.delta` (text="的") | **不落库** | 立即 `emitNow` | UI 流式显示"好的" |
| 5 | `item.completed` (message, "好的，我来查") | `addMessage(role='assistant', content='')`（首次创建 assistant message 行）+ `insertEvent(kind='text', payload={"text":"好的，我来查"})` + `updateMessageContent(msgId, "好的，我来查")` | 立即 `emitNow` | UI 把流式的"好"+"的"替换为完整文本 |
| 6 | `item.started` (tool_call c1) | 记 `toolCallStartedAt.set('c1', ts)` | 立即 `emitNow` | UI 显示"调用 weather_lookup..." |
| 7 | `item.completed` (tool_call c1) | `insertEvent(kind='tool_call', call_id='c1', payload={call_id, name, arguments, started_at, completed_at})` | 立即 `emitNow` | UI 显示工具调用详情卡片 |
| 8 | `item.completed` (tool_result c1) | `handleToolResult('c1', payload)` → 首次立即 `upsertEvent(kind='tool_result', call_id='c1', payload={call_id, output:"北京 26℃ 晴", ...})` | 立即 `emitNow` | UI 显示工具结果 |
| 9 | `item.completed` (message "今天北京 26℃...") | 复用同一 assistantMessageId + `insertEvent(kind='text', ...)` + `updateMessageContent` 累积文本 | 立即 `emitNow` | UI 显示最终回答 |
| 10 | `session.ended` (reason=completed, stats) | `flushPendingToolResults()` + `updateSessionStats(sess-abc, {numTurns, contextTokens, contextWindow})` + `transitionSessionStatus(sess-abc, 'human')` + 异步 `pullWorkspaceUsage` + `notify(...)` 推送完成通知 | **入队延迟发**（等 persist 的 UPDATE sessions 落库后）→ `emitNow` | 收到 `session.ended` → 前端 `onSessionEnded(sess-abc, 'completed', stats)` |

#### 6.1 三张表的主键 id 生成规则（对齐到颗粒度）

上表里"INSERT session_events"用的是哪条 id？谁生成的？什么时候生成？这三件事要先钉死，否则看不懂恢复路径。

| 表 | 主键 | 生成方 | 生成时机 | 格式 |
|----|------|--------|---------|------|
| `sessions` | `id` | **agent 端**（不是 cp） | agent 创建新 session 时生成，cp 在 `session.started` 事件里拿到 `session_id` 字段后直接用作主键 | agent 端 UUID 风格字符串 |
| `messages` | `id` | **cp 端** | `addMessage` 第一次持久化 user/assistant 消息时调用 `generateId()` | `Math.random().toString(36).substring(2, 10)` -- **8 字符随机 base36 串**（如 `k4f9x2qp`） |
| `session_events` | `id` | **cp 端派生** | `insertEvent` / `upsertEvent` 时根据 `messageId + ordinal` 拼出 | `<messageId>-<5位前导零序号>`，如 `k4f9x2qp-00042` |

关键代码三段，拉通看：

```typescript
// control-plane/src/services/db/pool.ts:78
export function generateId(): string {
  return Math.random().toString(36).substring(2, 10)   // 8 字符随机 base36
}

// control-plane/src/services/db/messages.ts:4
export async function addMessage(workspaceId, sessionId, role, content) {
  const id = generateId()                              // ← cp 生成
  await client.query(
    'INSERT INTO messages (id, workspace_id, session_id, role, content) VALUES ($1, ...)',
    [id, ...],
  )
  return await getMessage(id)
}

// control-plane/src/services/db/messages.ts:85
function eventId(messageId: string, ordinal: number): string {
  return `${messageId}-${padOrdinal(ordinal)}`         // ← 派生
}
function padOrdinal(n: number): string {
  return n.toString().padStart(5, '0')                 // 00001、00002...
}
```

而 `sessions.id` 走的是另一条路 -- **cp 不生成，直接用 agent 给的**（`sessions.ts:10 createSession` 把 `sessionId` 当参数传进来）：

```typescript
// sse.ts:856  persist 插件处理 session.started
case 'session.started': {
  const newSid = evt.session_id                        // ← 从事件字段拿
  state.sessionId = newSid
  queue.run(async () => {
    if (isNew) {
      await createSession(workspaceId, newSid, '', callerUserId, source)
      //                                  ↑↑↑ 直接当主键 INSERT，不 generateId
    }
    ...
  })
}
```

> 顶层设计一句话：**sessions 的 id 跟着 agent 走，messages / session_events 的 id 跟着 cp 走**。这条边界很重要 -- 它决定了 cp 重启恢复时哪条 id 能"重新拿到"，哪条只能"复用旧的"。

#### 6.2 cp 重启恢复时，id 如何保持生成一致

> 这是整个 SSE 持久化最容易踩坑的地方。`generateId()` 是 `Math.random`，重启后重新调一次**必然生成不同的 id**。那 cp 重启后重连 agent，怎么保证写到的是原来的 message 行、而不是新建一行？

**底层逻辑：cp 不重新生成 id，而是从 DB 读回旧的 id 复用**。这条恢复链路在 `control-plane/src/lib/session-recovery.ts:86-106`，分四步：

**Step A. 启动时扫"孤儿 session"**（`session-recovery.ts:17`）

```typescript
// 找所有 chat_status='agent' 但 cp 已经没活跃流的 session
const { rows } = await pool.query(
  "SELECT id, workspace_id FROM sessions WHERE status = 'active' AND chat_status = 'agent'"
)
```

cp 重启前正在跑的 turn，DB 里 `chat_status` 还是 `agent` -- 这些就是要恢复的孤儿 session。

**Step B. 重连 agent 的 buffered SSE**（`session-recovery.ts:56`）

```typescript
response = await fetch(`${address}/sessions/${sessionId}/reconnect`, { method: 'POST' })
```

agent 如果还留着 buffered 事件（turn 还在跑或者刚结束事件没吐完），会返回一个新的 SSE Response。详见 §6.4 agent 侧怎么 buffer。

**Step C. 从 DB 读回旧 messageId + 旧 blocks**（`session-recovery.ts:88-100`）

```typescript
let initialAssistant: { id: string; content: string; blocks: any[] } | undefined
const last = await getLastMessageWithBlocks(sessionId)         // ← 读最后一条 assistant message
if (last && last.role === 'assistant') {
  initialAssistant = {
    id: last.id,                                               // ← 旧 messageId 复用！
    content: last.content ?? '',
    blocks: last.blocks,                                       // ← 已有事件列表
  }
}
```

- 如果 cp 死在 assistant message 已建之后：读到的 `last` 是这条半成品 message，**它的 id 被原封不动塞进 `initialAssistant.id`**
- 如果 cp 死在 assistant message 建之前（比如刚写完 user message）：`last.role` 是 `user` 或 `null`，`initialAssistant` 保持 `undefined`，让 persist 插件在第一条事件到达时正常 `addMessage`（这时本来就没有旧行要复用）

**Step D. persist 插件从 `initialAssistant` seed 全部内部状态**（`sse.ts:688-713`）

这是 id 一致性的真正落地之处。persist 插件初始化时：

```typescript
// sse.ts:691-713  关键代码
let textContent = ctx.initialAssistant?.content ?? ''           // ← 沿用旧文本
let assistantMessageId: string | null = ctx.initialAssistant?.id ?? null  // ← 沿用旧 messageId！
let eventOrdinal = ctx.initialAssistant?.blocks.length ?? 0     // ← ordinal 从已有 blocks 数量继续
const toolCallOrdinal = new Map<string, number>()
const toolCallStartedAt = new Map<string, number>()
const toolResultOrdinal = new Map<string, number>()

if (ctx.initialAssistant) {
  ctx.initialAssistant.blocks.forEach((b, i) => {                // ← 遍历已落库的 blocks
    if (b.type === 'tool_call' && b.call_id) {
      toolCallOrdinal.set(b.call_id, i)                          // ← 旧 call_id 复用旧 ordinal
      if (typeof b.started_at === 'number') toolCallStartedAt.set(b.call_id, b.started_at)
    }
    if (b.type === 'tool_result' && b.call_id) toolResultOrdinal.set(b.call_id, i)
  })
}
```

**关键效果**：恢复后 agent 重放同一批事件时：

| 字段 | 恢复路径的值 | 与 DB 已有行的关系 |
|------|------------|-----------------|
| `assistantMessageId` | 旧 messageId（从 DB 读回） | `session_events.message_id` 命中旧行 |
| `eventOrdinal` | 从 `blocks.length` 继续 | 新事件序号紧接已有事件，不冲突 |
| `toolCallOrdinal['c1']` | 旧 call_id 对应的旧序号 | 重放 `tool_call` 事件时拼出**同一个 `session_events.id`** |
| `toolResultOrdinal['c1']` | 旧 call_id 对应的旧序号 | 重放 `tool_result` 时命中旧行做 `DO UPDATE` |

**Step E. DB 层的幂等兜底**（`messages.ts:102, 134`）

```typescript
// insertEvent 用 ON CONFLICT DO NOTHING
INSERT INTO session_events (id, ...) VALUES ($1, ...)
ON CONFLICT (id) DO NOTHING                              // 重放同 id 事件 -> 什么都不做

// upsertEvent 用 ON CONFLICT DO UPDATE
INSERT INTO session_events (id, ...) VALUES ($1, ...)
ON CONFLICT (id) DO UPDATE SET payload = EXCLUDED.payload  // tool_result 增量 -> 覆盖
```

`createSession` 也幂等（`sessions.ts:21`）：

```typescript
INSERT INTO sessions (id, ...) VALUES ($1, ...)
ON CONFLICT (id) DO UPDATE SET last_active_at = NOW()   // 重放 session.started -> 刷新时间
```

#### 6.3 三种 cp 死亡时机的 id 一致性对照表

把恢复链路按"cp 死在哪个阶段"拉一遍，验证 id 一致性的闭环：

| cp 死亡时机 | DB 已有的行 | `initialAssistant` | 恢复后 id 行为 |
|-----------|-----------|-------------------|--------------|
| `session.started` 之前 | 无 session 行 | undefined | agent 重连后重发 `session.started` 同一个 `session_id`（agent 端的），cp `createSession` 新建一行用 agent id -- **一致** |
| `session.started` 之后、首条 assistant 文本之前 | session 行 + user message 行 | undefined（last role=user） | persist 第一条事件调 `addMessage` 调 `generateId()` 生成新 messageId -- 没有旧行冲突，**新 id 合法** |
| 首条 assistant 文本已落、中途死亡 | session + user + assistant message + 部分 session_events | `{id: <旧messageId>, content: <旧文本>, blocks: [...]}` | **复用旧 messageId**，重放事件拼出**相同 session_events.id** -> `ON CONFLICT DO NOTHING` 命中旧行 -> **完全幂等** |
| `session.ended` 已落、广播前 | 全部行已就位 | `{id: <旧messageId>, ..., blocks: [全部事件]}` | persist 跑一遍 `enqueueEvent` 但全部 `DO NOTHING` -> broadcast `enqueueDeferredEmit` 但流已结束 -> onEnd flush -> **零副作用** |

> 闭环一句话：**id 一致性靠"agent 给的 session_id 跨重连不变 + cp 从 DB 读回旧 messageId 复用 + session_events.id 派生自 (messageId, ordinal) + ON CONFLICT 兜底"四层兜底**。`generateId()` 的随机性对恢复路径**不构成风险** -- 因为恢复路径根本不调 `generateId()`，它走的是 `initialAssistant` 这条复用通道。

#### 6.4 agent 侧的重连与幂等（SessionSink 是怎么实现的）

> 上面 Step B 说"agent 如果还留着 buffered 事件，会返回一个新的 SSE Response" -- 这一层在 agent 实现里怎么落地的？为什么 cp 多次重连同一个 session，agent 吐出的事件是**同一批**而不是新的一批？

实际实现以 `agents/claude-code/src/server.ts` 为准。核心是一个叫 **SessionSink** 的结构 -- 一个 turn 内部**可替换的 SSE writer**，自带 buffer。

**SessionSink 接口**（`server.ts:84`）：

```typescript
interface SessionSink {
  write: (event: string, data: string) => Promise<void>   // 当前实际 writer（可替换）
  buffer: Array<{ event: string; data: string }>          // 缓冲区
  disconnected: boolean                                     // 是否处于 buffer 模式
  doneResolve: () => void                                  // turn 完成时 resolve
  donePromise: Promise<void>                               // 等待 turn 结束
}

const activeSinks = new Map<string, SessionSink>()         // sessionId -> sink
```

**关键设计**：`sink.write` 是一个**可替换的函数指针**。在 turn 生命周期里，根据 cp 是否在线，`write` 会在三种形态间切换：

| `sink.write` 当前指向 | 触发时机 | 行为 |
|---------------------|---------|------|
| `chatWriter`（首次连接的 writer） | `/chat` 进来，turn 刚开始 | 直接 `stream.writeSSE` 发给当前 SSE |
| `bufferWriter`（buffer 模式） | `switchToBufferMode` 被调（cp 断开） | 不发，只 `sink.buffer.push(...)` |
| `reconnectWriter`（重连后的新 writer） | `/sessions/:id/reconnect` 接管 | 发给新 SSE stream |

**关键状态切换函数 `switchToBufferMode`**（`server.ts:94`）：

```typescript
function switchToBufferMode(sessionId: string, expectedWriter?: SessionSink['write']) {
  const sink = activeSinks.get(sessionId)
  if (!sink) return
  // ↑↑↑ 关键防呆：只有当前 writer 还是"我"时才切 buffer
  // 防止 /chat 的 late disconnect 把 /reconnect 已经接管的 writer 错误覆盖成 buffer
  if (expectedWriter !== undefined && sink.write !== expectedWriter) {
    console.log(`[agent] Skipping switchToBufferMode session=${sessionId} (writer replaced by another handler)`)
    return
  }
  sink.disconnected = true
  sink.write = async (event, data) => { sink.buffer.push({ event, data }) }
}
```

> 这个 `expectedWriter` 比较是个真实 bug 修复点 -- 早期版本里 `/chat` 的 abort 信号迟到（比如 TCP 半关），会误把 `/reconnect` 已经接管的 writer 踩回 buffer 模式，导致重连后再被偷偷切走。颗粒度对齐到"谁拥有 writer 谁负责切换"这条所有权规则。

**`/chat` 路由的 sink 装配**（`server.ts:116`）：

```typescript
app.post('/chat', async (c) => {
  // ...
  return streamSSE(c, async (stream) => {
    const chatWriter = async (event, data) => {
      try { await stream.writeSSE({ event, data }) } catch { /* 静默丢弃 */ }
    }
    const sink: SessionSink = { write: chatWriter, buffer: [], disconnected: false, ... }
    if (sessionId) activeSinks.set(sessionId, sink)        // ← 立即注册，cp 后续才能 reconnect

    // 客户端断开（abort 信号）-> 切 buffer 模式
    c.req.raw.signal.addEventListener('abort', () => {
      switchToBufferMode(sessionId, chatWriter)           // ← 带上 expectedWriter 防呆
    })

    // 跑 turn，事件都通过 sink.write 发出
    await runAgentTurn(sink)
    sink.doneResolve()
  })
})
```

**`/sessions/:id/reconnect` 路由的回放逻辑**（`server.ts:290`）：

```typescript
app.post('/sessions/:id/reconnect', async (c) => {
  const sessionId = c.req.param('id')
  const sink = activeSinks.get(sessionId)
  if (!sink) return c.json({ error: 'No active session to reconnect' }, 404)

  return streamSSE(c, async (stream) => {
    // 1. 把 buffer 里的事件"按原序"重放给新连接
    const buffered = [...sink.buffer]                    // ← 复制一份
    sink.buffer = []                                      // ← 清空！buffer 只重放一次
    for (const { event, data } of buffered) {
      await stream.writeSSE({ event, data })
    }

    // 2. 接管 sink.write，后续新事件直接发新连接
    const reconnectWriter = async (event, data) => {
      try { await stream.writeSSE({ event, data }) } catch {}
    }
    sink.write = reconnectWriter                          // ← 替换 write 指针
    sink.disconnected = false                             // ← 退出 buffer 模式

    // 3. 新连接又断开 -> 再次切 buffer 模式（积累新事件）
    c.req.raw.signal.addEventListener('abort', () => {
      switchToBufferMode(sessionId, reconnectWriter)
    })

    // 4. 一直保持 stream 开着，直到 turn 真正结束
    await sink.donePromise
  })
})
```

**buffer 的真实生命周期 -- 我之前的描述要修正**：

| 时刻 | sink.write 指向 | sink.buffer 状态 |
|------|----------------|----------------|
| T0 `/chat` 连上 | chatWriter | `[]` 空 |
| T1 cp 断开，agent 继续 turn | bufferWriter | `[事件A, 事件B, 事件C]` 累积 |
| T2 cp `/reconnect` | 把 buffered 全部 replay 后清空 -> reconnectWriter | `[]` 清空 |
| T3 新事件 D、E 来了 | reconnectWriter 直接发给 cp | `[]` 保持空 |
| T4 cp 又断开 | bufferWriter | `[事件F]`（只累积 T4 之后的新事件） |
| T5 cp 再次 `/reconnect` | replay `[事件F]` 后清空 -> reconnectWriter | `[]` 清空 |

> 关键修正：**buffer 不是"永远不清空"，而是"每次重连后清空，从断开时刻开始重新累积"**。所以多次重连不是每次都重放完整 buffer -- 而是每次只重放"上次重连之后到现在为止"的新事件。**cp 端的 `ON CONFLICT DO NOTHING` 兜底依然必要**，因为断开/重连的时间窗口里可能有"已发但 cp 还没确认收到"的事件会被 replay 一次。

**为什么这套设计能保证幂等**：

| 设计点 | 对幂等的贡献 |
|-------|------------|
| **buffer 里存的是原始 SSE 帧**（`{event, data}` 字面值） | 重放时事件字节级一致 -> cp 端 `evt.session_id` / `evt.item.item_id` / `evt.item.content[].call_id` 全部不变 |
| **agent 端的 session_id 在 turn 全程不变** | cp 端 `state.sessionId` 跨重连稳定 -> 所有事件 `evt.session_id` 都指向同一行 sessions |
| **item_id（如 `i1`）和 call_id（如 `c1`）在 turn 内部由 agent 生成且稳定** | cp 端 `toolCallOrdinal['c1']` 在恢复路径被 seed 成旧值 -> 重放 `tool_call` 拼出**同一个 `session_events.id`** |
| **buffer 重放是"原序 + 原值"** | cp 端 `eventOrdinal` 即使重新递增，序号会和已有重叠 -> `insertEvent` 的 `ON CONFLICT (id) DO NOTHING` 兜住 |
| **`expectedWriter` 防呆防"writer 被踩"** | 避免一次重连被 late disconnect 误切回 buffer -> 避免"明明 cp 已收到的事件被偷偷再 buffer 一份" |

**cp-A 死、cp-B 接管的完整时间线**：

```
T0: cp-A POST /chat, agent 建 sink, 注册到 activeSinks[sid]
    agent 跑 turn, 事件 -> chatWriter -> cp-A
T1: cp-A 进程被 kill
    agent 检测到 /chat 的 stream abort
    switchToBufferMode(sid, chatWriter)              ← 切 buffer 模式
    agent 继续 turn, 事件 A/B/C 进 buffer
T2: cp-B 启动, recoverOrphanedSessions 扫到 sid
T3: cp-B POST /sessions/sid/reconnect
    agent 找到 activeSinks[sid]
    把 buffered [A,B,C] replay 给 cp-B               ← 一次重放
    sink.buffer = []                                  ← 清空
    sink.write = reconnectWriter
T4: 新事件 D/E 来了
    agent -> reconnectWriter -> cp-B                  ← 直接发，不进 buffer
T5: cp-B 也挂了
    /reconnect 的 stream abort
    switchToBufferMode(sid, reconnectWriter)         ← 再次切 buffer
    新事件 F 进 buffer（不是 A~E！）
T6: cp-C 接管 POST /sessions/sid/reconnect
    agent replay [F] 给 cp-C                         ← 只重放 F
    后续新事件直接发给 cp-C
```

> 三层幂等闭环（修正版）：**agent buffer "重连后清空、断开后累积"的滑动窗口 + cp `initialAssistant` seed 复用旧 messageId（跨进程重启 id 一致）+ DB `ON CONFLICT DO NOTHING/UPDATE`（最后一道兜底，扛住"已发但未确认"的边缘事件）**。三层叠加后即使 cp 反复重启也不会产生脏数据 -- agent 保证事件不丢、cp 保证 id 不漂移、DB 保证不重复。

#### 6.5 question 的请求-响应闭环（人机交互怎么走完一圈）

> question 是整条 SSE 流里唯一一个"事件流方向与回复方向相反"的场景：常规 turn 里事件从 agent 流向用户，question 里则是 agent 先问用户、用户再回 agent。底层逻辑是把 SDK 的 `canUseTool` 钩子当抓手，用一个 Promise 把 turn 卡住，等用户的答案通过独立的 HTTP 通道绕回来 resolve 掉，turn 才能继续。这个闭环里 agent 是 owner，cp 只做转发和状态机切换。

**端到端时序**

```
┌──────┐  ┌──────┐  ┌─────┐  ┌──────┐  ┌────┐
│ Web  │  │  cp  │  │ DB  │  │agent │  │SDK │
└──┬───┘  └──┬───┘  └──┬──┘  └──┬───┘  └─┬──┘
   │         │         │        │        │
   │  [turn 进行中，SDK 调 AskUserQuestion 工具]      │
   │         │         │        │        │
   │         │         │        │ canUseTool(AskUserQuestion, input)
   │         │         │        │◄───────│
   │         │         │        │ requestId = crypto.randomUUID()
   │         │         │        │ pendingResponses.set(requestId, {resolve, originalInput})
   │         │         │        │ pendingQuestionsBySession.set(sid, {requestId, questions})
   │         │         │        │ onAskUser({requestId, questions}) → sink.write
   │         │         │        │        │
   │         │ SSE: question.requested     │
   │         │◄────────│────────│        │
   │         │ UPDATE chat_status='human' │
   │         │────────►│        │        │
   │ SSE: question.requested (转发)       │
   │◄────────│         │        │        │
   │ onQuestionRequested → 渲染问题卡片   │
   │         │         │        │        │
   │  [SDK 在等 resolve，turn 暂停]       │
   │         │         │        │        │
   │ 用户点选答案                        │
   │ POST /api/agents/<wid>/proxy/sessions/:sid/answer
   │────────►│         │        │        │
   │         │ proxy 透传到 agent /sessions/:sid/answer
   │         │─────────────────►│        │
   │         │         │   respondToQuestion(requestId, answers)
   │         │         │        │ pendingResponses.get(requestId).resolve(
   │         │         │        │   {behavior:'allow', updatedInput:{...originalInput, answers}}
   │         │         │        │ )
   │         │         │        │ pendingResponses.delete(requestId)
   │         │         │        │ pendingQuestionsBySession.delete(sid)
   │         │         │        │────────►│
   │         │         │        │        │ SDK 拿到 updatedInput，重新执行 built-in AskUserQuestion
   │         │         │        │        │ → answers 灌进后续逻辑
   │         │         │        │        │
   │         │         │        │ 继续吐 item.delta / item.completed ...
   │         │         │        │ sink.write → SSE
   │         │ SSE: item.delta (转发)
   │◄────────│         │        │        │
   │         │   ... 最终 session.ended ... │
   │         │◄────────│────────│        │
   │         │ UPDATE chat_status='human' │
   │         │────────►│        │        │
   │ SSE: session.ended                  │
   │◄────────│         │        │        │
```

**关键代码颗粒度对齐**

1. **拦截点：canUseTool 钩子**（`agents/claude-code/src/agent.ts:263-294`）
   ```typescript
   canUseTool: async (toolName, input, { signal }) => {
     // Only intercept AskUserQuestion - auto-approve everything else
     if (toolName !== 'AskUserQuestion') {
       return { behavior: 'allow' as const, updatedInput: input }
     }

     const requestId = crypto.randomUUID()
     const questions = (input as Record<string, unknown>).questions as unknown[]
     callbacks.onAskUser?.({ requestId, questions })

     // Store by session for recovery after UI refresh
     if (resultSessionId) {
       pendingQuestionsBySession.set(resultSessionId, { requestId, questions })
     }

     return new Promise((resolve, reject) => {
       pendingResponses.set(requestId, { resolve, originalInput: input as Record<string, unknown> })
       signal.addEventListener('abort', () => {
         pendingResponses.delete(requestId)
         if (resultSessionId) pendingQuestionsBySession.delete(resultSessionId)
         reject(signal.reason ?? new Error('aborted'))
       })
     })
   }
   ```
   - 只拦 `AskUserQuestion`，其他工具一律放行（auto-approve）
   - `requestId = crypto.randomUUID()` 是这次问答的幂等键
   - 同时塞两个 Map：`pendingQuestionsBySession`（按 session 反查用）+ `pendingResponses`（按 requestId 反查用）
   - 返回的 Promise 不 resolve，SDK 的 turn 在这里**挂起**

2. **respondToQuestion：resolve 掉挂起的 Promise**（`agents/claude-code/src/agent.ts:101-116`）
   ```typescript
   export function respondToQuestion(requestId: string, answers: Record<string, string>): boolean {
     const pending = pendingResponses.get(requestId)
     if (!pending) return false
     pending.resolve({ behavior: 'allow', updatedInput: { ...pending.originalInput, answers } })
     pendingResponses.delete(requestId)
     for (const [sid, q] of pendingQuestionsBySession) {
       if (q.requestId === requestId) {
         pendingQuestionsBySession.delete(sid)
         break
       }
     }
     return true
   }
   ```
   - 关键细节：`updatedInput` 必须**展开 `originalInput` 再覆盖 `answers`**，不能只发 `{ answers }`
   - 原因在文件顶部注释（`agent.ts:80-85`）：SDK 拿到 updatedInput 后会**重新执行** built-in AskUserQuestion 工具，工具内部做 `questions.map(...)`。如果 `questions` 字段丢了，就会抛 "undefined is not an object"
   - 双 delete：`pendingResponses`（按 requestId）+ `pendingQuestionsBySession`（按 sid 反查），保证两个 Map 都不会泄漏

3. **agent HTTP 入口：/sessions/:id/answer**（`agents/claude-code/src/server.ts:280-285`）
   - 接收 `{ answers }` body
   - 调 `respondToQuestion(requestId, answers || {})`
   - 返回 `{ ok: true/false }` 给 cp

4. **cp persist 插件处理 question.requested**（`control-plane/src/lib/sse.ts:1069-1076`）
   ```typescript
   case 'question.requested': {
     queue.run(async () => {
       if (state.sessionId) {
         await transitionSessionStatus(state.sessionId, 'human')
       }
     }, logError('transition to human (question)'))
     break
   }
   ```
   - 只做一件事：把 `sessions.chat_status` 从 `agent` 切到 `human`，告诉外部"这个 session 现在在等人"
   - **不落 session_events 表**：question.requested 不写 `session_events`，重启后这一帧的 trace 会丢（设计 gap，详见 §6.4 之前的分析）

5. **前端 POST 路径**：`POST /api/agents/<wid>/proxy/sessions/:sid/answer`
   - 经 `control-plane/src/routes/proxy.ts` 透传到 agent
   - body：`{ request_id, answers }`

**前端刷新后的恢复路径**

> 用户在 question 卡片渲染出来后刷新页面，UI 状态丢失，但只要 agent 进程没重启，question 状态就能恢复。抓手是 agent 暴露的 `/sessions/:sid/pending-question` 反查端点。

1. 前端刷新后通过 `GET /api/agents/<wid>/proxy/sessions/:sid/pending-question` 反查
2. 经 proxy 透传到 agent `server.ts:269` 路由
3. agent 调 `getPendingQuestion(sid)`（`agent.ts:118-122`）从 `pendingQuestionsBySession` 拿回 `{requestId, questions}`
4. 前端拿到后重新渲染问题卡片
5. 用户选答案后走和正常流程一样的 POST /answer 路径

**幂等保证与边缘场景**

| 边缘场景 | 行为 |
|---------|------|
| 用户重复点 POST /answer | `pendingResponses.get(requestId)` 第二次返回 undefined，`respondToQuestion` 返回 false，cp 把这个 false 透传回去告诉前端"已答过了" |
| 用户答完之后 agent 重启 | `pendingResponses` 是内存 Map，进程一死就没了。SDK 也会被一起 abort，turn 不会再继续。这种场景属于"agent 死亡"，不是 question 闭环要解决的问题 |
| 前端刷新后反查 pending-question | 只要 agent 进程没死，`pendingQuestionsBySession` 里还有数据，就能恢复 UI |
| agent 重启 + cp 没重启 | question.requested 没落 session_events，cp 也没有 pending 状态记录。这是真实 gap：用户会卡死在 `chat_status='human'` 状态，无法重新发起 turn（cp 路由会拒绝） |

> 闭环设计的 owner 是 agent：两个 Map + 一个 Promise 都在 agent 进程里，cp 只做转发和状态机切换。这种设计的好处是 agent 死了 question 自然作废（不会留僵尸状态），坏处是 agent 重启后无法恢复（前面 §6.4 分析过的 gap，question 这条路径上更明显）。如果未来要让 question 也能跨 agent 重启恢复，需要把 `pendingQuestionsBySession` 序列化到 cp 的 DB 里，并在 agent 启动时通过 reconnect 拉回来 -- 但这会增加 cp 与 agent 的耦合，目前没做。

### Step 7. turn 结束、清理

```
runTurn 的 onEnd 钩子（两个插件都跑）
  ├── persist.onEnd:
  │     ├── flushPendingToolResults()         ← 强制 flush 待合并的 tool_result
  │     ├── await queue.flush()               ← 等所有 DB 写完成
  │     └── if (!sessionEndedSeen) transitionSessionStatus(sid, 'idle')  ← 兜底
  ├── broadcast.onEnd:
  │     ├── await queue.flush()               ← 二次 flush（幂等）
  │     ├── if (!sessionEndedSeen && !draining) emitNow({type:'error', ...})  ← 兜底报错
  │     ├── activeStream.done = true
  │     ├── clearInterval(heartbeatTimer)
  │     ├── for (c of controllers) c.close()   ← 关闭所有前端连接
  │     └── activeStreams.delete(key)          ← 从全局 map 摘除
  └── sse.ts:538 .finally():
        if (sessionEndedSeen && endReason === 'completed' && !draining)
          void import('executeChat').then(({ drainPendingMessage }) =>
            drainPendingMessage(workspaceId, sid))   ← 用户排队下一条 query 自动发
```

### Step 8. 异常路径速查

| 场景 | 行为 |
|------|------|
| 前端刷新页面 | 新连接 POST `cp-reconnect?session_id=sess-abc` → cp 找到活跃 stream → 加新 controller 到 `controllers` Set → 后续事件扇出到新连接 |
| cp 中途重启 | `drainActiveStreams` 5s 内 flush 完 DB → 接替 pod 启动时 `recoverOrphanedSessions` 扫 `chat_status='agent'` → 重连 agent `/sessions/:id/reconnect` 拿 buffered 事件 → 继续 runTurn |
| agent 流断（没 session.ended） | `runTurn` 尝试一次 `reconnectFactory` → POST agent `/sessions/:id/reconnect` → 拿到 buffered 事件继续消费 |
| agent 24h 没出事件 | `AbortSignal.timeout(24h)` 触发 → runTurn 走 error 路径 → broadcast 插件 emit `{type:'error', message:'Agent stream timed out'}` |
| 用户在 turn 中发新消息 | `pending_message` 字段存进 sessions 表 → turn 结束的 `.finally` 钩子调 `drainPendingMessage` 自动起一个新 turn |

### 时序图

```
┌──────┐    ┌──────┐    ┌─────────┐    ┌─────┐    ┌────┐
│ Web  │    │  cp  │    │   DB    │    │agent│    │push│
└──┬───┘    └──┬───┘    └────┬────┘    └──┬──┘    └─┬──┘
   │ POST /chat │          │            │         │
   │──────────►│            │           │         │
   │           │  fetch /chat           │         │
   │           │─────────────────────────►        │
   │           │           │            │         │
   │           │   SSE: data: session.started ... │
   │           │◄─────────────────────────────────│
   │           │           │            │         │
   │           │ persist:  │            │         │
   │           │ INSERT session,         │         │
   │           │ UPDATE chat_status=agent │         │
   │           │ INSERT user message     │         │
   │           │─────────►│             │         │
   │           │           │            │         │
   │           │ broadcast:              │         │
   │ data: session.started               │         │
   │◄──────────│           │            │         │
   │           │           │            │         │
   │           │   SSE: data: item.delta ...      │
   │           │◄─────────────────────────────────│
   │           │ broadcast (不落库)                │
   │ data: item.delta                   │         │
   │◄──────────│           │            │         │
   │           │           │            │         │
   │           │   SSE: data: item.completed ... │
   │           │◄─────────────────────────────────│
   │           │ persist:  │            │         │
   │           │ INSERT session_events  │         │
   │           │ UPDATE messages.content │         │
   │           │─────────►│             │         │
   │           │ broadcast:              │         │
   │ data: item.completed                │         │
   │◄──────────│           │            │         │
   │           │           │            │         │
   │           │   SSE: data: session.ended      │
   │           │◄─────────────────────────────────│
   │           │ persist:  │            │         │
   │           │ UPDATE chat_status=human │       │
   │           │ UPDATE last_turn_stats  │        │
   │           │─────────►│             │         │
   │           │ broadcast (延迟入队)              │
   │           │ await queue.flush() ← 等DB写完   │
   │ data: session.ended                 │         │
   │◄──────────│           │            │         │
   │           │           │            │         │
   │           │ drainPendingMessage → 如果有下一条 │
   │           │─────────►│ (递归 Step 1)          │
```

---

## 4. 一句话总结（给新人 review 时握在手里的）

> **SSE 流在平台里的命运：从 agent 吐出来的是字节流 → `readSSE` 切成事件 → `runTurn` 喂给两个插件 → persist 插件按事件种类落 `messages` + `session_events` 表（同 call_id 合并、`<msgId>-<5位序号>` 当主键）→ broadcast 插件把 SSE 原文扇出给所有连到 cp 的前端控制器 → 前端 `runTurn` 再解析一遍把事件转成 `onItemDelta` / `onItemCompleted` 等回调 → UI 流式渲染。**
>
> cp 中间这一刀是整个设计的核心：它把短期不可靠的 agent 流和前端连接解耦，**变成一个可持久化、可重连、可多客户端共享的广播源**。所有"刷新不丢消息"、"cp 重启能恢复"、"多 tab 同步"的能力，都来自这一刀。
>
> 排查 SSE 问题的标准三步：
> 1. **看 `readSSE` 解析对不对** → 帧边界、字段名（`data:` vs `event:`）
> 2. **看 persist 插件的 queue** → DB 写顺序对不对、有没有漏 flush
> 3. **看 broadcast 插件的 controllers** → 前端连接有没有加进 Set、心跳有没有在跑
>
> 加一种新事件类型的标准三步：
> 1. `internal/types/events.ts` 加 `UniversalEvent` 的 type 字面量
> 2. `sse.ts:853` persist 插件的 switch 加 case 决定怎么落库
> 3. 前端 `web/src/lib/api/sse.ts:76` 的 `handlersToPlugin` switch 加 case 决定怎么渲染
>
> 不用改 `readSSE`、不用改 `runTurn`、不用改 DB schema -- 这就是这套分层架构的扩展性边界。
