---
name: env-runner-core
description: @neutree-ai/env-runner-core 包深度解析 — PlacementTransport 抽象与 reconcile 循环
type: domain-doc
created: 2026-07-06T02:01:58Z
updated: 2026-07-10T16:53:51Z
owner: platform
status: active
---

# `@neutree-ai/env-runner-core` 深度解析

> 顶层设计：cp 是 source of truth，**写 desired**；runner 是 reconciler，**拉 actual**。这两端之间的"协议"就是 `PlacementTransport`。runner-core 把 reconcile 算法写成与 transport 无关的纯函数 — `PlacementTransport` 是 HTTP 还是 DB，算法都一样。

## 1. 能力边界

| 能力 | 类型 / 函数 | 责任 | 谁来满足 |
|---|---|---|---|
| 数据形状 | `PlacementRow`（蛇形命名） | DB 列 / `/env/v1` JSON 一比一 | `DbTransport`（`pg.Pool`）或 `HttpTransport`（`fetch`） |
| 写回形状 | `ObservedUpdate` | 收敛结果（phase + endpoint + 可选 version/message） | 同上 |
| Transport 抽象 | `PlacementTransport` | 4 个方法：`listPlacements / writeObserved / deletePlacement / heartbeat` | 两个实现类 |
| Reconcile 算法 | `reconcileOnce` | 一次 pass，遍历所有 placement 决策 | runner-core |
| 调度循环 | `startReconcileLoop` | 每 `intervalMs` 调一次 `reconcileOnce`，返回 `stop()` | runner-core |
| 幂等保障 | `recordIfChanged` | 仅在 phase 变化时写 DB | runner-core |

### 1.1 明确**不**做的事

- 不做调度（`setInterval`/`cron` 都不做，调用方决定）
- 不做并发协调（同一 runner 单进程；多副本由 leader-election 在外层解决）
- 不解析 `WorkspaceSpec`（只当 `unknown` 透传给 `EnvironmentProvider`）
- 不写 `endpoint` 之外的元数据（`message` 可选但极简）

## 2. 核心心智模型：Desired ↔ Observed 对账

```
         cp 写 desired                  runner 拉 actual
              │                              │
              ▼                              ▼
      ┌────────────────┐            ┌────────────────┐
      │ PlacementRow   │            │  ObservedState │
      │ (snake_case)   │            │  (camelCase)   │
      └────────────────┘            └────────────────┘
              │                              │
              └────► reconcileOnce ◄─────────┘
                          │
            ┌─────────────┼─────────────┐
            ▼             ▼             ▼
       apply()        start()        stop()    destroy()
            │             │             │             │
            ▼             ▼             ▼             ▼
        k8s SSA      start ws      scale=0      delete all
```

**关键不变量**：
- **`PlacementRow` 是 snake_case**（DB 列名）；`ObservedState` 是 camelCase（前端/类型）
- Transport 实现负责两者之间的转换；reconcile 算法只接触 `PlacementRow` 和 `ObservedState`，**不接触 DB / HTTP**
- `EnvironmentProvider` 只接触 `WorkspaceSpec` + `ObservedState`；**不接触** `PlacementRow`

> 抓手：这种"翻译层只放一个地方"的拆分，是为什么 runner-k8s 同时能跑直接-DB 模式和远端-HTTP 模式。

## 3. `PlacementTransport` 接口契约

```ts
export interface PlacementTransport {
  /** Desired-state set this runner is responsible for. */
  listPlacements(): Promise<PlacementRow[]>

  /** Report observed state for one workspace. */
  writeObserved(workspaceId: string, o: ObservedUpdate): Promise<void>

  /** Drop a placement row after destroy. */
  deletePlacement(workspaceId: string): Promise<void>

  /** Liveness + capability refresh, once per reconcile pass. */
  heartbeat(capabilities: Record<string, unknown>): Promise<void>
}
```

### 3.1 两个实现

| 实现 | 用途 | listPlacements 范围 | heartbeat 行为 |
|---|---|---|---|
| `DbTransport` | built-in（同集群） | `WHERE e.is_builtin = true` | no-op（cp 自己 watch 自己集群） |
| `HttpTransport` | 远端 BYOI | `GET /env/v1/placements`（已按 env-token 隐式限定） | `POST /env/v1/heartbeat` 上报 capabilities |

**重要 — 为什么 DbTransport 限定 `is_builtin = true`**：

> 远端 BYOI 也是 `kind='kubernetes'`，**但必须由它的 HttpTransport 跑**。直接-DB runner 看到一行就 reconcile，会在 cp 自家集群起一个 ws 副本 → 双重 reconcile。所以 DbTransport 用 `is_builtin` 做硬隔离。

### 3.2 为什么 heartbeat 是 Transport 的事

- DB 模式：cp 自己 watch pod 健康，runner 不需要再报一次
- HTTP 模式：cp 不知道远端集群的 pod 死活，**必须**靠 runner 周期性自报
- **统一的方法签名**让 reconcile 循环无脑调一次，不关心模式

## 4. Reconcile 决策表

`reconcilePlacement` 是单 placement 的决策函数。读起来像一组嵌套 if，但每个分支都有**明确的"为什么"**：

```
┌──────────────────┬──────────────────────────────────┬──────────────┐
│ 条件 (desired)   │ 行为                              │ 返回         │
├──────────────────┼──────────────────────────────────┼──────────────┤
│ desired=deleted  │ provider.destroy → deletePlacement│ 'destroy'    │
│ desired=stopped  │ 不在 stopped → provider.stop       │ 'stop'       │
│                  │ 已在 stopped → recordIfChanged     │ 'none'       │
│ desired=running  │ spec drift (spec_v > obs_v) →      │              │
│                  │   provider.apply → record          │ 'apply'      │
│                  │ 不存在 → provider.apply → record   │ 'apply'      │
│                  │ 存在但 stopped → provider.start    │ 'start'      │
│                  │ 存在但 starting/pending/error →    │              │
│                  │   recordIfChanged（让 cp 等下一轮）│ 'none'       │
│                  │ 已 running → recordIfChanged       │ 'none'       │
└──────────────────┴──────────────────────────────────┴──────────────┘
```

### 4.1 为什么 stopped 时不 apply spec

```ts
if (p.desired_phase === 'stopped') {
  if (current.phase !== 'stopped' && exists) {
    await provider.stop(p.workspace_id)
    ...
    return 'stop'
  }
  await recordIfChanged(transport, p, current)
  return 'none'
}
```

> ⚖ 决策：用户主动 stop 一个 ws 是**省电/省费**意图；这期间 cp 改了 spec（比如升级了 agent 镜像）我们**不主动 apply**。等到下次 desired 翻回 running，spec 一起 apply。
> 不然：用户停机省的钱被"偷偷运行的 apply"花了 — 业务事故。

### 4.2 spec drift 怎么才算"drift"

```ts
if (p.spec_version > (p.observed_version ?? 0)) {
  await provider.apply(...)
}
```

> 决策锚是 `spec_version` 单调递增，**不是 spec 内容 hash**。`EnvironmentProvider.apply` 内部用 K8s Server-Side Apply 做真收敛；reconcile 层面只看"版本号有没有变"。

### 4.3 在 in-flight 时为什么 recordIfChanged

- 状态是 `starting` / `pending` / `error` 时，**infra 自己正在做事**
- runner 不应该去抢着 apply 或 restart
- 调一次 `recordIfChanged` 把当前观察到的 phase 写回去（可能与 `observed_phase` 一样就 no-op）
- 等下一轮 pass 再看

> 闭环：reconcile 不"催促"infra，只**记录**。infra 自己的 controller-loop（k8s deployment rollout）才是真正的驱动器。

### 4.4 `recordIfChanged` 为什么是核心

```ts
async function recordIfChanged(transport, p, current): Promise<void> {
  if (current.phase !== p.observed_phase) {
    await transport.writeObserved(p.workspace_id, { phase: current.phase, endpoint: current.endpoint })
  }
}
```

- 一个有 50 个 ws 的 runner，**全部收敛**时每 pass 0 次 DB 写
- 50 个全在 starting 时，每 pass 50 次（因为 phase 一直在变）
- 没有这个判断：每 pass 50 次无意义 UPDATE → 触发 DB 复制、binlog、慢日志、监控告警

## 5. `reconcileOnce` 全流程

```ts
async function reconcileOnce(provider, transport) {
  // 1. 一次性拉所有 desired
  const placements = await transport.listPlacements()

  // 2. 一次性观察所有 actual（O(1) 远端读）
  const observedAll = provider.observeAll ? await provider.observeAll() : null

  // 3. 遍历决策
  let acted = 0, noop = 0, failed = 0
  for (const p of placements) {
    try {
      const current = observedAll
        ? (observedAll.get(p.workspace_id) ?? { phase: 'unknown' })
        : await provider.observe(p.workspace_id)        // 无 batch 时的回退
      const action = await reconcilePlacement(provider, transport, p, current)
      if (action === 'none') noop++; else acted++
    } catch (err) {
      failed++
      console.error(`[env-runner] reconcile failed for ${p.workspace_id}:`, err)
    }
  }

  // 4. 心跳
  try { await transport.heartbeat(provider.capabilities()) }
  catch (err) { console.error('[env-runner] heartbeat failed:', err) }

  return { acted, noop, failed }
}
```

### 5.1 `observeAll` 的批量化意义

- 没用 batch：N 个 placement → N 次 `provider.observe` → N 次 k8s API call → N×网络延迟
- 用了 batch：N 个 placement → **1 次 k8s LIST**（参考 [P3] 的 LIST call）
- 没有 `observeAll` 的 provider 怎么办？降级到 `observe`，仍然正确，**只是慢**

> 颗粒度：算法**优先**批量化，但**不依赖**批量化。

### 5.2 错误隔离

```ts
for (const p of placements) {
  try { ... } catch (err) { failed++; continue }
}
```

- 一个 placement 失败**不影响其他**
- failed 计数进日志，可观测
- 没有这条 → 一个 ws 的 k8s API 超时挂掉整个对账循环

## 6. `startReconcileLoop` 生命周期

```ts
export function startReconcileLoop(provider, transport, intervalMs): () => void {
  let stopped = false
  let timer: ReturnType<typeof setTimeout> | undefined

  const tick = async () => {
    if (stopped) return
    try {
      const { acted, noop, failed } = await reconcileOnce(provider, transport)
      console.log(`[env-runner] reconcile pass: ${acted} acted, ${noop} noop, ${failed} failed`)
    } catch (err) {
      console.error('[env-runner] reconcile pass failed:', err)
    }
    if (!stopped) timer = setTimeout(tick, intervalMs)   // ★ 不用 setInterval
  }

  void tick()
  return () => {
    stopped = true
    if (timer) clearTimeout(timer)
  }
}
```

### 6.1 为什么用 `setTimeout` 串行而不是 `setInterval`

| 维度 | `setInterval` | 串行 `setTimeout` |
|---|---|---|
| 慢 pass 行为 | 下一轮在上一轮**还没结束**时就触发 → 堆叠 | 永远在上一次结束后才排下一次 |
| 取消语义 | 要存 interval id + `clearInterval` | 设 `stopped` 标志位即可 |
| 错误恢复 | 一次 throw 后定时器**继续触发**（不重置） | try/catch 自动继续排下一轮 |

> 抓手：**一段慢逻辑的循环，永远用 setTimeout 串行**。

### 6.2 stop() 的正确性

- 标记 `stopped = true`
- `clearTimeout` 当前定时器
- 已发出的 `tick` await **不会被打断**（await 完成时 `stopped` 已 true → 不再排下一轮）

> 这意味着 `stop()` 后最坏情况要等"当前 pass + 一次 `await observe`"的时间才完全停。生产上够用。

### 6.3 uncaughtException / unhandledRejection 守护

`env-runner-k8s/src/index.ts:19-24`：
```ts
process.on('uncaughtException', (err) => {
  console.error('[fatal] Uncaught exception (process kept alive):', err)
})
process.on('unhandledRejection', (reason) => {
  console.error('[fatal] Unhandled rejection (process kept alive):', reason)
})
```

> ⚖ 决策：runner 是基础设施进程，**不主动 crash**。一次未捕获错误可能是 k8s watch 回调里的 bug，crash 后由 Deployment 重启会丢最近一次对账的进度。记日志 + 继续跑。

## 7. 完整调用栈（启动 → 一轮 pass）

```
env-runner-k8s/src/index.ts
  │
  ├─ const provider = makeDefaultProvider()                    // k8s impl
  ├─ const transport = MODE === 'remote'
  │     ? new HttpTransport(cpUrl, envToken)                   // /env/v1
  │     : new DbTransport()                                    // pg pool
  │
  ├─ if (MODE === 'remote') startTunnel(cpUrl, token)          // 反向拨出
  │
  └─ startReconcileLoop(provider, transport, INTERVAL_MS)      // 调 runner-core
        │
        ▼
  env-runner-core/reconcile.ts:startReconcileLoop
        │
        └─ tick() async
              ├─ reconcileOnce(provider, transport)
              │     ├─ transport.listPlacements()              // 拉 desired
              │     ├─ provider.observeAll()                   // 拉 actual
              │     ├─ for each p:
              │     │     ├─ reconcilePlacement(p, current)
              │     │     │     ├─ provider.apply/start/stop/destroy(...)
              │     │     │     ├─ provider.observe(...)         // 验证结果
              │     │     │     └─ transport.writeObserved(...) // 写回
              │     │     └─ catch → failed++
              │     └─ transport.heartbeat(provider.capabilities())
              ├─ console.log 摘要
              └─ setTimeout(tick, INTERVAL_MS)
```

## 8. 关键不变量（设计 §13.1）

1. **stopped 不 apply spec** — 见 §4.1
2. **desired = observed + spec_version = observed_version ⇒ no-op** — 一行不写
3. **失败不传染** — 单 placement 异常只增加 `failed` 计数
4. **观察用 batch 优先** — `observeAll` 有就用，没有就降级
5. **心跳每个 pass 一次** — liveness + capabilities 一起报
6. **endpoint 变化**不**触发** writeObserved**（设计决策）** — endpoint 由 `provider.observe` 决定，runner-core 信任它；只有 phase 变化才写

> 6 是个微妙的不变量：endpoint 重新计算了但 phase 没变，**不写**。这避免了 DNS 重解析触发的写放大。

## 9. 自检清单

- [ ] 改 `reconcilePlacement`：是否破坏了"stopped 不 apply"？
- [ ] 加新 transport：4 个方法签名是否完全一致？`listPlacements` 是否限定了 runner 该负责的范围？
- [ ] 加新 provider：是否提供了 `observeAll`？`capabilities()` 返回的对象是否稳定（cp 拿来做 allowlist 判断）？
- [ ] 调短 `intervalMs`：是否考虑了 observe batch 的限流（k8s 默认 50 QPS）？
- [ ] 让 runner 多副本：当前实现**不支持**（两个 runner 会同时 apply）；leader-election 在外层

## 10. 关键代码索引

| 关注点 | 文件:行 |
|---|---|
| `PlacementRow` / `ObservedUpdate` / `PlacementTransport` | `internal/env-runner-core/transport.ts:16-43` |
| `reconcilePlacement`（决策树） | `internal/env-runner-core/reconcile.ts:39-118` |
| `reconcileOnce`（单 pass） | `internal/env-runner-core/reconcile.ts:121-155` |
| `startReconcileLoop`（调度） | `internal/env-runner-core/reconcile.ts:158-182` |
| `recordIfChanged`（收敛护栏） | `internal/env-runner-core/reconcile.ts:26-37` |
| `EnvironmentProvider` 接口 | `internal/types/environments.ts:121-144` |
| `WorkspaceSpec` / `ObservedState` | `internal/types/environments.ts:59-108` |
| `DbTransport`（直接模式） | `env-runner-k8s/src/db-transport.ts:15-53` |
| `HttpTransport`（远端模式） | `env-runner-k8s/src/http-transport.ts:11-63` |
| 入口 + 守护 + 调度 | `env-runner-k8s/src/index.ts:19-92` |

## 11. 附录：afs-controller 部署位置与 `rev:afs` 死链分析

> 本节回答两个高频困惑：① afs-controller 到底部署在 runner 侧还是 cp 侧？② cp 端 `rev:afs` reverseTarget 和 runner 端 :39100 listener 到底有没有人用？结论先行：**afs-controller 始终跟 runner 同集群部署**；**`rev:afs` 是预留但未启用的死链** -- 监听器和路由都存在，但当前没有任何客户端连接 runner:39100。

### 11.1 afs-controller 部署在哪？

**始终部署在 runner 所在集群**，由 `charts/env-runner-k8s/templates/afs.yaml` 渲染：

| 模式 | runner 所在集群 | afs-controller 所在集群 | 部署模板 |
|---|---|---|---|
| direct (builtin) | 平台集群 | 平台集群（跟 cp 同集群） | `charts/env-runner-k8s/templates/afs.yaml` |
| remote (BYOI) | 客户集群 | 客户集群 | 同一模板，install 到客户集群 namespace |

`afs.yaml` 部署三件套：
- **Deployment `afs-controller`**（`afs.yaml:43-90`）：端口 9100，SQLite 元数据 DB 在 RWX PVC 上（`afs.yaml:26-41`，controller.db 丢失则所有 dir 映射消失、mount 全失败）
- **Service `afs-controller:9100`**（`afs.yaml:92-104`）：集群内 cluster DNS 可达
- **ConfigMap `afs-controller-config` / `afs-fuse-config`**（`afs.yaml:106-133`）：fuse sidecar 的 `controller_addr` 也指向 `afs-controller.<ns>.svc:9100`

`afs.yaml:8-10` 注释明确：
> "reaches `afs-controller` over cluster DNS - **no tunnel is involved** for afs."

afs 数据面（RWX PVC + fuse gRPC）是集群本地的，**不能跨 WAN**，这就是为什么 remote 模式下 afs-controller 必须部署在客户集群。

### 11.2 `rev:afs` 的两端定义

`rev:afs` 在 tunnel 架构中有完整定义，但两端都是"准备好了等人连"的状态：

**cp 侧 reverseTarget**（`control-plane/src/routes/env-gateway.ts:12,16`）：
```ts
const AFS_CONTROLLER_ADDR = process.env.AFS_CONTROLLER_ADDR || 'afs-controller.default.svc:9100'
// ...
meta === 'rev:afs' ? AFS_CONTROLLER_ADDR : null
```
cp 进程的 `AFS_CONTROLLER_ADDR` 指向 **cp 所在集群的** afs-controller。`rev:afs` 流到达 cp 后 pipe 到这个地址。

**runner 侧 reverse listener**（`env-runner-k8s/src/tunnel-dataplane.ts:62`）：
```ts
listenAndTunnel(AFS_PROXY_PORT, '0.0.0.0', () => getMux()?.openStream('rev:afs') ?? null),
```
runner 在 `:39100`（`AFS_PROXY_PORT`，`tunnel-dataplane.ts:20`）监听 TCP，等待 sidecar 连进来。

### 11.3 谁连 runner:39100？ -- 没人

workspace pod 的两个 afs 相关 env 决定了 sidecar/fuse 的连接目标：

| env 变量 | 来源（config.ts -> workspace-spec.ts） | 默认值 | remote 模式实际值 | 连接路径 |
|---|---|---|---|---|
| `AFS_FUSE_SERVER` | `workspace-spec.ts:71` = `cfg.afs.fuseServerAddr` = `AFS_FUSE_SERVER_ADDR`（`config.ts:24`） | `127.0.0.1:9101` | `127.0.0.1:9101` | pod 内 loopback（fuse sidecar 同 pod），不走 tunnel |
| `AFS_CONTROLLER` | `workspace-spec.ts:70` = `cfg.afs.controllerAddr` = `AFS_CONTROLLER_ADDR`（`config.ts:23`） | `afs-controller.default.svc:9100` | `afs-controller.<ns>.svc:9100`（`deployment.yaml:89-90` 注入） | cluster DNS 直连，不走 tunnel |

remote 模式下 runner 进程的 `AFS_CONTROLLER_ADDR` 由 `charts/env-runner-k8s/templates/deployment.yaml:89-90` 注入：
```yaml
- name: AFS_CONTROLLER_ADDR
  value: "afs-controller.{{ .Release.Namespace }}.svc:9100"
```

**没有任何 env 变量指向 runner:39100**。workspace pod 的 fuse 连 localhost:9101（同 pod sidecar），controller 连客户集群 DNS:9100（cluster DNS 直连）。两条路径都绕过了 runner 的 :39100 reverse listener。

### 11.4 结论：`rev:afs` 是死链

| 组件 | 状态 | 证据 |
|---|---|---|
| runner :39100 listener | ✅ 已启动 | `tunnel-dataplane.ts:62` 在 `startReverseListeners` 中无条件调用 |
| cp `rev:afs` reverseTarget | ✅ 已注册 | `env-gateway.ts:16` 在符号枚举中 |
| 连接 runner:39100 的客户端 | ❌ 不存在 | workspace pod 的 `AFS_CONTROLLER` / `AFS_FUSE_SERVER` 均不指向 :39100 |

对比：**`rev:cp`（:38000）是活跃使用的** -- sidecar 回调 cp HTTP API 时连 runner:38000（见 `cp-agent-traffic.md` §6）。**`rev:afs`（:39100）是预留但未启用的** -- runner 启动时绑了端口、cp 注册了符号，但两端之间没有客户端。

### 11.5 为什么 `rev:afs` 没用上？

remote 模式下 afs 的两条数据路径都不需要 reverse tunnel：

1. **fuse -> controller（数据面）**：fuse sidecar 在 workspace pod 内，controller 在同集群，cluster DNS 直连（`afs.yaml` 注释："no tunnel is involved for afs"）。afs 的 RWX 数据面不能跨 WAN，所以 controller 必须跟 fuse 同集群 -- 不存在"sidecar 要回调远端 cp 侧 controller"的场景。

2. **cp -> afs（控制面）**：cp 对 afs 的操作（createDir / mount / unmount）走 **afsctl control 流**（链路 3，`cp-agent-traffic.md` §7），这是 cp 主动发起的 forward 方向 control 流，不是 sidecar 回调的 reverse 流。

`rev:afs` 的设计意图推测：预留给"sidecar 需要回调 cp 侧（平台集群）的 afs-controller"的场景。但实际架构中 remote 模式把 afs-controller 部署到了客户集群，sidecar 用 cluster DNS 就能直连，不需要绕 tunnel 回调平台集群的 controller。因此 `rev:afs` 变成了未使用的能力。

> ★ 抓手：如果将来架构变化（例如 remote 模式不再部署本地 afs-controller，改为 sidecar 回调平台集群的 controller），`rev:afs` 可以直接启用 -- listener 和 reverseTarget 都已就位。但当前它是死链，调试时看到 runner :39100 有 listen 但无连接是正常的。
