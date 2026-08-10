# AgentFS (AFS) 架构走读

> 对象:`self-host/manifests/afs.yaml` + `control-plane/proto/afs.proto` + `control-plane/src/services/afs.ts` + `env-runner-k8s/src/afs-control.ts`
> 目标:讲清楚 AFS 解决什么问题、四个角色怎么配合、一次"创建/授权/读文件/撤销"的完整调用链,以及对账 `afs-control.ts` 里 `dispatch` 的每一种 `op` 对应什么业务动作。

---

## 1. AFS 真正解决的问题

**AFS 是"跨 workspace 共享文件"的 capability 层,不是"workspace 持久化的替代品"。**

NAP 里每个 Agent 跑在一个 workspace pod 里,workspace 自己的 `/workspace` 持久化由它自己的 PVC 负责(那是另一条链路,跟 AFS 无关)。AFS 解决的是一个完全不同的问题:

> Agent A 想把一批文件(需求文档、测试用例、生成的图片/PDF/代码包)交给 Agent B,希望 B 能在自己的 workspace 里像读本地文件一样读到这些文件 —— 而不是把文件 base64 塞进 prompt,或者挂一个外部对象存储让 B 自己拉。

证据链:

- MCP 工具名叫 `share_folder` / `grant_access` / `unshare_from_all`(`control-plane/src/mcp/tools/afs.ts:36,77,131`),命名直接对应业务
- 描述写的是"hand files off to another agent"、"target agent mounts the folder at /mnt/afs/<name>"
- Agent 侧 `agents/claude-code/src/index.ts:153` 只有一个常量:`AFS_MOUNT_BASE = '/mnt/afs'`,通过 dufs 暴露在 8001 端口,`server.ts:524` 用 `/afs-files/*` 反向代理
- **AFS 不管 workspace 自己的 `/workspace` / `/prompts` / `/files`** — README 老版本里把这几个路径写成 AFS 挂载点,这是错的,那是 workspace 自己的事

> 顶层设计:把 AFS 当成一个"挂载即赋权"的文件保险箱。**创建文件夹 = 拿到一把钥匙(dir_id + access_key);授权 = 把钥匙给别人的 fuse sidecar;撤销 = 控制器作废钥匙并强制所有挂载点掉线。**

---

## 2. 四个角色(及它们在哪个进程里跑)

| 角色 | 跑在哪 | 端口 | 职责 | 关键配置 |
|---|---|---|---|---|
| **afs-controller** | 独立 Deployment,`replicas:1` | 9100 | dir/accessKey 颁发、token 校验、会话/撤销中心 | SQLite 存 dirs 表,Recreate 策略 |
| **afs-fuse sidecar** | 每个 workspace pod 注入 | 9101 | FUSE 挂载 `/data/afs` 到 `/mnt/afs/<name>`,**先找 controller 验 token 再挂** | `AFS_BOOTSTRAP_URL` 自愈拉取挂载列表 |
| **afs storage** | PVC (`afs-shared-storage`,RWX) | — | 所有 share 共享的物理存储后端 | `fs_type: 'local'`, `base_path: '/data/afs'` |
| **control-plane / runner** | CP 进程 / env-runner-k8s 进程 | — | 把 CP 业务语义(createDir / mount / revoke)翻译成 gRPC | 内置环境直连 gRPC,BYOI 环境走 tunnel 的 `afsctl` 流 |

老 README 把"afs-shared-storage PVC"当主角,实际上它只是**后端存储**。主角是 controller(发证)+ fuse sidecar(验证 + FUSE)+ CP/runner(翻译业务)。

---

## 3. 数据模型:三张表,两张映射,`name` 只活在 CP 这一侧

老 README 那张"dir_id → base_path → workspace_id"表是错的。真实模型(看 `control-plane/proto/afs.proto` 消息定义 + `control-plane/src/services/db/afs-shares.ts`)是这样的:

```
controller.db (SQLite, 跑在 afs-controller 里)
┌────────────────────────────────────────────────────────┐
│ filesystems                                           │
│   name        fs_type   config                        │
│   "shared"    "local"   { base_path: "/data/afs" }    │
└────────────────────────────────────────────────────────┘
┌────────────────────────────────────────────────────────┐
│ dirs          ←  没有 name 列!  dir 就是一把钥匙         │
│   id     fs_name    permission   status   created_at   │
│   <uuid> "shared"   READ_WRITE   active   ...          │
│                                                        │
│ CreateDir(fs_name) → { id, access_key, permission }    │
└────────────────────────────────────────────────────────┘
┌────────────────────────────────────────────────────────┐
│ sessions  (sidecar RegisterSession 后写入)              │
│   session_id  dir_id  mountpoint  stream_id            │
└────────────────────────────────────────────────────────┘

cp DB (PostgreSQL, 跑在 control-plane 里)
┌─────────────────────────────┐  ┌──────────────────────────────┐
│ afs_shares                  │  │ afs_share_members            │
│  id                         │  │  share_id                    │
│  owner_workspace_id         │──│  workspace_id                │
│  name         ←  share 的  │  │  permission  read_only|      │
│               显示名,只存  │  │             read_write       │
│               在 CP 这一侧 │  │  mounted_at                  │
│  afs_dir_id                 │  └──────────────────────────────┘
│  access_key  ← controller  │
│              给的,CP 也存  │
│  created_at                 │
└─────────────────────────────┘
```

### 3.1 三层命名,千万别混

| 名字 | 在哪 | 例子 | 用途 |
|---|---|---|---|
| **filesystem name** | `controller.filesystems.name` | `"shared"` | 唯一,一个环境一个;`RegisterFs` 注册一次,后续 `CreateDir` 用 `fs_name` 引用 |
| **share name** | `cp.afs_shares.name` | `"req"` | 人类可读,出现在 mountpoint 路径 `/mnt/afs/<share_name>`;CP 的幂等键;**controller 完全不知道这个字段** |
| **dir id** | `controller.dirs.id` | `8f3a-...-c91` | controller 发的 capability 句柄;fuse sidecar `Mount(id, ...)` 用的就是这个;**没有名字,只有 id + access_key** |

> **关键结论**:`getAfsShareByName(wsA, "req")` 是**纯 CP 侧 PostgreSQL 查询**,查的是 `afs_shares WHERE owner_workspace_id = $1 AND name = $2`(见 `control-plane/src/services/db/afs-shares.ts:36-45`)。它根本不跟 controller 通信,也不会去问 controller "有没有叫 req 的 dir"。controller 那边对"req"这个名字一无所知 —— controller 眼里只有 `(dir.id, dir.access_key)`,看到这两个就能 ValidateToken 给你权限。

### 3.2 一次 `share_folder("req")` 跑完后,两张 db 里的实际数据示例

**CP PostgreSQL**(`SELECT * FROM afs_shares JOIN afs_share_members`):
```
afs_shares:
  id          = "shr_01HXY..."
  owner_workspace_id = "ws-aaa-111"
  name        = "req"           ← 人类可读,只在这里
  afs_dir_id  = "8f3a-c91..."   ← 指向 controller 那一行
  access_key  = "ak_live_..."   ← controller 给的,CP 留底
  created_at  = "2026-07-06T16:30:00Z"

afs_share_members:
  share_id     = "shr_01HXY..."
  workspace_id = "ws-aaa-111"
  permission   = "read_write"   ← owner 给自己加的
  mounted_at   = "2026-07-06T16:30:01Z"
```

**Controller SQLite**(`SELECT * FROM filesystems; SELECT * FROM dirs`):
```
filesystems:
  name     = "shared"
  fs_type  = "local"
  config   = { base_path: "/data/afs" }

dirs:
  id          = "8f3a-c91..."   ← 和 afs_shares.afs_dir_id 对得上
  fs_name     = "shared"       ← 引用上面那个 filesystem
  permission  = 1 (READ_WRITE)
  status      = "active"
  created_at  = "2026-07-06T16:30:00Z"
  ─────────────
  ← 没有 name 列! 也没有 owner 列!  controller 不知道 "req" 是谁的
```

### 3.3 关键点

- `name` **只存在于 CP 的 `afs_shares`**,controller.db 里没有任何"分享名字"的概念。controller 看到的就是一把钥匙(id + access_key),不关心这个钥匙叫什么、属于谁
- `base_path` 存在 `filesystems.config` 里,**不是**存在 `dirs` 行里。一个 filesystem 一份 base_path,所有 dir 共享它
- `access_key` 是 controller 颁发给 CP 的 bearer secret,CP 在 `afs_shares.access_key` 里**留底** —— 因为授权新成员时要把它交给对方的 fuse sidecar
- `workspace_id ↔ dir_id` 的关系**完全在 CP 侧维护**(`afs_shares.owner_workspace_id` + `afs_share_members`),controller 不知道 workspace 这个概念
- CP 拿 `name` 当幂等键 — `getAfsShareByName` 命中就直接返回**已有的 share**(`afs-shares.ts:163`),不会再去问 controller 要新 dir(否则就是浪费一把钥匙)

---

## 4. 创建 share(对账 `dispatch({op: 'createDir'})`)

### 4.1 业务入口

MCP 工具:`share_folder({name: "testcase-2026-04"})`
HTTP 等价:`POST /:id/afs/shares {name}`

### 4.2 完整调用链(以"workspace A 创建 share 'req'"为例)

```
MCP share_folder("req")
   │
   │ 1. getAfsShareByName(wsA, "req")   ── CP PG 查 afs_shares 表,完全不碰 controller
   │                                       命中? 直接返回 201(idempotent,不会再去拿新 dir)
   │ 2. afsEnvForWorkspace(wsA)         ── 决定走 built-in 还是 BYOI tunnel
   │ 3. ensureDefaultFs(env)             ── Controller.RegisterFs "shared" (幂等)
   │ 4. createDir(env)                  ── Controller.CreateDir → 拿到 {id, accessKey, permission}
   │                                       注意:这一步不传 "req",controller 不知道这个 share 叫啥
   │ 5. createAfsShare(wsA, "req", id, accessKey)  ── CP PG 留底,这里 "req" 第一次入库
   │ 6. mountAtWorkspace(env, wsA, id, accessKey, "req", false)
   │                                       "req" 在这一步被拼成 mountpoint = /mnt/afs/req
   │ 7. addAfsShareMember(share.id, wsA, "read_write")
   ▼
最终:wsA 的 Agent 看到 /mnt/afs/req
```

### 4.3 对账 `afs-control.ts:71-83` 的 `case 'ensureDefaultFs'` / `'createDir'`

| `dispatch` 的 op | 实际 gRPC | 业务动作 |
|---|---|---|
| `ensureDefaultFs` | `Controller.RegisterFs{name: "shared", fs_type: "local", config: {base_path: "/data/afs"}}` | 在 controller 里注册一个叫 "shared" 的本地文件系统,fs_type=local 表示后端是普通目录(不是 NFS / S3 之类)。**幂等**:`ALREADY_EXISTS` 直接吞,多次重启不会爆 |
| `createDir` | `Controller.CreateDir{fs_name: "shared"}` → `{id, access_key, permission}` | 在 controller 眼里"建一个 dir"= 发一把钥匙。**没创建真实子目录**,只是拿到一个带权限的 capability 句柄。Controller 返回的 `id` 是后续一切操作的句柄 |

> 抓手:`CreateDir` 的请求体只有 `fs_name`,没有 `path` / `name` / `metadata` —— 这是和"mkdir"最大的区别,AFS 的 dir 是**逻辑凭证**不是**物理路径**。

### 4.4 为什么需要 `mountAtWorkspace` 在创建时调一次

`createDir` 之后只是"在 controller 拿到一把钥匙",**AFS 的 dir 不会自动出现在 wsA 的文件系统里**。必须显式调一次 `mountAtWorkspace(wsA, ...)`:

```
control-plane/src/services/afs.ts:135  build mountpoint = `/mnt/afs/${name}`
                                  gRPC → wsA 的 afs-fuse sidecar (tos-<wsA>.svc:9101)
                                  MountRequest{ id, access_key, mountpoint, controller_addr, readonly }
```

fuse sidecar 拿到请求后**不会**直接挂,而是先调 `Controller.ValidateToken` 验 access_key,通过后再 FUSE-mount,最后 `RegisterSession` 告诉 controller "我正在挂着这个 dir"。这一段在 `afs-control.ts` 里被封装成 `mount` op(见第 5 节)。

---

## 5. 授权给另一个 workspace(对账 `dispatch({op: 'mount'})`)

### 5.1 业务入口

MCP:`grant_access({name: "req", slug: "<target-ws>", readonly: true})`
HTTP:`POST /:id/afs/shares/:shareId/members {workspace_id, readonly}`

### 5.2 关键校验链(`afs-shares.ts:339-353`)

授权前 CP 要先做几道闸:

1. **目标 workspace 存在**(`getWorkspace(targetId)`)
2. **同用户**:`target.user_id === caller.user_id`(不允许跨用户共享)
3. **同环境**:`targetEnv.environmentId === afsEnv.environmentId` —— 因为 share 的 dir_id 和 storage 都活在 owner 的环境里,跨环境根本挂不上去,这里直接 400 拒绝
4. **已存在成员**:先 `unmountAtWorkspace` 再 `mountAtWorkspace`(fuse 拒绝重复 mountpoint,这是绕过限制的标准做法)

### 5.3 对账 `afs-control.ts:93-103` 的 `case 'mount'`

```ts
case 'mount': {
  await unary(fuse(req.workspaceId), 'Mount', {
    id: req.dirId,
    access_key: req.accessKey,
    mountpoint: `/mnt/afs/${req.name}`,
    controller_addr: AFS_CONTROLLER_ADDR,
    readonly: !!req.readonly,
  })
  return { ok: true }
}
```

| 字段 | 含义 |
|---|---|
| `id` (即 `dirId`) | controller 颁发的 capability ID |
| `access_key` | 同一个 dir 共享同一把钥匙(被授权方拿到 access_key 就能挂) |
| `mountpoint` | `tos-<targetWs>.svc:9101` 上的 fuse sidecar 收到后,挂到 pod 内 `/mnt/afs/<name>` |
| `controller_addr` | 告诉 sidecar 找哪个 controller 验 token,**不是**让它去那里存东西 |
| `readonly` | fuse sidecar 接到 true 会用 read-only 选项挂文件系统,内核层拒绝写 |

**注意 `controller_addr` 的细节**:fuse 验完 token 就丢弃这个地址,真正的物理 IO 走 sidecar 自己 `mount: /data/afs` 那条 volume,数据不出 pod 节点。

### 5.4 Mount 在 fuse sidecar 内部做了什么

1. 调 `Controller.ValidateToken(id, access_key)` → controller 查 dirs 表 + 校验 hash,返回 `{valid, permission, fs_type, fs_config}`
2. 拿到 `fs_config.base_path`(`/data/afs`)
3. 比对请求里的 `readonly` 和 controller 颁发的 `permission`(`READ_ONLY` 试图挂成 read_write → 拒绝,反之允许)
4. 调 `fuse` 把 base_path 挂到 `mountpoint`
5. 调 `Controller.RegisterSession{dir_id, mountpoint, stream_id}` 把这次挂载登记成 session,后续心跳用

第 5 步是关键 —— **session 是 controller 反向推 `ForceUnmount` 的抓手**。

---

## 6. 撤销(对账 `dispatch({op: 'revokeDir'})` + `'unmount'`)

### 6.1 业务入口

MCP:`unshare_from_all({name: "req"})`
HTTP:`DELETE /:id/afs/shares/:shareId`

### 6.2 三步串联(`afs-shares.ts:217-241`)

```
1. revokeDir(env, share.afs_dir_id, share.access_key)
   └─ Controller.RevokeDir → 标记 dirs.status=revoked,扫 sessions 表,sessions_revoked=N
   └─ 通过 SessionStream(stream_id) 给每个 session 推送 ForceUnmount{mountpoint}
   └─ 那些 session 所在的 fuse sidecar 收到推送,内核层 umount,即使网络断 CP 也能反推
2. 对 listAfsShareMembers 的每个成员,best-effort unmountAtWorkspace
   └─ 兜底:即使 controller 推 ForceUnmount 失败(网络/sidecar 离线),CP 也主动 unmount 一次
3. deleteAfsShare(share.id)  清 CP DB
```

### 6.3 对账 `afs-control.ts:80-92, 104-107`

| `dispatch` 的 op | 实际 gRPC | 业务动作 |
|---|---|---|
| `revokeDir` | `Controller.RevokeDir{id, access_key}` → `{sessions_revoked, errors}` | **集群级撤销**:controller 作废 access_key,并通过 SessionStream 反推所有挂载点。errors 是部分 session 已经离线时的容忍计数 |
| `unmount` | `FuseService.Unmount{mountpoint}` | **本 pod 撤销**:只清掉当前 sidecar 上那个 mountpoint 路径,不碰 controller(适合"我主动下挂"的场景,比如 leave share) |

> 颗粒度提示:`revokeDir` 是"全局作废",`unmount` 是"本地下挂"。前者走 controller 反推,后者是 sidecar 端本地动作。`grant_access` 接受一个之前是 member 的 target 时,CP 会先 `unmount` 再 `mount` 来换权限,这种就是 `unmount` op 的典型用法。

---

## 7. Pod 重启自愈(`AFS_BOOTSTRAP_URL`)

`internal/k8s-provider/workspace-spec.ts:142` 给 afs-fuse sidecar 注了一个环境变量:

```ts
AFS_BOOTSTRAP_URL = `${cfg.cpServiceUrl}/_cp/workspaces/${workspaceId}/afs-mounts`
```

`control-plane/src/routes/internal.ts:95` 这个 endpoint:

```ts
internal.get('/workspaces/:id/afs-mounts', async (c) => {
  const rows = await listAfsMountsForWorkspace(id)   // 查 afs_share_members JOIN afs_shares
  return c.json({ mounts: rows.map((r) => ({
    id: r.afs_dir_id, access_key: r.access_key,
    mountpoint: `/mnt/afs/${r.share_name}`,
    readonly: r.permission === 'read_only',
  }))})
})
```

**业务动作**:pod 重建(任何原因 — 节点驱逐、镜像升级、scale down/up)后,新的 afs-fuse sidecar 起来第一件事是 GET 这个 URL,拿到"我应该挂哪些 share",然后挨个发 `Mount` RPC(就是上面第 5 节的流程)。这样**不需要 CP 主动推**,新 pod 就能恢复挂载状态,不会因为 pod 重建丢文件。

> 这就是为什么 CP 必须在 `afs_shares.access_key` 里留底 —— 授权时的密钥,授权完之后 CP 必须自己存一份,不然新 pod 拉列表时拿不到 access_key 就挂不回去。

---

## 8. 完整时序图(创建 → 授权 → 读文件 → 撤销)

```
wsA  Agent                          CP                            controller       fuse-A        fuse-B       afs storage
  │                                  │                                │              │             │              │
  │  share_folder("req")             │                                │              │             │              │
  ├─────────────────────────────────►│  ensureDefaultFs              │              │             │              │
  │                                  │ ─RegisterFs "shared"──────────►│              │             │              │
  │                                  │ ◄─ ok ─────────────────────────│              │             │              │
  │                                  │  createDir                    │              │             │              │
  │                                  │ ─CreateDir────────────────────►│              │             │              │
  │                                  │ ◄─{id, accessKey, READ_WRITE}─│              │             │              │
  │                                  │  createAfsShare + addMember   │              │             │              │
  │                                  │  (PG insert)                  │              │             │              │
  │                                  │  mountAtWorkspace(wsA)        │              │             │              │
  │                                  │ ─Mount{id, key, /mnt/afs/req}─────────────────►│             │              │
  │                                  │                               │ ValidateToken│             │              │
  │                                  │ ◄─────────────────────────────{valid, base=/data/afs}       │              │
  │                                  │                               │              │ FUSE mount /data/afs→/mnt/afs/req     │
  │                                  │                               │              │ RegisterSession             │
  │  ◄ /mnt/afs/req 就绪             │                               │              │             │              │
  │                                  │                                │              │             │              │
  │  grant_access(slug=wsB)          │                                │              │             │              │
  ├─────────────────────────────────►│  mountAtWorkspace(wsB)        │              │             │              │
  │                                  │ ─Mount{id, key, /mnt/afs/req}───────────────────────────────►│              │
  │                                  │                               │              │             │ ValidateToken│
  │                                  │                               │              │             │ FUSE mount   │
  │                                  │                               │              │             │ RegisterSession│
  │                                  │  addMember(wsB, read_only)    │              │             │              │
  │  wsB Agent 读 /mnt/afs/req/x.md  │                                │              │             │              │
  │                                  │                                │              │             │ 直接读 /data/afs/x.md
  │                                  │                                │              │             │              │
  │  unshare_from_all("req")         │                                │              │             │              │
  ├─────────────────────────────────►│  revokeDir                    │              │             │              │
  │                                  │ ─RevokeDir────────────────────►│              │             │              │
  │                                  │                               │ 遍历 sessions  │             │              │
  │                                  │                               │ push ForceUnmount mountpoint  │              │
  │                                  │ ◄─────────────────────────────{sessions_revoked: 2}        │              │
  │                                  │  best-effort unmount (per member)          │             │              │
  │                                  │  deleteAfsShare (PG)          │              │             │              │
  │  /mnt/afs/req 消失(双方都)       │                                │              │  umount    │  umount      │
```

---

## 9. 内置环境 vs BYOI 远程环境:同一个 API 两条路径

老 README 没碰这一段,但 `afs-control.ts` 的存在就是为了解决它。`control-plane/src/services/afs.ts:8-12` 注释说得很直白:

> A remote (BYOI) environment's afs data plane lives in the customer cluster, so the op is shipped over the tunnel and executed by that environment's runner (option A) — see env-runner-k8s/src/afs-control.ts.

具体而言:

| 调用 | 内置环境(`isBuiltin: true`) | BYOI 环境(`isBuiltin: false`) |
|---|---|---|
| `createDir` | 直接 gRPC 到 `afs-controller.default.svc:9100` | `openControlStream(env, 'afsctl')` → CP 通过 tunnel 把 `{op: 'createDir', fsName}` 推到远端 runner → runner 调本地 gRPC |
| `mountAtWorkspace` | 直连 `tos-<wsId>.svc:9101`(在 cp 集群里) | 同样过 tunnel,runner 在自己集群里 dial 远端的 `tos-<wsId>` Service |
| `revokeDir` | 直连 controller | 过 tunnel 让 runner 调远端 controller |

`env-runner-k8s/src/afs-control.ts` 的 `dispatch` 就是一个**完全镜像** CP 侧 API 的本机实现 — 它的 5 个 `case`(ensureDefaultFs / createDir / revokeDir / mount / unmount)和 `control-plane/src/services/afs.ts` 导出的 5 个函数一一对应,只是少了"找 controller / 找 fuse"的地址解析(直接用本机环境变量 `AFS_CONTROLLER_ADDR` / `tos-<wsId>.<NAMESPACE>.svc:9101`)。

> 闭环:CP 只看业务(创建 share / 授权 / 撤销),不关心环境拓扑;runner 负责把业务请求翻成具体的 gRPC 调用,环境差异(同集群 vs 跨集群)被完全吸收在 `callRemote()` 这一层。

---

## 10. 老 README 的具体错误清单(便于复核)

1. ❌"AFS 解决 workspace 持久化问题" → ✅ AFS 是跨 workspace 文件共享,workspace 自己的持久化不归 AFS 管
2. ❌"agent 看到 /prompts /files" → ✅ AFS 挂载点是 `/mnt/afs/<name>`,`/prompts` `/files` 是 workspace 自己的事
3. ❌"controller.db 存 dir_id → base_path 映射" → ✅ base_path 存在 `filesystems.config`,dirs 表里只有 `(id, fs_name, permission, status)`
4. ❌"sidecar 问 controller dir_id 对应哪个 base_path" → ✅ sidecar 调 `ValidateToken(id, access_key)` 验权限,顺便拿到 fs_type/fs_config(包含 base_path)
5. ❌"NFS 共享物理存储" → ✅ 默认 `fs_type: 'local'`,`base_path: /data/afs`,后端是 PVC(可以是 NFS,也可以是别的),由 `self-host/manifests/afs.yaml` 决定,不是 AFS 协议本身
6. ❌"controller.db 丢了等于全丢" → ✅ 部分对,但更准确的说法是:access_key 校验依赖 controller 的 dirs 表,丢了就**不能再颁发新挂载**,老挂载只要 session 还在就还能用直到撤销/重启
7. ❌"Recreate 策略 = SQLite 不允许多写" → ✅ 对,但忽略了 controller 是单实例,根本不该有 RollingUpdate 的可能,这条主要是给运维看的"不要手贱改"
8. ❌完全没提 `AFS_BOOTSTRAP_URL` 自愈 → ✅ 新 pod 不靠 CP 推,自己拉 `/_cp/workspaces/:id/afs-mounts`
9. ❌完全没提 BYOI 远程环境路径 → ✅ `env-runner-k8s/src/afs-control.ts` 才是跨集群部署的真正实现
10. ❌"workspace 自己独占 /shared/ws-xxx/prompts" 这套目录布局 → ✅ 实际只有一个 `/data/afs` 根,所有 share 共用,isolation 靠 FUSE + token,不是靠物理子目录

---

## 11. 一句话总结

> AFS 的底层逻辑是"controller 发 capability token(dir_id + access_key)+ fuse sidecar 验 token 后 FUSE-mount 共享存储 + SessionStream 反推撤销" —— 它解决的是**"Agent A 怎么把一堆文件交给 Agent B"**,不是"workspace 怎么省 PVC"。`createDir` 拿到的是钥匙不是目录,`mount` 是验 token 不是查 base_path,`revokeDir` 是集群级作废不只清本地挂载,跨 workspace 的 sharing 是 AFS 的整个存在意义。`env-runner-k8s/src/afs-control.ts` 把这套语义通过 tunnel 复用到 BYOI 远端环境,API 表面和内置环境完全一致。
