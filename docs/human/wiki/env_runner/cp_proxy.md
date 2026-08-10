---
name: cp-proxy
description: cp 数据面 forward proxy（reconcile cron / env-projection / remote-proxy / workspace-address）与 executeChat 请求 proxy 的时序分析
type: domain-doc
created: 2026-07-14T05:45:03Z
updated: 2026-07-14T06:05:42Z
owner: platform
status: active
---

# cp 数据面 forward proxy

> 顶层设计：cp 代码用 `fetch(addr)` 打 workspace 的 agent。built-in workspace 走集群 DNS；remote (BYOI) workspace 的 pod 在 NAT 后面、集群 DNS 够不着，于是 cp 在本地起一个 `127.0.0.1:<port>` 的 TCP 转发代理，把每条连接 pipe 进 env-gateway 隧道直达 pod。这条 proxy 的生命周期由 reconcile 里的一把 15s cron 驱动 `env-projection` 投影循环--reachable+running 时建、其余状态拆。

本文回答三个具体疑问：**(a)** Cron/startReconcileLoop 的启动流程（§2）；**(b)** 代码如何区分 builtin 与 remote、built-in 模式会不会误建 remote proxy（§3）；**(c)** `executeChat` fetch 时 proxy 会不会还没启动（§7，结论在前，推导见 §4–§6）。

## 1. 代码地图

| 文件 | 角色 | 关键导出 |
|---|---|---|
| `control-plane/src/lib/reconcile.ts` | 启动入口 + 所有 cron 的注册处；含 built-in 的 list+watch 循环 | `startReconcileLoop` / `fullReconcile` / `listWatchCycle` |
| `control-plane/src/services/env-projection.ts` | 投影循环：把 runner 上报的 `observed_phase` + env 心跳收敛成 cp 侧 workspace 状态，并同步 forward proxy | `runEnvProjection(thresholdSec)` |
| `control-plane/src/services/db/environments.ts` | builtin/remote 的唯一裁决者：`is_builtin` 列上的所有过滤查询 | `listRemoteWorkspaceObservations` / `getRemoteWorkspaceIds` / `markStaleEnvironmentsOffline` |
| `control-plane/src/lib/remote-proxy.ts` | forward proxy 本体：in-process Map 管理每个 remote workspace 的 localhost TCP 转发 | `ensureRemoteProxy` / `dropRemoteProxy` / `getRemoteProxyPort` |
| `control-plane/src/lib/workspace-address.ts` | 路由缝合点（design §6）：决定 fetch 走集群 DNS 还是本地 proxy | `getWorkspaceAddress` / `resolveAgentAddress` / `postToAgent` |
| `control-plane/src/services/chat/executeChat.ts` | chat 回合入口：先确保 workspace running，再 POST agent `/chat` | `executeChat` / `drainPendingMessage` |
| `control-plane/src/services/workspace-autostart.ts` | 自动拉起 stopped/starting workspace 直到 `/health` 通过 | `ensureWorkspaceRunning` |

> 抓手：先看 **谁驱动**（reconcile cron）、**谁建 proxy**（projection）、再看 **谁读 proxy**（workspace-address）、最后看 **谁触发请求**（executeChat）。四层对齐后，"会不会 race""built-in 会不会误建"都是时序/过滤题。

## 2. 启动与驱动：startReconcileLoop 与 Cron

`startReconcileLoop()`（`reconcile.ts:153`）在 `index.ts:513` cp 启动时调用**一次**。它用 `croner` 的 `Cron`（6 字段含秒）注册若干周期作业，再起一个常驻 `while(true)` 循环跑 list+watch：

| Cron 表达式 | 周期 | 选项 | 函数 | 干什么 |
|---|---|---|---|---|
| `*/10 * * * * *` | 每 10s | - | `reconcileDeletingCredentials` | 清理 deleting 状态的用户凭证 |
| `*/30 * * * *` | 每 30 分钟 | `protect:true` | `sweepRunningWorkspaces` | 从 running agent 拉 per-turn usage 写账本（per-turn pull 的兜底） |
| `0 * * * *` | 每小时 | gated `IDLE_WORKSPACE_GC_DAYS>0` | `runIdleWorkspaceGC` | 停掉长期空闲 workspace 回收资源 |
| **`*/15 * * * * *`** | **每 15s** | **`protect:true`** | **`runEnvProjection(60)`** | **本系列主角：投影 remote 状态 + 建/拆 forward proxy** |

常驻循环：`listWatchCycle` = `fullReconcile`（全量拉 k8s Deployment 对账 built-in workspace 状态）+ `watchDeployments`（监听变化事件）持续 `WATCH_CYCLE_MS=10min` 后重建。

与本问最相关的是第 4 把 cron：

- `ENV_PROJECTION_INTERVAL = '*/15 * * * * *'` -- 每 15 秒触发一轮 `runEnvProjection`
- `ENV_HEARTBEAT_TIMEOUT_SEC` 默认 60 -- runner 60s 没心跳 -> env offline
- `protect:true` -- 上一轮没跑完时跳过本次，不堆积

> 拉通：**proxy 的创建不是请求触发的，而是这把 15s cron 周期性扫出来的。** `ensureRemoteProxy` 的调用方只有 `runEnvProjection` 一处，`runEnvProjection` 的调用方只有这把 cron。链路单向：`cron -> runEnvProjection -> ensureRemoteProxy -> proxies Map`。executeChat 只读 Map，不触发创建。

## 3. builtin vs remote 的区分（is_builtin 红线）

区分的唯一标尺是 `environments.is_builtin` 布尔列：

- `is_builtin = true`：built-in 环境，由 migration 种子。API 永远不能创建/改/删它（`createEnvironment` 写死 `false`，`updateEnvironment` / `deleteEnvironment` 都带 `WHERE is_builtin = false`）。
- `is_builtin = false`：remote (BYOI) 环境，用户通过 API 注册，runner 连上来后心跳。
- 无 placement 行的 legacy workspace：`getWorkspacePlacementEnv` 返回 `null` -> "treat as built-in"（走集群 DNS）。

所有 remote 相关查询都以 `is_builtin = false` 为过滤条件：

| 查询 | 过滤条件 | 作用 |
|---|---|---|
| `listRemoteWorkspaceObservations` | `WHERE e.is_builtin = false AND p.desired_phase <> 'deleted'` | 投影的输入：所有 remote ws 的 observed_phase + env 心跳新鲜度 |
| `markStaleEnvironmentsOffline` | `WHERE is_builtin = false AND status = 'online' AND 心跳过期` | 把失联 env 翻成 offline |
| `getRemoteWorkspaceIds` | `WHERE e.is_builtin = false` | 给 watch-k8s 用：跳过这些 ws |
| `listReapableWorkspaces` | `status='deleting' AND 无 placement` | built-in 删除是同步的、永不进 deleting，所以这把只收 remote 反向删除 |

`fullReconcile`（watch-k8s 路径）显式跳过 remote：

```ts
const remoteIds = await getRemoteWorkspaceIds()
for (const ws of allWorkspaces) {
  if (remoteIds.has(ws.id)) continue   // remote 在 cp 集群没 Deployment
  if (ws.status === 'deleting') continue   // 投影负责收尾，别让 k8s clobber 成 stopped
  const dep = deployments.get(ws.id)
  const resolved = k8s.resolveDeploymentStatus(dep)
  ...
}
```

> 闭环：**两条状态来源井水不犯河水** -- built-in workspace 由 watch-k8s（`fullReconcile` + `watchDeployments`）驱动状态；remote workspace 由 env-projection cron 驱动状态。`is_builtin` 是这条红线的唯一裁决者，proxy 只可能出现在 remote 这条线上。

### built-in 模式下，startReconcileLoop 会创建 remote proxy 吗？

**不会。** 在纯 built-in 部署（没有任何 `is_builtin=false` 的环境）下，`runEnvProjection` 每轮：

1. `markStaleEnvironmentsOffline` -> `[]`（无 remote env 可翻）
2. `listReapableWorkspaces` -> `[]`（built-in 删除同步、永不进 `deleting`）
3. `listRemoteWorkspaceObservations` -> `[]`（无 remote 行匹配 `is_builtin = false`）
4. for 循环遍历 `[]` -> **既不调用 `ensureRemoteProxy`，也不调用 `dropRemoteProxy`**

`proxies` Map 永远空，`getRemoteProxyPort` 永远返回 `undefined`，`getWorkspaceAddress` 永远走集群 DNS。正如 `env-projection.ts:16` 注释所说："Cheap no-op when there are no remote environments."

> 颗粒度：cron 的"触发"和 proxy 的"创建"是两回事。触发永远发生（15s 一次），创建只在有 remote workspace 且 reachable+running 时发生。built-in 模式下，触发照常、创建为零。

## 4. env-projection：投影循环（proxy 的唯一创建方）

`runEnvProjection(thresholdSec)` 一轮三步：

1. **心跳裁剪** `markStaleEnvironmentsOffline(thresholdSec)` -- 心跳过期的 remote env 标 offline，其下所有 workspace 状态塌成 `unknown`。
2. **收割远端反向删除** `listReapableWorkspaces()` -- runner 已销毁 pod 并移除 placement 时，cp 这边收尾：`dropRemoteProxy(wsId)` -> `deleteWorkspace(wsId)`（CASCADE 删 config/sessions/schedules）。
3. **逐个 workspace 投影** `listRemoteWorkspaceObservations(thresholdSec)` -- 每条观察：

```ts
const status = o.env_offline ? 'unknown' : mapObservedToStatus(o.observed_phase)
const ws = await getWorkspace(o.workspace_id)
if (ws && ws.status !== status) {
  await applyStatusChange(o.workspace_id, status, ws.status)   // ① 先写 DB 状态
}
// Forward proxy lifecycle
if (!o.env_offline && status === 'running') {
  await ensureRemoteProxy(o.workspace_id, o.environment_id)     // ② 紧随其后建 proxy
} else {
  dropRemoteProxy(o.workspace_id)                              //    非 running 立刻拆
}
```

`mapObservedToStatus` 映射表：`running->running`、`stopped->stopped`、`error->error`、`pending/starting->starting`、其余->`unknown`。

> 颗粒度注意：① 与 ② 在同一轮 pass、同一轮 for 循环里**顺序 await**。DB 状态先变 running，proxy 紧随其后入 map。这就是 §7 窗口分析的根据。

## 5. remote-proxy：forward proxy 本体（被读方）

`control-plane/src/lib/remote-proxy.ts` 维护一个 in-process Map（cp 单副本单进程，in-process Map 足够）：

```ts
const proxies = new Map<string, RemoteProxy>()   // { port, server }
```

- **`getRemoteProxyPort(wsId)`** -- 同步 O(1) 查表，返回 port 或 `undefined`。`workspace-address` 每次解析地址都调它。
- **`ensureRemoteProxy(wsId, envId, targetPort=3001)`** -- 幂等。已存在直接返回；否则 `net.createServer`，每条入站连接 `openForwardStream(envId, 'fwd:wsId:port')` 开一条隧道子流，`socket.pipe(stream).pipe(socket)` 透传字节。`listen(0, '127.0.0.1')` 拿系统分配的随机端口，毫秒级返回。
- **`dropRemoteProxy(wsId)`** -- `server.close()` + 移出 map。

> 红线：built-in workspace **永远不进这个 map**（§3 保证）。所以 built-in 的路径与无 proxy 时代逐字节相同（cluster DNS 直连）。这是 remote 能力叠加进来不污染 built-in 的保证。

## 6. workspace-address：路由缝合点

```ts
export function getWorkspaceAddress(workspaceId: string): string {
  const remotePort = getRemoteProxyPort(workspaceId)
  if (remotePort !== undefined) return `http://127.0.0.1:${remotePort}`   // remote
  return builtinAddress(workspaceId)                                      // built-in / fallback
}
```

`builtinAddress` = `http://tos-${wsId}.${NAMESPACE}.svc.cluster.local:3001`（集群 DNS）。

关键：**当 `getRemoteProxyPort` 返回 `undefined` 时，无论是不是 remote workspace，都会 fallback 到集群 DNS。** 这是 §7 "窗口期 fetch 失败" 的根因--对 remote workspace，集群 DNS 不可达。

## 7. executeChat 请求 proxy 时，proxy 还没启动的情况会出现吗？

### 7.1 Built-in workspace（v1 默认、绝大多数场景）：不会出现

整条路径**不走 proxy**（§3 已保证 built-in 永不入 map）：

```
ensureWorkspaceRunning(ws)  ── status=running -> 立即返回
        │
        ▼
resolveAgentAddress(wsId)
  └─ getRemoteProxyPort -> undefined   (built-in 从不进 map)
  └─ builtinAddress -> 集群 DNS
        │
        ▼
fetch(`${addr}/chat`)   ── 集群 DNS 直达 pod
```

`ensureWorkspaceRunning` 通过集群 DNS 轮询 `/health` 直到 pod ready；`executeChat` 的 fetch 也走集群 DNS。**没有 proxy，不存在"proxy 没启动"的问题。** 这条红线由 `is_builtin` 过滤 + `remote-proxy` 只被 projection 调用共同保证。

### 7.2 Remote (BYOI) workspace：理论上存在一个极窄窗口，实际几乎不命中

投影 pass 内的时序（§4 ①②）：

```
runner 报 observed_phase=running
        │
        ▼
runEnvProjection 一轮 pass，for 循环遍历到该 ws：
        │
        ├─ applyStatusChange(wsId, 'running')   ← ① DB 状态先变 running
        │        (中间只隔一次 getWorkspace 的 DB 读)
        └─ ensureRemoteProxy(wsId, envId)       ← ② proxy 紧随入 map
              └─ net.createServer + listen(0)   (毫秒级)
```

① 与 ② 之间的窗口：**DB 已是 `running`、proxy 尚未入 map**。若恰在此窗口内 `executeChat` 进来：

| 步骤 | 行为 | 结果 |
|---|---|---|
| `ensureWorkspaceRunning` | 见 `status === 'running'` -> 立即返回（line 69） | 不触发任何兜底 |
| `getRemoteProxyPort` | map 里没有 -> `undefined` | - |
| `getWorkspaceAddress` | fallback 到 `builtinAddress` | 集群 DNS |
| `fetch(addr/chat)` | remote pod 在 NAT 后、集群 DNS 不可达 | 抛异常 |
| `executeChat` catch | -> 502 "Agent unavailable" | 用户重试即可 |

**为什么实际几乎不命中：**

1. **窗口极窄**：① 与 ② 在同一 for 循环里顺序 `await`，中间只有一次 `getWorkspace` DB 读；`ensureRemoteProxy` 本身是 `net.createServer + listen(0)`，毫秒级。
2. **同进程串行**：cp 单副本单进程，projection 的 `await ensureRemoteProxy` 不返回、这轮 pass 没结束；executeChat 读到 `running` 通常在 projection 已完成的那轮之后。
3. **用户路径**：用户看到 workspace 变 `running` 再发起 chat，此时建 proxy 的那轮 projection 早已跑完，proxy 已在 map 里。

> 闭环：窗口客观存在，但被"单进程串行 + 窗口毫秒级 + 用户感知滞后"三重因素压到几乎不可观测。命中时表现为一次性 502，用户重试即恢复。

### 7.3 一个更值得注意的边界：cold-start 期间的 proxy 缺位

`ensureWorkspaceRunning` 对 `starting` 状态的 workspace 也会轮询 `/health`（`workspace-autostart.ts:53`）。而 projection 只在 `status === 'running'` 时 `ensureRemoteProxy`，`starting` 阶段走 `else` 分支 `dropRemoteProxy`：

```
executeChat(stopped remote ws)
  └─ ensureWorkspaceRunning
        └─ runStart(needsScale=true)
              ├─ startWorkspaceInstance -> setDesiredPhase=running, DB status='starting'
              └─ pollHealth  ── getWorkspaceAddress
                    └─ getRemoteProxyPort -> undefined  (starting 阶段 proxy 被拆)
                    └─ fallback 集群 DNS -> remote 不可达
                    └─ 每 1s 重试，90s 后 -> WorkspaceStartError("did not become ready within 90s")
```

即：**remote workspace 若由 `executeChat` 触发冷启动，`/health` 轮询会因 proxy 缺位而 fallback 到不可达的集群 DNS，90s 后超时失败。** 实际部署中 remote workspace 通常由 reconcile 直接拉起、projection 观察到 `running` 建好 proxy 之后用户再 chat--"chat 触发 remote 冷启动"这条路径本身不被当前设计保证。

> 拉通：这不是 executeChat fetch 阶段的 race（§7.2 那个），而是更早一步 `ensureWorkspaceRunning` 阶段的 proxy 缺位。两者同源--projection 只在 `running` 建 proxy，`starting` 期间 remote 没有可达通路。

## 8. 端到端时序（remote workspace 正常路径）

```
┌─ runner (NAT 后) ─┐                    ┌─ cp (单进程) ──────────────────────┐
│                   │                    │                                    │
│ pod ready         │  observed_phase    │ runEnvProjection (15s cron):      │
│ report running ───┼───────────────────►│   applyStatusChange(running)  ①   │
│                   │                    │   ensureRemoteProxy  ──────  ②    │
│                   │                    │     net.createServer              │
│                   │  隧道子流 (fwd)     │     listen(0) -> port 入 map       │
│  openForwardStream◄────────────────────│   (用户看到 running)              │
│                   │                    │                                    │
│                   │                    │ executeChat:                       │
│                   │                    │   ensureWorkspaceRunning -> 立即返回│
│                   │                    │   resolveAgentAddress              │
│                   │                    │     getRemoteProxyPort -> port ✓    │
│                   │  HTTP /chat 字节    │   fetch(127.0.0.1:port/chat) ───►│
│   agent 处理  ◄───┼────────────────────│   (pipe 经隧道直达 pod)            │
│                   │                    │                                    │
└───────────────────┘                    └────────────────────────────────────┘
```

## 9. 关键不变量（不要破坏）

1. **`is_builtin` 是 builtin/remote 的唯一裁决者** -- 所有 remote 查询都以 `is_builtin = false` 过滤；built-in 行由 migration 种子、API 不可改不可删。破了这条，两条状态来源会串线。
2. **built-in 永不入 proxy map** -- `remote-proxy` 只被 projection 对 remote workspace 调用 `ensureRemoteProxy`（projection 输入已被 `is_builtin=false` 过滤）。不要在 executeChat / chat handler 里直接 `ensureRemoteProxy`，否则会出现双建、状态与 proxy 错位、甚至给 built-in 误建 proxy。
3. **proxy 的唯一创建方是 projection cron** -- 其他代码只读 `getRemoteProxyPort`。proxy 不是请求触发，是 15s cron 周期性扫出来的。
4. **`getWorkspaceAddress` fallback 到集群 DNS 是设计而非 bug** -- 对 built-in 是正路；对 remote 是"proxy 缺位时的失效表现"（fetch 失败而非静默错路由）。不要改成"remote 无 proxy 就抛错"，会污染 built-in 路径。
5. **projection 一轮 pass 内 ① 状态 ② proxy 顺序不能反** -- 先 proxy 后状态，会让"DB 还没 running 但 proxy 已建"的窗口里 executeChat 误以为 stopped 而触发冷启动；先状态后 proxy 才是当前 §7.2 的窄窗口（影响最小）。
6. **`ensureRemoteProxy` 必须幂等** -- 已存在直接返回，否则 projection 每轮 pass 会重建 server、端口漂移、旧连接泄漏。

## 10. 变更前的自检清单

- [ ] 改 `reconcile.ts`：env projection cron 是否仍是 `*/15 * * * * *` + `protect:true`？`ENV_HEARTBEAT_TIMEOUT_SEC` 默认是否仍 60s？
- [ ] 改 `environments.ts`：`listRemoteWorkspaceObservations` / `getRemoteWorkspaceIds` / `markStaleEnvironmentsOffline` 是否都带 `is_builtin = false`？是否误加了 built-in 行会匹配的条件？
- [ ] 改 `env-projection.ts`：① 与 ② 的顺序是否仍是"先状态后 proxy"？非 `running` 分支是否仍 `dropRemoteProxy`？
- [ ] 改 `remote-proxy.ts`：是否仍只在 projection 路径被调？`ensureRemoteProxy` 是否仍幂等？`listen(0)` 拿随机端口的契约是否被改成固定端口？
- [ ] 改 `workspace-address.ts`：`undefined` 时是否仍 fallback 到 `builtinAddress`？是否误加了"remote 无 proxy 抛错"分支？
- [ ] 想给 remote cold-start 补 proxy：是否在 `ensureWorkspaceRunning` 的 `starting` 路径上也对 remote workspace `ensureRemoteProxy`（需基于 environment_id 判 remote）？这会打通 §7.3 的缺位，但要注意 runner 还没把 pod 起起来时隧道子流也连不通。
