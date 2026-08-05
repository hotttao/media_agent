export const teamworkChapter = {
  id: "teamwork-flow",
  title: "3. Teamwork：从 UI 组队到多 Agent 协作",
  summary:
    "先完整体验用户在 Teamwork UI 中看到的流程：创建任务、选择 coordinator、添加成员、发送目标、观察各 Agent 会话并查看共享文件。理解界面以后，再向下对应 task_context、call_agent、AFS 和 teamwork_sessions 的源码实现。",

  uiJourney: {
    eyebrow: "UI WALKTHROUGH · RELEASE REVIEW",
    title: "用户如何在 Teamwork 里完成一次发布评审",
    intro:
      "示例中，lead 负责协调，writer 生成发布说明，reviewer 做风险审阅。用户始终只在 Teamwork 页面操作；成员之间通过 Agent 调用和团队共享目录协作。",
    preview: {
      task: "release-review",
      brief: "生成发布说明，并按 P0 / P1 / P2 完成风险审阅",
      coordinator: "lead",
      members: ["writer", "reviewer"],
      folder: "/mnt/afs/team-task_123",
      files: ["release-note.md", "review-result.md"],
      userMessage: "根据本周变更生成发布说明，请 reviewer 审阅后给我最终结论。",
      coordinatorReply: "发布说明与审阅已完成。发现 1 项 P0 风险，建议暂缓发布。",
    },
    steps: [
      {
        screen: "Teamwork 任务列表",
        title: "打开 Teamwork，点击“新建任务”",
        action:
          "用户进入 Teamwork 应用。没有任务时页面显示空状态和“新建任务”；已有任务时显示任务卡片、任务简介和 coordinator 名称。点击“新建任务”打开创建弹窗。",
        visible:
          "此时还没有创建 Agent 会话，也没有给任何成员发消息。用户只是在建立一个团队工作的容器。",
        backend:
          "列表由 GET /api/teamwork 返回当前用户的 teamwork_tasks；创建弹窗同时读取本人可作为 coordinator 的 workspaces。",
      },
      {
        screen: "新建任务弹窗",
        title: "填写任务名称、简介，并选择 coordinator",
        action:
          "名称填写 release-review；简介填写发布目标和验收标准；Coordinator 选择正在运行的 lead。未运行的 workspace 会在选择器中标记为不可用。点击“创建”。",
        visible:
          "创建成功后出现成功提示，弹窗关闭，页面直接进入 release-review 的任务详情，而不是停留在列表页。",
        backend:
          "Web 调 POST /api/teamwork。CP 先写 teamwork_tasks，再创建 team-<taskId> AFS share、把它 RW 挂载给 lead，并回填 afs_share_id。AFS provision 失败会删除刚创建的 task。",
      },
      {
        screen: "任务详情",
        title: "先认识详情页：左边管团队，右边做协作",
        action:
          "左栏依次显示任务简介、成员列表和共享文件；右侧是 coordinator 会话，上方是 Teamwork Timeline。用户可以随时编辑简介，也可以从左上角返回任务列表。",
        visible:
          "lead 带皇冠和 Coordinator 标记，但不会重复出现在普通成员列表里。共享文件区域已经锁定到该 task 的 AFS 目录。",
        backend:
          "详情页分别读取 task、participants、roster candidates 和 workspaces。共享文件面板以 coordinator workspace 身份打开，并把根目录固定为 /team-<taskId>/。",
      },
      {
        screen: "成员列表",
        title: "添加 writer 和 reviewer",
        action:
          "在“添加成员”选择器中依次选择 writer、reviewer 并点击添加。如果自己的 workspace 没有 slug，UI 会就地出现 slug 输入框，保存 slug 后再加入；未运行成员和无法修复 slug 的跨用户成员会被禁用。",
        visible:
          "添加成功后成员立即出现在左栏。若目标 Pod 的共享目录挂载失败，页面显示错误提示，该 workspace 不会被假装成已加入状态。",
        backend:
          "POST participants 先写 teamwork_participants，再把同一团队 share RW 挂载到目标 Pod。挂载失败会回滚 participant。其他用户的候选 workspace 必须 public；自己的 workspace 可为 private/user/public。",
      },
      {
        screen: "Coordinator 会话",
        title: "在右侧输入完整目标，只发给 lead",
        action:
          "用户输入：“根据本周变更生成发布说明，请 reviewer 审阅后给我最终结论。”然后发送。用户不需要分别打开 writer 和 reviewer，也不需要手动填写共享目录。",
        visible:
          "Timeline 首先出现 lead 的 coordinator lane，聊天区流式展示 lead 的思考、工具调用和最后答复。用户原始消息仍显示为自己输入的自然语言。",
        backend:
          "Web 只在发送给 lead 的第一条消息后追加 task_context，其中包含 brief、/mnt/afs/team-<taskId> 和成员 slug。POST /api/teamwork/:id/chat 创建或复用 coordinator session，并登记 teamwork_sessions(role=coordinator)。",
      },
      {
        screen: "Teamwork Timeline",
        title: "观察 lead 派工，而不是由 UI 直接群发",
        action:
          "lead 根据任务先调用 writer，再调用 reviewer。call_agent 返回成员 session_id 后，Timeline 才出现 writer、reviewer lane。点击任一 lane，可以切换查看该成员的只读会话；点击 lead 回到可继续输入的 coordinator 会话。",
        visible:
          "成员标签展示的是实际发生过的 Agent 会话，不是简单把 roster 全部列出来。尚未被调用的成员不会凭空出现会话。成员面板只读，用户的后续要求仍发给 lead。",
        backend:
          "Coordinator 通过 MCP call_agent 派工。成员 session 写入 teamwork_sessions(role=member)，同时 UI 从 coordinator 消息中的 call_agent tool result 提取 session_id。Teamwork 不会自动遍历 roster、并行或重试。",
      },
      {
        screen: "共享文件",
        title: "在左侧直接检查团队产物",
        action:
          "writer 写入 release-note.md，reviewer 可以读取它并写入 review-result.md。用户无需切换到另一个 Files 应用，在详情页左侧共享文件区即可打开、刷新或下载这些产物。",
        visible:
          "lead、writer、reviewer 的 Pod 中路径都叫 /mnt/afs/team-task_123，但文件只存一份；刷新后看到的是同一后端目录的最新内容。",
        backend:
          "每个 Pod 有自己的 /mnt/afs FUSE mount；各自 afs-fuse sidecar 把相同 dir_id 映射到同一个 RWX afs-storage PVC。文件读写不经过 CP 或 AFS Controller。",
      },
      {
        screen: "继续沟通与结束任务",
        title: "追问仍发给 lead；完成后可移除成员或删除任务",
        action:
          "用户可以继续在 lead 会话中要求“让 reviewer 复查 P0 修复”。lead 可使用先前 session_id 续接 reviewer。任务结束后，用户可移除单个成员；点击页头删除按钮并确认，则删除整个 task。",
        visible:
          "移除成员后它从 roster 消失；删除任务成功后返回任务列表。成员执行失败时，lead 会在当前会话中拿到错误并决定重试、改派或向用户说明。",
        backend:
          "移除成员会删除 participant 并 best-effort unmount。删除 task 会 revoke 团队 AFS dir、强制卸载在线 Pod、清理 share 和 task。Teamwork 没有内置 DAG、自动补偿或统一重试策略。",
      },
    ],
    conclusion:
      "从 UI 看，用户只维护任务、成员、共享文件并与 coordinator 对话；从实现看，coordinator 用 task_context 获得团队边界，用 call_agent 创建成员会话，用 AFS 交换文件，最后把结果收口到同一个用户会话。",
  },

  sourceRefs: [
    "web/src/components/teamwork/TeamworkSection.tsx：任务列表、创建弹窗、详情页、成员选择、共享文件和 coordinator/member 会话。",
    "web/src/components/teamwork/TeamworkTimeline.tsx：coordinator 与实际成员会话的 Timeline 展示。",
    "control-plane/src/routes/teamwork.ts：task、participant、chat、session registration 与删除 API。",
    "control-plane/src/services/teamwork.ts：团队 AFS share provision、成员挂载、卸载与销毁。",
    "control-plane/src/services/db/teamwork.ts：teamwork_tasks / participants / sessions 的读写与 roster fallback。",
    "control-plane/src/services/chat/dispatchChatTurn.ts 与 executeChat.ts：session token 下发和 coordinator session 登记。",
    "control-plane/src/mcp/index.ts 与 control-plane/src/mcp/tools/agent.ts：由 session token 恢复 taskId，并在 roster 范围内 call_agent。",
  ],

  bullets: [
    "Coordinator 是用户唯一需要主动对话的 Agent；member session 在 UI 中只读，后续任务仍交给 coordinator 组织。",
    "task_context 只追加到 coordinator 的第一条消息。成员只会收到 coordinator 在 call_agent.prompt 中明确写出的任务、路径和输出要求。",
    "Timeline 展示实际发生的 call_agent 会话，不等于 roster；加入成员不会自动创建 member session。",
    "团队 share 对 coordinator 和 participant 都是 RW，这与普通 grant_access 默认只读不同。",
    "成员新 session 的 parent_session_id 当前通常仍为空；任务归属通过 teamwork_sessions 和 coordinator 的 tool call/result 表达。",
    "成员失败会作为 call_agent 错误或 running 状态回到 coordinator；Teamwork 层没有自动重试、补偿或 DAG 调度。",
  ],

  codeBlocks: [
    {
      title: "UI 在第一条 coordinator 消息后追加的 task_context",
      code: `<task_context>
  <name>release-review</name>
  <description>
    生成发布说明，并按 P0 / P1 / P2 完成风险审阅
  </description>
  <shared_folder>/mnt/afs/team-task_123</shared_folder>
  <members>
    <member slug="writer" />
    <member slug="reviewer" />
  </members>
</task_context>`,
    },
    {
      title: "Coordinator 可能执行的实际派工",
      code: `call_agent({
  slug: "writer",
  prompt: "生成发布说明，写入 /mnt/afs/team-task_123/release-note.md，并返回摘要。"
})

call_agent({
  slug: "reviewer",
  prompt: "读取 release-note.md，审阅风险，把结果写入 review-result.md，并返回 P0/P1/P2 清单。"
})`,
    },
  ],

  tools: [
    {
      group: "UI 数据",
      names: ["Task list", "Task detail", "Members", "Timeline", "Shared files"],
      purpose: "把任务配置、实际 Agent 会话和共享产物集中在同一个 Teamwork 界面。",
    },
    {
      group: "任务状态",
      names: ["teamwork_tasks", "teamwork_participants", "teamwork_sessions"],
      purpose: "分别保存任务、可调用 roster，以及归属于任务的协调/成员会话。",
    },
    {
      group: "执行能力",
      names: ["task_context", "call_agent", "/mnt/afs/team-<taskId>"],
      purpose: "把团队边界交给 coordinator，由它派工，并让各 Pod 通过同一目录协作。",
    },
  ],

  diagram: `Teamwork 后端调用时序（理解 UI 后再看）
sequenceDiagram
  participant U as User
  participant W as Teamwork UI
  participant CP as Control Plane
  participant AFS as AFS sidecars / storage
  participant L as lead coordinator
  participant WR as writer
  participant RV as reviewer

  U->>W: 新建 task，选择 lead
  W->>CP: POST /api/teamwork
  CP->>AFS: create team share + mount lead RW
  U->>W: 添加 writer、reviewer
  W->>CP: POST participants
  CP->>AFS: mount same share to members RW
  U->>W: 在 coordinator chat 输入目标
  W->>CP: POST task chat(message + task_context)
  CP->>L: /chat
  L->>CP: call_agent(writer)
  CP->>WR: /chat
  WR->>AFS: write release-note.md
  WR-->>L: result + session_id
  L->>CP: call_agent(reviewer)
  CP->>RV: /chat
  RV->>AFS: read release-note.md; write review-result.md
  RV-->>L: result + session_id
  L-->>W: final answer
  W-->>U: 结果、Timeline 与共享文件`,

  examples: [
    {
      title: "UI 失败反馈：成员挂载失败",
      request: "用户选择 reviewer 并点击添加，但 reviewer Pod 当前不可达",
      payload: {
        ui: "Add member",
        server: "INSERT participant → mount team share",
      },
      response: {
        ui: "显示 addFailed 错误提示，reviewer 不出现在成员列表",
        database: "删除刚插入的 teamwork_participants 记录",
      },
      notes: [
        "原 task、coordinator 和其他已加入成员保持可用。",
        "这与成员执行任务失败不同；执行失败会回到 coordinator 的 call_agent tool result。",
      ],
    },
  ],
};
