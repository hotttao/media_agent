---
name: env-runner-internals
description: 内部包 @neutree-ai/env-tunnel 与 @neutree-ai/env-runner-core 的领域文档索引
type: domain-doc
created: 2026-07-06T02:01:58Z
updated: 2026-07-06T02:01:58Z
owner: platform
status: active
---

# `internal/env-*` 领域文档索引

> 顶层设计：cp 写 desired、runner 拉 actual。**隧道** 解决"runner 在 NAT 后面、cp 主动连不进去"的问题；**runner-core** 解决"desired ↔ actual 对不齐"的问题。两个包分别解决**通路**和**控制面**，互不耦合，可独立替换。

## 1. 包职责边界

| 包 | 解决什么问题 | 关键不变量 | 典型使用方 |
|---|---|---|---|
| `@neutree-ai/env-tunnel` | 在一对端（cp↔runner）之间建立一条逻辑隧道，承载任意条独立的双工字节流 + 一类请求/响应控制帧 | 一条连接、若干子流；子流是 Node `Duplex`；mux 层不解析业务字节 | `env-runner-k8s`（远端）、`control-plane` env-gateway |
| `@neutree-ai/env-runner-core` | 跨 `EnvironmentProvider` × `PlacementTransport` 的 reconcile 循环；将 desired 收敛到 actual | reconcile 是幂等的；一次 pass 内 O(1) 远端读 + 至多一次远端写 | `env-runner-k8s`（直接 + 远端） |

> 抓手：先看 **能力边界**，再看**调用方**，最后看**源码**。这三层对齐后，源码就是细节。

## 2. 文档清单

| 文档 | 内容 | 何时看 |
|---|---|---|
| [env-tunnel.md](./env-tunnel.md) | `Mux` 多路复用 + `pipe` TCP 黏合 + `control` 请求/响应帧；含 §3 Mux 代码逐行走读（逐行讲清 `conn.pipe(plex).pipe(conn)` 的"谁链接谁"）、§4 三类使用路径（forward / reverse / control，每条带完整调用栈到 `mux.ts` 行号）、§6 完整握手时序 | 需要理解"为什么 cp 能连到 NAT 后的 workspace"、调试隧道流丢包、改写控制协议、review mux 自举代码 |
| [env-runner-core.md](./env-runner-core.md) | `PlacementTransport` 抽象 + `reconcileOnce` 决策表 + `startReconcileLoop` 生命周期 | 写/改 runner 行为、调对账频率、读懂为什么"已经收敛"了还在写 DB |
| [usage-examples.md](./usage-examples.md) | 端到端示例：cp 推 spec → runner 收到 → reconcile 决策 → 通过隧道回包 | 第一次接触这两个包时按顺序读 |

## 3. 一句话架构图

```
┌──────── control-plane (cp) ────────┐         ┌──── env-runner-k8s (NAT) ────┐
│                                   │         │                               │
│  reconcile (in-process)           │  HTTP    │  env-runner-core             │
│     │                             │  ─────►  │     │                         │
│     ▼                             │ /env/v1  │     ▼                         │
│  PlacementTransport (Http)        │         │  EnvironmentProvider (k8s)   │
│     │                             │         │     │                         │
│     │  byte-level data plane      │   WSS   │     │  in-cluster dial        │
│     └───────────────────────────► │  mux   │ ◄────────────────────────────┘
│           env-gateway WS route    │         │  tunnel-client               │
│                                   │         │     │                         │
│  registry: Mux per env            │ ◄─────  │  Mux (forward + reverse)    │
└───────────────────────────────────┘         └───────────────────────────────┘
```

> 闭环：cp 写 desired → runner 拉 → reconcile 决动作 → 写回 observed → cp 看到收敛 → 下一轮 pass 默认 no-op。
> 颗粒度：mux 在字节层透明，reconcile 在语义层收敛；两层职责**永远不混**。

## 4. 关键不变量（不要破坏）

1. **mux 层不解析子流字节** — 任何业务协议（HTTP / gRPC / 自定义）原样透传。`pipeToTcp` 是唯一允许解 socket 的地方。
2. **mux 子流是 Node `Duplex`** — backpressure 走 Node 自带机制；不要在子流上做半包/合包。
3. **reconcile 必须幂等** — 同样的 desired 输入跑两遍，副作用集合相同。`EnvironmentProvider` 的 `apply/start/stop/destroy` 内部必须各自幂等（K8s 走 SSA 收敛，所以这是天然满足的）。
4. **`recordIfChanged` 是收敛态的护栏** — 没变化就不写 DB。这是为什么一个 50 个 placement 的 runner 在稳态时几乎不打 DB。
5. **控制面与数据面分离** — `Mux.openStream` 打开的是字节子流；`callControl/serveControl` 是在一个字节子流**之上**再叠一层长度前缀+JSON 帧。`afsctl` 是唯一走这层的 stream，**不是** 所有 stream。

## 5. 变更前的自检清单

- [ ] 改 `mux.ts`：是否还保证子流是 `Duplex`？`multiplex` 库的 `id` 语义是否被改变？
- [ ] 改 `control.ts`：`MAX_FRAME = 1 MiB` 是协议契约。改了要同步两端。
- [ ] 改 `reconcile.ts`：是否破坏"stopped 时不 apply spec"的不变量（设计 §13.1）？
- [ ] 加新 `PlacementTransport`：是否覆盖 `listPlacements/writeObserved/deletePlacement/heartbeat` 四个方法？
- [ ] 加新 `EnvironmentProvider`：是否提供 `observeAll` 以享受 O(1) 远端读？

> 拉通：两份代码风格不同（tunnel 是 Node Stream 范式、reconcile 是 controller-pattern 范式），但**边界都画在序列化边界处** — 数据面边界是字节，控制面边界是 JSON 帧，对账面边界是 `PlacementRow`。这条线一旦乱就回不去了。
