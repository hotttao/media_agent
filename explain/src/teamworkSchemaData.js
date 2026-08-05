export const teamworkDomain = {
  id: "teamwork",
  label: "Teamwork",
  description: "团队任务、成员 roster、协调/成员会话，以及团队 AFS share 的关联。",
};

export const teamworkTables = {
  teamwork_tasks: {
    domain: "teamwork",
    title: "Teamwork Task",
    purpose:
      "Teamwork 的任务主表。它记录任务 owner、coordinator workspace，以及该任务专用的 AFS share。它不保存调度步骤；成员调用顺序由 coordinator Agent 在会话中决定。",
    readWhen:
      "查看一个团队任务属于谁、由哪个 workspace 协调、共享目录对应哪个 AFS share，或删除 coordinator/owner 后为何级联删除任务时看。",
    importantFor: ["runtime", "afs"],
    fields: [
      ["id", "text", "PK", "任务 ID；同时用于生成团队 share 名 team-<taskId>。", true],
      ["owner_user_id", "text", "FK users.id", "创建并拥有任务的用户；删除用户时任务级联删除。", true],
      ["name", "text", "NOT NULL", "面向用户的任务名称，例如 release-review。", true],
      ["brief", "text", "nullable", "任务描述；Web 会把它放进 coordinator 的 task_context。", true],
      [
        "coordinator_workspace_id",
        "text",
        "FK workspaces.id",
        "协调 Agent 的 workspace；必须属于 owner 且创建任务时处于 running。删除该 workspace 时任务级联删除。",
        true,
      ],
      [
        "afs_share_id",
        "text",
        "FK afs_shares.id, nullable",
        "团队共享目录对应的 CP AFS share。任务先创建，share provision 成功后再回填；share 删除时置 NULL。",
        true,
      ],
      ["created_at", "timestamptz", "default now()", "任务创建时间。", false],
      ["updated_at", "timestamptz", "default now()", "名称、brief 或 afs_share_id 最近更新时间。", false],
    ],
    notes: [
      "创建顺序是 INSERT teamwork_tasks → provision AFS share → UPDATE afs_share_id；provision 失败时 service 会删除刚创建的 task。",
      "coordinator 不写入 teamwork_participants，因为它通过 coordinator_workspace_id 单独表达。",
      "索引 teamwork_tasks_owner 支持用户任务列表；teamwork_tasks_coordinator 支持按协调 workspace 查询。",
    ],
    sample: {
      id: "task_123",
      owner_user_id: "usr_alice",
      name: "release-review",
      brief: "生成发布说明并完成风险审阅",
      coordinator_workspace_id: "ws_lead",
      afs_share_id: "shr_team_task_123",
      created_at: "2026-08-05T10:00:00Z",
      updated_at: "2026-08-05T10:00:01Z",
    },
  },

  teamwork_participants: {
    domain: "teamwork",
    title: "Teamwork Participant",
    purpose:
      "团队 roster 关联表。一行表示某个 workspace 是某个 task 的成员，并应该读写挂载该任务的 AFS share。",
    readWhen:
      "查看 coordinator 可以在 task 范围内调用哪些成员、某个 workspace 为什么出现在 roster，或成员挂载失败为何要回滚时看。",
    importantFor: ["config", "afs"],
    fields: [
      [
        "task_id",
        "text",
        "PK/FK teamwork_tasks.id",
        "所属任务；删除 task 时 participant 级联删除。",
        true,
      ],
      [
        "workspace_id",
        "text",
        "PK/FK workspaces.id",
        "成员 workspace；与 task_id 构成复合主键，同一 workspace 不会重复加入同一任务。",
        true,
      ],
      ["joined_at", "timestamptz", "default now()", "成员加入 roster 的时间，也用于列表排序。", false],
    ],
    notes: [
      "添加流程先 INSERT participant，再把团队 share RW 挂载到目标 sidecar；挂载失败会 DELETE 这行，避免 roster 与实际挂载状态不一致。",
      "coordinator 不能作为 participant 重复加入。本人 workspace 可使用任意 visibility；其他用户只允许 public workspace。",
      "这张表也是 task-aware call_agent 的授权边界：普通 slug 解析失败时，只允许回退到当前 task roster 中属于调用者的 workspace。",
      "workspace_id 上有独立索引 teamwork_participants_workspace，支持反向查某 workspace 加入的任务。",
    ],
    sample: {
      task_id: "task_123",
      workspace_id: "ws_reviewer",
      joined_at: "2026-08-05T10:02:00Z",
    },
  },

  teamwork_sessions: {
    domain: "teamwork",
    title: "Teamwork Session",
    purpose:
      "把普通 sessions 记录归入某个 Teamwork task，并标记它是 coordinator 会话还是成员会话。CP 通过 session_id 反查 taskId，UI 也据此展示团队会话。",
    readWhen:
      "查看 X-Session-Token 如何恢复 teamwork task 上下文、某个成员 session 属于哪个任务，或为什么删除 session/task 会清理关联时看。",
    importantFor: ["runtime"],
    fields: [
      [
        "task_id",
        "text",
        "PK/FK teamwork_tasks.id",
        "所属团队任务；删除 task 时关联会话登记级联删除。",
        true,
      ],
      [
        "session_id",
        "text",
        "PK/FK sessions.id",
        "实际 Agent 会话；与 task_id 构成复合主键。删除 session 时登记级联删除。",
        true,
      ],
      [
        "role",
        "text",
        "coordinator/member",
        "会话在任务中的角色。数据库 CHECK 只允许 coordinator 或 member。",
        true,
      ],
      [
        "parent_session_id",
        "text",
        "FK sessions.id, nullable",
        "预留的父会话引用；父 session 删除时置 NULL。当前 call_agent 新建 member session 时通常仍写 NULL。",
        true,
      ],
      ["created_at", "timestamptz", "default now()", "会话被登记到 task 的时间。", false],
    ],
    notes: [
      "当前 MCP 不依赖旧的 X-Task-Id。CP 先把 X-Session-Token 解析为 session_id，再查询 teamwork_sessions 找到 task_id。",
      "新 coordinator session 在 session.started 后登记；复用 coordinator session 时在 dispatch 阶段幂等登记。",
      "call_agent 创建目标 session 后，把它登记为 role=member。ON CONFLICT(task_id, session_id) DO NOTHING 保证重复登记安全。",
      "session_id 有独立索引 teamwork_sessions_session，支持 findTaskBySession；parent_session_id 只对非空值建部分索引。",
    ],
    sample: {
      task_id: "task_123",
      session_id: "sess_reviewer_7f3c",
      role: "member",
      parent_session_id: null,
      created_at: "2026-08-05T10:05:00Z",
    },
  },
};

export const teamworkEdges = [
  ["users", "teamwork_tasks", "users.id → teamwork_tasks.owner_user_id"],
  ["workspaces", "teamwork_tasks", "workspaces.id → teamwork_tasks.coordinator_workspace_id"],
  ["afs_shares", "teamwork_tasks", "afs_shares.id → teamwork_tasks.afs_share_id"],
  ["teamwork_tasks", "teamwork_participants", "teamwork_tasks.id → teamwork_participants.task_id"],
  ["workspaces", "teamwork_participants", "workspaces.id → teamwork_participants.workspace_id"],
  ["teamwork_tasks", "teamwork_sessions", "teamwork_tasks.id → teamwork_sessions.task_id"],
  ["sessions", "teamwork_sessions", "sessions.id → teamwork_sessions.session_id / parent_session_id"],
];
