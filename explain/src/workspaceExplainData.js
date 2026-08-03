export const workspaceExplainData = {
  "title": "Workspace：control-plane 平台能力实现",
  "intro": "这一章从 workspace 的视角解释 control-plane 如何把平台能力暴露给 agent。先看抽象：agent 看到的是 tos-platform MCP tools；control-plane 根据 workspace、session token 和 X-Builder header 注册工具；真正执行时再转发到 browser-service、sandbox-service、数据库或 agent pod。",
  "chapters": [
    {
      "id": "platform-tools",
      "title": "1. 平台级 tools：agent 看到的能力入口",
      "summary": "平台 MCP 的核心是：agent 把 tos-platform 当成普通 HTTP MCP server 请求；CP 的 /mcp 每次收到 MCP JSON-RPC 请求时，handleMcpRequest 都临时创建一个 McpServer，把当前 workspace/session/builder 上下文注入工具注册过程，然后交给 MCP SDK 处理当前请求。单看 handleMcpRequest，它就是一个“解析上下文 -> 注册工具 -> 交给 SDK”的函数。",
      "sourceRefs": [
        "control-plane/src/index.ts:392-395：app.all(\"/mcp\") 把所有 MCP 请求交给 handleMcpRequest。",
        "control-plane/src/mcp/server.ts:12-80：handleMcpRequest 读取 headers、解析 session/task、创建 McpServer 和 transport。",
        "control-plane/src/mcp/tools.ts:27-36：registerTools 按 workspace 上下文注册所有平台工具。",
        "agents/claude-code/src/agent.ts:199-254：Claude 每个 turn 动态组装 mcpServers，并注入 tos-platform。",
        "agents/codex/src/config.ts:390-444：Codex ACP session 动态注入 tos-platform 和 HTTP MCP headers。",
        "control-plane/src/routes/internal.ts:279-304：普通用户 MCP 配置注入 workspace header，并按 OAuth 情况改写成 CP proxy。"
      ],
      "bullets": [
        "先看函数内部：handleMcpRequest 不直接判断 method 是 tools/list 还是 tools/call，也不手写 JSON-RPC 分发。它只准备 MCP server 和 transport，真正的协议分发交给 SDK。",
        "X-Workspace-ID 是必需参数。没有它，CP 不知道这次平台工具应该绑定哪个 workspace，所以直接返回 400。",
        "X-Session-Token 是可选增强上下文。能解析并且 workspace 匹配时，CP 得到 sessionId；如果还能找到 teamwork task，并确认 task 的 coordinator_workspace_id 是当前 workspace，才把 taskId 放进工具上下文。",
        "registerTools 是平台工具清单的来源。tools/list 返回什么，取决于这一次 registerTools 给 server 挂了哪些工具；不是查数据库里的“工具表”。",
        "X-Builder 会影响 registerBuilderTools。没有 builder cap，builder 相关工具不注册，所以 agent 的 tools/list 里看不到这些工具。",
        "tools/list 和 tools/call 进入 CP 后，前半段流程一样：都要解析 headers、new McpServer、registerTools、connect transport。区别是 SDK 在 transport.handleRequest 里根据 JSON-RPC method 决定返回工具列表还是调用某个 handler。",
        "和普通 MCP 的本质差异不在协议。协议都是 initialize/tools/list/tools/call。差异在工具 handler 在哪里：平台 MCP 的 handler 在 CP 进程里；普通 HTTP MCP 的 handler 在第三方 server；stdio MCP 的 handler 在 agent 容器里的本地进程。"
      ],
      "tools": [
        {
          "group": "handleMcpRequest",
          "names": [
            "read X-Workspace-ID",
            "resolve X-Session-Token",
            "validate task",
            "new McpServer",
            "registerTools",
            "transport.handleRequest"
          ],
          "purpose": "把一条 HTTP MCP JSON-RPC 请求变成一个带 workspace/session/task 上下文的平台 MCP server 请求。"
        },
        {
          "group": "tools/list",
          "names": [
            "initialize",
            "tools/list",
            "dynamic tool registry"
          ],
          "purpose": "agent 初始化 MCP server 后列出工具；CP 每次现注册工具，返回本 workspace/header 下可见的工具清单。"
        },
        {
          "group": "tools/call",
          "names": [
            "create_browser",
            "create_sandbox",
            "call_agent",
            "share_folder",
            "workspace_*_propose"
          ],
          "purpose": "模型选择具体平台工具后，agent 发 tools/call；CP 在 tool handler 中执行业务逻辑。"
        },
        {
          "group": "普通 MCP 对比",
          "names": [
            "user HTTP MCP",
            "CP proxy",
            "OAuth token injection",
            "stdio MCP"
          ],
          "purpose": "普通 MCP 的工具来自外部 server；CP 不注册这些工具，只负责把配置交给 agent，或在 OAuth HTTP MCP 场景做 proxy。"
        }
      ],
      "diagram": "平台 MCP vs 普通 MCP：跨服务差异才适合看泳道图\nsequenceDiagram\n  participant Agent as Agent MCP client\n  participant CP as control-plane\n  participant Platform as CP /mcp tos-platform\n  participant Proxy as CP MCP proxy\n  participant Third as 第三方 MCP server\n  participant Stdio as 本地 stdio MCP process\n\n  Agent->>Platform: tools/list 或 tools/call\nurl=CP /mcp\nheaders 含 X-Workspace-ID/X-Session-Token\n  Platform->>Platform: CP 进程内 new McpServer\nregisterTools 动态注册平台工具\n  Platform-->>Agent: 平台工具列表或执行结果\n\n  Agent->>Proxy: tools/list 或 tools/call\nurl=CP /_proxy/mcp/...\nheaders 含 X-Session-Token\n  Proxy->>Proxy: 校验/转换 session token\n必要时注入 OAuth access token\n  Proxy->>Third: 转发 MCP JSON-RPC\n  Third-->>Proxy: 第三方工具列表或执行结果\n  Proxy-->>Agent: 原样返回\n\n  Agent->>Stdio: 启动/连接本地 command MCP\n  Stdio-->>Agent: 本地进程提供 tools/list/tools/call\n  Note over CP: stdio MCP 不经过 CP /mcp\nCP 不参与注册工具，也不注入 session token",
      "examples": [
        {
          "title": "1. tools/list：agent 怎么拿到平台所有 tools",
          "request": "POST /mcp JSON-RPC method=tools/list",
          "payload": {
            "method": "POST",
            "url": "http://control-plane:3000/mcp",
            "headers": {
              "Content-Type": "application/json",
              "X-Workspace-ID": "ws_codex_demo",
              "X-Session-Token": "cp_minted_session_token",
              "X-Builder": "workspace"
            },
            "body": {
              "jsonrpc": "2.0",
              "id": 2,
              "method": "tools/list",
              "params": {}
            }
          },
          "response": {
            "jsonrpc": "2.0",
            "id": 2,
            "result": {
              "tools": [
                {
                  "name": "create_browser",
                  "description": "Create a browser session",
                  "inputSchema": "{...}"
                },
                {
                  "name": "create_sandbox",
                  "description": "Create a sandbox",
                  "inputSchema": "{...}"
                },
                {
                  "name": "call_agent",
                  "description": "Ask another workspace agent",
                  "inputSchema": "{...}"
                },
                {
                  "name": "workspace_prompt_propose",
                  "description": "Only appears when X-Builder enables workspace cap",
                  "inputSchema": "{...}"
                }
              ]
            }
          },
          "notes": [
            "handleMcpRequest 不手写 tools/list 响应；它注册工具后，由 MCP SDK 根据 server 上的工具 registry 生成响应。",
            "如果去掉 X-Builder，builder 相关工具不会被 registerBuilderTools 注册，tools/list 里就没有 workspace_prompt_propose。"
          ]
        },
        {
          "title": "2. tools/call：agent 调平台 tool 后 CP 做什么",
          "request": "POST /mcp JSON-RPC method=tools/call",
          "payload": {
            "method": "POST",
            "url": "http://control-plane:3000/mcp",
            "headers": {
              "X-Workspace-ID": "ws_codex_demo",
              "X-Session-Token": "cp_minted_session_token"
            },
            "body": {
              "jsonrpc": "2.0",
              "id": 3,
              "method": "tools/call",
              "params": {
                "name": "create_browser",
                "arguments": {
                  "timeout_seconds": 3600
                }
              }
            }
          },
          "response": {
            "jsonrpc": "2.0",
            "id": 3,
            "result": {
              "content": [
                {
                  "type": "text",
                  "text": "{\"browser_id\":\"br_123\",\"connect_command\":\"agent-browser connect ...\"}"
                }
              ]
            }
          },
          "notes": [
            "tools/call 进入 handleMcpRequest 后，仍然先临时创建 server 并注册工具。",
            "MCP SDK 根据 params.name 找到 create_browser handler。这个 handler 在 CP 进程内执行，所以可以查 DB、取 platform token、调用 browser-service。"
          ]
        },
        {
          "title": "3. 和普通 MCP 交互的对比",
          "request": "同样是 MCP JSON-RPC，但 server 所在位置不同",
          "payload": {
            "platform_mcp": {
              "url": "http://control-plane:3000/mcp",
              "who_registers_tools": "control-plane registerTools()",
              "auth_context": "X-Workspace-ID + X-Session-Token + X-Builder",
              "execution_place": "CP process / CP 调内部服务"
            },
            "user_http_mcp": {
              "url": "第三方 MCP URL 或 CP proxy URL",
              "who_registers_tools": "第三方 MCP server",
              "auth_context": "CP proxy 可能注入 OAuth access token；X-Session-Token 不直接给上游",
              "execution_place": "第三方 MCP server"
            },
            "stdio_mcp": {
              "url": null,
              "command": "本地 command + args",
              "who_registers_tools": "本地 MCP 子进程",
              "auth_context": "本地 env/args；不走 CP /mcp",
              "execution_place": "agent 容器内的 stdio MCP process"
            }
          },
          "response": {
            "conclusion": "协议动作相同，都是 initialize/tools/list/tools/call；差异在谁提供工具、谁执行 handler、上下文从哪里来。"
          },
          "notes": [
            "平台 MCP：CP 是 MCP server。",
            "用户 HTTP MCP：CP 可能只是 proxy，真正 MCP server 在外部。",
            "stdio MCP：agent 本地启动进程，CP 不参与每次 tools/list/tools/call。"
          ]
        }
      ],
      "codeBlocks": [
        {
          "title": "handleMcpRequest 源码骨架：它不是业务工具本身，而是平台 MCP 请求入口",
          "code": "// control-plane/src/mcp/server.ts\nexport async function handleMcpRequest(request: Request): Promise<Response> {\n  const workspaceId = request.headers.get('x-workspace-id')\n  if (!workspaceId) {\n    return new Response(JSON.stringify({ error: 'X-Workspace-ID header required' }), {\n      status: 400,\n      headers: { 'Content-Type': 'application/json' },\n    })\n  }\n\n  let sessionId: string | null = null\n  let taskId: string | null = null\n  const tokenHeader = request.headers.get('x-session-token')\n\n  if (tokenHeader) {\n    const record = await resolveToken(tokenHeader).catch(() => null)\n    if (record && record.workspaceId === workspaceId) {\n      sessionId = record.sessionId\n    }\n\n    if (sessionId) {\n      taskId = await findTaskBySession(sessionId).catch(() => null)\n      if (taskId) {\n        const task = await getTeamworkTask(taskId)\n        if (!task || task.coordinator_workspace_id !== workspaceId) {\n          taskId = null\n        }\n      }\n    }\n  }\n\n  const server = new McpServer({ name: 'tos-platform', version: '0.1.0' })\n  registerTools(server, { workspaceId, sessionId, taskId, headers: request.headers })\n\n  const transport = new WebStandardStreamableHTTPServerTransport({\n    sessionIdGenerator: undefined,\n    enableJsonResponse: true,\n  })\n  await server.connect(transport)\n\n  return transport.handleRequest(request)\n}"
        },
        {
          "title": "逐行理解：handleMcpRequest 内部每一步在干什么",
          "code": "1. 读取 X-Workspace-ID\n   这是平台 MCP 的 workspace 边界。没有 workspaceId，后面的工具不知道该查哪个 workspace、哪个 owner、哪些配置。\n\n2. 尝试解析 X-Session-Token\n   这是 CP 在 chat dispatch 时下发给 agent 的 opaque token。\n   handleMcpRequest 用 resolveToken(token) 反查 session_tokens，得到 workspaceId/sessionId。\n\n3. 校验 token 的 workspace 是否匹配 header 里的 workspace\n   如果 token 属于 ws_a，但请求 header 写 ws_b，CP 不使用这个 sessionId。\n   这样避免把一个会话 token 拿去冒充另一个 workspace 的上下文。\n\n4. 尝试找到 teamwork taskId\n   如果 sessionId 属于一个 teamwork task，再确认这个 task 的 coordinator_workspace_id 就是当前 workspace。\n   只有校验通过，call_agent 这类 task-aware 工具才拿到 taskId。\n\n5. new McpServer({ name: 'tos-platform' })\n   这里创建的是本次 HTTP 请求临时使用的 MCP server 对象。\n   它不是常驻全局 server，也不是数据库实体。\n\n6. registerTools(server, ctx)\n   把 memory、agent、browser、sandbox、skills、AFS、export、builder tools 挂到这个 server。\n   ctx 里的 workspaceId/sessionId/taskId/headers 会被具体工具使用。\n\n7. 创建 Streamable HTTP transport 并 server.connect(transport)\n   这一步把 MCP SDK server 和 HTTP transport 接起来。\n\n8. transport.handleRequest(request)\n   从这里开始才是 MCP 协议分发：\n   - method=initialize：返回初始化结果\n   - method=tools/list：返回刚才注册过的工具列表\n   - method=tools/call：找到 params.name 对应的 tool handler 并执行"
        },
        {
          "title": "registerTools：tools/list 的工具来源",
          "code": "// control-plane/src/mcp/tools.ts\nexport function registerTools(server: McpServer, ctx: McpRequestContext) {\n  const { workspaceId, taskId, headers } = ctx\n\n  registerMemoryTools(server, workspaceId)\n  registerAgentTools(server, workspaceId, taskId)\n  registerBrowserTools(server, workspaceId)\n  registerSandboxTools(server, workspaceId)\n  registerSkillsTools(server, workspaceId)\n  registerAfsTools(server, workspaceId)\n  registerExportFileUrlTool(server, workspaceId)\n  registerBuilderTools(server, workspaceId, headers)\n}\n\n// 重点：\n// tools/list 返回的是这些 registerXxxTools 调 server.registerTool(...) 后形成的集合。\n// X-Builder 影响 registerBuilderTools，所以同一个 /mcp，在不同 header 下 tools/list 可以不同。"
        },
        {
          "title": "agent 侧如何注入 tos-platform：它看起来就是一个普通 HTTP MCP server",
          "code": "// agents/claude-code/src/agent.ts\nmcpServers['tos-platform'] = {\n  type: 'http',\n  url: CP_URL + '/mcp',\n  headers: {\n    'X-Workspace-ID': WORKSPACE_ID,\n    'X-Agent-ID': WORKSPACE_ID,\n    'X-Session-Token': sessionToken,\n    // 可能还包含 X-Builder\n  }\n}\n\n// agents/codex/src/config.ts\nservers.push({\n  type: 'http',\n  name: 'tos-platform',\n  url: CP_URL + '/mcp',\n  headers: Array.from(platformHeaders, ([name, value]) => ({ name, value })),\n})\n\n// 重点：\n// agent 不需要知道 CP 内部怎么注册工具。\n// 它只按 MCP 协议连接 tos-platform，然后发 initialize/tools/list/tools/call。"
        }
      ]
    },
    {
      "id": "browser-flow",
      "title": "2. Browser：agent-browser 驱动远程 Chrome",
      "summary": "Browser 能力的抽象是：agent 不在自己容器里启动 Chrome，而是让 CP 代表 workspace owner 调 browser-service 创建远程浏览器。agent 拿到 connect_command 后，用本地预装的 agent-browser CLI 连接远程浏览器。下载文件仍在远程 browser sandbox，必须通过 list_browser_files 和 get_browser_file_url 取回。",
      "sourceRefs": [
        "internal/agent-skills/platform/__platform__/reference/browser.md：agent 使用约定。",
        "control-plane/src/mcp/tools/browser.ts：MCP tool 注册与返回字段。",
        "control-plane/src/services/browser.ts：CP 到 browser-service 的 HTTP client。",
        "docs/wiki/browser-service/README.md：browser-service 整体说明。"
      ],
      "bullets": [
        "create_browser 先取 workspace owner 的 platform token，再 POST browser-service /api/browsers，并写入 browser.workspace_id metadata。",
        "CP 等 browser status 变成 running 后，再访问 /cdp/{id}/json/version 获取 webSocketDebuggerUrl，并把内部地址改写成外部可访问的 wss connect_url。",
        "connect_command 给 agent-browser 用；cdp_url 只给 Playwright connectOverCDP 用，不能混用。",
        "live_view_url 可以直接发给用户观看；下载文件不在 /workspace，而在 browser sandbox 的 /downloads。"
      ],
      "diagram": "Browser 创建和连接\nsequenceDiagram\n  participant Agent as Agent\n  participant MCP as CP create_browser tool\n  participant DB as workspace/user token\n  participant BS as browser-service\n  participant CLI as agent-browser CLI\n  participant User as User\n\n  Agent->>MCP: create_browser({timeout_seconds})\n  MCP->>DB: getWorkspace + getPlatformToken(owner)\n  MCP->>BS: POST /api/browsers\nmetadata browser.workspace_id\n  BS-->>MCP: browser id + cdp/live endpoints\n  MCP->>BS: poll GET /api/browsers/{id}\n直到 running\n  MCP->>BS: GET /cdp/{id}/json/version\n  BS-->>MCP: webSocketDebuggerUrl\n  MCP-->>Agent: browser_id, connect_command, live_view_url\n  Agent->>CLI: agent-browser connect \"wss://...\"\n  Agent-->>User: share live_view_url if needed",
      "examples": [
        {
          "title": "创建浏览器后 agent 实际拿到什么",
          "request": "MCP tool: create_browser",
          "payload": {
            "timeout_seconds": 3600
          },
          "response": {
            "browser_id": "br_123",
            "status": "running",
            "connect_command": "agent-browser connect \"wss://browser.example/devtools/browser/...\"",
            "cdp_url": "https://browser.example/cdp/br_123?token=...",
            "live_view_url": "https://browser.example/live/br_123?token=..."
          },
          "notes": [
            "agent-browser 用 connect_command。Playwright 才用 cdp_url。",
            "live_view_url 是给人看的，不是 agent-browser 的连接地址。"
          ]
        },
        {
          "title": "下载文件为什么要再取 URL",
          "request": "MCP tools: list_browser_files -> get_browser_file_url",
          "payload": {
            "browser_id": "br_123",
            "path": "/downloads/report.pdf"
          },
          "response": {
            "ready": true,
            "url": "http://nap-browser:3005/api/browsers/br_123/files/content?path=%2Fdownloads%2Freport.pdf&token=...",
            "size": 48291
          },
          "notes": [
            "文件在远程 browser sandbox，不在 agent 的 /workspace。",
            "delete_browser 后这些文件会消失，所以先取 URL 再清理。"
          ]
        }
      ]
    },
    {
      "id": "sandbox-flow",
      "title": "3. Sandbox：临时隔离容器，不污染 workspace",
      "summary": "Sandbox 能力的抽象是：agent 需要一个临时运行环境时，不在当前 workspace 安装一堆依赖，而是让 CP 调 sandbox-service 创建容器。sandbox 容器默认 cwd 是 /，没有 /workspace；需要你显式创建 /home/user 或传 cwd。",
      "sourceRefs": [
        "internal/agent-skills/platform/__platform__/reference/sandbox.md：agent 使用约定。",
        "control-plane/src/mcp/tools/sandbox.ts：MCP tool 注册。",
        "control-plane/src/services/sandbox.ts：CP 到 sandbox-service 的 HTTP client。",
        "docs/wiki/sandbox-service/README.md：sandbox-service 整体说明。"
      ],
      "bullets": [
        "create_sandbox 把 env 数组转成对象，POST sandbox-service /api/sandboxes，并带 metadata.workspace_id。",
        "sandbox_run_command 是阻塞式执行，长时间 dev server 应该用 nohup 后台运行，然后再调用 sandbox_get_preview_url。",
        "sandbox_get_preview_url 不请求 sandbox-service，它根据 SANDBOX_PUBLIC_URL 拼出 https://{sandbox_id}-{port}.host/。",
        "kill_sandbox 会销毁容器，容器内数据丢失；这是临时执行环境，不是 workspace 持久化。"
      ],
      "diagram": "Sandbox 运行和预览\nsequenceDiagram\n  participant Agent as Agent\n  participant MCP as CP sandbox tool\n  participant DB as workspace/user token\n  participant SS as sandbox-service\n  participant App as sandbox container\n  participant User as User\n\n  Agent->>MCP: create_sandbox({image, resource, timeout})\n  MCP->>DB: getWorkspace + getPlatformToken(owner)\n  MCP->>SS: POST /api/sandboxes\nmetadata workspace_id\n  SS-->>MCP: sandbox_id/status\n  MCP-->>Agent: sandbox_id\n  Agent->>MCP: sandbox_run_command({cwd, command})\n  MCP->>SS: POST /api/sandboxes/{id}/exec\n  SS->>App: run shell command\n  App-->>SS: stdout/stderr/exitCode\n  SS-->>MCP: command result\n  MCP-->>Agent: stdout/stderr/exit_code\n  Agent->>MCP: sandbox_get_preview_url({port})\n  MCP-->>Agent: public preview URL\n  Agent-->>User: share preview URL",
      "examples": [
        {
          "title": "创建 node sandbox 并启动 dev server",
          "request": "MCP tools: create_sandbox -> sandbox_run_command -> sandbox_get_preview_url",
          "payload": {
            "image": "node:22-bookworm",
            "resource": {
              "cpu": "2",
              "memory": "4Gi"
            },
            "command": "mkdir -p /home/user/app && cd /home/user/app && npm create vite@latest . -- --template react && npm install && nohup npm run dev -- --host 0.0.0.0 > /tmp/dev.log 2>&1 & echo $!",
            "cwd": "/"
          },
          "response": {
            "sandbox_id": "sbx_123",
            "exec": {
              "stdout": "12345",
              "stderr": "",
              "exit_code": 0
            },
            "preview": {
              "port": 5173,
              "url": "https://sbx_123-5173.sandbox.example/"
            }
          },
          "notes": [
            "不要默认 cwd=/workspace；sandbox 默认 cwd=/。",
            "长进程用 nohup，否则 sandbox_run_command 会一直阻塞。"
          ]
        }
      ]
    },
    {
      "id": "builder-flow",
      "title": "4. Builder Mode：把配置修改变成用户审批卡片",
      "summary": "Builder Mode 的抽象是 human-in-the-loop 配置修改。agent 不能直接改 workspace 配置；它只能 propose，一个 pending agent_request 让 UI 展示审批卡。用户 Approve 后，agent 再 apply，CP 用 CAS 抢占这条 request 并执行真正的业务修改。",
      "sourceRefs": [
        "internal/agent-skills/platform/__platform__/reference/builder-mode.md：agent 使用约定。",
        "control-plane/src/mcp/tools/builder-mode/index.ts：解析 X-Builder 并注册工具。",
        "control-plane/src/mcp/tools/builder-mode/define-action.ts：propose/apply 通用协议。",
        "control-plane/src/mcp/tools/builder-mode/workspace.ts：workspace scope actions。",
        "control-plane/src/mcp/tools/builder-mode/actions/config.ts：workspace config/prompt 修改。",
        "control-plane/src/mcp/tools/builder-mode/actions/schedule.ts：schedule 创建/更新/删除。"
      ],
      "bullets": [
        "X-Builder 决定 tools 是否出现：没有 cap 时，连 read tools 都不注册；cap=workspace 注册 workspace_* tools；cap=global 注册 prompt library tools。",
        "读工具始终先用：list_prompts/get_prompt/list_schedules/get_workspace_config 等只读工具帮助 agent 基于现实设计 diff。",
        "propose 阶段只写 agent_requests，状态是 pending；不会改 workspace_config、schedules、prompts。",
        "approve 是 UI/用户动作；apply 必须拿真实 request_id，且 request status 必须是 approved。markAgentRequestApplied 是 CAS，防止并发重复 apply。",
        "真正业务修改在 action.apply：例如 workspace_prompt_apply 调 applyWorkspaceConfigUpdate；schedule_apply 调 createSchedule 并 enqueue pg-boss timer。"
      ],
      "diagram": "Builder Mode propose -> approve -> apply\nsequenceDiagram\n  participant Agent as Agent\n  participant MCP as Builder MCP tool\n  participant DB as PostgreSQL\n  participant UI as Web UI\n  participant User as User\n  participant Biz as CP business service\n\n  Agent->>MCP: get_workspace_config/list_prompts\n  MCP->>DB: read current state\n  DB-->>MCP: current config/prompts\n  MCP-->>Agent: text report\n  Agent->>MCP: workspace_prompt_propose(payload)\n  MCP->>DB: INSERT agent_requests(status=pending)\n  DB-->>MCP: request_id\n  MCP-->>Agent: pending request envelope\n  UI->>DB: read pending agent_request\n  UI-->>User: render approval card\n  User->>UI: Approve\n  UI->>DB: UPDATE agent_requests status=approved\n  UI-->>Agent: system message: call *_apply(request_id)\n  Agent->>MCP: workspace_prompt_apply({request_id})\n  MCP->>DB: verify approved + CAS mark applied\n  MCP->>Biz: action.apply(payload)\n  Biz->>DB: update workspace_config/schedules/...\n  MCP-->>Agent: user-facing result text",
      "examples": [
        {
          "title": "把 workspace prompt 切到 library prompt",
          "request": "MCP tools: get_workspace_config -> list_prompts -> workspace_prompt_propose -> workspace_prompt_apply",
          "payload": {
            "propose": {
              "prompt_id": "prm_abc"
            },
            "apply": {
              "request_id": "ar_123"
            }
          },
          "response": {
            "propose": {
              "request_id": "ar_123",
              "kind": "builder.prompt_set",
              "status": "pending"
            },
            "apply": "Workspace system prompt switched to library prompt prm_abc."
          },
          "notes": [
            "prompt_id 和 system_prompt 是 XOR；设置 prompt_id 会清空 inline system_prompt。",
            "apply 只能在用户批准后调用，不能伪造 request_id。"
          ]
        },
        {
          "title": "创建定时任务",
          "request": "MCP tools: list_schedules -> workspace_schedule_propose -> workspace_schedule_apply",
          "payload": {
            "propose": {
              "name": "Daily Review",
              "cron": "0 9 * * *",
              "timezone": "Asia/Shanghai",
              "prompt": "总结昨天的工作并列出今天计划"
            }
          },
          "response": {
            "propose": {
              "request_id": "ar_456",
              "status": "pending"
            },
            "apply": "Schedule \"Daily Review\" created (id=sch_123)."
          },
          "notes": [
            "timezone 必填，不能默认 UTC。",
            "apply 创建 schedule 后还会 enqueue schedule timer；timer 注册失败会回滚删除 schedule。"
          ]
        }
      ]
    }
  ]
}
