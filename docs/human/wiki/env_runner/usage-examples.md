---
name: env-runner-usage-examples
description: env-tunnel 与 env-runner-core 的端到端使用示例 — 从 cp 推 spec 到 runner 通过隧道回包
type: workflow-doc
created: 2026-07-06T02:01:58Z
updated: 2026-07-06T02:01:58Z
owner: platform
status: active
---

# 端到端使用示例

> 闭环：本文档把所有片段拼成 3 个完整故事。每个故事都从**真实用户/系统动作**出发，跟到**字节/字段级**。
> 阅读顺序：示例 1（最完整，覆盖三类使用）→ 示例 2（DB 模式，与示例 1 对比）→ 示例 3（debug）。

---

## 示例 1：BYOI 模式下，cp 委托远端 runner 挂载 afs

> 这是 `env-tunnel` 最有代表性的使用路径：**双向 + 控制面 + 数据面**都用上。

### 1.1 前置条件

```bash
# 假设 cp 在 we-cluster, runner 在 customer-cluster
# customer 集群里已经跑了一个 k8s Deployment：env-runner-k8s
# env token 已经在 cp 注册并注入到 runner 环境变量
CP_URL=https://cp.example.com
ENV_TOKEN=eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
ENV_RUNNER_INTERVAL_MS=15000
RUNNER_MODE=remote
CP_URL=https://cp.example.com
TUNNEL_RECONNECT_MS=5000
```

### 1.2 启动 runner（30 秒全流程）

```bash
# 1. runner 启动
node env-runner-k8s/dist/index.js
#   [env-runner-k8s] starting (mode=remote, reconcile interval 15000ms)

# 2. reconcile loop 启动
#   [env-runner-k8s] reconcile pass: 0 acted, 0 noop, 0 failed
#   （cp 上还没有 placement，listPlacements 返回 []）

# 3. tunnel 拨出
#   [env-runner-k8s] tunnel data plane up

# 4. reverse listener 启动
#   listen(38000), listen(39100)   ← kube-proxy/Service 把这些端口暴露
```

### 1.3 cp 收到"创建 workspace"请求

```ts
// 业务代码（伪）
const ws = await db.workspaces.create({
  environmentId: 'env-customer-acme',       // BYOI env
  agentType: 'claude',
  resources: { cpu: '2', memory: '4Gi' },
  // features 决定是否走 afs
  features: { sharedFs: true },
})

// 写到 workspace_placements：
await db.query(`
  INSERT INTO workspace_placements
    (workspace_id, environment_id, desired_phase, spec, spec_version, observed_phase, observed_version)
  VALUES ($1, $2, 'running', $3, 1, NULL, NULL)
`, [ws.id, ws.environmentId, { agentType, resources, version: 1 }])
```

### 1.4 runner 端 reconcile（接下来的一个 pass）

```
T+0     HttpTransport.listPlacements()
        → GET https://cp.example.com/env/v1/placements
        ← 200 { placements: [PlacementRow] }

T+10ms  provider.observeAll()                    // 一次 k8s LIST
        → 没有这个 workspace → { phase: 'unknown' }（不在 map 里）

T+15ms  reconcilePlacement(provider, transport, row, current)
        ├─ desired_phase = 'running'
        ├─ current.phase = 'unknown' → 走 "不存在 → apply"
        ├─ provider.apply(wsId, spec)              // K8s SSA: 建 Deployment+Service
        │     ↓
        │     实际上 k8s rollout 需要 30s+ 才到 running
        ├─ provider.observe(wsId) → { phase: 'starting' }
        ├─ transport.writeObserved(wsId, { phase: 'starting', endpoint: {...}, version: 1 })
        └─ return 'apply'

T+500ms transport.heartbeat({ sharedFs: true, persistentMemory: false })
        → POST /env/v1/heartbeat

T+...   下一轮 pass（15s 后）
        k8s 已经把 Pod 跑起来 → current.phase = 'running'
        → recordIfChanged（'starting' → 'running'）写一次 DB
        → 后续 pass 都是 no-op
```

### 1.5 cp 委托挂载 afs（**控制面**走 tunnel）

```ts
// cp 业务代码 — 收到"用户点了挂载共享目录"
import { callControl } from '@neutree-ai/env-tunnel'
import { openControlStream } from '../services/env-gateway/registry'

async function mountSharedDir(workspaceId: string, dirName: string, readonly: boolean) {
  const envId = await getEnvIdForWorkspace(workspaceId)
  const stream = openControlStream(envId, 'afsctl')           // 1. 拿一条 mux 子流
  if (!stream) throw new Error(`runner for env ${envId} offline`)

  try {
    const res = await callControl<{ ok: true }>(stream, {     // 2. RPC
      op: 'mount',
      workspaceId,
      dirId: '...',
      name: dirName,
      accessKey: '...',
      readonly,
    }, 15_000)                                                  // 15s 超时
    return res
  } catch (err) {
    throw new Error(`mount failed: ${(err as Error).message}`)
  }
}
```

字节在 mux 里的样子：

```
cp writes:    00 00 00 4E   {"op":"mount","workspaceId":"ws-123","dirId":"d-1",
                              "name":"home","accessKey":"ak-x","readonly":false}
              └─────┬────┘   └────────────────── 78 bytes JSON ───────────────────┘
              len=78

runner reads: 同上
runner handler (afs-control.ts:dispatch) → gRPC: Mount({...}) → pod:9101
runner writes: 00 00 00 07   {"ok":true}
cp reads:     {"ok":true}
```

### 1.6 sidecar 上报状态（**反向数据面**走 tunnel）

```
sidecar (pod:9100)                            runner pod                              cp
  │                                              │                                     │
  │  TCP 10.0.0.5:39100  (其实是 127.0.0.1:39100)│                                     │
  ├─────────────────────────────────────────────►│                                     │
  │                                              │ listenAndTunnel(39100, "0.0.0.0", │
  │                                              │   () => mux.openStream("rev:afs"))│
  │                                              │                                     │
  │                                              │ s = mux.openStream("rev:afs")      │
  │                                              │ socket.pipe(s).pipe(socket)        │
  │                                              │ ── substream "rev:afs" ──►         │
  │                                              │                                     ▼
  │                                              │                       onStreamOpen(s, "rev:afs")
  │                                              │                                     │ reverseTarget("rev:afs")
  │                                              │                                     │   → "afs-controller.default.svc:9100"
  │                                              │                                     ▼
  │                                              │                       pipeToTcp(s, "afs-controller", 9100)
  │                                              │                                     │
  │                                              │                                     ▼
  │                                              │                       net.connect → afs-ctrl:9100
  │                                              │                                     │
  │  HTTP resp ◄──────────────────────────────  │  bytes ◄──────────────────────────  │
```

> ★ **关键**：sidecar 完全感知不到 tunnel；它连的是 runner pod 的本地端口。tunnel 的存在让"远端 cp 内部服务"看起来像"本地端口"。

---

## 示例 2：直接-DB 模式（与示例 1 对比）

### 2.1 启动

```bash
# runner 直接跑在 cp 同集群，能直连 pg
RUNNER_MODE=direct
DATABASE_URL=postgres://cp:...@10.96.0.1:5432/cp
ENV_RUNNER_INTERVAL_MS=15000
node env-runner-k8s/dist/index.js
#   [env-runner-k8s] starting (mode=direct, reconcile interval 15000ms)
#   没有 tunnel data plane 日志
#   没有 reverse listener（直接模式没有）
```

### 2.2 reconcile 数据来源

```ts
// DbTransport.listPlacements 跑了什么 SQL：
const { rows } = await pool.query(`
  SELECT p.workspace_id, p.environment_id, p.desired_phase, p.spec,
         p.spec_version, p.observed_phase, p.observed_version
    FROM workspace_placements p
    JOIN environments e ON e.id = p.environment_id
   WHERE e.is_builtin = true                       // ★ 只看 built-in
`)
// → 1ms 内返回
```

### 2.3 与远端模式的关键差异

| 维度 | direct (DB) | remote (HTTP) |
|---|---|---|
| 数据来源 | 直连 pg | `GET /env/v1/placements` |
| desired 范围 | `is_builtin = true` | env-token 隐式限定 |
| 数据面 | 不需要（cp 直连 cluster） | 必需（mux tunnel） |
| 控制面 | 不需要（afs gRPC 直连） | 必需（afsctl 走 tunnel） |
| `heartbeat` | no-op | 上报 capabilities |
| 重连 | 不适用 | `TUNNEL_RECONNECT_MS` 退避 |

> 抓手：**一段 reconcile 算法、两个 transport**。这就是 `PlacementTransport` 接口的价值。

---

## 示例 3：调试"cp 看不到 workspace"

> 假设：用户报告"我创建的 ws 卡在 pending 看不到终端"。

### 3.1 第一步：确认 desired 已经写到 DB

```sql
SELECT workspace_id, environment_id, desired_phase, observed_phase,
       spec_version, observed_version, reported_at
  FROM workspace_placements
 WHERE workspace_id = 'ws-123';
```

期望：cp 端 INSERT 后能看到 `desired_phase='running'`, `observed_phase=NULL`。

### 3.2 第二步：确认是哪个 runner 在管

```sql
SELECT id, name, is_builtin FROM environments WHERE id = 'env-...';
```

- `is_builtin = true` → 看 built-in runner 日志
- `is_builtin = false` → 看远端 runner 状态（`heartbeat`）

### 3.3 内置 runner 场景

```bash
# 1. 看 reconcile 日志
kubectl logs -n platform deploy/env-runner-k8s | tail -50
#   找 "[env-runner] reconcile pass: X acted, Y noop, Z failed"
#   Z > 0 → 看 failed 行的具体 workspace_id 和 error

# 2. 看 describe 报错
kubectl logs ... | grep "ws-123"
# 常见：provider.apply 失败（k8s RBAC、image pull backoff、resource quota）
```

### 3.4 远端 runner 场景

```bash
# 1. 远端 runner 是否在跑
kubectl -n customer logs deploy/env-runner-k8s | tail -50
#   找 "[env-runner-k8s] tunnel data plane up"  ← 必须有这行

# 2. cp 端是否收到 heartbeat
#   SELECT reported_at FROM environments WHERE id = 'env-...';
#   reported_at > now() - 30s  → 在线
#   否则：runner 拨不通 cp

# 3. tunnel 拨不通的根因
#   常见：
#     - ENV_TOKEN 过期 → cp 端 WS 升级返回 401/403
#     - CP_URL 写错 → DNS 解析失败
#     - 防火墙挡了出站 WSS → 调网络
```

### 3.5 tunnel 通了但 forward 失败

```bash
# runner 端开 debug：在 onTunnelStream 里加 console.log
# 但更直接的是 tcpdump（如果 runner pod 有权限）
kubectl -n customer exec -it deploy/env-runner-k8s -- tcpdump -i any -A 'port 38000'
# 看是否有流量；如果有但 ws 不响应 → cp 端 openForwardStream 返回 null
```

### 3.6 控制面超时

```bash
# mount 报 "control frame timeout"  → handler 超过 15s
# 排查：
#   - 看看 runner 端 handler 的 gRPC 调用有没有卡住
#   - kubectl -n customer logs ... | grep "afs"
#   - afs-controller 健康吗？（in-cluster DNS 可达吗）
```

---

## 附录 A：代码片段速查

### A.1 在 runner 里加一个新控制命令

```ts
// 1. afs-control.ts 加 case
case 'snapshot': {
  const r = await unary(controller(), 'Snapshot', { fs_name: AFS_DEFAULT_FS })
  return { id: r.id }
}

// 2. cp 业务代码用 callControl
const r = await callControl<{ id: string }>(stream, { op: 'snapshot' }, 15_000)
```

### A.2 暴露一个新的 workspace 端口

```ts
// 1. tunnel-dataplane.ts 白名单加
const FORWARD_PORTS = new Set([3001, 9101, 9102, 9200])   // +9200
// 2. cp 端路由配 cp-internal service
// 3. cp 业务代码：openForwardStream(envId, `fwd:${wsId}:9200`)
```

### A.3 写一个自定义 EnvironmentProvider

```ts
import type { EnvironmentProvider, ObservedState, WorkspaceSpec } from '@neutree-ai/types/environments'

class DockerProvider implements EnvironmentProvider {
  async apply(wsId: string, spec: WorkspaceSpec): Promise<void> { /* ... */ }
  async start(wsId: string): Promise<void> { /* ... */ }
  async stop(wsId: string): Promise<void> { /* ... */ }
  async destroy(wsId: string): Promise<void> { /* ... */ }
  async observe(wsId: string): Promise<ObservedState> { /* ... */ }
  async observeAll(): Promise<Map<string, ObservedState>> { /* 一次 docker ps */ }
  capabilities() { return { sharedFs: false, persistentMemory: false } }
}
```

---

## 附录 B：术语对照

| 文档用词 | 仓库中命名 | 含义 |
|---|---|---|
| 隧道 | `tunnel` / `Mux` | cp↔runner 之间的单条双向 WSS 连接 + 字节层多路复用 |
| 子流 | `Mux.openStream` 返回的 `Duplex` | tunnel 里一条独立的双工字节流 |
| 控制流 | 走 `callControl/serveControl` 的子流 | 上面叠了一层"4 字节长度 + JSON" |
| 数据面 | 子流 ⇋ TCP 黏合 | forward / reverse 流量 |
| 控制面 | 走控制流的 RPC | 当前只有 `afsctl` |
| desired | `PlacementRow.desired_phase` + `spec` + `spec_version` | cp 写，runner 读 |
| observed | `PlacementRow.observed_phase` + `observed_version` | runner 写，cp 读 |
| reconcile | runner-core 的 `reconcileOnce` | 一次对账 pass |
| convergence | desired == observed 且 spec_version == observed_version | 稳态 |
| drift | desired != observed 或 spec_version > observed_version | 需要动作 |
| BYOI | Bring Your Own Infrastructure | 远端 runner 模式 |
| builtin | 同集群直接-DB runner | `is_builtin = true` |
