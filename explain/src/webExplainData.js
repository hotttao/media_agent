export const webExplainData = {
  "title": "Web：界面结构与用户交互流程",
  "intro": "这里同时解释 Web 页面结构和关键用户操作如何进入后端。每篇文章独立说明入口、状态变化、失败分支和实现边界。",
  "chapters": [
    {
      "id": "interrupt-agent-turn",
      "title": "用户中断 Agent：从停止按钮到 Turn 终态",
      "summary": "中断不是把数据库状态直接改成 interrupted。正确流程是先记录 interrupt_requested，再把取消命令送到 Agent Runtime；只有 Runtime 确认停止或返回 interrupted 终止事件后，Gateway 才提交 Turn 终态。",
      "sourceRefs": [
        "目标设计：docs/ddd/media_agent_gateway/workflow/0001-run_turn/run_turn.md",
        "目标领域：docs/ddd/media_agent_gateway/domain/0001-agent_session/agent_session.md",
        "旧平台入口：control-plane/src/routes/workspaces/sessions.ts:221-260",
        "旧平台转发：control-plane/src/routes/workspaces/_shared.ts:19-39",
        "旧 Agent 中断：agents/claude-code/src/server.ts:342-348",
        "旧 Agent AbortController：agents/claude-code/src/agent.ts:69,124-139",
        "旧平台终止清理：control-plane/src/lib/sse.ts:567-590"
      ],
      "outline": [
        { "label": "核心结论", "target": "interrupt-agent-turn-overview" },
        { "label": "端到端时序", "target": "interrupt-agent-turn-code-0" },
        { "label": "Turn 状态机", "target": "interrupt-agent-turn-code-1" },
        { "label": "接口结果语义", "target": "interrupt-agent-turn-code-2" },
        { "label": "Pending Message", "target": "interrupt-agent-turn-code-3" },
        { "label": "事务与幂等", "target": "interrupt-agent-turn-code-4" },
        { "label": "旧实现对应关系", "target": "interrupt-agent-turn-code-5" },
        { "label": "阅读结论", "target": "interrupt-agent-turn-takeaways" }
      ],
      "bullets": [
        "用户只能中断自己有权访问的 Session。Edge 验证 Cookie JWT 或用户 Service Token，Gateway 再读取 Session 的真实 owner 和 workspace 做最终判断。",
        "pending 且尚未启动 Runtime 的 Turn 可以在数据库内直接进入 interrupted；running Turn 必须先进入 interrupt_requested，再通知 Runtime。",
        "delivered 表示取消请求到达 Runtime；interrupted 表示 Runtime 确实找到并终止了 active Turn。两者不能合并成一个 success。",
        "Runtime 不可达时，Turn 保持 interrupt_requested。Gateway 后台重试或通过 Runtime 终止事件协调，不能谎报 interrupted。",
        "中断和自然完成可能并发。若 Runtime 已自然完成，completed 是真实结果，晚到中断不能覆盖它。",
        "中断只能阻止后续执行，不能撤销已经发生的外部 MCP、消息发送、Browser 创建、生成任务或对象上传。",
        "同一 Session 不允许并发 Turn。运行期间输入的 follow-up 只是 pending message；中断或失败后不自动执行它。"
      ],
      "codeBlocks": [
        {
          "title": "端到端时序：请求中断、Runtime 确认、提交终态",
          "code": "UI                 Edge              media_agent_gateway       Gateway DB             Agent Runtime\n |                  |                         |                    |                       |\n |-- 点击停止 ----->|                         |                    |                       |\n |                  |-- mTLS + Principal ---->|                    |                       |\n |                  |                         |-- 读取 Session --->|                       |\n |                  |                         |   校验 owner/workspace/active_turn_id        |\n |                  |                         |                    |                       |\n |                  |                         |-- TX1: running -> interrupt_requested ------>|\n |                  |                         |-- CancelTurn(turn_id, request_id) ----------->|\n |                  |                         |                    |          AbortController.Cancel()\n |                  |                         |<-- accepted=true -----------------------------|\n |                  |                         |<-- TurnEnded(reason=interrupted) -------------|\n |                  |                         |-- TX2: interrupt_requested -> interrupted --->|\n |<-- delivered=true, interrupted=true -------|                    |                       |\n\n关键顺序：\n  1. 先持久化 interrupt_requested，避免取消请求丢失。\n  2. 再调用 Runtime，数据库事务不覆盖网络调用。\n  3. Runtime 的终止事件才是 interrupted 终态的依据。"
        },
        {
          "title": "Turn 状态机：为什么需要 interrupt_requested",
          "code": "pending -------------------------------> interrupted\n   |                                      尚未启动 Runtime，可本地结束\n   v\nrunning ---> interrupt_requested ---> interrupted\n   |                 |                     Runtime 确认中断\n   |                 +-------------------> completed\n   |                 |                     中断到达前已经自然完成\n   |                 +-------------------> failed\n   |                                       Runtime 以错误结束\n   +-------------------------------------> completed / failed\n\n终态：completed、failed、interrupted。\n规则：\n  - 终态不能互相覆盖。\n  - interrupt_requested 不是“已经停止”，只是“平台已接受中断意图”。\n  - 晚到的 completed/failed/interrupted 事件使用 expected_status 做 CAS。\n  - 客户端断线不是中断，不改变 Turn 状态。"
        },
        {
          "title": "接口结果语义：delivered 和 interrupted 必须分开",
          "code": "场景                                  delivered   interrupted   Gateway 行为\n--------------------------------------------------------------------------------\nTurn=pending，尚未启动 Runtime            true        true          本地提交 interrupted\nRuntime 找到 active Turn 并接受取消        true        true          等终止事件后提交终态\nRuntime 已经自然结束                       true        false         协调并返回 completed/failed\nRuntime 收到请求但没有这个 Turn            true        false         查询真实状态，不伪造终态\nRuntime 网络不可达                         false       false         保持 interrupt_requested 并重试\nTurn 已经 interrupted                      true        true          幂等返回既有结果\nTurn 已经 completed/failed                  true        false         返回真实终态\n\nHTTP 请求成功只说明 Gateway 处理了请求，不等于 Agent 已经停止。UI 应依据 delivered、interrupted 和 turn_status 展示结果。"
        },
        {
          "title": "Pending Message：中断后为什么不能自动执行 follow-up",
          "code": "同一 Session：\n\nTurn A = running\n用户提交 follow-up B\n  -> 不创建并发 Turn\n  -> 保存 session.pending_message = B\n\nTurn A completed\n  -> 原子 take-and-clear B\n  -> 创建 Turn B\n\nTurn A interrupted / failed\n  -> 不消费 B\n  -> B 保留为 pending message\n  -> 用户选择继续、修改或清除\n\n原因：用户刚点击“停止”，系统如果立刻执行排队输入，会产生“明明停止了却又开始运行”的反直觉行为。旧 agent-platform 也是只在 session.ended(reason=completed) 后 drain pending message。"
        },
        {
          "title": "事务、幂等和晚到事件",
          "code": "TX1（中断意图）\n  条件：turn.status = running\n  写入：status=interrupt_requested, interrupt_request_id, requested_by, requested_at\n\n事务外\n  调用 Runtime.CancelTurn(turn_id, interrupt_request_id)\n\nTX2（终态）\n  条件：status IN (running, interrupt_requested)\n  写入：status=interrupted, finished_at, terminal event + Outbox\n\n幂等键：turn_id + interrupt_request_id\n\n晚到规则：\n  - 重复 CancelTurn：Runtime 返回同一接受结果。\n  - 重复 TurnEnded：Gateway 按 event_id / expected_status 去重。\n  - interrupted 后的普通增量事件：只保留诊断，不生成新的用户可见内容。\n  - completed 已提交后再收到 Cancel：返回 interrupted=false，不覆盖 completed。"
        },
        {
          "title": "旧 agent-platform 如何实现这一语义",
          "code": "Web route\n  control-plane/src/routes/workspaces/sessions.ts\n  POST /:workspaceId/sessions/:sessionId/interrupt\n\nControl-plane -> Agent\n  interruptAgentSession() 调 Agent /sessions/:id/interrupt\n  返回 { delivered, interrupted }，明确区分“送达”和“真正中断”。\n\nAgent Runtime\n  activeControllers: Map<sessionId, AbortController>\n  interruptSession(sessionId) 找到 controller 后调用 abort()。\n\n终止事件\n  Agent 输出 session.ended(reason=interrupted)。\n  SSE 持久化链清理 active turn，并释放 turn admission slot。\n\nPending follow-up\n  只有 reason=completed 才调用 drainPendingMessage()。\n  error/interrupted 会保留 pending message。\n\n新架构保留这些业务语义，但把状态所有权收回 media_agent_gateway，并增加持久化 interrupt_requested 和 CAS 终态。"
        }
      ],
      "takeaways": [
        "中断是一个跨 Gateway 与 Runtime 的协作流程，不是一次数据库状态赋值。",
        "interrupt_requested 表达中断意图，interrupted 表达 Runtime 已经终止执行。",
        "delivered 与 interrupted 分离后，UI 才能准确展示网络失败、已自然完成和真正中断。",
        "中断不补偿已经发生的外部副作用，也不会自动执行 Session 的 pending follow-up。",
        "同一 Session 只有一个 active Turn；不同 Session 仍可以并发执行。"
      ]
    },
    {
      "id": "chat-component",
      "title": "Chat Component：从页面结构到代码结构",
      "summary": "Chat 面板不是 WorkspacePage 里直接写死的一块 JSX。它是 workspace shell 里的一个 app：布局决定 chat 默认出现在哪个 slot，SlotContainer 把 active app 包进 AppWindow，ChatApp 再渲染 WorkspaceChatPanel。WorkspaceChatPanel 自己负责消息区、提问卡、输入框和 header portal 控件。",
      "sourceRefs": [
        "web/src/App.tsx:46-75：/w/:workspaceId 路由进入 Desktop + WorkspacePage。",
        "web/src/components/shell/Desktop.tsx:54-155：判断 fleet/ws scope，创建 SlotProvider，渲染 Menubar/main/Dock。",
        "web/src/pages/WorkspacePage.tsx:17-101：选 workspace，包 AgentSessionProvider，渲染 ActiveLayout/filled Slot/PopoutLayer。",
        "web/src/components/shell/layout/ThreeColumnLayout.tsx:13-74：三列布局，slot-c 默认打开 chat。",
        "web/src/components/shell/layout/TwoColumnLayout.tsx:10-56：两列布局，slot-b 默认打开 chat。",
        "web/src/components/shell/layout/OneColumnLayout.tsx:9-15：单列布局，slot-a 默认打开 chat。",
        "web/src/components/shell/SlotContainer.tsx:13-43：按 slot active app 找 Component，并包进 AppWindow。",
        "web/src/components/shell/windows/AppWindow.tsx:30-112：提供窗口外壳和 header portal target。",
        "web/src/components/shell/apps/wsApps.tsx:195-198：ChatApp 从当前 workspace 渲染 WorkspaceChatPanel。",
        "web/src/components/workspace/WorkspaceChatPanel.tsx:253-284：读取 chat store 中的 session/messages/loading/pendingQuestion。",
        "web/src/components/workspace/WorkspaceChatPanel.tsx:773-1130：渲染 chat 面板主体、消息列表、问题卡和输入框。"
      ],
      "bullets": [
        "第一层是路由：访问 /w/:workspaceId 时，App.tsx 把页面放进 Desktop shell，WorkspacePage 成为 shell 的 children。",
        "第二层是桌面 shell：Desktop 渲染 Menubar、main、Dock，并在 workspace scope 下创建 SlotProvider。SlotProvider 保存每个 slot 打开了哪些 app、哪个 app 当前 active、哪个 app popout。",
        "第三层是 workspace 页面：WorkspacePage 不直接关心 chat 长什么样。它只负责选中 workspace、加载 session 初始状态，并用 AgentSessionProvider 包住布局。",
        "第四层是布局：ActiveLayout 来自 LAYOUTS[layoutId]。三列布局里 chat 默认在 slot-c；两列布局里 chat 默认在 slot-b；单列布局里 chat 默认就是 slot-a。",
        "第五层是 app 容器：SlotContainer 读取当前 slot 的 active app，比如 appId=chat，然后把 ChatApp 放进 AppWindow。AppWindow 提供统一窗口边框、顶部 header 区和右侧窗口控制按钮。",
        "第六层才是 chat 业务组件：ChatApp 只是一个很薄的适配器，它用 useCurrentWorkspace 拿 workspace，然后渲染 WorkspaceChatPanel。",
        "WorkspaceChatPanel 的 header 控件不在它自己的 DOM 位置渲染，而是通过 useAppHeaderSlot + createPortal 注入 AppWindow 的 header。",
        "WorkspaceChatPanel 的主体可以按“消息区 -> TurnStatsBar -> pending question -> composer”理解。消息区负责展示 MessageBubble；composer 负责图片、slash/mention/file mention、语音、发送/停止。"
      ],
      "layoutTree": {
        "title": "页面嵌套关系：Chat 在整个 Web 页面里的位置",
        "root": {
          "label": "App.tsx",
          "meta": "React Router 根组件",
          "file": "web/src/App.tsx:46-75",
          "children": [
            {
              "label": "ProtectedRoute",
              "meta": "未登录跳 /login；已登录才进入 workspace 页面",
              "file": "web/src/App.tsx:17-43",
              "children": [
                {
                  "label": "Desktop",
                  "meta": "全局桌面壳：Menubar + main + Dock + SlotProvider",
                  "file": "web/src/components/shell/Desktop.tsx:54-155",
                  "children": [
                    {
                      "label": "Menubar",
                      "meta": "顶部菜单栏",
                      "file": "Desktop.tsx:124"
                    },
                    {
                      "label": "main",
                      "meta": "页面主体区域，children 在这里渲染",
                      "file": "Desktop.tsx:129-140",
                      "children": [
                        {
                          "label": "WorkspacePage",
                          "meta": "workspace 路由页面，不直接写 chat UI",
                          "file": "web/src/pages/WorkspacePage.tsx:17-101",
                          "children": [
                            {
                              "label": "AgentSessionProvider",
                              "meta": "给 ChatPanel/PopoutLayer 提供 session store 上下文",
                              "file": "WorkspacePage.tsx:73-101",
                              "children": [
                                {
                                  "label": "ActiveLayout",
                                  "meta": "OneColumn / TwoColumn / ThreeColumn 中的一个",
                                  "file": "WorkspacePage.tsx:23-25,95",
                                  "children": [
                                    {
                                      "label": "SlotContainer(slot-c / slot-b / slot-a)",
                                      "meta": "根据 layout 决定 chat 所在 slot",
                                      "file": "layout/*.tsx + SlotContainer.tsx:13-43",
                                      "children": [
                                        {
                                          "label": "AppWindow",
                                          "meta": "统一窗口外壳，提供 header portal target",
                                          "file": "web/src/components/shell/windows/AppWindow.tsx:70-112",
                                          "children": [
                                            {
                                              "label": "ChatApp",
                                              "meta": "app registry 里的 chat 入口",
                                              "file": "web/src/components/shell/apps/wsApps.tsx:195-198",
                                              "children": [
                                                {
                                                  "label": "WorkspaceChatPanel",
                                                  "meta": "真正的 chat 面板 UI 和交互逻辑",
                                                  "file": "web/src/components/workspace/WorkspaceChatPanel.tsx:253-1130"
                                                }
                                              ]
                                            }
                                          ]
                                        }
                                      ]
                                    }
                                  ]
                                },
                                {
                                  "label": "PopoutLayer",
                                  "meta": "弹出窗口层；chat popout 后仍在 AgentSessionProvider 下",
                                  "file": "WorkspacePage.tsx:97-100"
                                }
                              ]
                            }
                          ]
                        }
                      ]
                    },
                    {
                      "label": "Dock",
                      "meta": "底部 app 启动/切换入口",
                      "file": "Desktop.tsx:142"
                    }
                  ]
                }
              ]
            }
          ]
        }
      },
      "slotDiagram": {
        "title": "布局 slot 与 chat 默认位置",
        "layouts": [
          {
            "name": "3col 默认布局",
            "file": "ThreeColumnLayout.tsx:13-74",
            "columns": [
              {
                "slot": "slot-a",
                "width": "左侧",
                "apps": [
                  "sessions"
                ],
                "note": "会话列表/历史入口"
              },
              {
                "slot": "slot-b",
                "width": "中间",
                "apps": [
                  "files",
                  "browser",
                  "skills",
                  "terminal",
                  "automation",
                  "memory",
                  "settings"
                ],
                "note": "工作区工具主区域"
              },
              {
                "slot": "slot-c",
                "width": "右侧",
                "apps": [
                  "chat"
                ],
                "note": "Chat 默认面板"
              }
            ]
          },
          {
            "name": "2col 默认布局",
            "file": "TwoColumnLayout.tsx:10-56",
            "columns": [
              {
                "slot": "slot-a",
                "width": "左侧/主区域",
                "apps": [
                  "files",
                  "browser",
                  "skills",
                  "terminal",
                  "automation",
                  "memory",
                  "settings"
                ],
                "note": "工具主区域"
              },
              {
                "slot": "slot-b",
                "width": "右侧",
                "apps": [
                  "chat"
                ],
                "note": "Chat 默认面板"
              }
            ]
          },
          {
            "name": "1col 默认布局",
            "file": "OneColumnLayout.tsx:9-15",
            "columns": [
              {
                "slot": "slot-a",
                "width": "全屏",
                "apps": [
                  "chat"
                ],
                "note": "单列时 chat 占整个可用区域"
              }
            ]
          }
        ]
      },
      "panelTree": {
        "title": "WorkspaceChatPanel 内部结构：面板里从上到下有什么",
        "root": {
          "label": "WorkspaceChatPanel",
          "meta": "props: workspace / readonly / transformFirstMessage / onMessages",
          "file": "WorkspaceChatPanel.tsx:253-258",
          "children": [
            {
              "label": "Header Portal",
              "meta": "session selector、share、新会话、搜索、重命名、字体、reset 等控件注入 AppWindow header",
              "file": "WorkspaceChatPanel.tsx:339,640-769"
            },
            {
              "label": "panel body: flex column",
              "meta": "chat 面板主体容器",
              "file": "WorkspaceChatPanel.tsx:773",
              "children": [
                {
                  "label": "messages area",
                  "meta": "可滚动消息区；空态、搜索浮层、虚拟列表/普通列表都在这里",
                  "file": "WorkspaceChatPanel.tsx:774-918",
                  "children": [
                    {
                      "label": "Search overlay",
                      "meta": "搜索栏浮在消息区上方",
                      "file": "WorkspaceChatPanel.tsx:775-834"
                    },
                    {
                      "label": "MessageBubble list",
                      "meta": "来自 @neutree-ai/ui-sdk，渲染每条 transcript message",
                      "file": "WorkspaceChatPanel.tsx:872-898"
                    },
                    {
                      "label": "scroll-to-bottom button",
                      "meta": "不在底部时出现",
                      "file": "WorkspaceChatPanel.tsx:907-917"
                    }
                  ]
                },
                {
                  "label": "TurnStatsBar",
                  "meta": "非 loading 且有消息时显示 token/turn 统计",
                  "file": "WorkspaceChatPanel.tsx:920-928"
                },
                {
                  "label": "AskUserQuestionPanel",
                  "meta": "pendingQuestion 存在时显示用户回答卡片",
                  "file": "WorkspaceChatPanel.tsx:930-932"
                },
                {
                  "label": "HintBar",
                  "meta": "agent 正在运行时显示提示文案",
                  "file": "WorkspaceChatPanel.tsx:934"
                },
                {
                  "label": "composer form",
                  "meta": "输入区；readonly=false 时才显示",
                  "file": "WorkspaceChatPanel.tsx:935-1130",
                  "children": [
                    {
                      "label": "attached image strip",
                      "meta": "图片附件预览和删除",
                      "file": "WorkspaceChatPanel.tsx:940-966"
                    },
                    {
                      "label": "slash / mention / file mention menus",
                      "meta": "命令、agent mention、文件 mention 的浮层",
                      "file": "WorkspaceChatPanel.tsx:968-972"
                    },
                    {
                      "label": "Textarea",
                      "meta": "主输入框，处理输入、粘贴、快捷键、语音占位",
                      "file": "WorkspaceChatPanel.tsx:1040-1065"
                    },
                    {
                      "label": "left actions",
                      "meta": "上传图片、compact、语音输入",
                      "file": "WorkspaceChatPanel.tsx:1070-1120"
                    },
                    {
                      "label": "right actions",
                      "meta": "loading 时 stop/queue；空闲时 send",
                      "file": "WorkspaceChatPanel.tsx:1121-1130"
                    }
                  ]
                }
              ]
            },
            {
              "label": "Rename Dialog",
              "meta": "会话重命名弹窗，不属于主垂直布局",
              "file": "WorkspaceChatPanel.tsx:1188-1214"
            }
          ]
        }
      },
      "codeBlocks": [
        {
          "title": "最小调用链：从路由到 ChatPanel",
          "code": "App.tsx\n  /w/:workspaceId\n    <Desktop>\n      <WorkspacePage />\n    </Desktop>\n\nDesktop.tsx\n  <SlotProvider slots={LAYOUTS[layoutId].slots} apps={useWsApps(workspaceId)}>\n    <Menubar />\n    <main>{children}</main>\n    <Dock />\n  </SlotProvider>\n\nWorkspacePage.tsx\n  <AgentSessionProvider workspaceId={selectedWorkspace.id}>\n    <ActiveLayout />\n    <PopoutLayer />\n  </AgentSessionProvider>\n\nThreeColumnLayout.tsx\n  slot-a -> sessions\n  slot-b -> files/browser/terminal/...\n  slot-c -> chat\n\nSlotContainer.tsx\n  active appId=chat\n  <AppWindow>\n    <ChatApp instanceId=... />\n  </AppWindow>\n\nwsApps.tsx\n  ChatApp -> <WorkspaceChatPanel workspace={ws} />"
        },
        {
          "title": "读 WorkspaceChatPanel 时先抓住这几个状态来源",
          "code": "const activeSessionId = useAgentSessionStore((s) => s.activeSessionId)\nconst messages = useAgentSessionStore((s) => s.messages)\nconst isLoading = useAgentSessionStore((s) => s.isLoading)\nconst pendingQuestion = useAgentSessionStore((s) => s.pendingQuestion)\nconst pendingMessage = useAgentSessionStore((s) => s.pendingMessage)\nconst lastTurnStats = useAgentSessionStore((s) => s.lastTurnStats)\nconst actions = useAgentSessionActions()\n\n// 页面上看到什么，大多由这些状态决定：\n// messages.length === 0 -> 空态\n// messages.length > 0 -> MessageBubble 列表\n// isLoading -> stop/queue/hint/loading composer strip\n// pendingQuestion -> AskUserQuestionPanel\n// readonly -> 隐藏输入 form 和 header portal 控件"
        },
        {
          "title": "为什么 header 控件不在 ChatPanel DOM 的顶部",
          "code": "// WorkspaceChatPanel.tsx\nconst headerSlot = useAppHeaderSlot()\n\n{!readonly && headerSlot && createPortal(\n  <>\n    <Select>session switcher</Select>\n    <ShareSessionButton />\n    <AppHeaderButton icon={SquarePen} />\n    <DropdownMenu>search / rename / font / reset</DropdownMenu>\n  </>,\n  headerSlot,\n)}\n\n// AppWindow.tsx\n<header>\n  <div ref={setHeaderSlot} />  // ChatPanel 的 header portal 会挂到这里\n  <WindowControls />           // popout / fill / fullscreen / close\n</header>\n<div className=\"body\">\n  {children}                   // WorkspaceChatPanel 主体在这里\n</div>"
        }
      ],
      "takeaways": [
        "读页面结构时先别从 WorkspaceChatPanel 开始。先从 App.tsx 和 Desktop.tsx 看外壳，再看 WorkspacePage 和 layout，最后才进入 ChatPanel。",
        "Chat 是一个 workspace app，不是固定右栏组件。它默认在哪个位置由当前 layout 的 slot defaultOpened 决定。",
        "AppWindow 是理解 header 的关键：窗口外壳归 shell 管，chat 业务控件通过 portal 注入 header。",
        "AgentSessionProvider 是理解状态的关键：SlotContainer 切换 app 会 remount，但 chat session 状态放在更高层 provider 里。"
      ]
    },
    {
      "id": "chat-css-layout",
      "title": "WorkspaceChatPanel：CSS 布局框图",
      "summary": "这一节只看布局，不讲业务。下面的文本框从外到内标出每个组件/区域，以及它身上最关键的 CSS class。读代码时可以按这个框图一层层对照。",
      "sourceRefs": [
        "web/src/components/shell/windows/AppWindow.tsx:70-112：AppWindow 的 header/body 外壳。",
        "web/src/components/workspace/WorkspaceChatPanel.tsx:613-934：ChatPanel 外层、header portal、消息区。",
        "web/src/components/workspace/WorkspaceChatPanel.tsx:935-1130：底部 composer 输入区。"
      ],
      "bullets": [
        "先看框图里的组件边界：AppWindow 是窗口外壳，WorkspaceChatPanel 是聊天面板，message region 是消息区，scroll viewport 才是真正滚动的地方。",
        "每个框里的 class 只保留影响布局的部分。颜色、边框、hover、阴影这类装饰 class 先忽略。",
        "读源码时按顺序找：AppWindow body -> WorkspaceChatPanel root -> message region -> scroll viewport -> composer form。"
      ],
      "codeBlocks": [
        {
          "title": "文本框图：WorkspaceChatPanel 页面布局",
          "code": "+--------------------------------------------------------------------------------+\n| AppWindow                                                                       |\n|                                                                                |\n|  +---------------------------------------------------------------------------+ |\n|  | header                                                                    | |\n|  | class: flex h-9 shrink-0 items-center border-b                            | |\n|  |                                                                           | |\n|  |  +--------------------------------------+  +----------------------------+ | |\n|  |  | headerSlot                           |  | WindowControls             | | |\n|  |  | ChatPanel 用 createPortal 放控件      |  | popout / fill / close      | | |\n|  |  | class: min-w-0 flex-1                |  | class: flex shrink-0       | | |\n|  |  +--------------------------------------+  +----------------------------+ | |\n|  +---------------------------------------------------------------------------+ |\n|                                                                                |\n|  +---------------------------------------------------------------------------+ |\n|  | AppWindow body                                                            | |\n|  | class: flex min-h-0 min-w-0 flex-1 flex-col                               | |\n|  |                                                                           | |\n|  |  +------------------------------------------------------------------------+|\n|  |  | WorkspaceChatPanel root                                                 ||\n|  |  | class: flex h-full flex-col                                             ||\n|  |  |                                                                        ||\n|  |  |  +------------------------------------------------------------------+  ||\n|  |  |  | message region                                                   |  ||\n|  |  |  | class: relative min-h-0 flex-1                                  |  ||\n|  |  |  |                                                                  |  ||\n|  |  |  |  +------------------------------------------------------------+  |  ||\n|  |  |  |  | search overlay                                             |  |  ||\n|  |  |  |  | class: absolute top-2 right-2 left-2 z-10                  |  |  ||\n|  |  |  |  +------------------------------------------------------------+  |  ||\n|  |  |  |                                                                  |  ||\n|  |  |  |  +------------------------------------------------------------+  |  ||\n|  |  |  |  | scroll viewport                                            |  |  ||\n|  |  |  |  | class: absolute inset-0 overflow-y-auto                    |  |  ||\n|  |  |  |  |                                                            |  |  ||\n|  |  |  |  |  empty state / virtual list / MessageBubble list / error   |  |  ||\n|  |  |  |  |  list class: p-3 space-y-3                                 |  |  ||\n|  |  |  |  +------------------------------------------------------------+  |  ||\n|  |  |  |                                                                  |  ||\n|  |  |  |  scroll button                                                   |  ||\n|  |  |  |  class: absolute bottom-3 left-1/2 z-10                         |  ||\n|  |  |  +------------------------------------------------------------------+  ||\n|  |  |                                                                        ||\n|  |  |  +------------------------------------------------------------------+  ||\n|  |  |  | TurnStatsBar                                                     |  ||\n|  |  |  | condition: !isLoading && messages.length > 0                    |  ||\n|  |  |  +------------------------------------------------------------------+  ||\n|  |  |                                                                        ||\n|  |  |  +------------------------------------------------------------------+  ||\n|  |  |  | AskUserQuestionPanel                                             |  ||\n|  |  |  | condition: pendingQuestion                                       |  ||\n|  |  |  +------------------------------------------------------------------+  ||\n|  |  |                                                                        ||\n|  |  |  +------------------------------------------------------------------+  ||\n|  |  |  | HintBar                                                          |  ||\n|  |  |  | condition: !readonly                                             |  ||\n|  |  |  +------------------------------------------------------------------+  ||\n|  |  |                                                                        ||\n|  |  |  +------------------------------------------------------------------+  ||\n|  |  |  | composer form                                                    |  ||\n|  |  |  | class: shrink-0 border-t p-3                                    |  ||\n|  |  |  |                                                                  |  ||\n|  |  |  |  attached images: flex flex-wrap gap-1.5 pb-2                   |  ||\n|  |  |  |  input shell: relative overflow-hidden rounded-lg border        |  ||\n|  |  |  |  Textarea: resize-none min-h-0 overflow-y-auto max-h-[50vh]     |  ||\n|  |  |  |  action row: flex items-center justify-between                  |  ||\n|  |  |  +------------------------------------------------------------------+  ||\n|  |  +------------------------------------------------------------------------+|\n|  +---------------------------------------------------------------------------+ |\n+--------------------------------------------------------------------------------+"
        },
        {
          "title": "同一结构对应到 JSX 骨架",
          "code": "<AppWindow>\n  <header className=\"flex h-9 shrink-0 items-center border-b\">\n    <div ref={setHeaderSlot} className=\"min-w-0 flex-1\" />\n    <WindowControls />\n  </header>\n\n  <div className=\"flex min-h-0 min-w-0 flex-1 flex-col\">\n    <WorkspaceChatPanel />\n  </div>\n</AppWindow>\n\nfunction WorkspaceChatPanel() {\n  return (\n    <div className=\"flex h-full flex-col\">\n      <div className=\"relative min-h-0 flex-1\">\n        {searchOpen && <div className=\"absolute top-2 right-2 left-2 z-10\" />}\n\n        <div ref={scrollRef} className=\"absolute inset-0 overflow-y-auto\">\n          <div className=\"p-3 space-y-3\">\n            {messages.map((msg) => <MessageBubble message={msg} />)}\n          </div>\n        </div>\n\n        {showScrollBtn && <button className=\"absolute bottom-3 left-1/2 z-10\" />}\n      </div>\n\n      {!isLoading && messages.length > 0 && <TurnStatsBar />}\n      {pendingQuestion && <AskUserQuestionPanel />}\n      {!readonly && <HintBar />}\n      {!readonly && <form className=\"shrink-0 border-t p-3\">composer</form>}\n    </div>\n  )\n}"
        },
        {
          "title": "只记这 5 个布局点",
          "code": "1. AppWindow body\n   class: flex min-h-0 min-w-0 flex-1 flex-col\n   作用: 给 ChatPanel 一个能撑满窗口、又能正确滚动的父容器。\n\n2. WorkspaceChatPanel root\n   class: flex h-full flex-col\n   作用: 聊天面板从上到下排列。\n\n3. message region\n   class: relative min-h-0 flex-1\n   作用: 消息区吃掉 composer 之外的剩余高度。\n\n4. scroll viewport\n   class: absolute inset-0 overflow-y-auto\n   作用: 真正发生滚动的是这里，不是整个 ChatPanel。\n\n5. composer form\n   class: shrink-0 border-t p-3\n   作用: 输入区固定在底部，不被消息列表压缩掉。"
        }
      ],
      "takeaways": [
        "这一节只需要记住一个形状：外层窗口 body 给高度，中间消息区 flex-1，里面 scroll viewport 负责滚动，底部 composer shrink-0。",
        "看代码时先忽略颜色、边框、hover、动画，只看 flex、h-full、min-h-0、relative、absolute、overflow-y-auto、shrink-0。"
      ]
    },
    {
      "id": "chat-state",
      "title": "WorkspaceChatPanel：核心 state 要分成三层看",
      "summary": "这个组件状态很多，但不是每个状态都同等重要。可以分成三层：AgentSession 全局状态决定当前会话和消息；本地 UI state 决定输入框、搜索、重命名、字体、附件；服务端 query 状态补充 session 列表、agent 类型、来源信息。",
      "sourceRefs": [
        "web/src/stores/agent-session-store.ts:85-149：AgentSessionState 和 AgentSessionActions 的核心定义。",
        "web/src/components/workspace/WorkspaceChatPanel.tsx:276-285：从 useAgentSessionStore 读取会话、消息、loading、问题、统计。",
        "web/src/components/workspace/WorkspaceChatPanel.tsx:291-338：读取 session 列表、draft、附件、语音、agentInfo、headerSlot。",
        "web/src/components/workspace/WorkspaceChatPanel.tsx:339-433：sessionSource、callerAgent、rename、fontSize、scroll、search、virtualization。",
        "web/src/components/workspace/WorkspaceChatPanel.tsx:459-520：handleSubmit 处理发送、排队和新会话首条消息 transform。"
      ],
      "bullets": [
        "第一层是 AgentSession store。这一层可以理解成“聊天业务状态”：当前 session 是谁、消息列表是什么、agent 是否正在跑、有没有 pending question、有没有 pendingMessage、最后一轮 token 统计是什么。",
        "第二层是组件本地 UI state。这一层可以理解成“这个面板自身的交互状态”：输入框草稿、附件图片、文件 input ref、语音状态、搜索框、重命名弹窗、字体大小、滚动 ref。",
        "第三层是服务端 query/context 状态。这一层不是 chat 消息本身，但会影响页面展示：workspace 下所有 sessions、当前 agent 类型、session 是从哪个 channel 来的、是不是被另一个 agent 调起。",
        "derived state 不单独存数据库，也不需要手动同步。例如 isChatBusy = isLoading || isDeleting；isSessionLocked = isDeleting || isSwitching；virtualizeList = !searchOpen && messages.length > 30。",
        "handleSubmit 是理解输入区状态的核心：如果 agent 正在 loading，提交不会立即开新 turn，而是更新 pendingMessage；如果 agent 空闲，提交会 sendMessage 并清空 draft 和附件。",
        "pendingQuestion 和 pendingMessage 是两个不同概念：pendingQuestion 是 agent 主动问用户问题，需要用户回答；pendingMessage 是用户在 agent 正在运行时提前排队的下一条消息。",
        "fontSize 存 localStorage，只影响消息滚动区 style={{ fontSize }}；它不是 session 数据。",
        "attachedImages 是发送前的临时附件状态。发送成功或排队成功后会清空；真正进入消息后，图片会作为 message block 进入 ChatMessage。"
      ],
      "panelTree": {
        "title": "状态分层：每类 state 控制页面哪里",
        "root": {
          "label": "WorkspaceChatPanel state",
          "meta": "不要逐个 useState 背诵，按职责分组读",
          "file": "WorkspaceChatPanel.tsx:276-433",
          "children": [
            {
              "label": "AgentSession store",
              "meta": "聊天业务主状态，由 AgentSessionProvider 创建并通过 context 暴露",
              "file": "agent-session-store.ts:85-149",
              "children": [
                {
                  "label": "activeSessionId / loadedSessionId",
                  "meta": "决定当前打开哪段会话历史",
                  "file": "agent-session-store.ts:103-112"
                },
                {
                  "label": "messages",
                  "meta": "MessageBubble 的唯一主要输入",
                  "file": "agent-session-store.ts:113"
                },
                {
                  "label": "isLoading / isSwitching / isDeleting / error",
                  "meta": "控制 loading、禁用、错误提示、发送按钮状态",
                  "file": "agent-session-store.ts:114-117"
                },
                {
                  "label": "pendingQuestion",
                  "meta": "控制 AskUserQuestionPanel 是否出现",
                  "file": "agent-session-store.ts:118"
                },
                {
                  "label": "pendingMessage",
                  "meta": "用户在 agent 忙时排队的下一条消息",
                  "file": "agent-session-store.ts:126-130"
                },
                {
                  "label": "actions",
                  "meta": "sendMessage、switchSession、respondToQuestion、stop、deleteSession 等操作入口",
                  "file": "agent-session-store.ts:133-149"
                }
              ]
            },
            {
              "label": "Local UI state / refs",
              "meta": "只影响当前 ChatPanel 的输入和显示",
              "file": "WorkspaceChatPanel.tsx:312-433",
              "children": [
                {
                  "label": "input draft",
                  "meta": "useDraft(workspace.id, activeSessionId)，按 workspace + session 保存输入框草稿",
                  "file": "WorkspaceChatPanel.tsx:313"
                },
                {
                  "label": "attachedImages",
                  "meta": "待发送图片附件，发送/排队后清空",
                  "file": "WorkspaceChatPanel.tsx:319"
                },
                {
                  "label": "voiceState",
                  "meta": "idle / recording / transcribing，控制语音面板和 Textarea readOnly",
                  "file": "WorkspaceChatPanel.tsx:323-325"
                },
                {
                  "label": "renameOpen / renameValue",
                  "meta": "会话重命名弹窗状态",
                  "file": "WorkspaceChatPanel.tsx:346-369"
                },
                {
                  "label": "fontSize",
                  "meta": "消息区字体大小，持久化到 localStorage",
                  "file": "WorkspaceChatPanel.tsx:372-381"
                },
                {
                  "label": "scrollRef / panelRef / search state",
                  "meta": "自动滚动、局部搜索和搜索命中跳转",
                  "file": "WorkspaceChatPanel.tsx:411-428"
                }
              ]
            },
            {
              "label": "Server query/context",
              "meta": "补充页面上下文，不直接构成消息块",
              "file": "WorkspaceChatPanel.tsx:291-342",
              "children": [
                {
                  "label": "sessions = useSessions(workspace.id)",
                  "meta": "header 里的会话选择列表、callerAgent 来源",
                  "file": "WorkspaceChatPanel.tsx:291"
                },
                {
                  "label": "agentInfo = useAgentInfo(workspace.id)",
                  "meta": "决定 TranscriptProviders 的 agentType",
                  "file": "WorkspaceChatPanel.tsx:326-327"
                },
                {
                  "label": "sessionSource = useSessionSource(activeSessionId)",
                  "meta": "显示 via connector_name 的来源 badge",
                  "file": "WorkspaceChatPanel.tsx:341"
                },
                {
                  "label": "slotCtx = useSlotContext()",
                  "meta": "判断 sessions app 是否已经打开，避免 header 再放完整会话选择器",
                  "file": "WorkspaceChatPanel.tsx:329-337"
                }
              ]
            }
          ]
        }
      },
      "codeBlocks": [
        {
          "title": "核心 store 状态：决定聊天主流程",
          "code": "const activeSessionId = useAgentSessionStore((s) => s.activeSessionId)\nconst messages = useAgentSessionStore((s) => s.messages)\nconst isLoading = useAgentSessionStore((s) => s.isLoading)\nconst isSwitching = useAgentSessionStore((s) => s.isSwitching)\nconst isDeleting = useAgentSessionStore((s) => s.isDeleting)\nconst error = useAgentSessionStore((s) => s.error)\nconst pendingQuestion = useAgentSessionStore((s) => s.pendingQuestion)\nconst pendingMessage = useAgentSessionStore((s) => s.pendingMessage)\nconst lastTurnStats = useAgentSessionStore((s) => s.lastTurnStats)\nconst actions = useAgentSessionActions()\n\nconst isChatBusy = isLoading || isDeleting\nconst isSessionLocked = isDeleting || isSwitching"
        },
        {
          "title": "核心本地状态：决定输入区和面板 UI",
          "code": "const { draft: input, setDraft: setInput, clearDraft } = useDraft(workspace.id, activeSessionId)\nconst [attachedImages, setAttachedImages] = useState([])\nconst fileInputRef = useRef(null)\nconst inputRef = useRef(null)\nconst [voiceState, setVoiceState] = useState('idle')\nconst [renameOpen, setRenameOpen] = useState(false)\nconst [renameValue, setRenameValue] = useState('')\nconst [fontSize, setFontSize] = useState(() => Number(localStorage.getItem('tos-chat-font-size')) || 13)\n\nconst { scrollRef, showScrollBtn, markPendingScroll, handleScrollBtnClick } = useAutoScroll([messages], activeSessionId)\nconst { searchOpen, searchQuery, searchMatches, searchIndex } = useChatSearch(scrollRef, panelRef, messages)"
        },
        {
          "title": "handleSubmit：空闲发送，运行中排队",
          "code": "function handleSubmit(e) {\n  e.preventDefault()\n  if (slashVisible || mentionVisible || fileMentionVisible) return\n  if (isDeleting) return\n\n  if (isLoading) {\n    // agent 正在跑：这次提交不会立即创建新 turn，而是写入 pendingMessage\n    actions.updatePendingMessage(text, images)\n    setAttachedImages([])\n    return\n  }\n\n  // agent 空闲：真正发送给当前 session；如果没有 activeSessionId，后端会创建新 session\n  actions.sendMessage(text, images)\n  clearDraft()\n  setAttachedImages([])\n  markPendingScroll()\n}"
        },
        {
          "title": "状态到 UI 的粗略对应关系",
          "code": "messages.length === 0      -> 空态 EmptyHero 或切换 session 的 Spinner\nmessages.length > 0       -> MessageBubble 列表\nmessages.length > 30      -> 关闭搜索时启用虚拟列表\nsearchOpen === true       -> 搜索浮层出现，并关闭虚拟列表\nisLoading === true        -> 显示 HintBar、stop 按钮，提交变成 queue\npendingMessage != null    -> composer 顶部显示 pending strip\npendingQuestion != null   -> 显示 AskUserQuestionPanel\nerror != null             -> 消息列表底部显示 Alert\nreadonly === true         -> 隐藏 header 控件和 composer form"
        }
      ],
      "takeaways": [
        "读 state 不要从 useState 数量判断复杂度。先分清“聊天业务状态”和“面板 UI 状态”。",
        "messages、isLoading、pendingQuestion、pendingMessage 是最值得优先理解的四个状态。它们直接决定用户看到的是消息、等待、提问卡，还是排队消息。",
        "这个组件里很多状态是派生状态，不需要找数据库字段。例如 isChatBusy、isSessionLocked、virtualizeList 都只是为了渲染和禁用按钮方便。"
      ]
    },
    {
      "id": "chat-message-rendering",
      "title": "WorkspaceChatPanel：message 渲染和 ToolRenderer 怎么接上",
      "summary": "WorkspaceChatPanel 并不自己画每一种 tool 卡片。它只把 messages 交给 MessageBubble。MessageBubble 遍历 message.blocks，遇到 text 就 Markdown，遇到 tool 就交给 ToolCallBlock。ToolCallBlock 再按 tool name、agentType、插件注册表选择具体 renderer。",
      "sourceRefs": [
        "web/src/stores/agent-session-store.ts:407-590：SSE item started/delta/completed 转成 message blocks。",
        "web/src/components/workspace/WorkspaceChatPanel.tsx:850-904：普通列表和虚拟列表都渲染 MessageBubble。",
        "internal/ui-sdk/src/components/MessageBubble.tsx:59-143：按 block.type 渲染 text/tool/status/image。",
        "internal/ui-sdk/src/components/ToolCallBlock.tsx:40-123：解析 renderer，展示折叠头、preview、input/result。",
        "internal/ui-sdk/src/tool-renderers/registry.ts:44-153：内置 renderer、agent fallback、resolveRenderer。",
        "internal/ui-sdk/src/tool-renderers/plugin-registry.ts:1-31：registerToolRenderer 的全局注册表。",
        "web/src/components/chat/TranscriptProviders.tsx:21-43：给消息渲染提供 Markdown、agentType、lazy tool renderer。",
        "web/src/plugins/index.ts:24-45：启动时注册 builder-mode propose tool 的 renderer。",
        "web/src/components/chat/tool-renderers/mcp/agent-request.tsx:1-170：propose tool 的 Approve/Reject 卡片渲染逻辑。",
        "internal/ui-sdk/src/tool-renderers/registry.ts:130-145：unwrapExecuteDispatchName 从 Codex execute input 中取真实 tool 名。",
        "web/src/components/chat/tool-renderers/mcp/agent-request.tsx:73-170：交互卡片使用 useAgentSessionStore、useAgentSessionActions 和 api。"
      ],
      "bullets": [
        "第一步，后端 SSE 事件进入 agent-session-store。文本增量 onItemDelta 会拼进 assistant message 的 text block。tool_call 开始时 onItemStarted 会创建 tool block，并尽早解析 arguments 放进 tool.input。",
        "第二步，tool_result 到达时，store 按 call_id 找到之前的 tool block，把 result、isError、resultAt 写回去。这样 UI 上同一张 tool 卡片会从 spinning 状态变成完成或失败。",
        "第三步，WorkspaceChatPanel 渲染 messages。普通情况是 messages.map；消息超过 30 条且搜索没打开时启用虚拟列表，只虚拟化历史消息，最后一条流式消息保留普通流式渲染。",
        "第四步，MessageBubble 根据 message.role 和 block.type 分流。user 消息是右侧气泡；assistant 的 text block 走 Markdown；tool block 交给 ToolCallBlock；status/image 分别渲染成状态 pill 或图片。",
        "第五步，ToolCallBlock 先把 MCP 或 Codex 包装过的 tool name 还原成展示名，再调用 resolveRenderer(tool, agentType)。找到 renderer 就用它的 getPreview/renderInput/renderResult；找不到就退回 DefaultInput/DefaultResult。",
        "第六步，renderer 来源有三类：ui-sdk 内置 renderer、web app 启动时 registerToolRenderer 注册的 renderer、lazy plugin renderer。agentType fallback 是最后兜底，例如 codex fallback 或 claude fallback。",
        "builder-mode 的 *_propose tool 是特殊例子。它的 renderer 不在 ui-sdk 里内置，因为它需要访问 web app 的 AgentSessionProvider、api、actions。web 启动时 registerBuiltinPlugins 会把这些 propose tool 注册到 ui-sdk registry。",
        "agentRequestProposeRenderer 的结果不是直接相信 tool.input。tool result 里会有 request_id/kind/label envelope；交互卡片再请求 api.getAgentRequest(workspaceId, request_id) 获取权威状态，然后 approve/reject 后发一条 <agent-sys> 消息让 agent 调 apply tool。",
        "为什么 builder-mode propose renderer 要在 web app 注册：ui-sdk 是通用 transcript 渲染包，它不知道当前 workspaceId、AgentSessionProvider、api client、React Query 缓存，也不能假设自己一定运行在 workspace 页面里。builder-mode 的 approve/reject 卡片不是单纯展示 tool result，它要读取 agent_requests 权威状态、调用 resolve 接口、再向当前 session 发送一条 <agent-sys> 消息推动 agent 继续 apply。因为这些能力都来自 web app 的运行上下文，所以 renderer 必须由 web app 启动时注册进去。",
        "“需要状态”更准确地说是“需要宿主应用状态和宿主应用副作用”。普通 renderer 只根据 tool.input/tool.result 展示内容，可以放在 ui-sdk。propose renderer 需要 workspaceId、当前 session actions、登录后的 api client、是否存在 AgentSessionProvider、以及共享页 readonly fallback，这些都不是 ui-sdk 自己能安全拥有的。",
        "Codex 的 MCP tool 包装细节：Codex transcript 里可能看到的 tool.name 不是 read_memory/create_browser 这种真实工具名，而是统一的 execute。真实目标藏在 tool.input.tool 里，同时 input.server 表示 MCP server，input.arguments 才是真实工具参数。所以 ToolCallBlock 不能直接用 name=execute 去找 renderer，否则只会落到默认 renderer。",
        "unwrapExecuteDispatchName 的作用就是修正这个问题：当 name === execute，且 input 是对象，并且 input.tool 是字符串、input.server 存在时，renderer lookup 使用 input.tool。比如 execute + { server: \"platform\", tool: \"read_memory\", arguments: {...} } 会被当成 read_memory 去匹配 readMemoryRenderer。"
      ],
      "panelTree": {
        "title": "消息渲染链路：从 SSE 到具体 tool 卡片",
        "root": {
          "label": "control-plane SSE stream",
          "meta": "返回 message、tool_call、tool_result、status、question 等事件",
          "file": "agent-session-store.ts:390-590",
          "children": [
            {
              "label": "agent-session-store",
              "meta": "把事件归一化成 ChatMessage[]，存到 messages",
              "file": "agent-session-store.ts:407-590",
              "children": [
                {
                  "label": "text delta -> text block",
                  "meta": "onItemDelta 拼接 assistant 文本",
                  "file": "agent-session-store.ts:452-468"
                },
                {
                  "label": "tool_call -> tool block",
                  "meta": "onItemStarted 创建 tool block，解析 arguments 得到 input",
                  "file": "agent-session-store.ts:407-448"
                },
                {
                  "label": "tool_result -> 更新 tool block",
                  "meta": "按 call_id 写入 result/isError/resultAt",
                  "file": "agent-session-store.ts:560-590"
                }
              ]
            },
            {
              "label": "WorkspaceChatPanel",
              "meta": "读取 messages，决定普通 map 还是虚拟列表",
              "file": "WorkspaceChatPanel.tsx:850-904",
              "children": [
                {
                  "label": "MessageBubble",
                  "meta": "按 role 和 block.type 渲染 transcript",
                  "file": "MessageBubble.tsx:59-143",
                  "children": [
                    {
                      "label": "text -> Markdown",
                      "meta": "assistant 文本块交给 MarkdownProvider 提供的 Markdown",
                      "file": "MessageBubble.tsx:96-111"
                    },
                    {
                      "label": "tool -> ToolCallBlock",
                      "meta": "工具调用块不在 ChatPanel 内部展开",
                      "file": "MessageBubble.tsx:112-113"
                    },
                    {
                      "label": "status/image -> inline UI",
                      "meta": "状态 pill 或图片预览/放大",
                      "file": "MessageBubble.tsx:114-143"
                    }
                  ]
                }
              ]
            },
            {
              "label": "ToolCallBlock",
              "meta": "resolveRenderer 后渲染 preview/input/result",
              "file": "ToolCallBlock.tsx:40-123",
              "children": [
                {
                  "label": "built-in renderer",
                  "meta": "Claude/Codex/Goose/MCP 常见 tool 的内置卡片",
                  "file": "registry.ts:44-114"
                },
                {
                  "label": "plugin registered renderer",
                  "meta": "web app 或插件通过 registerToolRenderer 注册",
                  "file": "plugin-registry.ts:12-20"
                },
                {
                  "label": "agent fallback/default",
                  "meta": "没匹配到具体 renderer 时，用 agent fallback 或 DefaultInput/DefaultResult",
                  "file": "registry.ts:116-153"
                }
              ]
            }
          ]
        }
      },
      "codeBlocks": [
        {
          "title": "WorkspaceChatPanel 本身只负责把 message 交给 MessageBubble",
          "code": "{messages.map((msg) => (\n  <TranscriptI18nProvider key={msg.id} locale={i18n.language}>\n    <MessageBubble message={msg} />\n  </TranscriptI18nProvider>\n))}\n\n// 超过 30 条消息且没有打开搜索：历史消息走 virtualizer，最后一条仍普通渲染\nconst virtualizeList = !searchOpen && messages.length > 30"
        },
        {
          "title": "store 把 SSE 工具事件变成 tool block",
          "code": "onItemStarted(item) {\n  if (item.kind === 'tool_call') {\n    const input = JSON.parse(tc.arguments || '{}')\n    appendBlock({\n      type: 'tool',\n      tool: {\n        id: tc.call_id,\n        name: tc.name,\n        input,\n        startedAt: Date.now(),\n        parentToolUseId: item.parent_tool_use_id ?? null,\n      },\n    })\n  }\n}\n\nonItemCompleted(item) {\n  if (item.kind === 'tool_result') {\n    updateMatchingToolByCallId({\n      result: tr.output,\n      isError: tr.is_error,\n      resultAt: Date.now(),\n    })\n  }\n}"
        },
        {
          "title": "MessageBubble 的核心分流",
          "code": "assistant message:\n  for block in message.blocks:\n    block.type === 'text'   -> <Markdown>{block.text}</Markdown>\n    block.type === 'tool'   -> <ToolCallBlock tool={block.tool} />\n    block.type === 'status' -> 状态 pill\n    block.type === 'image'  -> 图片预览\n\nuser message:\n  右侧气泡 + 图片附件\n\nagent-sys message:\n  虽然 role 是 user，但视觉上按中性 assistant-side markdown 展示"
        },
        {
          "title": "ToolCallBlock 如何选择 renderer",
          "code": "const agentType = useAgentType()\nconst lazy = useLazyToolRenderers()\nconst effectiveName = unwrapExecuteDispatchName(tool.name, tool.input)\nconst renderer = resolveRenderer(tool, agentType)\n\nconst preview = renderer?.getPreview(tool) ?? jsonPreview(tool.input)\n\n{renderer?.renderInput(tool) ?? <DefaultInput tool={tool} />}\n{tool.result !== undefined && (\n  renderer?.renderResult(tool) ?? <DefaultResult tool={tool} />\n)}"
        },
        {
          "title": "resolveRenderer 的优先级",
          "code": "resolveRenderer(tool, agentType):\n  1. getToolDisplayName(): 去掉 mcp__server__tool、Tool: server/tool、server__tool 这些前缀\n  2. unwrapExecuteDispatchName(): Codex 的 execute 包装里，从 input.tool 取真实 tool 名\n  3. toolRenderers[displayName]: ui-sdk 内置 renderer\n  4. getPluginToolRenderer(displayName): web app/plugin 注册的 renderer\n  5. agentFallbacks[agentType]: claude-code / codex / goose 的兜底 renderer\n  6. null: ToolCallBlock 使用 DefaultInput / DefaultResult"
        },
        {
          "title": "为什么 builder-mode propose renderer 在 web app 注册",
          "code": "// web/src/main.tsx\nregisterBuiltinPlugins()\nawait loadExternalPlugins()\n\n// web/src/plugins/index.ts\nfor (const name of AGENT_REQUEST_PROPOSE_TOOLS) {\n  registerToolRenderer(name, agentRequestProposeRenderer)\n}\n\n// agent-request renderer 需要 web app 状态：\napi.getAgentRequest(workspaceId, request_id)\napi.resolveAgentRequest(workspaceId, request_id, decision)\nactions.sendMessage('<agent-sys>...please call apply...</agent-sys>')"
        },
        {
          "title": "ToolRenderer 和 toolDispatcher 不是一回事",
          "code": "ToolRenderer:\n  负责把 tool call 画成 UI 卡片。\n  入口在 MessageBubble -> ToolCallBlock -> resolveRenderer。\n\ntoolDispatcher:\n  负责 tool_result 到达后的插件副作用。\n  入口在 agent-session-store 的 tool_result 处理里。\n  例如某个文件/skills/memory 插件可以在工具完成后刷新自己的面板。"
        },
        {
          "title": "builder-mode propose renderer 为什么不能放在纯 ui-sdk 里",
          "code": "ui-sdk renderer 适合处理：\n  输入: tool.name / tool.input / tool.result / agentType\n  输出: 一张只读展示卡片\n  特点: 不依赖当前 workspace 页面，不调用业务接口，不修改 session\n\nbuilder-mode propose renderer 实际要做：\n  1. 从 tool.result 解析 envelope\n     { request_id, kind, label, payload?, status? }\n\n  2. 使用 web app 当前上下文\n     workspaceId = useAgentSessionStore((s) => s.workspaceId)\n     actions = useAgentSessionActions()\n     hasSessionProvider = useHasAgentSessionProvider()\n\n  3. 请求 control-plane 权威数据\n     api.getAgentRequest(workspaceId, request_id)\n\n  4. 用户点击 Approve / Reject 后写回 control-plane\n     api.resolveAgentRequest(workspaceId, request_id, decision, reason)\n\n  5. 再向当前 session 发一条系统提示，让 agent 继续执行 apply tool\n     actions.sendMessage('<agent-sys>Request ... approved. Please call apply...</agent-sys>')\n\n结论：\n  这不是“展示一个 tool result”这么简单。\n  它需要 web app 的 session store、api client、登录态、workspaceId 和可交互上下文。\n  所以 ui-sdk 只提供 registerToolRenderer 扩展点，真正的 renderer 由 web app 注册。"
        },
        {
          "title": "propose renderer 在共享页和 workspace 页的区别",
          "code": "workspace chat 页面：\n  <AgentSessionProvider> 存在\n  useHasAgentSessionProvider() === true\n  渲染 AgentRequestCardInteractive\n  可以 fetch agent_request\n  可以 approve/reject\n  可以 actions.sendMessage(...) 让 agent 继续 apply\n\npublic share 页面：\n  没有 AgentSessionProvider\n  useHasAgentSessionProvider() === false\n  渲染 AgentRequestCardStatic\n  只能展示 envelope 里的 payload/status\n  不能调用 session actions\n  不能替用户 approve/reject\n\n这也是 renderer 放在 web app 里的原因之一：\n同一个 tool result，在不同页面上下文下要选择不同 UI 行为。"
        },
        {
          "title": "Codex MCP tool 被 execute 包装时，renderer 如何找到真实工具",
          "code": "// Codex transcript 里的 tool call 可能长这样\nconst tool = {\n  id: 'call_01',\n  name: 'execute',\n  input: {\n    server: 'platform',\n    tool: 'read_memory',\n    arguments: {\n      store_id: 'mem_project',\n      path: '/notes/overview.md'\n    }\n  },\n  result: '...'\n}\n\n// 如果直接用 tool.name 查 renderer：\ndisplayName = 'execute'\ntoolRenderers['execute']        // 找不到 readMemoryRenderer\n=> 只能 fallback/default 展示\n\n// 正确流程：\neffectiveName = unwrapExecuteDispatchName(tool.name, tool.input)\n// effectiveName = 'read_memory'\n\ndisplayName = getToolDisplayName(effectiveName)\n// displayName = 'read_memory'\n\nrenderer = resolveRenderer(tool, agentType)\n// 命中 registry.ts 里的 read_memory: readMemoryRenderer\n\n最终 UI：\n  ToolCallBlock 展示 read_memory 的专用卡片，而不是 execute 的通用 JSON。"
        },
        {
          "title": "unwrapExecuteDispatchName 的判断条件",
          "code": "export function unwrapExecuteDispatchName(name, input) {\n  // 不是 Codex execute 包装，保持原名\n  if (name !== 'execute' || typeof input !== 'object' || input === null) return name\n\n  const record = input\n\n  // 必须同时满足：\n  // 1. input.tool 是字符串：真实工具名\n  // 2. input.server 存在：说明这是 MCP server/tool 调用\n  if (typeof record.tool === 'string' && record.server) {\n    return record.tool\n  }\n\n  return name\n}\n\n为什么还要判断 server？\n  避免把普通 execute 调用误判成 MCP tool。\n  只有 input 同时带 server + tool，才认为这是 Codex MCP dispatch wrapper。"
        },
        {
          "title": "另一个 Codex MCP 示例：browser tool",
          "code": "const tool = {\n  name: 'execute',\n  input: {\n    server: 'platform',\n    tool: 'create_browser',\n    arguments: {\n      url: 'https://example.com'\n    }\n  }\n}\n\nresolveRenderer 过程：\n  name = 'execute'\n  input.tool = 'create_browser'\n  unwrapExecuteDispatchName(...) -> 'create_browser'\n  getToolDisplayName('create_browser') -> 'create_browser'\n  toolRenderers['create_browser'] -> createBrowserRenderer\n\n如果没有 unwrap：\n  页面上只能看到 execute + JSON 参数。\n\n有 unwrap：\n  页面上看到 Browser 专用 tool card。"
        }
      ],
      "takeaways": [
        "WorkspaceChatPanel 只负责消息列表容器和输入交互；真正的消息内容渲染在 ui-sdk。",
        "ToolRenderer 的选择依赖 tool.name、tool.input 和 agentType。Codex 的 MCP tool 经常包在 execute 里，所以必须通过 input.tool 才能找到真实 renderer。",
        "如果你要新增一个 tool 的展示样式，优先找 registry 里有没有内置 renderer；如果它需要 web app 的业务状态，就像 builder-mode propose 一样在 web/src/plugins/index.ts 注册。",
        "判断 renderer 应该放在哪里，可以看它是否只依赖 tool.input/tool.result。如果只读展示，适合放 ui-sdk；如果要读写 workspace/session/api 状态，应该由 web app 注册。",
        "Codex 的 execute 不是最终工具名，它更像一个 MCP 调度外壳。真正用于匹配 renderer 的名字通常在 input.tool。"
      ]
    },
    {
      "id": "tool-call-block-memo-lazy-registry",
      "title": "ToolCallBlock：memo 和 lazy renderer 订阅为什么这么写",
      "summary": "这篇单独解释 ToolCallBlock 里几个容易混淆的点：为什么导出组件时用 memo，而不是 useCallback；为什么同一个组件要同时订阅 lazy descriptor registry 和 tool renderer registry；以及 Collapsible/CollapsibleTrigger/CollapsibleContent 是怎么让整张 tool 卡片展开收起的。核心结论是：memo 缓存组件渲染，useMemo 缓存计算值，useCallback 缓存函数引用；两个 registry 分别代表“知道该加载哪个插件”和“插件已经注册了真正 renderer”；Trigger 里的 button 没有显式 onClick，点击逻辑由 Radix Trigger 通过 asChild 注入。",
      "sourceRefs": [
        "internal/ui-sdk/src/components/ToolCallBlock.tsx:41-143：ToolCallBlockImpl、preview useMemo、两个 useSyncExternalStore、memo 导出。",
        "internal/ui-sdk/src/tool-renderers/plugin-registry.ts:12-40：registerToolRenderer 写入 renderer registry，并通过 version/listener 通知 UI。",
        "internal/ui-sdk/src/lazy.tsx:1-40：LazyToolRenderersProvider 暴露 getPluginId/load/subscribe/getVersion 给 ui-sdk。",
        "internal/ui-sdk/src/ui/collapsible.tsx:1-9：Collapsible/CollapsibleTrigger/CollapsibleContent 是 Radix Collapsible primitive 的再导出。",
        "internal/ui-sdk/src/components/ToolCallBlock.tsx:68-137：Collapsible 受控 open/onOpenChange，Trigger 使用 asChild 包住 button，Content 承载展开内容。",
        "web/src/components/chat/TranscriptProviders.tsx:21-43：web app 把 lazy registry 和 ensurePluginLoaded 注入 ui-sdk。",
        "web/src/lib/plugin-lazy-registry.ts:21-70：lazy descriptor registry，保存 toolName -> pluginId，并提供 subscribe/getVersion。",
        "web/src/lib/plugin-loader.ts:129-201：启动加载插件 manifest；lazy 插件只注册 descriptor，真正 bundle 按需加载。",
        "web/src/contexts/AuthContext.tsx:24-35：auth 成功后重试 loadExternalPlugins，覆盖启动时 /api/plugins 401 的场景。"
      ],
      "bullets": [
        "ToolCallBlock 是一张独立 tool 卡片，MessageBubble 会在消息里渲染很多张。memo(ToolCallBlockImpl) 让父组件重渲染但 tool prop 引用没变时，React 可以跳过这张卡片的重新渲染。",
        "useCallback 不是这里要解决的问题。useCallback 缓存函数引用，常用于把稳定回调传给 memo 子组件；ToolCallBlock 这里直接把 setExpanded 传给 Collapsible，React 的 state setter 本来就是稳定引用，不需要额外包一层。",
        "useMemo 和 memo 也不是一回事。memo 包组件，决定组件要不要重新 render；useMemo 在组件内部缓存某个计算结果。ToolCallBlock 里的 preview 用 useMemo，是为了 renderer/tool 不变时不重复调用 getPreview 或 jsonPreview。",
        "lazy renderer 有两个阶段：第一阶段是 lazy descriptor registry 里出现 toolName -> pluginId，表示这个工具属于某个 lazy plugin，可以开始加载 bundle；第二阶段是 bundle 加载后调用 registerToolRenderer，renderer registry 里才出现真正的 rendererDef。",
        "订阅 lazy registry 是为了处理 descriptor 迟到。启动时 loadExternalPlugins 可能早于有效登录态，/api/plugins 可能 401；AuthContext 拿到 user 后重试，descriptor 才补注册。已经渲染出来的 ToolCallBlock 如果不订阅这个变化，就不会重新计算 lazyPluginId，也不会触发 ensurePluginLoaded。",
        "订阅 renderer registry 是为了处理 bundle 加载完成。ensurePluginLoaded 只是注入脚本；脚本执行后调用 registerToolRenderer。ToolCallBlock 必须在这个 registry 变化时重新 resolveRenderer，才能从 skeleton/default 切到真正的 renderInput/renderResult。",
        "Collapsible 这组三个组件来自 Radix：Root 管 open 状态，Trigger 负责响应用户点击并请求切换 open，Content 是展开后显示的区域。ToolCallBlock 把 open 受控到本地 expanded state：<Collapsible open={expanded} onOpenChange={setExpanded}>。",
        "CollapsibleTrigger 里的 button 没有手写 onClick，但不是不能点击。因为用了 asChild，Radix 不会额外包一层 button，而是把 Trigger 需要的 props、aria 属性、data-state、ref 和点击/键盘处理合并到这个子 button 上。用户点击 button 后，Radix Trigger 内部调用 Root 的 onOpenChange，最终执行 setExpanded(nextOpen)。",
        "ToolCallBlock 又额外写了 {expanded && <CollapsibleContent forceMount>...}。所以点击后的渲染变化有两层：Radix 更新受控 open；React 的 expanded state 变了以后，按钮图标从 ChevronRight/ChevronDown 切换，Content 区域也被条件渲染出来或移除。"
      ],
      "codeBlocks": [
        {
          "title": "memo、useMemo、useCallback 各自缓存什么",
          "code": "export const ToolCallBlock = memo(ToolCallBlockImpl)\n// memo 缓存组件级别的 render 判断：props 没变时跳过这张 ToolCallBlock 的 render。\n\nconst preview = useMemo(\n  () => renderer?.getPreview(tool) ?? jsonPreview(tool.input),\n  [renderer, tool],\n)\n// useMemo 缓存值：renderer/tool 没变时不重复算 preview。\n\n// useCallback 缓存函数引用。这里通常不需要：\nconst onOpenChange = useCallback((open) => setExpanded(open), [])\n// 因为 setExpanded 本身已经稳定，直接写 onOpenChange={setExpanded} 就可以。"
        },
        {
          "title": "两个 registry 分别表示什么",
          "code": "// lazy descriptor registry：知道谁应该负责这个 tool\ntoolName -> pluginId\n\n// renderer registry：知道具体怎么渲染这个 tool\ntoolName -> rendererDef\n\n// ToolCallBlock 先问同步 renderer：\nconst renderer = resolveRenderer(tool, agentType)\n\n// 没有 renderer 时，再问 lazy descriptor：\nconst lazyPluginId = renderer ? null : lazy?.getPluginId(displayName)\n\n// 有 lazyPluginId 才能启动加载：\nuseEffect(() => {\n  if (lazyPluginId) lazy.load(lazyPluginId)\n}, [lazyPluginId, lazy])"
        },
        {
          "title": "为什么两个 useSyncExternalStore 都需要",
          "code": "// 1. descriptor 迟到时触发重新判断是否该 load plugin\nuseSyncExternalStore(lazy?.subscribe ?? NOOP_SUBSCRIBE, lazy?.getVersion ?? ZERO)\n\n// 没有它：\n// /api/plugins 后来成功 -> lazy registry 有了 toolName -> pluginId\n// 但已经渲染的 ToolCallBlock 不会重新 render\n// lazyPluginId 仍然是 null，ensurePluginLoaded 不会被调用。\n\n// 2. renderer 注册后触发重新选择 renderer\nuseSyncExternalStore(subscribeToolRenderers, getToolRenderersVersion)\n\n// 没有它：\n// ensurePluginLoaded 成功 -> bundle 调 registerToolRenderer\n// 但 ToolCallBlock 不会重新 render\n// UI 可能停在 skeleton/default，直到父组件因别的原因刷新。"
        },
        {
          "title": "Collapsible 点击展开是怎么触发的",
          "code": "const [expanded, setExpanded] = useState(renderer?.defaultExpanded ?? false)\n\n<Collapsible open={expanded} onOpenChange={setExpanded}>\n  <CollapsibleTrigger asChild>\n    <button type=\"button\">\n      {expanded ? <ChevronDown /> : <ChevronRight />}\n      ...tool header...\n    </button>\n  </CollapsibleTrigger>\n\n  {expanded && (\n    <CollapsibleContent forceMount>\n      ...renderer input/result...\n    </CollapsibleContent>\n  )}\n</Collapsible>\n\n// button 上没有显式 onClick。\n// 点击处理来自 Radix CollapsibleTrigger。\n// asChild 的意思是：不要生成额外 DOM，把 Trigger 的行为注入到子元素 button 上。\n\n用户点击 button\n  -> Radix Trigger 的内部 click handler 执行\n  -> Root 计算 nextOpen\n  -> 调用 onOpenChange(nextOpen)\n  -> setExpanded(nextOpen)\n  -> ToolCallBlock re-render\n  -> 图标和 Content 展开状态更新"
        },
        {
          "title": "完整时序",
          "code": "启动时：\n  main.tsx -> loadExternalPlugins()\n  如果 /api/plugins 成功：注册 lazy descriptor\n  如果 401：暂时没有 lazy descriptor\n\nReact 已经渲染聊天：\n  ToolCallBlock resolveRenderer 找不到同步 renderer\n  getPluginId(toolName) 也可能暂时 undefined\n\n登录态确认后：\n  AuthContext -> loadExternalPlugins() 重试\n  registerLazyToolRenderer(toolName, pluginId)\n  lazy registry version++ -> ToolCallBlock 重新 render\n  算出 lazyPluginId -> ensurePluginLoaded(pluginId)\n\nbundle 加载后：\n  plugin 调 registerToolRenderer(toolName, rendererDef)\n  renderer registry version++ -> ToolCallBlock 重新 render\n  resolveRenderer 命中 rendererDef\n  skeleton/default -> renderInput/renderResult"
        }
      ],
      "takeaways": [
        "memo 是组件级优化；useMemo 是值级优化；useCallback 是函数引用优化。ToolCallBlock 需要的是 memo 和 useMemo，不需要为了 setExpanded 写 useCallback。",
        "lazy.subscribe 负责让组件知道“现在可以加载哪个 plugin 了”；subscribeToolRenderers 负责让组件知道“真正 renderer 已经注册好了”。",
        "这不是重复订阅，而是两个不同阶段的外部状态。一个触发加载，一个触发加载完成后的 UI 切换。",
        "CollapsibleTrigger 里的 button 是真正被点击的 DOM，但点击事件不是 ToolCallBlock 手写的；Radix 通过 asChild 把 Trigger 行为合并到这个 button 上，再通过 Collapsible Root 的 onOpenChange 调到 setExpanded。"
      ]
    },
    {
      "id": "workspace-file-link-flow",
      "title": "Streamdown：agent 文本、Markdown 解析和文件链接组件",
      "summary": "这篇重新从一个完整例子讲起：agent 为什么会输出 Markdown，为什么文本里可能带 /tmp/skill-*，Web 如何用 Streamdown 把 assistant text 解析成 React 节点，defaultRehypePlugins 在中间做了什么，components 里的 agent-sys、platform-cmd、a、img 又分别在什么时候被调用。最后再逐个拆开每个 component 的处理流程，跨服务路径用时序图表示。",
      "sourceRefs": [
        "internal/ui-sdk/src/components/MessageBubble.tsx:59-136：按 message role/block 决定用户消息、agent-sys 自动消息、assistant text block 如何进入 Markdown。",
        "web/src/components/ui/markdown.tsx:1-24：引入 Streamdown、rehypeRaw、rehypeKatex、rehypeSanitize、rehypeHarden，以及文件链接相关 hook。",
        "web/src/components/ui/markdown.tsx:26-56：WorkspaceFileLink 覆盖 Markdown a 标签，命中文件链接时调用 useWorkspaceFileLink。",
        "web/src/components/ui/markdown.tsx:63-97：WorkspaceFileImg 覆盖 Markdown img 标签，把 workspace/afs/tmp skill 图片改写成 agent-files URL。",
        "web/src/components/ui/markdown.tsx:104-132：sysSanitizeSchema 和 defaultRehypePlugins，决定哪些 HTML 标签保留、如何 sanitize、如何 harden link/image。",
        "web/src/components/ui/markdown.tsx:134-158：AgentSysBlock 负责渲染 <agent-sys> 自动系统消息。",
        "web/src/components/ui/markdown.tsx:167-194：Markdown 组件 memo 化，注册 components，并把 rehypePlugins 传给 Streamdown。",
        "web/src/components/ui/platform-cmd.tsx:1-121：PlatformCmd 解析 <platform-cmd> 的 YAML body，按 OS 展示不同命令。",
        "web/src/hooks/useWorkspaceFileLink.ts:58-111：解析 href，构造 onLinkClick/onPopoutClick，并驱动 Files app 状态。",
        "web/src/hooks/useSkillsBasePath.ts:1-31：仅在 /tmp/skill-* 场景请求 /_proxy/agent/:workspaceId/skills，并缓存 filesBrowsePath。",
        "web/src/lib/workspace-file-link.ts:20-105：识别 /tmp/skill-*，canonicalizeAgentPath 改写到 /workspace<skillsBasePath>/...，再解析 drive/filePath/line/column。",
        "web/src/lib/api/agent-files.ts:1-89：fileUrl/dirListUrl 根据 drive 生成 control-plane agent-files/dirs API。",
        "control-plane/src/routes/proxy.ts:41-116：/_proxy/agent/:workspaceId/* 校验 workspace/running 后转发到 agent pod。",
        "control-plane/src/routes/workspaces/agent.ts:111-139,490-515：registerFileRoutes 把 workspace/afs drive 映射到 agent pod 的 /files 或 /afs-files dufs 路径。",
        "internal/agent-skills/src/index.ts:209-224,314-457,605-653：SkillManager 把 skill 下载并解压到 /tmp/skill-{name}，再 symlink 到 workspace skillsDir。",
        "internal/agent-skills/src/routes.ts:37-48：agent GET /skills 返回 filesBrowsePath，供 Web 改写 /tmp/skill-*。"
      ],
      "bullets": [
        "先分清三个层次：agent 输出的是 assistant text；Markdown 解析发生在 Web 渲染阶段；/tmp/skill-* 是 agent 容器里的真实 skill 解压路径，不是浏览器能直接打开的路径。",
        "agent 什么时候“输出 Markdown”：不是调用一个专门的 Markdown API，而是普通 assistant text 里包含 Markdown 语法，例如 [标题](href)、![alt](src)、HTML tag、code fence。MessageBubble 把 assistant text block 交给 Markdown 组件，Streamdown 才把这些语法解析成 React 节点。",
        "agent 什么时候输出 /tmp/ 开头内容：当它引用运行环境里看到的真实路径时。skill package 默认解压在 /tmp/skill-{name}，agent 读 SKILL.md、引用 assets、打印命令输出、报错栈带路径，或把刚看到的路径写进回答，都可能出现 /tmp/skill-*。",
        "Streamdown 的职责是把 children 字符串变成 React 渲染树。它先按 Markdown/HTML/数学公式/代码块等语法解析，再经过 rehype 插件处理 HTML AST，最后根据 components 映射决定某些标签用哪个 React component 渲染。",
        "约定来源要分清：Markdown 文件/图片/path 输出来自 agent prompt 或 __platform__ skill；agent-sys 是 Web 业务自动消息约定；platform-cmd 是 Web inline help 文档约定。",
        "defaultRehypePlugins 是这层 Markdown 渲染的安全和兼容边界：rehypeRaw 允许 Markdown 里的原始 HTML 进入 AST；rehypeKatex 处理数学公式；rehypeSanitize 用定制 schema 清洗 HTML 并允许 agent-sys/platform-cmd；rehype-harden 统一加固链接和图片协议。",
        "components 不是主动运行的列表。它是传给 Streamdown 的 tag 到 React component 映射表。解析结果里出现 <agent-sys>，就调用 AgentSysBlock；出现 <platform-cmd>，就调用 PlatformCmd；出现 a/img 且 linkifyWorkspaceFiles=true，就调用 WorkspaceFileLink/WorkspaceFileImg。",
        "MessageBubble 里 assistant text block 总是以 linkifyWorkspaceFiles=true 渲染，所以 assistant 输出的 /workspace、/mnt/afs、/tmp/skill-* Markdown 链接会变成 Files 面板入口。用户普通蓝色气泡不是 Markdown；只有整个 user message 包成 <agent-sys> 时才走 Markdown。"
      ],
      "codeBlocks": [
        {
          "title": "总览：一个示例贯穿整条链路",
          "code": "示例背景：\n  workspace 启用了 report-writer skill。\n  agent 启动时下载 skill package，并解压到：\n    /tmp/skill-report-writer\n  同时创建 workspace 可浏览软链：\n    /.home/.codex/skills/report-writer -> /tmp/skill-report-writer\n\nagent 本轮回答输出 assistant text：\n  我参考了 [report-writer 的说明](/tmp/skill-report-writer/SKILL.md:12)。\n\n注意：\n  agent 只是输出一段 text。\n  text 里恰好有 Markdown 链接语法。\n  href 里恰好是 agent 运行时看到的真实 /tmp skill 路径。\n\nWeb 端处理：\n  SSE delta -> AgentSessionStore -> MessageBubble -> Markdown -> Streamdown\n  Streamdown 解析出 a 标签。\n  因为 components.a 被注册成 WorkspaceFileLink，a 标签不再走浏览器默认跳转。\n  WorkspaceFileLink 调 useWorkspaceFileLink，把 /tmp/skill-report-writer/SKILL.md:12 改写成：\n    /workspace/.home/.codex/skills/report-writer/SKILL.md:12\n  用户点击后，Files 面板打开 workspace 软链路径。\n  dufs 跟随软链，最终读到 /tmp/skill-report-writer/SKILL.md。"
        },
        {
          "title": "流程图：assistant text 如何进入 Streamdown",
          "code": "flowchart TD\n  A[Agent emits assistant text] --> B[SSE delta arrives]\n  B --> C[AgentSessionStore appends text block]\n  C --> D[MessageBubble renders message.blocks]\n  D --> E[Text block becomes Markdown component]\n  E --> F[Streamdown parses Markdown and raw HTML]\n  F --> G[defaultRehypePlugins transform and sanitize AST]\n  G --> H[Streamdown renders React nodes]\n  H --> I[components map replaces selected tags]\n  I --> J[agent-sys/platform-cmd/a/img custom behavior]"
        },
        {
          "title": "跨服务时序图：/tmp/skill-* 链接被点击",
          "code": "sequenceDiagram\n  participant AG as Agent\n  participant SSE as Chat SSE\n  participant Store as AgentSessionStore\n  participant MB as MessageBubble\n  participant SD as Streamdown\n  participant Link as WorkspaceFileLink\n  participant Hook as useWorkspaceFileLink\n  participant Base as useSkillsBasePath\n  participant CP as control-plane proxy\n  participant Pod as agent /skills\n  participant Files as Files app\n  AG->>SSE: text delta with [open](/tmp/skill-report-writer/SKILL.md:12)\n  SSE->>Store: append assistant text block\n  Store->>MB: render message\n  MB->>SD: Markdown children + linkifyWorkspaceFiles=true\n  SD->>Link: render a via components.a\n  Link->>Hook: useWorkspaceFileLink(href)\n  Hook->>Base: isSkillTmpPath=true\n  Base->>CP: GET /_proxy/agent/ws_123/skills\n  CP->>Pod: GET {agentAddress}/skills\n  Pod-->>CP: 200 { filesBrowsePath }\n  CP-->>Base: 200 { filesBrowsePath }\n  Base-->>Hook: /.home/.codex/skills\n  Hook-->>Link: handlers for Files navigation\n  Link->>Files: user click -> set instance state + activate"
        },
        {
          "title": "defaultRehypePlugins 的作用",
          "code": "const defaultRehypePlugins = [\n  rehypeRaw,\n  [rehypeKatex, { errorColor: \"var(--color-muted-foreground)\" }],\n  [rehypeSanitize, sysSanitizeSchema],\n  [harden, {\n    allowedImagePrefixes: [\"*\"],\n    allowedLinkPrefixes: [\"*\"],\n    allowedProtocols: [\"*\"],\n    defaultOrigin: undefined,\n    allowDataImages: true,\n  }],\n]\n\n执行意义：\n  1. rehypeRaw\n     Markdown 里如果有 <agent-sys> 或 <platform-cmd> 这种 HTML tag，先让它进入 rehype AST。\n     没有它，原始 HTML 可能只会被当文本或被丢掉，后续 components 就没有机会接管。\n\n  2. rehypeKatex\n     处理数学公式相关节点，让公式能按 KaTeX 样式渲染。\n\n  3. rehypeSanitize + sysSanitizeSchema\n     清洗不安全 HTML。\n     同时把 agent-sys、platform-cmd 加进允许标签，否则 sanitize 会把这些自定义标签删掉，components 也不会被调用。\n\n  4. rehype-harden\n     对链接、图片、协议做统一加固，避免 Markdown 里的链接/图片变成不受控的危险输出。\n\n关键点：\n  rehype plugins 处理的是 HTML AST。\n  components 处理的是最终 React 渲染时某个 tag 应该用哪个 React component。"
        },
        {
          "title": "components 是如何被调用的",
          "code": "const components = {\n  \"agent-sys\": AgentSysBlock,\n  \"platform-cmd\": PlatformCmd,\n  ...(linkifyWorkspaceFiles ? {\n    a: WorkspaceFileLink,\n    img: WorkspaceFileImg,\n  } : {}),\n}\n\n调用规则：\n  Markdown/HTML 解析结果里出现普通段落 -> Streamdown 默认渲染 p。\n  出现 <agent-sys>...</agent-sys> -> 调 AgentSysBlock({ children })。\n  出现 <platform-cmd>...</platform-cmd> -> 调 PlatformCmd({ children })。\n  出现 [text](href) -> AST 里是 a 标签。\n    linkifyWorkspaceFiles=false: Streamdown 默认 a。\n    linkifyWorkspaceFiles=true: WorkspaceFileLink({ href, children, ...props })。\n  出现 ![alt](src) -> AST 里是 img 标签。\n    linkifyWorkspaceFiles=true: WorkspaceFileImg({ src, alt, ...props })。\n\n所以 components 不是手动遍历 Markdown 字符串。\n它是 Streamdown 在渲染 AST 节点时查表调用的 React component 映射。"
        },
        {
          "title": "约束层：系统如何让 agent 输出 Markdown 和特定 HTML 标签",
          "code": "结论：这里只能说有约定，不能说有统一强制协议。\n\n1. Markdown 输出约定在 agent 侧。\n   platform prompt 和 __platform__ skill 会告诉 agent 平台路径、能力和部分输出格式。\n   具体例子：image-generation.md 要求生成图片后用 Markdown 图片语法：\n   ![generated](/workspace/.home/.codex/generated_images/<filename>)\n\n2. agent-sys 是 Web 业务消息约定。\n   不是找到 agent prompt 要求 agent 自己输出 agent-sys。\n   代码里是 agent-request approve/reject 后，Web 主动 sendMessage 包一层 agent-sys。\n   Markdown 渲染层只是放行并注册 AgentSysBlock。\n\n3. platform-cmd 是 Web 文档约定。\n   它出现在 web/src/docs/inline-help/*/skill-upload.md。\n   没找到 agent prompt 要求 agent 输出 platform-cmd。\n   Markdown 渲染层只是放行并注册 PlatformCmd。\n\n所以：\n   agent 侧约束：Markdown 文件/图片/path 这类输出格式。\n   Web 侧约定：agent-sys、platform-cmd 这类自定义 HTML 标签如何渲染。"
        },
        {
          "title": "流程图：自定义 HTML 标签从约定到组件调用",
          "code": "flowchart TD\n  A[Platform prompt or tool contract tells agent allowed markup] --> B[Agent emits assistant text with custom tag]\n  B --> C[MessageBubble passes text to Markdown]\n  C --> D[Streamdown parses Markdown]\n  D --> E[rehypeRaw turns raw HTML into AST nodes]\n  E --> F[rehypeSanitize checks sysSanitizeSchema]\n  F -->|tag allowed| G[Node survives]\n  F -->|tag not allowed| H[Node removed or escaped]\n  G --> I[Streamdown renders node]\n  I --> J[components map lookup by tag name]\n  J --> K[Call AgentSysBlock or PlatformCmd]"
        },
        {
          "title": "组件一：Markdown 根组件",
          "code": "输入：\n  children: assistant/user 自动消息文本\n  mode: streaming 或 static\n  linkifyWorkspaceFiles: 是否把 a/img 接到 Files 体系\n  rehypePlugins: 可覆盖默认插件栈\n\n处理：\n  1. useMemo 生成 components 映射。\n  2. 永远注册 agent-sys 和 platform-cmd。\n  3. linkifyWorkspaceFiles=true 时额外注册 a/img。\n  4. 渲染 Streamdown，把 children、mode、components、rehypePlugins 交进去。\n\n输出：\n  一棵 React 节点树。\n  其中自定义标签/链接/图片会被替换成业务组件。"
        },
        {
          "title": "组件二：AgentSysBlock",
          "code": "触发输入：\n  Markdown/HTML 中出现：\n    <agent-sys>自动消息内容</agent-sys>\n\n为什么能保留：\n  sysSanitizeSchema 把 agent-sys 加入允许 tag。\n  否则 rehypeSanitize 会把这个自定义 tag 清掉。\n\n处理：\n  1. useState(false) 保存 open。\n  2. 渲染一个中性样式 disclosure。\n  3. 点击 button 时 setOpen 取反。\n  4. open=true 时显示 children。\n\n输出：\n  用户看到的是“系统自动消息”的折叠块，不会误以为这是用户手敲的蓝色消息。"
        },
        {
          "title": "组件三：PlatformCmd",
          "code": "触发输入：\n  <platform-cmd>\n  macos: |\n    tar -czf skill.tar.gz -C /path/to/skill-dir .\n  linux: |\n    tar -czf skill.tar.gz -C /path/to/skill-dir .\n  windows: |\n    tar -czf skill.tar.gz -C C:\\\\path\\\\to\\\\skill-dir .\n  </platform-cmd>\n\n处理：\n  1. flattenChildrenToText 把 Streamdown 传进来的 children 压平成 YAML 字符串。\n  2. yaml.parse 解析 macos/linux/windows 三个字段。\n  3. loadPref 从 localStorage 读取用户上次选择；没有就 detectPlatform。\n  4. SegmentedControl 允许用户切换 OS。\n  5. 切换结果写回 localStorage。\n  6. 当前 OS 对应命令用 code 块展示。\n\n失败兜底：\n  YAML 解析失败时，不抛错，直接用 pre 展示原始文本。"
        },
        {
          "title": "组件四：WorkspaceFileLink",
          "code": "触发输入：\n  Markdown 链接：\n    [打开](/workspace/src/App.tsx:42)\n    [打开 AFS](/mnt/afs/share/a.md)\n    [打开 skill](/tmp/skill-report-writer/SKILL.md:12)\n\n处理：\n  1. 收到 Streamdown 传入的 href/children/props。\n  2. 调 useWorkspaceFileLink(href)。\n  3. 如果 hook 返回 null，就渲染普通外链 <a target=_blank>。\n  4. 如果 hook 返回 handlers，就渲染内联链接 + 外部打开按钮。\n  5. 点击文字链接时 preventDefault，然后 handlers.onLinkClick()。\n  6. 点击小图标时 stopPropagation，然后 handlers.onPopoutClick()。\n\n输出：\n  文件链接不离开当前页面，而是驱动 Files app 或 FileApp popout。"
        },
        {
          "title": "跨服务时序图：WorkspaceFileLink 点击文件",
          "code": "sequenceDiagram\n  participant User as User\n  participant Link as WorkspaceFileLink\n  participant Hook as useWorkspaceFileLink\n  participant API as dirListUrl fetch\n  participant CP as control-plane agent file route\n  participant Dufs as agent dufs\n  participant State as instance-state-store\n  participant Slot as SlotContext\n  participant Files as Files app\n  User->>Link: click Markdown link\n  Link->>Hook: handlers.onLinkClick()\n  Hook->>API: probeIsDir via dirListUrl\n  API->>CP: GET /api/workspaces/ws/agent/dirs?path=...\n  CP->>Dufs: GET {agentAddress}/files/.../?json\n  Dufs-->>CP: 200 listing or 404/not directory\n  CP-->>Hook: resp.ok true or false\n  Hook->>State: set viewingPath/viewingLine/viewingColumn/drive\n  Hook->>Slot: activate files instance\n  Slot-->>Files: Files panel renders target path"
        },
        {
          "title": "组件五：WorkspaceFileImg",
          "code": "触发输入：\n  Markdown 图片：\n    ![preview](/workspace/.home/foo.png)\n    ![preview](/mnt/afs/share/foo.png)\n    ![preview](/tmp/skill-report-writer/assets/preview.png)\n\n处理：\n  1. 从 useParams 取 workspaceId。\n  2. 如果 src 是 /tmp/skill-*，useSkillsBasePath 查询 filesBrowsePath。\n  3. canonicalizeAgentPath 把 /tmp/skill-report-writer/... 改成 /workspace{filesBrowsePath}/report-writer/...。\n  4. 根据 /workspace 或 /mnt/afs 判断 drive。\n  5. 用 fileUrl(workspaceId, path, drive) 生成认证过的图片 URL。\n  6. 返回 <img src={fileUrl(...)} />。\n\n不同点：\n  图片没有点击 handler。\n  浏览器加载 img.src 时才请求 agent-files endpoint 拉图片 bytes。"
        },
        {
          "title": "跨服务时序图：WorkspaceFileImg 加载 /tmp skill 图片",
          "code": "sequenceDiagram\n  participant SD as Streamdown\n  participant Img as WorkspaceFileImg\n  participant Base as useSkillsBasePath\n  participant Proxy as control-plane proxy\n  participant Skills as agent /skills\n  participant Browser as Browser img loader\n  participant CP as control-plane agent file route\n  participant Dufs as agent dufs\n  SD->>Img: render img src=/tmp/skill-report-writer/assets/preview.png\n  Img->>Base: isSkillTmpPath=true\n  Base->>Proxy: GET /_proxy/agent/ws/skills\n  Proxy->>Skills: GET {agentAddress}/skills\n  Skills-->>Proxy: { filesBrowsePath }\n  Proxy-->>Base: { filesBrowsePath }\n  Img-->>Browser: <img src=/api/workspaces/ws/agent/files?path=...>\n  Browser->>CP: GET /api/workspaces/ws/agent/files?path=...\n  CP->>Dufs: GET {agentAddress}/files/.home/.codex/skills/report-writer/assets/preview.png\n  Dufs-->>CP: image bytes\n  CP-->>Browser: image bytes"
        }
      ],
      "takeaways": [
        "agent 是否“输出 Markdown”取决于文本里有没有 Markdown/HTML 语法；真正解析发生在 Web 的 Streamdown 渲染阶段。",
        "/tmp/skill-* 来自 agent 运行环境里的 skill 解压目录。它常出现在 agent 引用 skill 文件、图片、命令输出、日志或错误栈时。",
        "defaultRehypePlugins 决定 HTML/公式/安全清洗/链接加固这条中间管线；如果 sanitize schema 不允许自定义 tag，components 就不会收到 agent-sys/platform-cmd。",
        "components 是 tag 到 React component 的映射。Streamdown 渲染到某个 tag 时查这个映射，命中后调用对应组件。",
        "a/img 只有在 linkifyWorkspaceFiles=true 时才被覆盖；这也是 assistant text 能打开 Files 面板，而普通用户蓝色气泡不参与文件链接解析的原因。",
        "跨服务链路只在需要真实文件信息时发生：/tmp skill 路径要查 /skills；点击文件要 probe dirs；图片加载要请求 agent-files。"
      ]
    },
    {
      "id": "chat-connect",
      "title": "Connect：Chat SSE 连接、断开与重连",
      "summary": "这篇回答三个问题：Web 前端如何判断 SSE 断开，刷新页面后什么时候应该重连；同一个 session 打开两个页面时是否都能收到消息；第一次会话结束、等待 10 分钟、再发第二次消息时，整个 chat 链路如何走。",
      "sourceRefs": [
        "web/src/lib/api/sse.ts:52-113：handlersToPlugin 把 SSE 事件和 runTurn 错误映射到前端 handlers。",
        "web/src/lib/api/sse.ts:115-134：consumeStream 使用 runTurn 消费 fetch 返回的 SSE response。",
        "web/src/lib/api/sse.ts:136-207：buildCPReconnectFactory，断线后最多 5 次 POST /_proxy/agent/:workspaceId/cp-reconnect。",
        "web/src/lib/api/sse.ts:211-267：createAgentChat，POST /api/workspaces/:id/chat 并消费 SSE。",
        "web/src/lib/api/sse.ts:269-306：createCPReconnectStream，刷新页面或切 session 后挂回正在运行的 turn。",
        "web/src/stores/agent-session-store.ts:209-245：startReconnect 创建一个前端 reconnect stream。",
        "web/src/stores/agent-session-store.ts:643-731：switchSession 先加载历史和 session 状态，chat_status=agent 才 startReconnect。",
        "web/src/stores/agent-session-store.ts:735-781：sendMessage 创建本地 user/assistant placeholder，然后 createAgentChat。",
        "web/src/stores/AgentSessionContext.tsx:101-117：刷新/首次进入页面时 initialSessionId 会触发 switchSession。",
        "control-plane/src/lib/sse.ts:25-61：ActiveSSEStream 维护 controllers、done、turnActive。",
        "control-plane/src/lib/sse.ts:153-207：setupActiveStream 按 workspaceId:sessionId 注册 active stream。",
        "control-plane/src/lib/sse.ts:225-234：emitSSE 写入所有 connected controllers。",
        "control-plane/src/lib/sse.ts:278-327：createReconnectSSEResponse，把 reconnect 客户端加入同一个 controllers Set。",
        "control-plane/src/lib/sse.ts:398-615：createInterceptedSSEResponse 拦截 agent SSE，负责持久化和广播。",
        "control-plane/src/routes/proxy.ts:20-35：POST /_proxy/agent/:workspaceId/cp-reconnect 返回 CP reconnect SSE。",
        "control-plane/src/services/chat/executeChat.ts:42-60：客户端断开不会杀死 agent turn，后续通过 cp-reconnect 续接。"
      ],
      "outline": [
        {
          "label": "先给结论",
          "target": "chat-connect-overview"
        },
        {
          "label": "前端如何判断断开",
          "target": "chat-connect-code-0"
        },
        {
          "label": "刷新页面是否重连",
          "target": "chat-connect-code-1"
        },
        {
          "label": "两个页面是否同时收到",
          "target": "chat-connect-code-2"
        },
        {
          "label": "其他 channel 的消息",
          "target": "chat-connect-code-3"
        },
        {
          "label": "10 分钟后第二次发送时序图",
          "target": "chat-connect-code-4"
        },
        {
          "label": "阅读结论",
          "target": "chat-connect-takeaways"
        }
      ],
      "bullets": [
        "前端不是用 EventSource，而是 fetch(POST /chat) 拿到 text/event-stream Response 后交给 runTurn 消费。",
        "断开判断主要来自 runTurn：如果没有 session.ended、没有 error event、也不是前端主动 abort，但流提前结束或读取失败，就进入 onError；如果还有 sessionId，会尝试 cp-reconnect。",
        "刷新页面不一定重连。刷新后 AgentSessionProvider 会重新创建 store，switchSession 先加载 DB 历史和 session 状态；只有 session.chat_status === agent，才认为这个 session 仍有正在运行的 turn，需要 startReconnect。",
        "如果 session 已经结束，比如 chat_status 已经是 human/idle，刷新页面只加载历史，不会再连 SSE。",
        "同一个 session 打开两个页面时，只要两个页面都连接到了同一个 active stream，它们会同时收到后续 SSE 事件。control-plane 的 ActiveSSEStream.controllers 是 Set，emitSSE 会写给所有 controller。",
        "其他 channel 的消息不会按“workspace 广播”自动塞给所有 web 页面。是否能收到取决于同一个 session 是否有 active stream，以及 web 是否通过 cp-reconnect attach 到这个 session。不同 session 的事件不会被当前页面接收；前端 handlers 里也会按 activeSessionId 过滤 evt.session_id。",
        "第一次发送结束后等 10 分钟再发第二次：中间没有长连接保持。第二次发送会重新 POST /api/workspaces/:id/chat，control-plane 再创建新的 active stream，前端再消费这次新的 SSE。"
      ],
      "codeBlocks": [
        {
          "title": "前端如何判断 SSE 已经断开",
          "code": "createAgentChat()\n  POST /api/workspaces/:workspaceId/chat\n  response = text/event-stream\n  consumeStream(response)\n    runTurn({ stream: response, reconnect, signal, idleTimeoutMs: 0 }, [web-handlers])\n\nhandlersToPlugin 里的关键规则：\n  1. 收到 session.ended\n     sessionEndedDispatched = true\n     onSessionEnded(...)\n     这表示正常结束，不当成断线错误。\n\n  2. 收到 error event\n     errorDispatched = true\n     onError(evt.message)\n     这表示服务端明确返回错误。\n\n  3. runTurn 自己发现读取失败 / 流提前结束\n     onError(err)\n     但如果 sessionEndedDispatched=true，忽略。\n     但如果 errorDispatched=true，忽略。\n     但如果 err.reason === 'aborted'，忽略。\n\n所以：\n  “断开”不是靠某个 isConnected state。\n  它是由 runTurn 消费 stream 的结果推导出来的：\n    没有正常 terminal event，却读不到流了 => 断开/异常。"
        },
        {
          "title": "断开后怎么重连，以及刷新页面是否应该重连",
          "code": "运行中断线：\n  createAgentChat 会传 reconnect factory 给 runTurn。\n  如果 primary SSE 在 session.ended 前断掉，runTurn 调 reconnect。\n\nreconnect 请求：\n  POST /_proxy/agent/:workspaceId/cp-reconnect?session_id=:activeSessionId\n\n前端最多尝试：\n  RECONNECT_MAX_ATTEMPTS = 5\n  delay: 500ms, 1000ms, 2000ms, 4000ms, 8000ms\n\n刷新页面：\n  WorkspacePage\n    useAutoSelectSession()\n      1. URL ?session= 优先\n      2. 否则选 session 列表第一条\n\n  AgentSessionProvider mount\n    store.switchSession(initialSessionId, initialContext)\n\n  switchSession(sessionId)\n    GET /api/workspaces/:wid/messages?session_id=:sid  // 加载历史\n    GET /api/workspaces/:wid/sessions/:sid             // 读取 chat_status\n\n    if chat_status === 'agent':\n      startReconnect()\n      POST /_proxy/agent/:wid/cp-reconnect?session_id=:sid\n    else:\n      不重连，只展示历史\n\n结论：\n  刷新后是否重连，看数据库 session.chat_status。\n  只有 agent 正在跑，才需要挂回 live SSE。"
        },
        {
          "title": "同一个 session 打开两个页面，会同时收到消息么",
          "code": "假设：\n  workspace_id = ws_1\n  session_id = s_100\n\nTab A：\n  POST /api/workspaces/ws_1/chat\n  CP createInterceptedSSEResponse()\n  activeStreams['ws_1:s_100'].controllers.add(controllerA)\n\nTab B：刷新或打开同一个 ?session=s_100\n  switchSession(s_100)\n  发现 chat_status === 'agent'\n  POST /_proxy/agent/ws_1/cp-reconnect?session_id=s_100\n  createReconnectSSEResponse()\n  activeStreams['ws_1:s_100'].controllers.add(controllerB)\n\nagent 后续输出一个 item.delta：\n  CP broadcast plugin\n    emitSSE(activeStream, rawEvent)\n      for controller of activeStream.controllers:\n        controller.enqueue(rawEvent)\n\n结果：\n  controllerA 收到\n  controllerB 收到\n\n所以：\n  是的，同一个 session 的两个页面可以同时收到后续消息。\n  前提是两个页面都 attach 到同一个 active stream。"
        },
        {
          "title": "如果同时存在其他 channel chat，web 会同时收到么",
          "code": "先分清两件事：\n\n1. source/channel 字段\n   web 发消息时：\n     body.source = 'web'\n   其他入口也可能调用同一套 dispatchChatTurn/executeChat，source 可能不是 web。\n   source 主要用于 session 审计/来源展示，不等于“广播频道”。\n\n2. SSE 广播边界\n   control-plane 的 activeStreams key 是：\n     workspaceId:sessionId\n   不是 workspaceId。\n\n因此：\n  其他 channel 如果发的是另一个 session：\n    当前 web 页面不会收到它的 SSE。\n    之后最多通过 session 列表刷新/历史加载看到那条 session 的结果。\n\n  其他 channel 如果续写的是同一个 session_id，且这个 session 正在 agent turn：\n    它会创建/替换这个 session 的 active stream。\n    attach 到这个 session 的 web 页面可以收到这个 session 的后续事件。\n\n  前端还有一层过滤：\n    handlersToPlugin 里会检查 evt.session_id。\n    如果 evt.session_id !== 当前 activeSessionId，会直接丢弃。\n\n一句话：\n  Web 不订阅“整个 workspace 的所有 channel 消息”。\n  Web 只看当前 active session。\n  同 session 才可能实时同收；不同 session 不会实时混进来。"
        },
        {
          "title": "时序图：第一次发送结束，等待 10 分钟，再发送第二次",
          "code": "参与者：\n  User\n  Web WorkspaceChatPanel\n  AgentSessionStore\n  CP /api/workspaces/:id/chat\n  CP activeStreams\n  Agent Pod\n  DB\n\n第一次发送：\n  User -> Web: 输入 “帮我读 README” 并点击发送\n  Web -> AgentSessionStore: actions.sendMessage(text)\n  AgentSessionStore:\n    追加本地 user message\n    追加本地 assistant placeholder\n    isLoading = true\n  AgentSessionStore -> CP /chat:\n    POST /api/workspaces/ws_1/chat\n    body = { message, session_id: null, source: 'web' }\n  CP /chat -> Agent Pod:\n    POST /chat\n    body = { message, source: 'web', session_token }\n  Agent Pod -> CP /chat:\n    SSE session.started(session_id=s_100)\n  CP /chat -> DB:\n    create sessions(s_100)\n    chat_status = 'agent'\n    insert user message\n  CP /chat -> CP activeStreams:\n    activeStreams['ws_1:s_100'] = ActiveSSEStream\n    controllers.add(webController)\n  CP activeStreams -> Web:\n    session.started(s_100)\n  Web -> AgentSessionStore:\n    activeSessionId = s_100\n    URL ?session=s_100\n\n流式输出：\n  Agent Pod -> CP /chat: item.started / item.delta / item.completed\n  CP /chat -> DB: persist assistant message/events\n  CP /chat -> CP activeStreams: emitSSE(rawEvent)\n  CP activeStreams -> Web: rawEvent\n  Web -> AgentSessionStore: 更新 messages blocks\n  Web -> User: MessageBubble 流式更新\n\n第一次结束：\n  Agent Pod -> CP /chat: session.ended(reason='completed')\n  CP /chat -> DB:\n    chat_status = 'human'\n    update stats / last_active_at\n  CP /chat -> CP activeStreams:\n    turnActive = false\n    emit session.ended after DB commit\n    stream done / cleanup\n  CP activeStreams -> Web: session.ended\n  Web -> AgentSessionStore:\n    isLoading = false\n    isBusy = false\n    lastTurnStats = stats\n\n等待 10 分钟：\n  没有 SSE 长连接需要保持。\n  页面只保留 React store 中的 messages。\n  DB 中 session s_100 的 chat_status 是 human。\n\n第二次发送：\n  User -> Web: 输入 “继续总结 package.json” 并点击发送\n  Web -> AgentSessionStore: actions.sendMessage(text)\n  AgentSessionStore:\n    追加新的 user message\n    追加新的 assistant placeholder\n    isLoading = true\n  AgentSessionStore -> CP /chat:\n    POST /api/workspaces/ws_1/chat\n    body = { message, session_id: 's_100', source: 'web' }\n  CP /chat -> DB:\n    先 transitionSessionStatus(s_100, 'agent')\n    先持久化本次 user message，防止刷新丢失\n  CP /chat -> Agent Pod:\n    POST /chat\n    body = { message, session_id: 's_100', source: 'web', session_token }\n  CP /chat -> CP activeStreams:\n    activeStreams['ws_1:s_100'] = 新的 ActiveSSEStream\n    controllers.add(webController)\n  Agent Pod -> CP -> DB/Web:\n    重复 item.started / item.delta / item.completed / session.ended 流程\n\n关键点：\n  第二次发送不是复用第一次的 SSE。\n  第一次 session.ended 后 SSE 已结束。\n  10 分钟后第二次发送会创建一个新的 HTTP POST + 新 SSE stream，但 session_id 仍是同一个 s_100。"
        }
      ],
      "takeaways": [
        "前端判断断线靠 runTurn 的消费结果，不是单独维护 WebSocket 那种 connected 标志。",
        "刷新后是否重连看 session.chat_status：agent 则 POST /cp-reconnect；human/idle 则只加载历史。",
        "同一个 session 的多个页面可以同时收到实时事件，因为 control-plane 对同一个 ActiveSSEStream 维护多个 controllers。",
        "Web 的实时边界是当前 active session，不是整个 workspace，也不是所有 channel。",
        "一次用户发送对应一次 /chat SSE。session 可以复用，但每一轮 turn 都是新的流。"
      ]
    },
    {
      "id": "ui-tooltip",
      "title": "Tooltip：Trigger 如何关联 Content",
      "summary": "这篇只解析 internal/ui-sdk/src/ui/tooltip.tsx 的封装关系。重点不是 Tooltip 的动画样式，而是理解：Trigger 和 Content 为什么写在同一个 Tooltip 下面就能互相配合。答案是 Radix Tooltip.Root 在内部提供共享上下文，Trigger 注册触发元素，Content 从同一个上下文读取 open 状态和触发元素位置。",
      "sourceRefs": [
        "internal/ui-sdk/src/ui/tooltip.tsx:1-3：引入 @radix-ui/react-tooltip、React 和 cn。",
        "internal/ui-sdk/src/ui/tooltip.tsx:5：TooltipProvider = TooltipPrimitive.Provider。",
        "internal/ui-sdk/src/ui/tooltip.tsx:7：Tooltip = TooltipPrimitive.Root。",
        "internal/ui-sdk/src/ui/tooltip.tsx:9：TooltipTrigger = TooltipPrimitive.Trigger。",
        "internal/ui-sdk/src/ui/tooltip.tsx:11-29：TooltipContent 用 forwardRef 包装 Radix Content，并合并 className 和 sideOffset。"
      ],
      "outline": [
        {
          "label": "这层封装做了什么",
          "target": "ui-tooltip-overview"
        },
        {
          "label": "源码对应关系",
          "target": "ui-tooltip-code-0"
        },
        {
          "label": "使用方式示例",
          "target": "ui-tooltip-code-1"
        },
        {
          "label": "极简实现：Root/Trigger/Content",
          "target": "ui-tooltip-code-2"
        },
        {
          "label": "Trigger 和 Content 如何关联",
          "target": "ui-tooltip-code-3"
        },
        {
          "label": "forwardRef 和 sideOffset",
          "target": "ui-tooltip-code-4"
        },
        {
          "label": "阅读结论",
          "target": "ui-tooltip-takeaways"
        }
      ],
      "bullets": [
        "这个文件没有自己实现 tooltip 的交互逻辑。真正的 hover、focus、delay、定位、ARIA 关联都在 @radix-ui/react-tooltip 里。",
        "ui-sdk 做的是二次封装：把 Radix 的 Provider、Root、Trigger 直接导出，再把 Content 包一层统一样式。",
        "Trigger 和 Content 能关联，不是靠变量名，也不是靠 DOM 相邻查找，而是因为它们都挂在同一个 TooltipPrimitive.Root 下面，共享同一个内部 context。",
        "TooltipProvider 是全局配置层，常用于统一 delayDuration、skipDelayDuration 等行为。Tooltip/Root 是单个 tooltip 实例。",
        "TooltipContent 用 React.forwardRef 是为了把外部 ref 继续传给 Radix Content，Radix 需要真实 DOM ref 来做定位、动画状态和可访问性处理。"
      ],
      "codeBlocks": [
        {
          "title": "tooltip.tsx 源码对应关系",
          "code": "import * as TooltipPrimitive from \"@radix-ui/react-tooltip\"\nimport * as React from \"react\"\nimport { cn } from \"../lib/utils\"\n\nconst TooltipProvider = TooltipPrimitive.Provider\nconst Tooltip = TooltipPrimitive.Root\nconst TooltipTrigger = TooltipPrimitive.Trigger\n\nconst TooltipContent = React.forwardRef((\n  { className, sideOffset = 4, ...props },\n  ref,\n) => (\n  <TooltipPrimitive.Content\n    ref={ref}\n    sideOffset={sideOffset}\n    className={cn(\"统一 tooltip 样式 class\", className)}\n    {...props}\n  />\n))\n\nexport { Tooltip, TooltipTrigger, TooltipContent, TooltipProvider }\n\n读法：\n  TooltipProvider / Tooltip / TooltipTrigger 基本是 Radix 原样导出。\n  TooltipContent 是加了默认 sideOffset 和统一 class 的 Radix Content。"
        },
        {
          "title": "业务里通常这样使用",
          "code": "<TooltipProvider>\n  <Tooltip>\n    <TooltipTrigger asChild>\n      <button>删除</button>\n    </TooltipTrigger>\n\n    <TooltipContent side=\"top\">\n      删除当前会话\n    </TooltipContent>\n  </Tooltip>\n</TooltipProvider>\n\n组件关系：\n  TooltipProvider\n    Tooltip Root        // 单个 tooltip 实例的共享上下文\n      TooltipTrigger    // 注册触发元素 button\n      TooltipContent    // 读取同一个 Root 上下文，显示内容并定位到 button 附近"
        },
        {
          "title": "极简 demo：不用 Radix，自己实现 Root/Trigger/Content 的关系",
          "code": "const TooltipContext = React.createContext(null)\n\nfunction MiniTooltip({ children }) {\n  const [open, setOpen] = React.useState(false)\n  const triggerRef = React.useRef(null)\n\n  const value = React.useMemo(() => ({\n    open,\n    setOpen,\n    triggerRef,\n  }), [open])\n\n  return (\n    <TooltipContext.Provider value={value}>\n      {children}\n    </TooltipContext.Provider>\n  )\n}\n\nfunction MiniTooltipTrigger({ children }) {\n  const ctx = React.useContext(TooltipContext)\n\n  return React.cloneElement(children, {\n    ref: ctx.triggerRef,\n    onMouseEnter: () => ctx.setOpen(true),\n    onMouseLeave: () => ctx.setOpen(false),\n    onFocus: () => ctx.setOpen(true),\n    onBlur: () => ctx.setOpen(false),\n    'aria-describedby': ctx.open ? 'mini-tooltip-content' : undefined,\n  })\n}\n\nfunction MiniTooltipContent({ children }) {\n  const ctx = React.useContext(TooltipContext)\n  const [style, setStyle] = React.useState({})\n\n  React.useLayoutEffect(() => {\n    if (!ctx.open || !ctx.triggerRef.current) return\n    const rect = ctx.triggerRef.current.getBoundingClientRect()\n    setStyle({\n      position: 'fixed',\n      left: rect.left,\n      top: rect.bottom + 4,\n    })\n  }, [ctx.open])\n\n  if (!ctx.open) return null\n  return (\n    <div id=\"mini-tooltip-content\" role=\"tooltip\" style={style}>\n      {children}\n    </div>\n  )\n}"
        },
        {
          "title": "极简 demo 的使用方式：Trigger 和 Content 通过 Root context 关联",
          "code": "<MiniTooltip>\n  <MiniTooltipTrigger>\n    <button>删除</button>\n  </MiniTooltipTrigger>\n\n  <MiniTooltipContent>\n    删除当前会话\n  </MiniTooltipContent>\n</MiniTooltip>\n\n执行过程：\n  1. MiniTooltip 创建 open 状态和 triggerRef。\n  2. MiniTooltipTrigger 从 context 里拿到 setOpen 和 triggerRef。\n  3. 用户 hover/focus button，Trigger 调 setOpen(true)。\n  4. MiniTooltipContent 也从同一个 context 里拿 open 和 triggerRef。\n  5. open=true 时，Content 读取 triggerRef.current.getBoundingClientRect()。\n  6. Content 根据 trigger 的位置计算自己的 left/top，然后显示。\n\n所以关联关系不是：\n  Trigger 手动 import Content\n  Content 手动查找上一个 DOM 节点\n  通过 id 字符串硬连\n\n而是：\n  Root context 保存共同状态和 trigger DOM ref。\n  Trigger 写入 ref + 改 open。\n  Content 读取 ref + open 后展示。"
        },
        {
          "title": "forwardRef、sideOffset、className 分别有什么用",
          "code": "const TooltipContent = React.forwardRef((\n  { className, sideOffset = 4, ...props },\n  ref,\n) => (\n  <TooltipPrimitive.Content\n    ref={ref}\n    sideOffset={sideOffset}\n    className={cn(\"默认样式\", className)}\n    {...props}\n  />\n))\n\nforwardRef:\n  外部如果传 ref，最终要落到 TooltipPrimitive.Content 对应的 DOM 上。\n  Radix/调用方可以拿到真实节点。\n\nsideOffset = 4:\n  Content 距离 Trigger 默认 4px。\n  例如 side=\"top\" 时，就是浮层底部和按钮顶部之间留 4px。\n\nclassName={cn(\"默认样式\", className)}:\n  ui-sdk 提供统一默认外观。\n  调用方仍可以传 className 追加或覆盖部分样式。\n\n{...props}:\n  side、align、delayDuration 之外的 Radix Content props 继续透传。"
        }
      ],
      "takeaways": [
        "这个文件的核心价值是“统一导出 + 统一样式”，不是重写 Tooltip 行为。",
        "Trigger 和 Content 的关联来自同一个 Root context：Trigger 更新 open/ref，Content 读取 open/ref。",
        "以后看到 Radix 的 Popover、Dropdown、Dialog 也可以用类似思路理解：Root 提供状态上下文，Trigger 改状态，Content 读状态并渲染浮层。"
      ]
    }
  ]
}
