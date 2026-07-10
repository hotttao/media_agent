---
name: env-tunnel
description: neutree-ai/env-tunnel 包深度解析 — Mux 多路复用、pipe TCP 黏合、control 请求/响应帧
type: domain-doc
created: 2026-07-06T02:01:58Z
updated: 2026-07-06T03:13:05Z
owner: platform
status: active
---

# `@neutree-ai/env-tunnel` 深度解析

> 底层逻辑：runner 在用户集群里，cp 在我们集群里；NAT 阻断了 cp→runner 的入站连接。**唯一可行的拓扑是 runner 反向拨出**。一旦拨出，就要在这条**单连接**上同时跑数据面（cp→workspace 的 forward）和控制面（runner→cp 的 reverse + 双向 RPC）。这就是 `env-tunnel` 的全部职责。

## 1. 能力边界

| 能力 | 函数 / 类 | 说明 | 外部依赖 |
|---|---|---|---|
| 多路复用 | `Mux`（包装 `multiplex` 库） | 单条 Duplex 连接 → 任意条独立 Duplex 子流；两端皆可开流 | `multiplex@^6.7.0` |
| TCP 黏合 | `pipeToTcp` | mux 子流 ⇋ 远端 TCP socket，任一端断开就双杀 | `node:net` |
| 反向监听 | `listenAndTunnel` | 本地 TCP 端口 → 每连接开一个 mux 子流（mux 不可用时拒连） | `node:net` |
| RPC 帧 | `readFrame` / `writeFrame` | 4 字节大端长度前缀 + JSON body，单流单请求单响应 | — |
| RPC 客户端 | `callControl(stream, req, timeoutMs=15s)` | 写一帧 → 读一帧 → 关流 | — |
| RPC 服务端 | `serveControl(stream, handler, timeoutMs=15s)` | 读一帧 → 调 handler → 写结果 → 关流 | — |

### 1.1 明确**不**做的事

- 不解析子流字节（HTTP/gRPC 透传）
- 不做认证（auth 在外层 WebSocket 握手的 `Authorization: Bearer <env-token>` 处完成）
- 不做 TLS 终止（外层 WS 升级时跑 `wss://`）
- 不做心跳（mux 本身不感知长连接健康度，依赖外层 WS ping/pong）
- 不做重连（重连是 `tunnel-client.ts` 的 `for(;;)` 循环负责）

> 颗粒度：tunnel 只负责**通路**；身份由外层、协议由内层、重连由调用方。三个层级互不打扰。

## 2. 核心心智模型：Mux ⇄ 字节流 ⇄ 业务协议

`Mux` 把一条 Duplex 字节流当作"隧道"，把里面的"逻辑连接"当作**子流**：

```
┌──────────────── cp 进程 ────────────────┐        ┌────────── runner 进程 ──────────┐
│                                          │        │                                   │
│  net.Socket (或 ws)                      │ bytes  │  net.Socket (或 ws)               │
│      │                                   │  WSS   │      ▲                            │
│      ▼                                   │ ─────► │      │ bytes                      │
│  Mux.plex (multiplex)                    │ ◄───── │  Mux.plex (multiplex)             │
│   ┌──────────┬──────────┬──────────┐    │        │   ┌──────────┬──────────┐         │
│   │stream #1 │stream #2 │stream #3 │    │        │   │stream #1 │stream #2 │         │
│   │"fwd:ws1: │"fwd:ws1: │"afsctl"  │    │        │   │"rev:cp"  │"rev:afs" │         │
│   │ 3001"    │ 9101"    │          │    │        │   │          │          │         │
│   └────┬─────┴────┬─────┴────┬─────┘    │        │   └────┬─────┴────┬─────┘         │
│        │ HTTP     │ gRPC     │ RPC帧     │        │        │ bytes   │ bytes          │
│        ▼          ▼          ▼          │        │        ▼         ▼                │
│   pod:3001    pod:9101    runner       │        │   cp:3000  afs-ctrl:9100          │
└──────────────────────────────────────────┘        └───────────────────────────────────┘
```

**关键点**：
- 每个 `Mux` 实例一条**物理连接**；`Mux.openStream(meta)` 开出的子流共享这条连接的带宽，但**互相独立**（流间不阻塞、流内走 Node backpressure）。
- `meta` 是**不透明**的标签字符串。`Mux` 自己不解析它；解析在调用方（`tunnel-dataplane.ts` 用 `^fwd:([^:]+):(\d+)$` 拆 host/port）。
- 两端皆可开流 — forward 走 cp 端 `openStream`，reverse 走 runner 端 `openStream`。

---

## 3. Mux 代码逐行走读（mux.ts 完整 walkthrough）

> 这一节**只看 mux.ts**，把它从第一行到最后一行的所有"链接关系"全部讲清。读完这一节再去读 §4 的三类使用路径，会发现它们只是在 mux 之上贴业务。

### 3.1 一句话：Mux 是"分线器"寄生在"线"上

把 mux.ts 想象成**在 conn（线）上装一台分线器（plex）**：

```
物理层 (net.Socket / WebSocket)
       │
       │  这是 conn —— 一个 Node Duplex（既能读也能写）
       ▼
┌──────────────────────────────────────────────────────┐
│  Mux.plex = multiplex(callback)                      │
│                                                      │
│  callback (stream, id) => {}  ← 对端开流时调用        │
│                                                      │
│  createStream(meta) → 主动开流                        │
└──────────────────────────────────────────────────────┘
       │
       │  这是 plex —— 也是一个 Node Duplex
       │  plex 内部管理一组 substreams（也是 Duplex）
       ▼
业务侧：拿到 substream 后 pipe 给 socket / gRPC client
```

> 抓手：**conn 是线、plex 是分线器、substream 是分出来的支线**。三者都是 Node `Duplex`，可以两两 pipe。

### 3.2 构造时：`conn.pipe(plex).pipe(conn)` 逐行解析

完整看一下 mux.ts 的构造（`mux.ts:22-36`）：

```ts
constructor(conn: Duplex) {
  this.plex = multiplex((stream, id) => {      // ① 建分线器，传入"对端开流"回调
    stream.on('error', () => {})                // ② 静默：被远端 reset 是正常生命周期
    this.onStream?.(stream, String(id))         // ③ 把对端开的流交给我方注册的 onStreamOpen
  })
  conn.pipe(this.plex).pipe(conn)               // ④ 把分线器接进线（双向字节环）
  conn.on('error', () => this.plex.destroy())   // ⑤ 物理层错 → 拆 mux
  conn.on('close', () => this.plex.destroy())   // ⑥ 物理层关 → 拆 mux
}
```

#### 3.2.1 第 ① 行：`this.plex = multiplex(cb)` 在做什么

- `multiplex`（mafintosh 出品）是 `Duplex` 流的工厂
- 调用 `multiplex(cb)` 立刻返回 `plex`（一个 Duplex），同时**注册 `cb` 为"对端开流"钩子**
- `cb: (stream, id) => void` 里：
  - `stream` 是 multiplex 在**本地**新建的子流（也是 Duplex）
  - `id` 是 multiplex 给的字符串标识（对端开流时 echo 过来的 meta）
- `cb` **不在构造时**触发；它在**对端发来"open stream"帧时**触发（详见 §3.3）

#### 3.2.2 第 ④ 行：`conn.pipe(this.plex).pipe(conn)` 是关键

> ★ 这是整个 mux.ts 最难懂的一行。把 `pipe` 的返回值想清楚就通了。

**Node.js 规则**：`a.pipe(b)` 把 `a` 的 readable 接给 `b` 的 writable，**并返回 `b`**。

所以这一行**实际上是两个 pipe 调用串起来**：

```ts
// 第 4 行的实际拆解：
const 返回值1 = conn.pipe(this.plex)   // 返回 plex
const 返回值2 = 返回值1.pipe(conn)     // = plex.pipe(conn)，返回 conn
// 返回值2 被丢弃；副作用是建了 2 条 pipe
```

**两条 pipe 各自的含义**：

| 表达式 | 含义 | 数据方向 |
|---|---|---|
| `conn.pipe(plex)` | 把 conn 收到的字节送给 plex 的可写端 | 远端 wire → plex |
| `plex.pipe(conn)` | 把 plex 产生的字节送回 conn 的可写端 | plex → 远端 wire |

**形象化**：

```
                mux.ts:33
              conn.pipe(plex).pipe(conn)

   conn (Duplex)                  plex (Duplex)
   ┌────────────┐                 ┌────────────┐
   │   r  →  w  │                 │   r  →  w  │
   └──────│─────┘                 └──────│─────┘
          │           ┌──────┐          │
          ├──────────►│ data ├──────────┤
          │  pipe #1  │      │  pipe #2 │
          │           └──────┘          │
          │   ◄────────┤    ├──────────►│
          │                            │
        wire                          wire
```

**更具体地说（一条入站字节的完整路径）**：

```
对端写一字节
  ↓
字节到 wire
  ↓
我方 conn 触发 'data' 事件（line ④ 注册的 listener 自动响应）
  ↓
listener 把字节写到 plex 的 writable 端
  ↓
plex 内部：multiplex 库解析 frame，按 id 路由到对应 substream
  ↓
substream 触发 'data' 事件给它的消费者
  ↓
消费者（业务代码 / pipeToTcp）拿到字节
```

**为什么 plex 自己没有"端口"**：
- plex 是一个 Duplex，**但它没有底层 socket**
- 它的"读"完全靠 conn 喂，plex 的"写"完全靠 conn 消费
- 形象说：plex 是一个**没插电的路由器**，靠 conn 这根线供电 + 传数据

#### 3.2.3 第 ⑤⑥ 行：物理层挂了把 mux 也拆掉

```ts
conn.on('error', () => this.plex.destroy())    // mux.ts:34
conn.on('close', () => this.plex.destroy())    // mux.ts:35
```

- `destroy()` 是 Node Stream 的强拆方法：发 'error' / 'close' 给所有 substream
- 这样 mux 的生命周期 = conn 的生命周期（强一致）
- 没有这两行：conn 断了 plex 还活着，业务继续写 substream → 字节去了虚空

### 3.3 接收流：`(stream, id) => {}` 何时触发

> 这一节回答"对端开了一条流，我方的回调**什么时候**被调用、**拿到的 stream 是什么**"。

**完整时序（对端调 `openStream('fwd:ws1:3001')` 之后）**：

```
对端                                                     我方
  │                                                       │
  │  (1) mux.openStream("fwd:ws1:3001")                   │
  │      → plex.createStream("fwd:ws1:3001")              │
  │      → multiplex 在对端写一个 open frame（带 id）     │
  │                                                       │
  │  (2) 字节经 wire                                      │
  │ ───────────────────────────────────────────────────►  │
  │                                                       │
  │                                            (3) 我方 conn 收到字节
  │                                                       │
  │                                            (4) 自动通过 pipe #1
  │                                                (mux.ts:33 的 conn.pipe(plex))
  │                                                字节被写到 plex 的 writable
  │                                                       │
  │                                            (5) plex 内部解析 frame
  │                                                multiplex 库识别出"对端新开流 #fwd:ws1:3001"
  │                                                       │
  │                                            (6) multiplex 在我方 plex 上 new 一个 substream
  │                                                (这个 substream 跟对端那个编号对应)
  │                                                       │
  │                                            (7) multiplex 调用我方构造时传入的 callback
  │                                                callback 参数 (stream, id):
  │                                                  - stream = 刚 new 的 substream
  │                                                  - id = "fwd:ws1:3001"
  │                                                       │
  │                                            (8) callback 体内:
  │                                                stream.on('error', () => {})         (mux.ts:28)
  │                                                this.onStream?.(stream, String(id))   (mux.ts:29)
  │                                                       │
  │                                                       ▼
  │                                            (9) 我方注册的 onStreamOpen 触发
  │                                                (env-gateway.ts:65 或 tunnel-dataplane.ts:46)
  │                                                       │
  │                                            (10) 我方拿到 stream:
  │                                                - 它是 Node Duplex
  │                                                - 对端写的字节会从 stream.read 出来
  │                                                - 我方写 stream.write 的字节会到对端
```

**关键点**：
- 第 (6) 步的 substream 是 multiplex **本地新建的**对象，跟对端的 substream 是**两个不同对象**
- 它们的内部编号一致（multiplex 内部维护映射），所以 multiplex 自动在它们之间搬运字节
- 我方拿到的 stream **不是对端的 socket** —— 它是 multiplex 在我方进程里虚拟出来的 Duplex
- 第 (8) 步那个 `stream.on('error', () => {})` 看着奇怪但**很重要**：对端 reset 子流时本地会发 'error'，不监听就崩整个进程

### 3.4 主动开流：`openStream(meta)` 做了什么

> 这一节回答"我方调 `mux.openStream('rev:cp')` 时字节是怎么流出去的"。

**完整时序**：

```
我方 (runner)                                            对端 (cp)
  │                                                       │
  │  (1) mux.openStream("rev:cp")                          │
  │      → mux.ts:45: const stream = this.plex.createStream("rev:cp")
  │                                                       │
  │  (2) plex.createStream("rev:cp") 内部：                │
  │      a. multiplex 在 plex 内部 new 一个 substream       │
  │         (分配一个内部 id)                              │
  │      b. multiplex 写一个 open frame 到 plex 的 writable│
  │         (frame 里带 meta="rev:cp" 作为 id)             │
  │                                                       │
  │  (3) plex 的 writable 端有字节要出去                    │
  │      → plex 触发 'data' 事件                            │
  │      → pipe #2 (mux.ts:33 的 plex.pipe(conn))           │
  │        把字节写到 conn 的 writable                     │
  │                                                       │
  │  (4) conn 写字节到 wire                                │
  │ ───────────────────────────────────────────────────►  │
  │                                                       │
  │                                            (5) 对端 conn 收字节
  │                                                       │
  │                                            (6) pipe #1 (对端的 conn.pipe(plex))
  │                                                字节到对端 plex
  │                                                       │
  │                                            (7) 对端 plex 解析 frame
  │                                                识别"对端(我方)新开流 #rev:cp"
  │                                                       │
  │                                            (8) 对端 callback 触发（§3.3 的流程）
  │                                                → 对端 onStreamOpen
  │                                                       │
  │  (5) 我方 openStream 立即返回 stream                    │
  │      stream 是 Node Duplex                             │
  │      我方写 stream.write 的字节会到对端                │
  │      对端写过来的字节会从 stream.read 出来             │
```

**对称性**：
- §3.3 是"对端开流我方收"：`createStream` 在对端触发，callback 在我方
- §3.4 是"我方开流对端收"：`createStream` 在我方触发，callback 在对端
- **两边的 callback 在同一行**（mux.ts:23），同一个 multiplex 实例化机制 —— 所以两端都只需要 `new Mux(conn)` 就够

### 3.5 全局图：一张表说清所有"链接关系"

| 谁 | 类型 | 作用 | 由谁创建 / 怎么传 | 在 mux.ts 哪一行 |
|---|---|---|---|---|
| `conn` | `Duplex` | 物理字节流（线） | 调用方传入（`net.Socket` 或 `createWebSocketStream(ws)`） | `mux.ts:22` 入参 |
| `plex` | `Duplex` | 分线器 | `multiplex(cb)` 工厂创建 | `mux.ts:23` |
| `callback` | 函数 | "对端开流"钩子 | multiplex 内部管理；plex 解析到 open frame 时调用 | `mux.ts:23-30` |
| `substream`（对端开） | `Duplex` | 一条逻辑连接 | multiplex 在 callback 里**本地** new | `mux.ts:23` 形参 `stream` |
| `substream`（我方开） | `Duplex` | 一条逻辑连接 | multiplex 在 `createStream` 里**本地** new | `mux.ts:45` 局部 `stream` |
| pipe `conn → plex` | — | 远端字节 → 分线器 | `conn.pipe(this.plex)` | `mux.ts:33` 第 1 段 |
| pipe `plex → conn` | — | 分线器字节 → 远端 | `this.plex.pipe(conn)` | `mux.ts:33` 第 2 段 |
| pipe `substream ↔ socket` | — | 业务字节 ⇋ TCP | `socket.pipe(s).pipe(socket)` | `pipe.ts:18, 44` |
| pipe `substream ↔ gRPC client` | — | 业务字节 ⇋ gRPC | 由 `@grpc/grpc-js` 内部 pipe；mux 不参与 | — |
| pipe `substream ↔ frame reader` | — | 字节 ⇋ 长度前缀 JSON | `readFrame` 内部 `stream.on('data', ...)` | `control.ts:36-60` |

> 一句话总结：**mux.ts 只做一件事 —— 把 pipe 的两刀接好，让 multiplex 库寄生在 conn 上**。子流具体 pipe 给什么（socket / gRPC / frame parser），是调用方的事。

### 3.6 三条"启动到可工作"的最小代码

#### 最小 1：cp 端（接 runner 拨入）

```ts
// control-plane/src/routes/env-gateway.ts:44-75
async onOpen(_evt, ws) {
  const raw = ws.raw as WsWebSocket
  const conn = createWebSocketStream(raw)       // ① 拿到 Duplex
  conn.on('error', () => {})
  const mux = new Mux(conn)                     // ② mux 自举：plex 寄生在 conn 上
  mux.onStreamOpen((stream, meta) => {          // ③ 注册"对端开流"回调
    const target = reverseTarget(meta)          //    符号枚举 → host:port
    if (!target) { stream.destroy(); return }
    pipeToTcp(stream, target.host, target.port) // ④ 子流 pipe 给 TCP
  })
  registerSession({ environmentId, mux })       // ⑤ 注册到 cp registry
}
```

#### 最小 2：runner 端（拨出 + 主动开 reverse 流）

```ts
// env-runner-k8s/src/tunnel-client.ts:26-44
ws.on('open', () => {
  const conn = createWebSocketStream(ws)
  conn.on('error', () => {})
  const mux = new Mux(conn)                     // ① mux 自举
  mux.onStreamOpen(opts.onStream)               // ② 注册"对端开流"回调（cp 开 fwd）
  resolve({ mux, close: ... })
})
```

```ts
// env-runner-k8s/src/tunnel-dataplane.ts:61-62
listenAndTunnel(CP_PROXY_PORT, '0.0.0.0', () => getMux()?.openStream('rev:cp') ?? null)
// ③ sidecar 连进来时，主动开一条 'rev:cp' 子流
```

#### 最小 3：cp 端主动开 forward 流

```ts
// control-plane/src/services/env-gateway/registry.ts:35-38
export function openForwardStream(envId, meta) {
  const session = sessions.get(envId)
  return session ? session.mux.openStream(meta) : null   // 拿一条子流，pipe 给业务侧
}
```

> 这三段代码背后是同一组机制（§3.2-§3.4），只是"谁开流、谁 pipe、谁消费"的位置不同。

---

## 4. 三类使用路径

> 看这一节时**对照 §3**。每条路径都是 mux 的某个子集在跑。

### 4.1 Forward：cp → workspace pod（**业务字节流**）

> 场景：cp 的 agent-proxy 要把 HTTP/gRPC 请求发到 NAT 后面的 workspace。

```
cp                                                  runner
│                                                       │
│  openForwardStream(envId, "fwd:ws1:3001")              │
│      │                                                 │
│      ▼                                                 │
│  Mux.openStream("fwd:ws1:3001")                        │
│      │  ─── substream bytes over WSS ───►              │
│      ▼                                                 ▼
│  (cp writes HTTP request)               onTunnelStream(s, "fwd:ws1:3001")
│      │                                                 │
│      │                                                 ▼
│      │                                       resolveForwardTarget("fwd:...")
│      │                                                 │   → { host, port }
│      │                                                 ▼
│      │                                       pipeToTcp(s, host, port)
│      │                                                 │
│      │                                                 ▼
│      │                                       net.connect → pod:3001
│      │                                                 │
│  ◄──── substream bytes = HTTP response ◄────  socket.pipe(s).pipe(socket)
```

**关键代码**（`env-runner-k8s/src/tunnel-dataplane.ts`）：
```ts
function resolveForwardTarget(meta: string): { host: string; port: number } | null {
  const m = /^fwd:([^:]+):(\d+)$/.exec(meta)
  if (!m) return null
  const port = Number(m[2])
  if (!FORWARD_PORTS.has(port)) return null          // ★ 白名单
  return { host: `${NAME_PREFIX}-${m[1]}.${NAMESPACE}.svc.cluster.local`, port }
}
```

**白名单**（`tunnel-dataplane.ts:16`）：
```ts
const FORWARD_PORTS = new Set([3001, 9101, 9102])
// 3001 → agent HTTP
// 9101 → afs-fuse gRPC
// 9102 → memory-fuse gRPC
```

> ⚖ 决策：cp 不能任意 `host:port`；白名单限定为 3 个工作端口。否则被劫持 runner 可以以 cp 身份扫内网。

### 4.2 Reverse：runner → cp（**业务字节流**）

> 场景：workspace 内的 sidecar（afs-controller、memory-fuse）需要回调 cp 内部服务。sidecar 不会自己跨 NAT — 它走 runner 暴露的**本地 TCP 端口**，由 runner 把这些本地连接"翻译"成 mux 反向流。

> ★ 为什么 reverse 要走 tunnel，而不是 sidecar 直接连 cp？因为 **cp 连不到 runner**（NAT 挡入站），而 sidecar 在 workspace pod 内同样跨 WAN 连不到 cp。
>
> | 谁连谁 | 能直连吗 | 怎么走 |
> |---|---|---|
> | runner -> cp（reconcile 拉 desired / 写 observed / heartbeat） | ✅ 能（出站，NAT 放行） | 直接 HTTP，`HttpTransport` 调 `/env/v1`，**不走 tunnel** |
> | cp -> workspace pod（访问 agent:3001 等） | ❌ 不能（NAT 挡入站） | 借 runner 拨出的 WSS 做 forward 子流（§4.1） |
> | sidecar -> cp（回调） | ❌ 不能（跨 WAN） | 连 runner 本地端口，runner 翻译成 reverse 子流经 WSS 回 cp（本节） |
>
> runner 拨出的 WSS 是**全双工**的，一旦建立两端都能发字节。reverse 流就是让 sidecar 借这条已有连接把请求搭便车送回 cp。**tunnel 是给 cp 用的，不是给 runner 用的**--runner 自己连 cp 直接 HTTP 就够（`index.ts:76` `HttpTransport`）；cp 连不到 runner，才需要借 runner 主动拨出的 WSS（`index.ts:77` `startTunnel`）做反向通道。两者的关系见下图：
>
> ```
>         NAT（客户集群边界）
>             │
> runner (客户内网)  ─── 出站 WSS ═══════════►  cp (公网)
>                      ◄══════════════════  cp 借这条连接把 forward 请求推回
>                      ═══════════════════►  runner 把 sidecar 请求经这条连接送回
> ```
>
> 颗粒度：`HttpTransport`（reconcile）和 `startTunnel`（数据面）是 `index.ts:69-77` 里**两个独立的并行调用**，不要混为一谈。reconcile 的 HTTP 控制流不走 tunnel，tunnel 只承载 forward/reverse/control 的实时字节流。

```
sidecar (in pod)                runner pod                    cp process
     │                              │                              │
     │  TCP connect 127.0.0.1:38000 │                              │
     ├─────────────────────────────►│                              │
     │                              │ listenAndTunnel(38000,...)  │
     │                              │   on(socket) {              │
     │                              │     const s = currentMux     │
     │                              │            .openStream("rev:cp") │
     │                              │     socket.pipe(s).pipe(socket)
     │                              │   }                          │
     │                              │ ── substream "rev:cp" ──►    │
     │                              │                              ▼
     │                              │                  onStreamOpen(s,"rev:cp")
     │                              │                              │ reverseTarget("rev:cp")
     │                              │                              │   → "127.0.0.1:3000"
     │                              │                              ▼
     │                              │                  pipeToTcp(s, "127.0.0.1", 3000)
     │                              │                              │
     │                              │                              ▼
     │                              │                  net.connect → cp:3000
     │                              │                              │
     │  HTTP resp ◄───────────────  │  bytes ◄─────────────────── │
     │                              │                              │
```

**关键代码**（`env-runner-k8s/src/tunnel-dataplane.ts:59`）：
```ts
export function startReverseListeners(getMux: () => Mux | null): Server[] {
  return [
    listenAndTunnel(CP_PROXY_PORT, '0.0.0.0', () => getMux()?.openStream('rev:cp') ?? null),
    listenAndTunnel(AFS_PROXY_PORT, '0.0.0.0', () => getMux()?.openStream('rev:afs') ?? null),
  ]
}
```

> ★ 关键设计：`openStream` 是**惰性**的（每次连接才调），且能返回 `null`。mux 还没握上时 sidecar 连进来 → `socket.destroy()`。**这让 reverse 监听器在 mux 重连时**不需要重绑端口**（参见 §7.1）。

**cp 端的 reverse 目标**（`control-plane/src/routes/env-gateway.ts:14`）：
```ts
const CP_INTERNAL_ADDR  = '127.0.0.1:' + PORT      // rev:cp
const AFS_CONTROLLER_ADDR = 'afs-controller.default.svc:9100'  // rev:afs
```
两地址都是**符号常量**。`meta` 字符串不是 `host:port`，是枚举值。

#### 4.2.1 完整调用栈（reverse 路径，对照 mux.ts 逐行）

> 把"sidecar 推一条状态"这个动作从第一行到最后一行的调用链拉直，**每一步都标文件:行号**。

**runner 端（主动开流的一侧）**：

| 步骤 | 文件:行 | 代码 | 在做什么 |
|---|---|---|---|
| 1 | `pipe.ts:31` | `net.createServer((socket) => { ... })` | 收到 sidecar 的 TCP 连接 |
| 2 | `pipe.ts:32` | `const stream = openStream()` | 调 runner-core 传入的工厂 |
| 3 | `tunnel-dataplane.ts:61` | `() => getMux()?.openStream('rev:cp') ?? null` | 工厂返回 `Mux.openStream('rev:cp')` |
| 4 | `mux.ts:44-48` | `openStream(meta)` 入口 | — |
| 5 | `mux.ts:45` | `this.plex.createStream('rev:cp')` | **★ 核心：让 multiplex 库建子流 + 写 open frame** |
| 5a | (multiplex 库内部) | 分配内部 id, new 一个 substream | substream 是本地对象 |
| 5b | (multiplex 库内部) | 写一个 `OPEN id=rev:cp` frame 到 plex 的 writable | 字节**此时**已经准备好要出去 |
| 5c | (multiplex 库内部) | multiplex 库的 `pipe #2` 钩子 | plex 触发 'data' 事件 |
| 6 | `mux.ts:33` 第二段 | `this.plex.pipe(conn)` | 自动把 plex 字节写回 conn |
| 7 | (Node Stream 内部) | conn.write(bytes) | 字节写入底层 ws / socket |
| 8 | (wire) | 字节发到对端 | — |
| 9 | `mux.ts:46` | `stream.on('error', () => {})` | 静默：子流生命周期错误不崩进程 |
| 10 | `mux.ts:47` | `return stream` | 工厂返回这个 substream |
| 11 | `pipe.ts:44` | `socket.pipe(stream).pipe(socket)` | sidecar 字节 ⇋ substream 字节 |
| 12 | (后续) | sidecar 发数据 → stream.write → plex → wire | 反向同理 |

**cp 端（接收对端开流的一侧）**：

| 步骤 | 文件:行 | 代码 | 在做什么 |
|---|---|---|---|
| 1 | (wire) | 字节到 cp 端 conn | — |
| 2 | `mux.ts:33` 第一段 | `conn.pipe(this.plex)` | conn 'data' 自动写到 plex |
| 3 | (multiplex 库内部) | plex 解析 `OPEN id=rev:cp` frame | — |
| 4 | (multiplex 库内部) | multiplex 在我方 plex 上 new 一个对应 substream | 编号与对端 substream 对应 |
| 5 | `mux.ts:23` | multiplex 调用我方构造时传入的 callback | `callback(stream, id) => {...}` |
| 6 | `mux.ts:28` | `stream.on('error', () => {})` | 静默 |
| 7 | `mux.ts:29` | `this.onStream?.(stream, String(id))` | 触发我方注册的 onStreamOpen |
| 8 | `env-gateway.ts:65` | `mux.onStreamOpen((stream, meta) => { ... })` | 我方注册的处理函数 |
| 9 | `env-gateway.ts:66-70` | `reverseTarget(meta) → { host, port }` | 符号枚举转地址 |
| 10 | `env-gateway.ts:71` | `pipeToTcp(stream, target.host, target.port)` | **★ 核心：substream ⇋ TCP** |
| 11 | `pipe.ts:11` | `net.connect({ host, port })` | cp 主动连本地 cp-internal 服务 |
| 12 | `pipe.ts:18` | `socket.pipe(stream).pipe(socket)` | TCP socket 字节 ⇋ substream 字节 |

> ★ 整条 reverse 链路的"链接"动作集中在 `mux.ts:33`（mux 自举的两刀）+ `mux.ts:45`（我方开流）+ `mux.ts:23`（对端开流触发 callback）+ `pipe.ts:18`（业务 pipe）。
> 看完这张表，`plex.pipe` 的角色就清楚了：**它是把"我要发给对端"的字节**主动推到 conn** 的自动装置**。

### 4.3 Control：双向 RPC（**长度前缀 + JSON 帧**）

> 场景：cp 委托 runner 调远端 afs-controller（CP 无法跨 WAN 触达用户集群的 RWX 数据面）。

```ts
// cp 侧 (env-gateway 之外的业务代码)
import { callControl } from '@neutree-ai/env-tunnel'
import { openControlStream } from '.../registry'

async function mountAfs(envId: string, wsId: string, dirId: string) {
  const stream = openControlStream(envId, 'afsctl')   // 拿一条子流
  if (!stream) throw new Error('runner offline')
  const res = await callControl<{ ok: true }>(stream, {
    op: 'mount',
    workspaceId: wsId,
    dirId,
    name: 'home',
    accessKey: '...',
    readonly: false,
  }, 15_000)                                          // 15s 超时
  return res
}
```

**runner 侧**（`env-runner-k8s/src/afs-control.ts`）：
```ts
export function handleAfsControl(stream: Duplex): void {
  void serveControl(stream, dispatch)  // dispatch 把 op 翻译成 gRPC 调用
}
```

**帧格式**：
```
┌──────────── 4 bytes (BE) ────────────┬────────── body bytes ──────────┐
│  body length (uint32, ≤ 1 MiB)       │  JSON payload (utf8)           │
└──────────────────────────────────────┴────────────────────────────────┘
```

**单流单请求单响应**：调用完 `callControl` 后 `stream.destroy()`；调用完 `serveControl` 后 `stream.end()`。**不复用**流。

> 为什么是 RPC 而不是 forward 字节流？因为 afs gRPC 调用需要**应答**（mount 成功/失败 + 错误信息），raw byte pipe 不携带语义。`callControl` + `serveControl` 是**在 mux 子流之上**的薄协议，仅这一处使用。

#### 4.3.1 完整调用栈（control 路径，对照 mux.ts 逐行）

> 把"cp 委托挂载 afs 目录"从 cp 入口到 runner gRPC 调用再到 cp 收到响应的完整链路拉直。

**cp 端（开流 + 写请求帧）**：

| 步骤 | 文件:行 | 代码 | 在做什么 |
|---|---|---|---|
| 1 | (业务代码) | `openControlStream(envId, 'afsctl')` | 拿一条 'afsctl' 子流 |
| 2 | `registry.ts:45-48` | `session.mux.openStream('afsctl')` | 调 `Mux.openStream` |
| 3 | `mux.ts:44-48` | `openStream('afsctl')` | 入口 |
| 4 | `mux.ts:45` | `this.plex.createStream('afsctl')` | **★ mux 自举的一刀**：开子流 + 写 open frame |
| 5 | (multiplex 库内部) | 字节经 plex.pipe(conn) → conn → wire | 同 §4.2.1 步骤 5-8 |
| 6 | `control.ts:64-69` | `writeFrame(stream, req)` | **★ 控制面写入**：长度前缀 + JSON |
| 6a | `control.ts:65-66` | `Buffer.from(JSON.stringify(value), 'utf8')` | 序列化 |
| 6b | `control.ts:67-68` | `header.writeUInt32BE(body.length, 0)` | 写 4 字节大端长度 |
| 6c | `control.ts:68` | `stream.write(Buffer.concat([header, body]))` | **写到 substream** |
| 7 | (multiplex 库内部) | substream.write → plex 内部加 frame header → plex 输出 | 自动 |
| 8 | (multiplex 库内部) | plex.pipe(conn) → conn.write | 自动 |
| 9 | (wire) | 字节到 runner | — |
| 10 | `control.ts:80-87` | `await readFrame(stream, timeoutMs)` | **★ 控制面读取**：阻塞读一帧 |
| 11 | `control.ts:86` | `stream.destroy()` | 读完关流 |

**runner 端（收流 + 读请求帧 + 调 handler + 写响应帧）**：

| 步骤 | 文件:行 | 代码 | 在做什么 |
|---|---|---|---|
| 1 | (wire) | 字节到 runner 端 conn | — |
| 2 | `mux.ts:33` | `conn.pipe(this.plex)` | conn 'data' 自动写到 plex |
| 3 | (multiplex 库内部) | plex 解析 `OPEN id=afsctl` frame | — |
| 4 | (multiplex 库内部) | multiplex new substream，调用 callback | — |
| 5 | `mux.ts:23` | `callback(stream, id) => { ... }` | 触发 |
| 6 | `mux.ts:28` | `stream.on('error', () => {})` | 静默 |
| 7 | `mux.ts:29` | `this.onStream?.(stream, 'afsctl')` | 触发我方 onStreamOpen |
| 8 | `tunnel-dataplane.ts:47-50` | `if (meta === 'afsctl') handleAfsControl(stream)` | 路由 afs control 流 |
| 9 | `afs-control.ts:114-116` | `serveControl(stream, dispatch)` | 入口 |
| 10 | `control.ts:95-113` | `serveControl` 整体 | 读一帧 → 调 handler → 写结果 |
| 11 | `control.ts:101` | `await readFrame(stream, timeoutMs)` | 读请求帧 |
| 12 | `afs-control.ts:71-111` | `dispatch(req)` | 把 op 翻译成 gRPC 调用 |
| 12a | `afs-control.ts:94-103` | `case 'mount': await unary(fuse(req.workspaceId), 'Mount', ...)` | 调 pod 内 afs-fuse gRPC |
| 13 | `control.ts:108` | `writeFrame(stream, res)` | **★ 写响应帧**：4 字节长度 + JSON |
| 14 | (multiplex 库内部) | substream.write → plex → conn → wire | 自动 |
| 15 | `control.ts:109` | `stream.end()` | 写完关流（半关闭） |

**cp 端（读响应帧）**：

| 步骤 | 文件:行 | 代码 | 在做什么 |
|---|---|---|---|
| 1 | (wire) | 字节到 cp 端 conn | — |
| 2 | `mux.ts:33` | `conn.pipe(this.plex)` | 同上 |
| 3 | (multiplex 库内部) | 解析 frame，按 id 路由到 'afsctl' substream | — |
| 4 | (multiplex 库内部) | substream 'data' 事件触发 | — |
| 5 | `control.ts:36-57` | `readFrame.onData(chunk)` | 读字节累加 |
| 6 | `control.ts:46-48` | `need = buf.readUInt32BE(0)` | 解析 4 字节长度 |
| 7 | `control.ts:49-55` | `total >= need + 4` → JSON.parse | 解析 body |
| 8 | `control.ts:83-84` | `return res as Res` | 返回给业务 |
| 9 | `control.ts:86` | `stream.destroy()` | 关闭 substream |
| 10 | (multiplex 库内部) | substream 关闭 → 写一个 close frame → 对端收 | — |

> ★ Control 路径在 mux 之上**额外叠了一层帧协议**（`control.ts`），不复用任何 forward/reverse 的代码。
> 这条路径的"链接"动作集中在：`mux.ts:45`（开流）+ `mux.ts:23`（收流触发 callback）+ `control.ts:64-69`（写帧）+ `control.ts:36-57`（读帧）+ `control.ts:108`（写响应）+ `control.ts:83-84`（返回响应）。

---

## 5. 关键技术点

### 5.1 Node Stream 范式 + `multiplex` 库

**为什么不自己写**：
- 字节级分帧 + 滑动窗口 + 流控 = 1-2k 行高质量代码，bug 风险极高
- `multiplex`（mafintosh 出品）已被 Cloudflare 等大厂用多年
- 它的产物就是 Node `Duplex`，意味着零适配层

**怎么用**（`mux.ts`，已逐行走读于 §3）：
```ts
constructor(conn: Duplex) {
  this.plex = multiplex((stream, id) => {       // 远端开流 → 触发回调
    stream.on('error', () => {})                // ★ 静默：被远端 reset 是正常生命周期
    this.onStream?.(stream, String(id))
  })
  conn.pipe(this.plex).pipe(conn)               // 双向字节管（§3.2.2 详述）
  conn.on('error', () => this.plex.destroy())
  conn.on('close', () => this.plex.destroy())
}
```

**两个地方要吞 error**（注释明确说"never let it crash"）：
- mux 创建的子流被远端 reset → 触发 `error` 事件
- 不监听会变成 unhandled rejection，把整个进程拉爆
- 调用方（`pipeToTcp`）自己再 attach teardown handler 做双杀

### 5.2 `meta` 作为**不透明路由键**

- `Mux` 完全不解析 `meta`，只 echo 给对端
- **两端必须就格式达成共识**。forward 是 `fwd:<wsId>:<port>`，reverse 是 `rev:cp` / `rev:afs`，control 是 `afsctl`
- 改格式？必须**同步** `tunnel-dataplane.ts`（runner）+ `env-gateway.ts`（cp）
- runner 端有**显式 allowlist**（`FORWARD_PORTS`），cp 端有**显式 reverseTarget**；两边都是"白名单 + 拒绝"，不是"黑名单 + 放行"

### 5.3 控制帧为什么用长度前缀

- 子流本身是字节流，**无消息边界**（TCP 是流协议）
- 必须告诉对端"读多少算一帧"
- 4 字节大端 uint32 = 最多 4 GiB；**实际限 1 MiB**（`MAX_FRAME = 1 << 20`）
- 为什么限 1 MiB？control 消息本质是 afs 元数据（mount/unmount 几个字段）；1 MiB 已经是 1000x 余量。**这是协议契约** — 改它要同步两端，且必须报"too large"而不是"截断"

### 5.4 控制帧为什么不用现成 RPC 框架

| 候选 | 否决理由 |
|---|---|
| gRPC | 太重，proto + codegen；而且我们已经在 tunnel 里**承载 gRPC 业务**（afs），框架会嵌套 |
| JSON-RPC | 多一层解析（id / method / params），但我们**没有多路复用请求**的需求 — 一流一请求 |
| MessagePack / Protobuf | 跨语言 OK 但增加依赖；JSON 调试极方便 |

> 终极答案：**协议最简化 = 1 个长度前缀 + JSON**。调试时 `tcpdump | xxd` 直接看。

### 5.5 mux ⇄ WebSocket 的 Duplex 适配

WebSocket 是消息流，但业务需要字节流（HTTP/gRPC/JSON 都是字节）。`ws` 库提供 `createWebSocketStream(ws)` 把消息流转成 `Duplex`：

```ts
const ws = new WebSocket(gatewayUrl, { headers: ... })
ws.binaryType = 'nodebuffer'                    // ★ 必须，否则 frame 是 string
const conn = createWebSocketStream(ws)          // → Duplex
const mux = new Mux(conn)                       // mux 看到的就是字节流
```

> 不设置 `binaryType = 'nodebuffer'` 会怎样？ws 库默认给 string frame；pipe 进 mux 时 toString 一次再回 buffer → 字节被 UTF-8 编码，**二进制协议全部坏掉**。

## 6. 端到端握手时序

```
runner                                                       cp
  │                                                            │
  │  WS upgrade: GET /env-gateway                              │
  │  Authorization: Bearer <env-token>                         │
  │ ──────────────────────────────────────────────────────►   │
  │                                                            │
  │                                                  verifyEnvironmentToken
  │                                                            │
  │  101 Switching Protocols                                    │
  │ ◄──────────────────────────────────────────────────────    │
  │                                                            │
  │  createWebSocketStream(ws) + new Mux(conn)                 │
  │ ────── mux 子流可开 ──────────────────────────────────►   │
  │                                                            │
  │  registerSession({ envId, mux })                            │
  │  onStreamOpen = (s, meta) => pipeToTcp(s, host, port)       │
  │                                                            │
  │  startReverseListeners(getMux)                             │
  │  listenAndTunnel(38000, "0.0.0.0", ...)                    │
  │  listenAndTunnel(39100, "0.0.0.0", ...)                    │
  │                                                            │
  │  ✅ 数据面 ready                                            │
```

> 失败处理：`unexpected-response` 立即 reject，runner 进入 backoff 重连。

## 7. 重连时的行为（设计 §6）

`env-runner-k8s/src/index.ts:39` 的 `startTunnel` 是经典"重连 for 循环"：

```ts
function startTunnel(cpUrl, token) {
  let currentMux: Mux | null = null
  startReverseListeners(() => currentMux)    // ★ 闭包：每次取当前 mux

  const run = async () => {
    for (;;) {
      try {
        const closed = new Promise<void>((resolve) => {
          connectTunnel({ ... , onClose: resolve })
            .then(c => { currentMux = c.mux })
            .catch(err => { resolve() })    // 连接失败也算"结束"
        })
        await closed
      } finally {
        currentMux = null                    // 断 → mux 不可用
      }
      await sleep(TUNNEL_RECONNECT_MS)       // backoff
    }
  }
  void run()
}
```

**为什么 reverse 监听器不需要重绑**：
- `listenAndTunnel` 接受一个 `openStream` **工厂函数**（不是 stream 实例）
- sidecar 每次连进来时才调用 `getMux()?.openStream('rev:cp')`
- mux 断时 `getMux()` 返回 `null` → `listenAndTunnel` 内部 `socket.destroy()`
- mux 重连后下一个 sidecar 连接会用**新的** mux 开流
- **端口永远活着**，但承载的 mux 一直在换

## 8. 安全模型（设计 §9）

| 攻击面 | 缓解 |
|---|---|
| 恶意 runner 调任意内网 | `reverseTarget` 是符号枚举（`rev:cp` / `rev:afs`），不接受任意 `host:port` |
| cp 假冒 runner 主动开 forward | cp 端 `openForwardStream` 也走 `reverseTarget` 同样的 allowlist；runner 端 `FORWARD_PORTS` 限制 workspace 暴露的 3 个端口 |
| 恶意 runner 在 control 帧发超长 body | `MAX_FRAME = 1 MiB` 硬截断 + 报错 |
| 重放 env-token | token 在 cp 端有 TTL；旋转由 cp 单独流程负责，tunnel 不参与 |
| 长时间不发数据被中间设备 reset | 隧道层不负责；外层 WS ping/pong 由 reverse proxy / k8s liveness 负责 |

> 闭环：白名单在两端独立维护，**任一端失守另一端仍能挡住**（纵深防御）。

## 9. 调试 Checklist

| 现象 | 第一时间看 |
|---|---|
| cp 看不到 workspace | `mux` 是否就绪？`registry` 里有没有对应 envId？`meta` 格式对吗？ |
| sidecar 连不上 runner | `startReverseListeners` 是否被调用？端口是否被 Service 暴露？`getMux()` 返回 null？ |
| 控制帧超时 | `MAX_FRAME` 是否被突破？`handler` 内部是否同步阻塞？JSON 是否被序列化坏？ |
| 重连风暴 | `TUNNEL_RECONNECT_MS` 设置？后端 WS 服务端是否健康？env-token 是否过期？ |
| 子流卡住不关 | `pipeToTcp` 的双杀回调是否被 attach？socket 的 `end` / `close` 是否触发？ |

## 10. 关键代码索引

| 关注点 | 文件:行 |
|---|---|
| `Mux` 类（包装 `multiplex`，含逐行走读） | `internal/env-tunnel/mux.ts:18-53`，见 §3 |
| `Mux` 构造：plex 自举的两刀 pipe | `internal/env-tunnel/mux.ts:33` |
| `Mux` 构造：callback 钩子注册 | `internal/env-tunnel/mux.ts:23-30` |
| `Mux.openStream`：我方开流 | `internal/env-tunnel/mux.ts:44-48` |
| `Mux.close`：拆 mux | `internal/env-tunnel/mux.ts:50-52` |
| `multiplex` ambient 声明 | `internal/env-tunnel/multiplex.d.ts:3-18` |
| `pipeToTcp`（子流 ⇋ TCP） | `internal/env-tunnel/pipe.ts:10-19` |
| `listenAndTunnel`（本地端口 → 子流） | `internal/env-tunnel/pipe.ts:26-48` |
| `readFrame` / `writeFrame` | `internal/env-tunnel/control.ts:16-69` |
| `callControl`（客户端） | `internal/env-tunnel/control.ts:76-88` |
| `serveControl`（服务端） | `internal/env-tunnel/control.ts:95-113` |
| cp 端 WS 升级 + mux 注册 | `control-plane/src/routes/env-gateway.ts:32-89` |
| cp 端 registry（按 envId 查 mux） | `control-plane/src/services/env-gateway/registry.ts:11-48` |
| runner 端 tunnel-client（拨出） | `env-runner-k8s/src/tunnel-client.ts:17-51` |
| runner 端数据面（dispatch + reverse） | `env-runner-k8s/src/tunnel-dataplane.ts:23-64` |
| runner 端 afs-control（接 `afsctl` 流） | `env-runner-k8s/src/afs-control.ts:114-116` |
| runner 端重连循环 | `env-runner-k8s/src/index.ts:39-66` |
