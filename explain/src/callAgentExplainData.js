export const callAgentChapter = {
  id: "call-agent",
  title: "2. call_agent：先理解调用，再进入源码",
  summary:
    "本章固定使用“协调 Agent 把共享需求文档交给 reviewer 审阅，并在同一会话中追问”这个例子。先不看代码，用文字走完新建会话、文件挂载、结果返回和 session_id 复用；之后再解释 slug、工具 description/参数、CP handler、目标 /chat 与 SSE 持久化。",

  callAgentJourney: {
    eyebrow: "UI WALKTHROUGH · CALL_AGENT",
    title: "在聊天界面中调用 reviewer，并在同一会话里继续追问",
    intro:
      "用户只需要表达“让 reviewer 审阅”。Coordinator 负责选择 call_agent、准备参数并保存返回的 session_id。下面先看界面上真正发生什么，再展开系统内部处理。",
    preview: {
      workspace: "coordinator",
      target: "reviewer",
      userMessage: "请让 reviewer 读取共享目录里的 spec.md，找出阻塞发布的问题。",
      path: "/mnt/afs/review-input/spec.md",
      sessionId: "sub_session_7f3c",
      firstResult: "发现 1 个阻塞问题：权限校验缺少 workspace 边界。",
      followup: "继续让 reviewer 按 P0 / P1 / P2 给出修复清单。",
    },
    steps: [
      {
        screen: "Workspace Settings",
        title: "先让 reviewer 成为可调用的 Agent",
        action:
          "目标 workspace 在设置页保存 slug=reviewer，并把 visibility 设为 user 或 public；状态必须是 running。显示名称不是调用地址，用户也不需要手写 workspace_id。",
        visible:
          "设置页显示 reviewer slug 和可见性。若 slug 重复、格式错误或 workspace 未运行，应先在这里修复，而不是到聊天时猜参数。",
        backend:
          "Web PATCH /api/workspaces/:id。数据库按 (user_id, slug) 保证唯一；call_agent 后续用 slug + visibility 解析目标 workspace。",
      },
      {
        screen: "Coordinator 聊天",
        title: "用户用自然语言要求调用 reviewer",
        action:
          "用户输入“请让 reviewer 读取共享目录里的 spec.md，找出阻塞发布的问题”。也可以使用 @agent/reviewer 明确点名。第一次任务与过去无关，因此用户不需要提供 session_id。",
        visible:
          "消息像普通聊天一样发送给 coordinator。用户面对的是当前 Agent，不是直接切换到 reviewer 的聊天窗口。",
        backend:
          "Coordinator 模型根据 call_agent description 识别目标 slug 和任务 prompt。@agent/reviewer 只是一种提示写法，传工具时会去掉 @agent/ 前缀。",
      },
      {
        screen: "call_agent 工具卡",
        title: "Coordinator 发起第一次调用",
        action:
          "聊天流中出现 call_agent 工具卡，显示 slug=reviewer、mode=foreground 和本轮任务。共享文件应已通过 AFS 授权，prompt 只引用 /mnt/afs/review-input/spec.md，不暴露 access_key。",
        visible:
          "工具处于运行状态时，用户可以看到 coordinator 正在等待 reviewer。第一次调用没有 session_id，表示创建 reviewer 的新会话。",
        backend:
          "CP 校验目标可见性和 running 状态，为 reviewer mint session token，然后把 prompt POST 到目标 Pod /chat。目标 session.started 后，CP 获得真实 session_id。",
      },
      {
        screen: "工具结果与最终回答",
        title: "reviewer 返回结果，Coordinator 保存会话句柄",
        action:
          "reviewer 完成后，工具卡显示 ended、回复摘要和 session_id；coordinator 再把审阅结果整理成面向用户的答案。如果前台等待超过 30 秒，则先显示 running，可继续轮询。",
        visible:
          "用户重点看自然语言结论；session_id 是后续续聊句柄，通常由 coordinator 从工具结果保存，不要求用户复制粘贴。",
        backend:
          "foreground 在 30 秒内完成返回 {session_id,status:'ended',text}；超时或 background 返回 running + session_id，之后调用 get_agent_result。",
      },
      {
        screen: "同一聊天中的追问",
        title: "用户说“继续”，Coordinator 复用 reviewer 会话",
        action:
          "用户继续输入“让 reviewer 按 P0 / P1 / P2 给出修复清单”。Coordinator 再次调用 reviewer，并把第一次返回的 sub_session_7f3c 放进 session_id。",
        visible:
          "第二张工具卡标记“继续会话”，目标仍是 reviewer。用户不需要重新描述 spec.md 的全部背景，reviewer 能看到第一次调用历史。",
        backend:
          "CP 验证 session 属于 reviewer 且 status=active，再把 session_id 和对应 session token 传给目标 /chat。目标 Pod load/reuse 原会话，而不是创建同名新会话。",
      },
      {
        screen: "异常反馈",
        title: "地址、会话或执行失败都会回到当前聊天",
        action:
          "如果 reviewer slug 不存在、不可见、未运行，或 session_id 属于另一个 Agent，工具卡会直接失败。目标执行超时不会丢失句柄，而是返回 running；用户可以让 coordinator 继续查询。",
        visible:
          "错误显示在 call_agent 工具结果中，coordinator 可以改正 slug、新建会话、轮询或向用户说明。系统不会悄悄换目标，也不会自动把错误 session 当成新会话。",
        backend:
          "目标解析和 session ownership 校验发生在 POST /chat 前；已经启动的后台任务用 get_agent_result 查询。call_agent 通用层不会自动重试模型或业务工具失败。",
      },
    ],
    conclusion:
      "从 UI 看，这是 coordinator 聊天中的一次工具调用与一次续聊；从协议看，slug 选择哪个 workspace Agent，session_id 选择该 Agent 的哪条历史会话，prompt 只是这一轮新增消息。",
  },

  translations: {
    descriptionTitle: "call_agent description：英文原文与中文翻译",
    original:
      "Call another workspace agent by its slug and send it a prompt.\nWhen the user mentions @agent/slug or @agent/user/slug, call this tool with the slug part (without the @agent/ prefix) and the rest as prompt.\n\nSlug formats:\n- \"slug\" — your own agent (visibility = user or public)\n- \"username/slug\" — another user's public agent\n\nConversation modes (pick based on intent):\n- New conversation (default, omit session_id): start a fresh session. Use this when the request is self-contained or unrelated to any previous dispatch — the sub-agent has no memory of prior calls.\n- Continue conversation (pass session_id from a prior call_agent result): resume the existing sub-agent session so it sees its full history. Use this for follow-ups, clarifications, or iterative refinement on the same task. The session_id must belong to the same target agent you originally called and must still be active.\n\nResult is always JSON with session_id and status (ended/running). On ended, it also includes text. On running (background mode or foreground timeout), poll with get_agent_result. Save the returned session_id if you might want to continue the conversation later.\n\nExamples:\n- @agent/reviewer check this code → call_agent(slug=reviewer, prompt=check this code)\n- Follow-up → call_agent(slug=reviewer, prompt=also flag perf issues, session_id=<prior session_id>)\n- ask @agent/translator to translate: hello → call_agent(slug=translator, prompt=translate: hello)\n- @agent/alice/formatter format this → call_agent(slug=alice/formatter, prompt=format this)\n- @agent/pipeline run ETL in background → call_agent(slug=pipeline, prompt=run ETL, mode=background)",
    chinese:
      "通过目标 workspace 的 slug 调用另一个 Agent，并向它发送一段提示词。\n当用户提到 @agent/slug 或 @agent/user/slug 时，去掉 @agent/ 前缀，把剩余地址作为 slug；其余任务内容作为 prompt。\n\nslug 格式：\n- \"slug\"：调用当前用户自己的 Agent，目标 visibility 必须是 user 或 public。\n- \"username/slug\"：调用其他用户公开的 Agent，目标 visibility 必须是 public。\n\n请根据用户意图选择会话模式：\n- 新会话（默认，省略 session_id）：创建全新 session。适合自包含任务或与之前派发无关的任务；目标 Agent 不会拥有之前其他调用的历史。\n- 继续会话（传入先前 call_agent 返回的 session_id）：恢复目标 Agent 的既有 session，使它能看到完整历史。适合追问、澄清和对同一任务迭代修改。session_id 必须属于最初调用的同一个目标 Agent，并且仍是 active。\n\n结果始终是包含 session_id 和 status（ended/running）的 JSON。ended 时还包含目标回复 text；background 或 foreground 超时得到 running 时，用 get_agent_result 轮询。如果之后可能续聊，必须保存返回的 session_id。\n\n示例：\n- @agent/reviewer check this code → slug=reviewer，prompt=check this code\n- 在同一审阅中追问 → slug=reviewer，prompt=also flag perf issues，并传先前 session_id\n- ask @agent/translator to translate: hello → slug=translator，prompt=translate: hello\n- @agent/alice/formatter format this → slug=alice/formatter，prompt=format this\n- @agent/pipeline run ETL in background → slug=pipeline，prompt=run ETL，mode=background",
    params: [      {
        name: "slug",
        schema: "string, required",
        original:
          "Target agent slug (\"my-agent\") or namespaced (\"username/agent\") for cross-user public agents.",
        chinese:
          "目标 Agent 的 workspace slug。调用自己的 Agent 时传 my-agent；跨用户调用公开 Agent 时传 username/agent。不要包含 @agent/ 前缀。",
      },
      {
        name: "prompt",
        schema: "string, required",
        original: "The prompt/message to send to the target agent.",
        chinese:
          "发送给目标 Agent 的本轮消息。CP 不增加“你是子 Agent”等包装，会把这段字符串原样作为 /chat.message 传给目标。应明确写出任务、输入路径、权限和输出格式。",
      },
      {
        name: "mode",
        schema: "'foreground' | 'background', default='foreground'",
        original: "foreground waits for result; background returns immediately with session_id.",
        chinese:
          "foreground 等待目标结果，但超过 30 秒会自动降级为 running；background 在拿到目标 session_id 后尽快返回，不等待最终文本。",
      },
      {
        name: "session_id",
        schema: "string, optional",
        original:
          "Provide a session_id returned by a prior call_agent invocation to continue that conversation. Omit to start a new session. Must belong to the same target agent and still be active.",
        chinese:
          "用于继续目标 Agent 的既有会话。值必须来自先前 call_agent 的返回结果；新会话不传。复用时必须和原目标 slug 对应，并且 session 仍是 active。",
      },
    ],
  },

  sourceRefs: [
    "control-plane/src/mcp/tools/agent.ts:520-728：call_agent/get_agent_result 的完整 description、参数 schema、slug/session 校验、/chat 请求与返回策略。",
    "control-plane/src/services/db/workspaces.ts:221-260：resolveWorkspaceBySlug 的本人/跨用户可见性查询。",
    "internal/types/api.ts:46-51,103-107：Workspace slug 的 REST 校验规则。",
    "control-plane/src/routes/workspaces/write.ts:245-300：Workspace Settings 保存 slug 的 PATCH API 与重复 slug 的 409。",
    "internal/types/builder.ts:224-238 与 builder-mode/actions/config.ts：通过 workspace_config_propose/apply 设置 slug/visibility。",
    "web/src/components/workspace/WorkspaceSettingsPanel.tsx:160-171,802-812：UI 中输入并保存 workspace slug。",
    "control-plane/migrations/001_init.sql:2153-2156：同一 user_id 下非空 workspace slug 的唯一索引。",
    "control-plane/src/lib/session-token.ts:27-126：新会话 token mint、session.started 后 bind、resume 时 token 复用。",
    "internal/acp-adapter/acp-server.ts:347-457,486-530：目标 Pod create/load/reuse session，并把 message 交给 bridge.prompt。",
    "control-plane/src/mcp/tools/afs.ts 与 internal/agent-skills/platform/__platform__/reference/file-sharing.md：共享文件约定。",
  ],

  bullets: [
    "slug 是 workspace 的可调用名字，不是 workspace.name，也不是 workspace.id。格式为小写字母/数字，可带中间连字符；不能以连字符开头或结尾。数据库按 (user_id, slug) 唯一，所以不同用户可以各自拥有 reviewer。",
    "设置 slug 的方式一：Workspace Settings 的 General/基础信息里填写 Slug 并保存，Web 会 PATCH /api/workspaces/:id。方式二：启用 Builder Mode 后，由 Agent 调 workspace_config_propose({slug, visibility})，用户批准后再 workspace_config_apply。",
    "visibility=private 的普通 workspace 不能被 call_agent 解析；visibility=user 可由同一用户的其他 Agent 用 slug 调用；visibility=public 还允许其他用户用 username/slug 调用。teamwork task 对本人 roster 中 private workspace 有一个额外 fallback。",
    "第一次调用省略 session_id。CP 给目标 workspace mint session_token；目标创建会话并通过 SSE session.started 暴露真实 session_id；CP 把它放入 call_agent tool result。",
    "复用会话时，调用方把前一次 tool result.session_id 原样放进下一次 call_agent.arguments.session_id。CP 先查 sessions 表，验证 workspace_id 等于当前 slug 解析出的目标且 status=active，再 POST /chat {message, session_id, session_token}。",
    "目标 Pod 优先复用内存 bridge；没有时 loadSession(session_id) 恢复持久化历史。它不会创建一个冒用该 id 的新会话；load 失败会显式报错。",
    "session_id 与 CP 传给目标的 session_token 不是同一个东西。session_id 是可返回、可续聊的会话句柄；session_token 是 CP 签发并注入目标 MCP headers 的内部身份代理。",
    "foreground 30 秒内完成返回 {session_id,status:'ended',text}；超过 30 秒或 background 返回 running + session_id。get_agent_result 有消息前返回 running envelope，有消息后当前实现直接返回最后一条 assistant message.content。",
    "共享文件必须先 share_folder → 写文件 → grant_access，再 call_agent 引用 /mnt/afs/<name>/file。prompt 不传 access_key 或 dir_id。grant_access 默认只读且要求双方在同一 environment。",
  ],

  tools: [
    {
      group: "地址选择",
      names: ["workspace.name ≠ slug", "slug", "username/slug", "visibility"],
      purpose: "slug 定位目标 workspace；visibility 决定调用方是否有资格解析该地址。",
    },
    {
      group: "会话选择",
      names: ["omit session_id = new", "same slug + session_id = resume", "status=active"],
      purpose: "session_id 只选择目标 workspace 内的具体历史，不改变目标身份。",
    },
    {
      group: "目标提示词栈",
      names: ["target system prompt", "platform reminder", "__platform__ skill", "raw prompt"],
      purpose: "目标使用自己的 workspace 配置；调用方的 prompt 只是本轮新增消息。",
    },
    {
      group: "共享目录",
      names: ["share_folder", "grant_access", "/mnt/afs/<name>", "readonly"],
      purpose: "双方通过同名绝对路径交换文件，凭据由平台控制面保管。",
    },
  ],

  diagram:
    "新建并复用 reviewer 会话\nsequenceDiagram\n  participant Caller as coordinator Agent\n  participant MCP as CP call_agent\n  participant DB as workspaces/sessions/tokens\n  participant Target as reviewer Pod\n  participant Bridge as reviewer ACP bridge\n\n  Caller->>MCP: call_agent(slug=reviewer, prompt=审阅)\n省略 session_id\n  MCP->>DB: resolve slug + mint target session_token\n  MCP->>Target: POST /chat {message, session_token}\n  Target->>Bridge: createSession + prompt(message)\n  Target-->>MCP: SSE session.started(sub_session_7f3c)\n  MCP->>DB: create reviewer session + bind token\n  Target-->>MCP: SSE text + session.ended\n  MCP-->>Caller: {session_id:sub_session_7f3c,status:ended,text}\n\n  Caller->>MCP: call_agent(slug=reviewer,\nsession_id=sub_session_7f3c, prompt=继续)\n  MCP->>DB: validate session belongs to reviewer + active\nensureTokenForSession\n  MCP->>Target: POST /chat {message, session_id, session_token}\n  Target->>Bridge: reuse/loadSession(sub_session_7f3c)\nprompt(继续)\n  Target-->>MCP: SSE session.started(same id) + text + ended\n  MCP-->>Caller: {session_id:same id,status:ended,text}",

  examples: [
    {
      title: "第一次调用：新建 reviewer 会话",
      request: "MCP tool call：省略 session_id",
      payload: {
        slug: "reviewer",
        prompt:
          "请读取 /mnt/afs/review-input/spec.md，按严重级别检查接口兼容性、错误处理和迁移风险。目录只读，请直接回复结论。",
        mode: "foreground",
      },
      response: {
        session_id: "sub_session_7f3c",
        status: "ended",
        text: "发现 1 个高风险和 2 个中风险问题……",
      },
      notes: [
        "调用方需要保存 response.session_id，后续续聊时再传回。",
        "这里的 session_id 属于 reviewer workspace，不属于 coordinator 当前会话。",
      ],
    },
    {
      title: "第二次调用：继续同一个 reviewer 会话",
      request: "MCP tool call：同一 slug + 第一次返回的 session_id",
      payload: {
        slug: "reviewer",
        session_id: "sub_session_7f3c",
        prompt: "继续上一轮审阅：请按 P0/P1/P2 给出修复清单。",
        mode: "foreground",
      },
      response: {
        session_id: "sub_session_7f3c",
        status: "ended",
        text: "P0：修复破坏兼容性的字段变更……",
      },
      notes: [
        "如果把 slug 改成 translator，CP 会报 session_id 不属于该目标。",
        "如果不传 session_id，哪怕 slug 相同也会新建独立会话，reviewer 看不到上一轮历史。",
      ],
    },
  ],

  codeBlocks: [
    {
      title: "slug 如何解析到 workspace",
      code:
        "// 本人 Agent：call_agent(slug='reviewer')\nSELECT * FROM workspaces\nWHERE slug = 'reviewer'\n  AND user_id = caller.user_id\n  AND visibility IN ('user', 'public');\n\n// 跨用户公开 Agent：call_agent(slug='alice/reviewer')\nSELECT w.* FROM workspaces w JOIN users u ON w.user_id = u.id\nWHERE w.slug = 'reviewer'\n  AND u.username = 'alice'\n  AND w.visibility = 'public';\n\n// 地址解析后继续校验\ntarget.id !== callerWorkspace.id\ntarget.status === 'running'",
    },
    {
      title: "如何给 workspace 设置 slug",
      code:
        "// UI 方式\nWorkspace Settings → General → Slug\n输入 reviewer，保存后：\nPATCH /api/workspaces/<workspace_id>\n{ \"slug\": \"reviewer\", \"visibility\": \"user\" }\n\n// Builder Mode 方式\nworkspace_config_propose({ slug: 'reviewer', visibility: 'user' })\n// 用户 Approve\nworkspace_config_apply({ request_id: '<approved request>' })\n\n// 规则\n/^[a-z0-9]([a-z0-9-]*[a-z0-9])?$/\nUNIQUE (user_id, slug) WHERE slug IS NOT NULL",
    },
    {
      title: "session_id 从第一次结果传到第二次 /chat",
      code:
        "// 第一次：调用方不传 session_id\nfirst = call_agent({ slug: 'reviewer', prompt: '审阅文档' })\n// CP <- target SSE: session.started { session_id: 'sub_session_7f3c' }\n// CP -> caller tool result:\n// { session_id:'sub_session_7f3c', status:'ended', text:'...' }\n\n// 第二次：调用方模型从上一个 tool result 取值\nsecond = call_agent({\n  slug: 'reviewer',\n  session_id: first.session_id,\n  prompt: '继续给出修复清单'\n})\n\n// CP 验证后发给目标 Pod\nPOST reviewerAddress + '/chat'\n{\n  message: '继续给出修复清单',\n  session_id: 'sub_session_7f3c',\n  session_token: ensureTokenForSession(reviewer.id, 'sub_session_7f3c')\n}\n\n// reviewer Pod\nbridge exists ? reuse : loadSession('sub_session_7f3c')\nbridge.prompt('sub_session_7f3c', message)",
    },
    {
      title: "prompt 是原样转发；目标仍使用自己的系统提示",
      code:
        "// call_agent handler\nbody: JSON.stringify({\n  message: prompt,\n  ...(session_id ? { session_id } : {}),\n  session_token: subSessionToken\n})\n\n// target /chat\nconst { message, session_id, session_token } = body\nawait bridge.prompt(currentSessionId, message, images)\n\n目标有效上下文：\n1. reviewer workspace 自己的 system prompt / AGENTS.md\n2. reviewer 的 platform reminder 与 skills\n3. reviewer 该 session 的既有历史（仅 resume 时）\n4. 本轮 call_agent.prompt\n\n不会注入 coordinator 的 system prompt 或 coordinator 会话历史。",
    },
  ],
};

