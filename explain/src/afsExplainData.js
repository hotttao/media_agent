export const afsExplainData = {
  volumePrimer: {
    title: "从第一性原理理解 AFS Volume 挂载",
    intro: "先忘掉 PVC、emptyDir 和 mount propagation。只从一个问题开始：容器里的路径，怎样才能连接到容器外的文件？",
    invariant: "路径只是入口；mount 决定入口通向哪个文件系统；真正的字节保存在后端存储。",
    stages: [
      {
        number: "01",
        label: "隔离",
        title: "两个容器默认看不到彼此的文件",
        question: "两个容器里都有 /mnt/afs，它们就是同一个目录吗？",
        answer: "不是。容器有各自的 mount namespace 和根文件系统。路径字符串相同，只代表名字相同，不代表背后是同一个文件系统。",
      },
      {
        number: "02",
        label: "接线",
        title: "Pod Volume 让两个路径指向同一个对象",
        question: "怎样让 agent 和 sidecar 共享一个目录底座？",
        answer: "Kubernetes 先在 Pod 层定义 afs-mnt=emptyDir，再把它分别 volumeMount 到两个容器的 /mnt/afs。此时两个路径才接到同一个 Pod volume。",
      },
      {
        number: "03",
        label: "覆盖",
        title: "FUSE mount 把 review 变成新的文件系统入口",
        question: "为什么 /mnt/afs 存在，还必须单独 mount review？",
        answer: "emptyDir 只提供空底座。afs-fuse 在 /mnt/afs/review 建立子挂载后，Linux mount table 才会把进入 review 的请求转交给 FUSE；没有该子挂载，就没有共享目录的虚拟 inode 树。",
      },
      {
        number: "04",
        label: "转发",
        title: "Agent 读路径，afs-fuse 读真实存储",
        question: "ls /mnt/afs/review 到底调用了谁？",
        answer: "内核发现 review 是 FUSE mountpoint，把 LOOKUP/READDIR 交给 afs-fuse。afs-fuse 再按 dir_id 查询 /data/afs 下的后端目录，并把结果转换成虚拟 inode 返回。",
      },
      {
        number: "05",
        label: "共享",
        title: "跨 Pod 共享的是后端字节，不是挂载本身",
        question: "Pod A 和 Pod B 为什么能看到同一个 spec.md？",
        answer: "两个 Pod 各有自己的 emptyDir、afs-fuse 进程、FUSE mount 和 inode table；两边的 backend 最终指向同一个 RWX PVC 中的 dir_id 目录，所以读到同一份字节。",
      },
    ],
  },


  title: "共享文件 AFS：从 Kubernetes Volume 到跨 Pod 文件协作",
  intro:
    "先用一个固定例子建立直觉：Agent A 在 /mnt/afs/review/spec.md 写文件，授权 Agent B 后，B 在自己 Pod 的同一路径读到相同内容。关键并不是两个 Pod 共享一个 emptyDir，而是每个 Pod 都有自己的 FUSE 挂载点，而各自的 afs-fuse sidecar 共同挂载同一个 RWX afs-storage PVC。",
  quickLinks: [
    { label: "Workspace 表", moduleId: "schema", domain: "config", table: "workspaces" },
    { label: "AFS Share 表", moduleId: "schema", domain: "afs", table: "afs_shares" },
    { label: "AFS Share Member 表", moduleId: "schema", domain: "afs", table: "afs_share_members" },
    { label: "Controller Mount Session", moduleId: "schema", domain: "afs", table: "afs_controller_sessions" },
  ],
  steps: [
    {
      title: "先看完整例子：A 写 spec.md，B 读取",
      actor: "Agent A → Control Plane → Agent B",
      request: "share_folder(review) → 写文件 → grant_access(review, B) → call_agent(B)",
      params: {
        owner_path: "/mnt/afs/review/spec.md",
        member_path: "/mnt/afs/review/spec.md",
        permission: "B 默认 readonly",
      },
      response: {
        outcome: "B 在自己的 Pod 中读取到 A 写入的同一份字节",
        identity: "两侧 mount 指向相同 dir_id，而不是复制一份文件",
      },
      implementation:
        "A 调 share_folder 后，CP 创建 dir_id/access_key 和 share 记录，并让 A 的 sidecar 挂载。A 正常写 /mnt/afs/review/spec.md。grant_access 校验 B 后，让 B 的 sidecar 把相同 dir_id 挂到相同逻辑路径；返回成功时 mount 已完成。之后 call_agent 的 prompt 只需引用路径，不传 access_key。",
      codeBlocks: [
        {
          title: "应用层调用顺序",
          code: `share_folder({ name: "review" })
# Agent A 正常写文件
/mnt/afs/review/spec.md

grant_access({ name: "review", agent: "reviewer" })
call_agent({
  slug: "reviewer",
  prompt: "读取 /mnt/afs/review/spec.md 并给出审阅意见"
})`,
        },
      ],
      note: "文件 IO 不经过 Control Plane 或 AFS Controller；它们负责控制面，实际读写走 Pod 内 FUSE sidecar 到共享存储。",
    },
    {
      title: "Kubernetes 基础：Volume、PVC 和 volumeMount 到底是什么",
      actor: "Kubernetes",
      request: "Pod spec.volumes[] + container.volumeMounts[]",
      params: {
        PV: "集群中的真实存储资源或其抽象",
        PVC: "Pod 对存储的声明/引用",
        volume: "Pod 级别给某个存储源起的名字",
        volumeMount: "把这个 volume 映射到某个容器内路径",
      },
      response: {
        rule_1: "同一 Pod 的多个容器可以 mount 同一个 Pod volume",
        rule_2: "不同 Pod 的同名 volume 并不会自动成为同一份存储",
        rule_3: "跨 Pod 共享要依赖它们引用同一个、支持 RWX 的 PVC",
      },
      implementation:
        "可以把 PVC 理解成仓库租约，把 Pod volume 理解成这个 Pod 对租约的接线端子，把 volumeMount 理解成容器内的门牌地址。emptyDir 是 Pod 临时房间：Pod 存续期间可由 Pod 内容器共享，Pod 被重建后消失，绝不能拿它解释跨 Pod 持久共享。",
      codeBlocks: [
      ],
      note: "这里最容易混淆：afs-mnt 是 emptyDir，只负责同一个 Pod 内把 FUSE mount 传播给 agent；afs-storage 才是跨 Pod 的共享持久存储。",
    },
    {
      title: "同一个 Pod 内：/mnt/afs 是底座，/mnt/afs/review 才是 FUSE mount",
      actor: "agent container / afs-fuse sidecar / afs-mnt emptyDir",
      request: "在同一个 workspace Pod 中观察 /mnt/afs 与子目录 review",
      params: {
        afs_root: "/mnt/afs → Kubernetes afs-mnt emptyDir",
        share_mount: "/mnt/afs/review → afs-fuse 创建的 FUSE 子挂载",
        process: "一个 Pod 只有一个 afs-fuse sidecar 进程，可管理多个 share mount",
      },
      response: {
        before_mount: "/mnt/afs 存在，但 review 通常不存在，也没有该 share 的 FUSE inode 树",
        after_mount: "agent container 能进入 /mnt/afs/review，并通过 FUSE 查看共享文件",
        multiple_shares: "review、assets、team-task-123 分别拥有独立 ActiveMount",
      },
      implementation: [
        "/mnt/afs 本身不是 afs-fuse 提供的文件系统。Kubernetes 把同一个 afs-mnt emptyDir 分别 volumeMount 到 agent container 和 afs-fuse sidecar 的 /mnt/afs。",
        "sidecar 收到 Mount(review) 后，在自己的 /mnt/afs 下创建 review 目录，再用 spawn_mount2 把一个 AfsFilesystem 挂到 /mnt/afs/review。",
        "sidecar 的 afs-mnt volumeMount 使用 Bidirectional mount propagation，因此它创建的 FUSE 子挂载能传播到 Pod volume；agent 的挂载使用 HostToContainer，因此能看到这个子挂载。",
        "没有成功挂载时，agent 看到的只是 emptyDir 根目录。review 通常不存在；若挂载中途失败，也可能只残留一个普通空目录。此时路径解析不会进入 FUSE，不会调用 afs-fuse 查询后端文件。",
        "同一个 Pod 中通常只有一个 afs-fuse 进程。它可以管理 /mnt/afs/review、/mnt/afs/assets 等多个子挂载，但每个子挂载都有独立的 AfsFilesystem、BackgroundSession、dir_id、权限和 ActiveMount。",
        "不同 Pod 不共享 afs-fuse 进程。每个 Pod 都有自己的 sidecar、emptyDir、FUSE sessions 和虚拟 inode table；它们只是通过相同 dir_id 映射到同一个 RWX 存储目录。",
      ],
      codeBlocks: [
        {
          title: "有无 FUSE mount 时，Agent 看到的区别",
          code: `未挂载 review
/mnt/afs                    # emptyDir 根
└─ review                   # 通常不存在；失败时可能只是普通空目录

挂载 review 成功
/mnt/afs                    # emptyDir 底座
└─ review                   # FUSE mountpoint
   ├─ spec.md               # FUSE 虚拟 inode
   └─ docs/

一个 afs-fuse 进程的内存状态
mounts: HashMap<String, ActiveMount>
├─ "/mnt/afs/review"        → AfsFilesystem(dir_review)
├─ "/mnt/afs/assets"        → AfsFilesystem(dir_assets)
└─ "/mnt/afs/team-task-123" → AfsFilesystem(dir_team)`,
        },
      ],
      note: "一句话：/mnt/afs 是用于承载和传播子挂载的 Kubernetes volume；/mnt/afs/review 才是 afs-fuse 创建、Linux 内核识别并把文件操作转交给用户态进程的 FUSE mountpoint。",
    },

    {
      title: "实际挂载拓扑：Pod 之间共享什么、不共享什么",
      actor: "Agent Pod A / Agent Pod B",
      request: "两个 Pod 都运行 agent container + afs-fuse sidecar",
      params: {
        per_workspace: "/workspace → 各自 workspace PVC，互不共享",
        per_pod: "/mnt/afs → 各自 afs-mnt emptyDir，互不共享",
        cross_pod: "/data/afs → 同一个 afs-shared-storage RWX PVC",
      },
      response: {
        visible_to_agent: "/mnt/afs/<share-name>",
        actual_backend: "/data/afs/<last2>/<previous2>/<dir_id>",
        controller: "不挂共享数据 PVC，只管理 capability、mount session 和 revoke",
      },
      implementation:
        "A 与 B 的 afs-fuse sidecar 分别在自己的 /mnt/afs 下创建 FUSE mount。Bidirectional mount propagation 让 sidecar 创建的子挂载传播到 Pod volume；agent 容器用 HostToContainer 看到它。两套 FUSE mount 最终把相同 dir_id 映射到共享 PVC 的同一后端目录，所以看到相同字节。",
      codeBlocks: [
      ],
      note: "RWX（ReadWriteMany）是部署前提。若 storageClass 只支持 RWO，例如常见的单节点 local-path，用同一 PVC 启动多个 Pod 时可能 Pending 或无法跨节点挂载。",
    },
    {
      title: "share_folder：创建 capability，并挂载给拥有者",
      actor: "Agent A / CP / AFS Controller / A sidecar",
      request: "MCP tools/call share_folder({ name: 'review' })",
      params: {
        share_name: "review",
        mount_path: "/mnt/afs/review",
        owner_permission: "read-write",
      },
      response: {
        share: "afs_shares 保存 dir_id、access_key、owner_workspace_id",
        member: "afs_share_members 写入 owner RW membership",
        ready: "工具返回时 owner mount 已完成",
      },
      implementation:
        "CP ensure filesystem 后向 controller CreateDir，得到不可猜测的 dir_id/access_key；随后创建数据库 share，调用 A sidecar 的 mount API，并写 owner membership。逻辑 share name 用于用户友好路径，真正后端身份是 dir_id。",
      codeBlocks: [
        {
          title: "share_folder 调用时序图",
          code: `sequenceDiagram
  participant A as Agent A
  participant CP as Control Plane
  participant DB as PostgreSQL
  participant C as AFS Controller
  participant F as A afs-fuse
  participant S as RWX Storage

  A->>CP: share_folder(name=review)
  CP->>C: CreateDir(filesystem)
  C-->>CP: dir_id + access_key
  CP->>DB: INSERT afs_shares
  CP->>F: mount(dir_id, key, /mnt/afs/review, RW)
  F->>C: validate capability
  C-->>F: allowed
  F->>S: bind backend /data/afs/<last2>/<previous2>/<dir_id>
  F-->>CP: mount ready
  CP->>DB: INSERT owner membership + mount session
  CP-->>A: /mnt/afs/review ready`,
        },
      ],
    },
    {
      title: "afs-fuse 收到 Mount 后：在本 Pod 内建立真正的 FUSE 文件系统",
      actor: "FuseServiceImpl / Linux kernel / storage backend",
      request: "gRPC FuseService.Mount(MountRequest)",
      params: {
        id: "controller 颁发的 dir_id",
        access_key: "验证 capability 的 secret",
        mountpoint: "/mnt/afs/review",
        controller_addr: "可覆盖 sidecar 默认 controller；空值使用启动配置",
        readonly: "调用方要求的降权开关",
      },
      response: {
        success: "MountResponse { mountpoint: '/mnt/afs/review' }",
        local_state: "mounts[mountpoint] = ActiveMount",
        agent_visible: "借助 mount propagation，agent container 看见新的子挂载",
      },
      implementation: [
        "1. 校验 id、access_key、mountpoint 都非空；任一为空直接返回 INVALID_ARGUMENT。",
        "2. 查询进程内 mounts HashMap。相同 mountpoint 已存在时返回 ALREADY_EXISTS，不重复执行 mount。",
        "3. 用 id + access_key 调 controller.ValidateToken。controller_addr 参数非空时使用它，否则使用 sidecar 启动配置；controller 不可达返回 UNAVAILABLE，凭证无效返回 PERMISSION_DENIED。",
        "4. 计算最终权限：请求 readonly=true，或者 controller 返回的 capability 本身是 READ_ONLY，结果都是只读；调用方只能降权，不能把只读 capability 升成读写。",
        "5. 根据 ValidateToken 返回的 fs_type/fs_config 创建 StorageBackend。local 使用 base_path（K8s 中通常是 /data/afs）；nfs 使用已经由系统挂载好的 mount_path，afs-fuse 自己不会执行 NFS mount 命令。",
        "6. 检查 dir_id 对应的物理目录是否存在，不存在则 lazy init。Local/NFS backend 使用两级散列布局：<base>/<id最后2位>/<id倒数第3-4位>/<完整id>，不是简单的 /data/afs/<dir_id>。",
        "7. tokio::fs::create_dir_all 创建 /mnt/afs/review。这个目录位于本 Pod 的 afs-mnt emptyDir 中，只是 FUSE mountpoint，不是文件内容的持久化位置。",
        "8. 创建 AfsFilesystem，保存 backend、dir_id、最终权限、Tokio handle 和从 root inode=1 开始的内存 inode/path 映射表。",
        "9. 调 fuser::spawn_mount2 把 AfsFilesystem 挂到 mountpoint。mount options 是 FSName=afs + AutoUnmount，ACL=All；这一步通过 /dev/fuse 和 Linux kernel 建立真正的 FUSE mount，并返回 BackgroundSession。",
        "10. mount 成功后 best-effort 调 controller.RegisterSession(dir_id, mountpoint, stream_id)。注册失败只记录 warning，不让已经成功的本地 mount 失败；后续 heartbeat 会用 mounts 快照补齐 controller sessions。",
        "11. 把 ActiveMount{id,mountpoint,permission,session_id,BackgroundSession} 写入进程内 HashMap。BackgroundSession 必须一直被持有，drop 时文件系统会卸载。最后才向调用方返回成功。",
      ],
      codeBlocks: [
        {
          title: "Mount 请求在 sidecar 内部的调用时序",
          code: `sequenceDiagram
  participant CP as Caller / Control Plane
  participant F as afs-fuse FuseService
  participant C as AFS Controller
  participant B as StorageBackend
  participant K as Linux kernel /dev/fuse
  participant M as mounts HashMap

  CP->>F: Mount(id, key, mountpoint, readonly)
  F->>F: validate required fields
  F->>M: reject duplicate mountpoint
  F->>C: ValidateToken(id, access_key)
  C-->>F: permission + fs_type + fs_config
  F->>F: effective permission = request RO OR capability RO
  F->>B: create Local/NFS backend
  F->>B: dir_exists(id); init_dir(id) if absent
  Note over B: local physical root uses hashed path\n/data/afs/<last2>/<previous2>/<id>
  F->>F: create_dir_all(/mnt/afs/review)
  F->>K: spawn_mount2(AfsFilesystem, mountpoint)
  K-->>F: BackgroundSession
  F->>C: RegisterSession(dir_id, mountpoint, stream_id)
  Note over F,C: best-effort; heartbeat can reconcile later
  F->>M: insert ActiveMount + keep BackgroundSession
  F-->>CP: MountResponse(mountpoint)`,
        },
        {
          title: "挂载完成后，一次 read/write 如何落到真实目录",
          code: `Agent 访问 /mnt/afs/review/docs/spec.md
  → Linux VFS 把 syscall 交给 FUSE
  → AfsFilesystem 用 inode table 得到相对路径 docs/spec.md
  → backend.read_file(dir_id, "docs/spec.md")
  → LocalStorage.full_path:
     /data/afs/<last2>/<previous2>/<dir_id>/docs/spec.md

只读 mount 的 write/create/mkdir/remove/rename
  → AfsFilesystem 在进入 backend 前返回 EACCES`,
        },
      ],
      note: "最关键的区分：/mnt/afs/review 是本 Pod 内的 FUSE 入口；/data/afs/<散列>/<dir_id> 是 sidecar 访问的共享存储后端。agent container 不直接 mount /data/afs。",
    },

    {
      title: "grant_access：让另一个 Pod 挂载同一个 dir_id",
      actor: "Agent A / CP / B sidecar",
      request: "MCP tools/call grant_access({ name: 'review', agent: 'reviewer' })",
      params: {
        target: "reviewer workspace slug",
        readonly: "默认 true",
        environment: "A 与 B 必须在同一 environment",
      },
      response: {
        mount: "B: /mnt/afs/review → same dir_id",
        durability: "afs_share_members 持久化授权关系",
        timing: "返回成功前等待 B mount 完成",
      },
      implementation:
        "CP 校验 share owner、目标 slug、运行状态和 environment，再把同一个 dir_id/access_key 交给 B sidecar。B 用自己的 afs-mnt emptyDir 建立本 Pod 的 FUSE mount，但后端仍是共享 PVC 上 /data/afs/<last2>/<previous2>/<dir_id>。",
      codeBlocks: [
        {
          title: "跨 Pod 授权与读取调用时序图",
          code: `sequenceDiagram
  participant A as Agent A
  participant CP as Control Plane
  participant DB as PostgreSQL
  participant FB as B afs-fuse
  participant C as AFS Controller
  participant S as Shared RWX PVC
  participant B as Agent B

  A->>CP: grant_access(review, reviewer, readonly=true)
  CP->>DB: resolve share + target workspace
  CP->>FB: mount same dir_id at /mnt/afs/review
  FB->>C: validate access_key + token
  C-->>FB: allowed; register mount session
  FB->>S: open /data/afs/<last2>/<previous2>/<same-dir-id>
  FB-->>CP: mount complete
  CP->>DB: UPSERT member permission
  CP-->>A: access granted
  A->>CP: call_agent(reviewer, read shared path)
  CP->>B: /chat
  B->>FB: read /mnt/afs/review/spec.md
  FB->>S: read same backend bytes
  S-->>B: file content
  B-->>A: review result`,
        },
      ],
      note: "普通 AFS grant 默认只读；Teamwork 在添加 participant 时显式为成员挂 RW，因为成员需要协作写团队目录。",
    },
    {
      title: "文件 IO 数据路径：为什么控制面不成为带宽瓶颈",
      actor: "agent process / kernel FUSE / afs-fuse / shared PVC",
      request: "open('/mnt/afs/review/spec.md')",
      params: {
        control_path: "Agent → MCP/CP → Controller/sidecar mount API",
        data_path: "Agent syscall → FUSE → sidecar → /data/afs/<last2>/<previous2>/<dir_id>",
      },
      response: {
        metadata: "CP/Controller 管授权、membership、mount session",
        bytes: "文件内容不经 CP 数据库，也不经 controller HTTP 转发",
      },
      implementation:
        "mount 完成后，Agent 把共享目录当普通 POSIX 文件系统使用。数据库只保存谁可访问什么；controller 负责验证 capability 与撤销；每个 Pod 的 sidecar 处理本地 FUSE 请求，并直接访问共享 PVC。",
      codeBlocks: [
        {
          title: "控制路径与数据路径",
          code: `控制路径（偶发）
Agent → MCP/CP → AFS Controller → afs-fuse.mount

数据路径（每次读写）
Agent read/write
  → /mnt/afs/<name> FUSE mount
  → 本 Pod afs-fuse sidecar
  → /data/afs/<last2>/<previous2>/<dir_id>
  → RWX PVC`,
        },
      ],
    },
    {
      title: "重启、撤权与失败边界",
      actor: "Kubernetes / CP / AFS Controller",
      request: "Pod restart | unshare_from_all | mount failure",
      params: {
        pod_restart: "afs-mnt emptyDir 与其中的 FUSE mounts 消失",
        persistence: "共享字节仍在 RWX PVC；membership 仍在 PostgreSQL",
        revoke: "controller force-unmount active sessions",
      },
      response: {
        remount: "sidecar bootstrap 或 CP 根据 membership 重建 mount",
        unshare: "撤销 capability、卸载各 Pod、删除 share/member metadata",
        retry: "mount 调用失败时返回错误，不应假装目标已可访问",
      },
      implementation:
        "Pod 重建并不丢共享文件，只会丢本 Pod 临时 mount 拓扑；系统必须重新挂载。unshare_from_all 是破坏性回收：controller revoke dir 并强制卸载，CP 再 best-effort 通知 sidecar、清理数据库。部分 sidecar 暂时不可达时，revoke 仍阻止旧 capability 继续建立新挂载。",
      codeBlocks: [
        {
          title: "生命周期速查",
          code: `Pod restart
  afs-mnt emptyDir: 消失
  /mnt/afs/<name> mount: 消失
  RWX PVC 文件: 保留
  afs_share_members: 保留
  → 重建 Pod 后 remount

unshare_from_all
  revoke dir capability
  → force unmount active sessions
  → best-effort unmount sidecars
  → delete share/member records`,
        },
      ],
      note: "AFS 的安全边界是 environment + workspace 授权 + capability。不要把 access_key 放进 prompt；Agent 只需要共享路径。",
    },
  ],
};
