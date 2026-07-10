---
name: cp-agent-traffic
description: cp ↔ tunnel ↔ agent 之间的通信过程 - 四条链路的代码位置、direct/remote 模式差异、端到端时序
type: domain-doc
created: 2026-07-10T15:38:28Z
updated: 2026-07-10T16:44:39Z
owner: platform
status: active
---

# cp ↔ tunnel ↔ agent 通信过程

> 底层逻辑：cp 管平台控制面，agent 跑在 workspace pod 里给用户干活。当 workspace 在 cp 同集群（builtin），cp 直接用 cluster DNS 够到 agent；当 workspace 在客户集群（BYOI/remote），NAT 阻断了 cp->pod 的入站，cp 必须借 runner 反向拨出的 tunnel 把请求送过去。本文档把 **cp / runner(tunnel) / agent 三者之间的四条通信链路**拉直，每条都标到代码行。

> 与 `env-tunnel.md` 的分工：那篇讲 **mux 底层机制**（子流怎么开、字节怎么 pipe）；本篇讲 **上层通信过程**（谁调谁、走哪条路、代码在哪）。读完本篇再回头看 `env-tunnel.md` 会发现四条链路只是 mux 的四种用法。

## 1. 能力边界

| 链路 | 方向 | 用途 | 承载 | 仅 remote? |
|---|---|---|---|---|
| 0. tunnel 握手 | runner -> cp | 建立单条 mux 连接 | WSS + env token | ✅ 是 |
| 1. forward | cp -> agent | cp 访问 workspace pod 内的服务（agent HTTP / afs-fuse / memory-fuse） | mux 子流 `fwd:<wsId>:<port>` | ✅ 是 |
| 2. reverse | sidecar -> cp | workspace pod 内的 sidecar 回调 cp 内部服务 | mux 子流 `rev:cp` / `rev:afs` | ✅ 是 |
| 3. control | cp -> runner -> afs | cp 委托 runner 在客户集群执行 afs gRPC | mux 子流 `afsctl` + 长度前缀 JSON 帧 | ✅ 是 |

### 1.1 明确**不**经 tunnel 的通信

- **builtin workspace 的所有流量** -- cp 直连 cluster DNS，不建 tunnel（链路 0-3 全部消失，见 §7）
- **browser-service / sandbox-service** -- 它们是 cp 侧平台能力服务，cp HTTP 直连，不归 runner 管、不走 tunnel
- **runner 的 reconcile 数据流** -- runner 读 desired / 写 observed 走的是 `PlacementTransport`（direct=postgres，remote=`/env/v1` HTTP），**不是** tunnel。tunnel 只管数据面/控制面，不管 reconcile。这是常见误解，见 §8

> 颗粒度：tunnel 只服务于"workspace pod 与 cp 之间的实时通信"；workspace 生命周期对账（reconcile）是另一条独立的 HTTP/DB 通路。

## 2. 三个角色

```
┌────────── cp (control-plane, :3000) ──────────┐
│  env-gateway WS 路由        registry(按 envId)  │
│  remote-proxy(localhost:port)  workspace-address │
│  afs 服务(builtin 直连 gRPC / remote 走 afsctl) │
└───────────────┬─────────────────────────────────┘
                │ WSS (env token)            ↑ 业务调用
                │                            │ openForwardStream
┌───────────────▼────── runner (env-runner-k8s) ─┐
│  tunnel-client (拨出)        tunnel-dataplane   │
│  ├─ onTunnelStream: forward -> dial pod          │
│  ├─ startReverseListeners: sidecar -> rev:cp     │
│  └─ afs-control: afsctl RPC -> gRPC pod:9101     │
│  reconcile loop (PlacementTransport, 独立于 tunnel)│
└───────────────┬──────────────────────────────────┘
                │ cluster DNS            ↑ sidecar TCP
┌───────────────▼────── workspace pod ──────────────┐
│  agent (:3001)   afs-fuse (:9101)   memory-fuse (:9102)│
│  sidecar: afs-controller (:9100) -- 回调 cp 用    │
└────────────────────────────────────────────────────┘
```

| 角色 | 进程位置 | 职责 | 本文关心的部分 |
|---|---|---|---|
| **cp** | 平台集群 | 平台控制面 | 怎么发请求到 agent、怎么收 sidecar 回调 |
| **runner** | remote: 客户集群 / direct: 平台集群 | workspace 对账 + tunnel 数据面 | 四条链路的 runner 端落点 |
| **agent** | workspace pod 内 :3001 | 执行用户 turn | 被谁访问、怎么被访问 |

## 3. 两种模式总览

| 维度 | direct（builtin） | remote（BYOI） |
|---|---|---|
| runner 位置 | 平台集群（跟 cp 同集群） | 客户集群 |
| cp 访问 agent | cluster DNS 直连 `tos-${wsId}:3001` | localhost 代理 -> tunnel forward |
| sidecar 回调 cp | 直连 cluster DNS | -> runner :38000 -> tunnel reverse |
| afs 操作 | cp 直连 gRPC（controller + fuse） | tunnel control `afsctl` 委托 runner |
| tunnel 握手 | **不存在** | 必需（runner 反向拨出） |
| reconcile 数据 | 直连 postgres | `GET/POST /env/v1/*` + env token |

> 抓手：**同一套 cp 业务代码，在 `getWorkspaceAddress` / `afsEnvForWorkspace` 这两个分界点分流**。builtin 走 cluster DNS / 本地 gRPC，remote 走 localhost 代理 / afsctl。分流逻辑集中在一处，不散落到 ~11 个 forward fetch 站点。

## 4. 链路 0：tunnel 握手（runner -> cp）

> 场景：remote 模式 runner 启动后，拨一条 WSS 到 cp 的 `/env-gateway`，建立唯一的 mux 连接。后续链路 1-3 全部复用这条连接的子流。

### 4.1 时序

```
runner                                                        cp
  │                                                            │
  │  WS upgrade: GET /env-gateway                              │
  │  Authorization: Bearer <env-token>                         │
  │ ──────────────────────────────────────────────────────►   │
  │                                                            │
  │                                            verifyEnvironmentToken  (env-gateway.ts:48)
  │                                                            │
  │  101 Switching Protocols                                    │
  │ ◄──────────────────────────────────────────────────────    │
  │                                                            │
  │  createWebSocketStream(ws) + new Mux(conn)                 │
  │ ────── mux 子流可开 ──────────────────────────────────►   │
  │                                                            │
  │                                            registerSession({envId, mux})  (registry.ts:18)
  │                                            onStreamOpen = (s, meta) =>     (env-gateway.ts:65)
  │                                              reverseTarget(meta) -> pipeToTcp
  │                                                            │
  │  startReverseListeners(getMux)                              │  (tunnel-dataplane.ts:59)
  │  listen(38000), listen(39100)                              │
  │                                                            │
  │  ✅ 数据面 ready                                            │
```

### 4.2 代码位置

| 步骤 | 文件:行 | 说明 |
|---|---|---|
| runner 拨出 | `env-runner-k8s/src/tunnel-client.ts:17` | `connectTunnel({gatewayUrl, token, onStream, onClose})` |
| runner 重连循环 | `env-runner-k8s/src/index.ts:39` | `startTunnel` 的 `for(;;)` + backoff |
| runner 启动 reverse listener | `env-runner-k8s/src/index.ts:42` | `startReverseListeners(() => currentMux)` |
| cp WS 升级入口 | `control-plane/src/routes/env-gateway.ts:36` | `app.get('/', upgradeWebSocket(...))` |
| cp 验证 token | `env-gateway.ts:48` | `verifyEnvironmentToken(token)`，失败 `ws.close(1008)` |
| cp 建 mux | `env-gateway.ts:61` | `new Mux(conn)`，`conn = createWebSocketStream(raw)` |
| cp 注册 session | `registry.ts:18` | `registerSession({environmentId, mux})` |
| cp supersede 旧 session | `registry.ts:22` | 新连接替换 stale 旧连接 |

> ⚖ 设计决策（env-gateway.ts:8-10）：reverse 目标是**符号枚举**（`rev:cp` / `rev:afs`），不是自由 `host:port`。被劫持的 runner 无法借 gateway 扫 cp 内网。

### 4.3 失败与重连

- env token 无效 -> cp 返回 1008，runner backoff 重试
- WSS 断开 -> `tunnel-client.ts:45` `onClose` 触发，`index.ts:62` `await sleep(TUNNEL_RECONNECT_MS)` 后重拨
- 重连时 reverse listener **不重绑端口**（`tunnel-dataplane.ts:59` 用 `getMux()` 工厂，mux 不可用时 sidecar 连进来被 `socket.destroy()`）

## 5. 链路 1：cp -> agent（forward，HTTP 业务字节流）

> 场景：cp 要给某个 workspace 的 agent 发 HTTP 请求（chat turn、health 探测、reload-config、skills 拉取等）。builtin 直连 cluster DNS；remote 经 localhost 代理走 tunnel。

### 5.1 路由分界点：`getWorkspaceAddress`

**这是整个 forward 流量的总开关**（`workspace-address.ts:25`）：

```ts
export function getWorkspaceAddress(workspaceId: string): string {
  const remotePort = getRemoteProxyPort(workspaceId)        // remote-proxy.ts:22
  if (remotePort !== undefined) return `http://127.0.0.1:${remotePort}`   // remote
  return builtinAddress(workspaceId)                       // builtin: cluster DNS
}

function builtinAddress(workspaceId: string): string {
  return `http://tos-${workspaceId}.${NAMESPACE}.svc.cluster.local:${AGENT_PORT}`   // :3001
}
```

> ★ 关键不变量（注释 `workspace-address.ts:14-24`）：builtin workspace 永远不进 remote-proxy map，所以 builtin 路径**字节级一致**（cluster DNS，零额外开销）。remote 只是一个同步 O(1) map 查找。~11 个 forward fetch 站点都调这个函数，分流逻辑集中在一处。

### 5.2 remote 模式完整链路

```
cp 业务代码
  │  fetch(`${getWorkspaceAddress(wsId)}/health`)   workspace-address.ts:80
  │
  ▼
http://127.0.0.1:<proxyPort>                          remote-proxy.ts:53 server.listen(0,'127.0.0.1')
  │
  │  net.createServer 入站连接                        remote-proxy.ts:39
  ▼
openForwardStream(envId, `fwd:${wsId}:3001`)          registry.ts:35  -> session.mux.openStream(meta)
  │
  │  mux 子流字节经 WSS                                (env-tunnel.md §3.4)
  ▼
runner: onTunnelStream(stream, "fwd:ws1:3001")        tunnel-dataplane.ts:46
  │
  ▼
resolveForwardTarget("fwd:ws1:3001")                  tunnel-dataplane.ts:23
  │  正则 ^fwd:([^:]+):(\d+)$                          :24
  │  port in FORWARD_PORTS? ✅ 3001                     :27 (白名单 :16)
  │  -> { host: "tos-ws1.default.svc:3001", port: 3001 }:28
  ▼
pipeToTcp(stream, host, port)                         tunnel-dataplane.ts:38
  │
  ▼
net.connect -> pod:3001                               pipe.ts:11
  │
  │  socket.pipe(stream).pipe(socket)                  pipe.ts:18  (双向字节管)
  ▼
agent 收到 HTTP 请求，回响应
```

### 5.3 forward 白名单

```ts
// tunnel-dataplane.ts:16
const FORWARD_PORTS = new Set([3001, 9101, 9102])
// 3001 -> agent HTTP
// 9101 -> afs-fuse gRPC
// 9102 -> memory-fuse gRPC
```

> ⚖ 设计决策（tunnel-dataplane.ts 注释 + env-tunnel.md §4.1）：cp 不能任意 `host:port`；白名单限定 3 个 workspace pod 端口。否则被劫持 runner 可以 cp 身份扫客户集群内网。**新增端口要同步改两端**：runner 的 `FORWARD_PORTS` + cp 端 `openForwardStream` 调用方（见 usage-examples.md 附录 A.2）。

### 5.4 代码位置

| 步骤 | 文件:行 | 说明 |
|---|---|---|
| cp 统一入口 | `control-plane/src/lib/workspace-address.ts:25` | `getWorkspaceAddress` |
| cp builtin 地址 | `workspace-address.ts:7` | cluster DNS `tos-${wsId}:3001` |
| cp remote 代理建立 | `control-plane/src/lib/remote-proxy.ts:31` | `ensureRemoteProxy`，每个 remote workspace 一个 localhost 端口 |
| cp 代理入站处理 | `remote-proxy.ts:39` | `createServer((socket) => openForwardStream(...))` |
| cp 开流 | `registry.ts:35` | `openForwardStream(envId, meta)` -> `session.mux.openStream(meta)` |
| cp 代理生命周期 | `services/env-projection.ts:69` | observe-running 时 `ensureRemoteProxy`，stop 时 `dropRemoteProxy` |
| runner 收流 | `env-runner-k8s/src/tunnel-dataplane.ts:46` | `onTunnelStream` dispatch |
| runner 解析 meta | `tunnel-dataplane.ts:23` | `resolveForwardTarget` 正则 + 白名单 |
| runner 连 pod | `tunnel-dataplane.ts:38` | `pipeToTcp(stream, host, port)` |
| 白名单 | `tunnel-dataplane.ts:16` | `FORWARD_PORTS = {3001, 9101, 9102}` |

### 5.5 代理生命周期

- **建立**：cp 观察到 remote workspace 变 running（`env-projection.ts:69` `ensureRemoteProxy`）
- **复用**：同一 workspace 的后续请求都走同一个 localhost 端口（`remote-proxy.ts:36` 幂等）
- **销毁**：workspace stop/destroy/env offline 时 `dropRemoteProxy`（`remote-proxy.ts:60`）

## 6. 链路 2：sidecar -> cp（reverse，业务字节流）

> 场景：workspace pod 内的 sidecar（afs-controller、memory-fuse 等）需要回调 cp 内部服务。sidecar 不会自己跨 NAT -- 它连 runner pod 的**本地 TCP 端口**，runner 把这些本地连接翻译成 mux 反向流。

### 6.1 链路图

```
sidecar (in pod)              runner pod                    cp process
   │                              │                              │
   │  TCP connect <runner-IP>:38000                              │
   ├─────────────────────────────►│                              │
   │                              │ listenAndTunnel(38000, ...)  │  tunnel-dataplane.ts:61
   │                              │   on(socket) {               │  pipe.ts:26
   │                              │     s = getMux()?.openStream('rev:cp')  │
   │                              │     socket.pipe(s).pipe(socket)│
   │                              │   }                          │
   │                              │ ── substream "rev:cp" ──►    │
   │                              │                              ▼
   │                              │                  onStreamOpen(s, "rev:cp")  env-gateway.ts:65
   │                              │                  reverseTarget("rev:cp")      env-gateway.ts:14
   │                              │                    -> { 127.0.0.1, 3000 }       :16
   │                              │                              ▼
   │                              │                  pipeToTcp(s, "127.0.0.1", 3000) env-gateway.ts:71
   │                              │                              ▼
   │                              │                  net.connect -> cp:3000          pipe.ts:11
   │                              │                              │
   │  HTTP resp ◄───────────────  │  bytes ◄──────────────────── │
```

### 6.2 符号枚举表

| `meta` | runner 侧端口 | cp 侧目标 | 含义 |
|---|---|---|---|
| `rev:cp` | 38000 (`CP_PROXY_PORT`) | `127.0.0.1:3000` (cp) | sidecar 回调 cp HTTP API |
| `rev:afs` | 39100 (`AFS_PROXY_PORT`) | `afs-controller.default.svc:9100` | sidecar 回调 afs-controller |

```ts
// env-gateway.ts:11-13
const CP_INTERNAL_ADDR  = process.env.CP_INTERNAL_ADDR  || `127.0.0.1:${process.env.PORT || '3000'}`
const AFS_CONTROLLER_ADDR = process.env.AFS_CONTROLLER_ADDR || 'afs-controller.default.svc:9100'

// env-gateway.ts:14-20
function reverseTarget(meta: string): { host: string; port: number } | null {
  const addr =
    meta === 'rev:cp' ? CP_INTERNAL_ADDR :
    meta === 'rev:afs' ? AFS_CONTROLLER_ADDR : null
  if (!addr) return null           // ★ 未知 meta 直接 drop
  ...
}
```

### 6.3 sidecar 如何区分 remote 和 builtin 模式

> **答案：sidecar 自己不做区分**。区分发生在**配置生成阶段** -- k8s-provider 根据 runner 进程自身的 `CP_SERVICE_URL` 环境变量，给 workspace pod 注入不同的 `CP_URL` 值。sidecar 只读这个 env 并连到给定地址，对模式无感知。

**workspace pod env `CP_URL` 的两条路径：**

| 模式 | runner 进程的 `CP_SERVICE_URL` | workspace pod 的 `CP_URL` | sidecar 实际连到 | 走 tunnel? |
|---|---|---|---|---|
| builtin | 未设置（默认 `http://nap-cp:3000`） | `http://nap-cp:3000` | cp cluster DNS 直连 | ❌ 否 |
| remote | `http://<runner-svc>:38000`（deployment.yaml 注入） | `http://<runner-svc>:38000` | runner reverse listener -> `rev:cp` 子流 -> cp | ✅ 是 |

配置生成代码流：
```
runner 进程 env: CP_SERVICE_URL
  └─> config.ts:79        cpServiceUrl = process.env.CP_SERVICE_URL || 'http://nap-cp:3000'
       └─> workspace-spec.ts:66   agentContainer.env: [{ name: 'CP_URL', value: cfg.cpServiceUrl }]
            └─> workspace pod 的 sidecar 启动时读 CP_URL，connect 这个地址
```
不要在 self-host 目录下去找 `CP_SERVICE_URL`。self-host 下默认不设置，使用的代码里的默认值。

**remote 模式下 `CP_SERVICE_URL` 的来源**（`charts/env-runner-k8s/templates/deployment.yaml:49-52`）：
```yaml
# Sidecars reach the control plane back through the runner's reverse
# tunnel listeners (this chart's Service).
- name: CP_SERVICE_URL
  value: "http://{{ include "env-runner-k8s.fullname" . }}:{{ .Values.tunnel.cpProxyPort }}"
```

> ★ 抓手：**模式区分是"配置时"的，不是"运行时"的**。k8s-provider 在生成 workspace pod spec 时已经把正确的地址烤进了 env，sidecar 启动后只管连 env 给的地址。这就是为什么 §6.1 的链路图里 sidecar 只画了"TCP connect runner-IP:38000" -- 在 remote 模式下 `CP_URL` 就是 runner 的 service 地址。sidecar 不知道、也不需要知道自己是 builtin 还是 remote。

同理，`AFS_CONTROLLER`（`workspace-spec.ts:70` = `cfg.afs.controllerAddr` = `AFS_CONTROLLER_ADDR` env）也由 runner 进程所在集群决定：
- builtin: 指向平台集群的 afs-controller（cp 直连 gRPC，见 §7.3）
- remote: `deployment.yaml:89-90` 指向客户集群的 afs-controller（sidecar cluster DNS 直连，不经 tunnel）

### 6.4 代码位置

| 步骤 | 文件:行 | 说明 |
|---|---|---|
| runner 起 reverse listener | `env-runner-k8s/src/tunnel-dataplane.ts:59` | `startReverseListeners(getMux)` |
| runner `rev:cp` 监听 | `tunnel-dataplane.ts:61` | `listenAndTunnel(38000, '0.0.0.0', () => getMux()?.openStream('rev:cp'))` |
| runner `rev:afs` 监听 | `tunnel-dataplane.ts:62` | `listenAndTunnel(39100, ..., 'rev:afs')` |
| runner 端口配置 | `tunnel-dataplane.ts:19-20` | `CP_PROXY_PORT` / `AFS_PROXY_PORT`（env 可覆盖） |
| cp 收 reverse 流 | `control-plane/src/routes/env-gateway.ts:65` | `mux.onStreamOpen((stream, meta) => ...)` |
| cp 符号 -> 地址 | `env-gateway.ts:14` | `reverseTarget(meta)` |
| cp 连本地服务 | `env-gateway.ts:71` | `pipeToTcp(stream, target.host, target.port)` |

> ★ 关键设计（env-tunnel.md §7.1）：`openStream` 是**惰性**的（每次 sidecar 连进来才调），且能返回 `null`。mux 还没握上时 sidecar 连进来 -> `socket.destroy()`。这让 reverse listener 在 mux 重连时**不需要重绑端口**。

## 7. 链路 3：cp -> runner -> afs（control，RPC）

> 场景：cp 要对 workspace 做 afs 操作（createDir / mount / unmount / revokeDir）。builtin: cp 直连 gRPC；remote: cp 通过 `afsctl` control 流委托 runner，runner 在客户集群内调 pod 的 afs-fuse gRPC。

### 7.1 路由分界点：`afsEnvForWorkspace`

```ts
// services/afs.ts:22
export async function afsEnvForWorkspace(workspaceId: string): Promise<AfsEnv> {
  const p = await getWorkspacePlacementEnv(workspaceId)
  return p
    ? { environmentId: p.environmentId, isBuiltin: p.isBuiltin }
    : { environmentId: 'builtin', isBuiltin: true }   // 无 placement 行视为 builtin
}
```

每个 afs 操作（`createDir` / `mountAtWorkspace` / ...）内部第一行就是 `if (!env.isBuiltin)` 分流：

```ts
// services/afs.ts:87 (createDir 为例)
export async function createDir(env: AfsEnv, fsName = AFS_DEFAULT_FS): Promise<AfsDir> {
  if (!env.isBuiltin) {
    const r = await callRemote(env, { op: 'createDir', fsName })    // -> afsctl control 流
    return { id: r.id, accessKey: r.accessKey, permission: r.permission }
  }
  // builtin: 直连 afs-controller gRPC
  const res = await unary(controllerClient, 'CreateDir', { fs_name: fsName })
  ...
}
```

### 7.2 remote 链路（afsctl control 流）

```
cp 业务代码
  │  await createDir(env, fsName)                       services/afs.ts:87
  │  env.isBuiltin === false
  ▼
callRemote(env, { op: 'createDir', fsName })           afs.ts:30
  │
  ▼
openControlStream(envId, 'afsctl')                      registry.ts:45 -> session.mux.openStream('afsctl')
  │
  ▼
callControl(stream, req, 15_000)                        internal/env-tunnel/control.ts:76
  │  writeFrame(stream, req)  [4字节长度 + JSON]        control.ts:64
  │  await readFrame(stream)                            control.ts:80
  ▼
─── mux 子流 "afsctl" 经 WSS ───►
  │
runner: onTunnelStream(stream, "afsctl")                tunnel-dataplane.ts:47
  │  if (meta === 'afsctl') handleAfsControl(stream)    :47-49
  ▼
handleAfsControl(stream)                                afs-control.ts:114
  │  serveControl(stream, dispatch)                     control.ts:95
  │    readFrame -> dispatch(req) -> writeFrame(res)    control.ts:101-108
  ▼
dispatch({ op: 'createDir', fsName })                   afs-control.ts (dispatch)
  │  case 'createDir': unary(controller(), 'CreateDir', ...)  -> pod:9100 gRPC
  ▼
afs-controller (客户集群) 返回 { id, access_key, permission }
  │
  │  writeFrame(stream, res)                            control.ts:108
  ▼
─── 响应帧经 mux 回 cp ───►
  │
cp: readFrame 解析                                      control.ts:80
  │  stream.destroy()                                   control.ts:86 (单流单请求，不复用)
  ▼
返回 { id, accessKey, permission }
```

### 7.3 builtin 链路（cp 直连 gRPC）

```ts
// services/afs.ts:60-68 (getFuseClient)
function getFuseClient(workspaceId: string) {
  const addr = `tos-${workspaceId}.${NAMESPACE}.svc:9101`      // cluster DNS -> pod
  c = new afsPkg.FuseService(addr, grpc.credentials.createInsecure())
  ...
}

// services/afs.ts:54 (controllerClient，单例)
const controllerClient = new afsPkg.ControllerService(
  AFS_CONTROLLER_ADDR,    // afs-controller.default.svc:9100
  grpc.credentials.createInsecure(),
)
```

builtin 模式下 cp 直接用 cluster DNS 连 `afs-controller:9100`（集群级）和 `tos-${wsId}:9101`（pod 级 fuse）。不经 tunnel。

### 7.4 代码位置

| 步骤 | 文件:行 | 说明 |
|---|---|---|
| cp 分流入口 | `control-plane/src/services/afs.ts:22` | `afsEnvForWorkspace` |
| cp remote 调用 | `afs.ts:30` | `callRemote` -> `openControlStream` + `callControl` |
| cp builtin controller | `afs.ts:54` | `controllerClient`（afs-controller gRPC 单例） |
| cp builtin fuse | `afs.ts:60` | `getFuseClient`（per-workspace fuse gRPC） |
| cp 开 control 流 | `registry.ts:45` | `openControlStream(envId, 'afsctl')` |
| cp RPC 客户端 | `internal/env-tunnel/control.ts:76` | `callControl`（写帧->读帧->关流） |
| runner 收 afsctl | `tunnel-dataplane.ts:47` | `if (meta === 'afsctl') handleAfsControl(stream)` |
| runner RPC 服务端 | `env-runner-k8s/src/afs-control.ts:114` | `serveControl(stream, dispatch)` |
| runner dispatch | `afs-control.ts` | `op` -> gRPC 调用（mount/unmount/createDir/...） |

> ⚖ 为什么 control 用 RPC 帧而不是 forward 字节流？afs gRPC 调用需要**应答**（mount 成功/失败 + 错误信息），raw byte pipe 不携带语义。`callControl` + `serveControl` 是在 mux 子流之上的薄协议，仅这一处使用。（env-tunnel.md §4.3）

## 8. direct 模式：哪些链路消失

> 场景：builtin environment，runner 跟 cp 同集群，cp 能直连 cluster DNS。

| 链路 | direct 模式行为 |
|---|---|
| 0. tunnel 握手 | **不执行**。`index.ts:69` `if (MODE === 'remote')` 才 startTunnel |
| 1. forward | `getWorkspaceAddress` 返回 cluster DNS，cp 直连 pod，**无 localhost 代理** |
| 2. reverse | sidecar 直连 cp 的 cluster DNS，**无 reverse listener** |
| 3. control | `afsEnvForWorkspace` 返回 `isBuiltin: true`，cp 直连 gRPC，**无 afsctl 流** |

```ts
// env-runner-k8s/src/index.ts:68-80
let transport: DbTransport | HttpTransport
if (MODE === 'remote') {
  ...
  transport = new HttpTransport(cpUrl, token)
  startTunnel(cpUrl, token)           // ★ 只有 remote 才建 tunnel
} else {
  transport = new DbTransport()       // direct: 直连 postgres
}
```

> 抓手：**MODE 只影响数据面/控制面是否走 tunnel，不影响 reconcile 算法本身**。reconcile 代码（`env-runner-core/reconcile.ts`）对两种模式完全一致，差异全在 `PlacementTransport` 接口的两个实现里（见 `env-runner-core.md` §6）。

## 9. 端到端时序：一个 chat turn 的完整流程

> 把四条链路串成一个真实场景：remote 模式下，用户发一条 chat 消息，cp 转发到 agent，agent 处理时需要挂载 afs 目录。

```
用户 ──chat──► cp
                │
                │  getWorkspaceAddress(wsId)          workspace-address.ts:25
                │  -> http://127.0.0.1:41234 (remote)  remote-proxy.ts
                │
                │  fetch("http://127.0.0.1:41234/chat", {...})
                │       │
                │       ▼ [链路1 forward]
                │  remote-proxy 入站 -> openForwardStream("fwd:ws1:3001")
                │  -> mux 子流 -> runner -> pipeToTcp -> pod:3001
                │       │
                │       ▼
                │  agent 收到 chat，开始处理 turn
                │
                │  (agent 处理中需要挂载 afs 目录)
                │  agent -> cp MCP: "mount afs"        (经链路2 reverse rev:cp 回 cp)
                │       │
                │       ▼ [链路2 reverse]
                │  sidecar -> runner:38000 -> "rev:cp" -> cp:3000
                │
                │  cp 收到 mount 请求
                │  afsEnvForWorkspace(wsId)            afs.ts:22
                │  -> isBuiltin: false (remote)
                │       │
                │       ▼ [链路3 control]
                │  openControlStream(envId, "afsctl")
                │  callControl(stream, { op: 'mount', wsId, dirId, ... })
                │  -> mux 子流 -> runner -> handleAfsControl -> gRPC pod:9101
                │  <- { ok: true }
                │
                │  cp 返回 mount 成功给 agent
                │       │
                │       ▼ [链路2 reverse 回程]
                │  cp:3000 -> "rev:cp" -> runner:38000 -> sidecar -> agent
                │
                │  agent 继续 turn，完成后响应 chat
                │       │
                │       ▼ [链路1 forward 回程]
                │  agent:3001 -> pipeToTcp -> mux 子流 -> remote-proxy -> cp
                │
用户 ◄──chat resp── cp
```

> ★ 一个 turn 用到了全部三条 tunnel 链路（forward + reverse + control）。链路 0（tunnel 握手）在 runner 启动时就已完成，后续所有 turn 复用同一条 mux 连接。

## 10. 关键代码索引

### cp 侧

| 关注点 | 文件:行 |
|---|---|
| env-gateway WS 路由（runner 拨入点） | `control-plane/src/routes/env-gateway.ts:36-86` |
| env token 验证 | `env-gateway.ts:48` |
| mux 创建 + onStreamOpen | `env-gateway.ts:61-72` |
| reverse 符号表 + reverseTarget | `env-gateway.ts:11-20` |
| session registry（按 envId 查 mux） | `control-plane/src/services/env-gateway/registry.ts:11-48` |
| `openForwardStream`（cp 开 forward 子流） | `registry.ts:35` |
| `openControlStream`（cp 开 afsctl 子流） | `registry.ts:45` |
| **forward 总开关** `getWorkspaceAddress` | `control-plane/src/lib/workspace-address.ts:25` |
| builtin agent 地址（cluster DNS） | `workspace-address.ts:7` |
| remote localhost 代理 | `control-plane/src/lib/remote-proxy.ts:31-66` |
| 代理生命周期（建/拆） | `control-plane/src/services/env-projection.ts:69` |
| **afs 分流** `afsEnvForWorkspace` | `control-plane/src/services/afs.ts:22` |
| afs remote 调用（callRemote） | `afs.ts:30` |
| afs builtin gRPC（controller / fuse） | `afs.ts:54, 60` |

### runner 侧

| 关注点 | 文件:行 |
|---|---|
| MODE 选择 + startTunnel | `env-runner-k8s/src/index.ts:27, 69-80` |
| tunnel-client（拨出 + mux） | `env-runner-k8s/src/tunnel-client.ts:17-51` |
| 重连循环 | `env-runner-k8s/src/index.ts:39-66` |
| forward dispatch（onTunnelStream） | `env-runner-k8s/src/tunnel-dataplane.ts:46-52` |
| forward 白名单 + resolveForwardTarget | `tunnel-dataplane.ts:16, 23-29` |
| reverse listener（rev:cp / rev:afs） | `tunnel-dataplane.ts:59-64` |
| afs control handler | `env-runner-k8s/src/afs-control.ts:114` |
| afs dispatch（op -> gRPC） | `afs-control.ts` |

### 底层 tunnel（详见 env-tunnel.md）

| 关注点 | 文件:行 |
|---|---|
| `Mux` 类 | `internal/env-tunnel/mux.ts:18-53` |
| `pipeToTcp`（子流 ⇋ TCP） | `internal/env-tunnel/pipe.ts:10-19` |
| `listenAndTunnel`（本地端口 -> 子流） | `internal/env-tunnel/pipe.ts:26-48` |
| `callControl` / `serveControl` | `internal/env-tunnel/control.ts:76-113` |
| `readFrame` / `writeFrame` | `internal/env-tunnel/control.ts:16-69` |

## 11. 调试 Checklist

| 现象 | 第一时间看 |
|---|---|
| remote workspace 的 agent 不可达 | `getRemoteProxyPort(wsId)` 返回 undefined？`ensureRemoteProxy` 是否在 observe-running 时被调？registry 里有没有对应 envId 的 session？ |
| forward 请求卡住 | `openForwardStream` 返回 null？（runner 离线）`resolveForwardTarget` 白名单命中？pod 在不在、3001 通不通？ |
| sidecar 连不上 cp | runner :38000 是否 listen？`getMux()` 返回 null？（tunnel 断）cp 端 `reverseTarget` 符号对不对？ |
| afs mount 超时 | `afsEnvForWorkspace` 返回 isBuiltin 对不对？remote 走 `afsctl` 的话 runner 端 `handleAfsControl` 有没有收到？gRPC 到 pod:9101 通不通？ |
| builtin 误走了 tunnel | `getWorkspacePlacementEnv` 返回的 `isBuiltin` 对不对？builtin workspace 不该进 remote-proxy map |
| tunnel 断了不恢复 | `TUNNEL_RECONNECT_MS` 设了没？env token 过期没？cp 端 `/env-gateway` 返回 401/403？ |
