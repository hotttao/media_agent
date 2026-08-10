# agent-requests：Human-in-the-Loop 审批原语

> **业务定位**：`/api/workspaces/:id/agent-requests/:reqId` 是平台 Human-in-the-Loop（HIL）审批流的对外 HTTP 接口。它**不是** agent 执行通道，而是承载 propose→resolve→apply 三阶段流程的中间状态读取 + 审批操作。本身不走消息队列、不触发 agent 执行，只做状态机推进。

---

## 〇、读前必看

### 0.1 一个常见误解

> ❌ "调用这个 API 就能让 agent 去做事"
>
> ✅ 正确理解：这个 API 只是 **审批窗口**。agent 自己通过 MCP 工具发起 propose，用户通过这个 API 表态 approve/reject，然后再由 agent 通过另一个 MCP 工具完成 apply。HTTP API 只覆盖中间一步。

### 0.2 本文导览

| 章节 | 回答什么问题 | 适合什么读者 |
|---|---|---|
| **一**、端点清单 | "有几个 HTTP 接口？路径长什么样？" | 所有人（先看） |
| **二**、数据模型与状态机 | "agent_requests 表长什么样？状态怎么流转？" | 想理解存储和 CAS 的人 |
| **三**、三阶段 HIL 流程 | "propose / resolve / apply 各做什么？" | 想理解整体设计的人 |
| **四**、后端实现细节 | "路由 / handler / DB 层具体怎么实现？" + "agent 怎么知道有这批工具、怎么用？" | 准备改后端的人 |
| **五**、前端实现细节 | "审批卡片怎么渲染？前端怎么识别 propose 工具？" | 准备改前端的人 |
| **六**、端到端时序（含示例） | "用户从提需求到看到结果，整条链路怎么走？" | 想做集成测试 / 排查问题的人 |
| **七**、Review 检查点 | "这个设计的边界在哪？有哪些 invariant 必须守？" | 做 design review 的人 |

---

## 一、端点清单

完整 URL 前缀为 `/api/workspaces`（control-plane/src/index.ts:313 挂载）。

| 方法 | 路径 | 用途 | 文件:行号 |
|---|---|---|---|
| GET | `/{id}/agent-requests/{reqId}` | 读取单个 agent_request，供前端拉取最新状态 | `control-plane/src/routes/workspaces/agent-requests.ts:20-50` |
| POST | `/{id}/agent-requests/{reqId}/resolve` | 批准或拒绝一个 pending 请求 | `control-plane/src/routes/workspaces/agent-requests.ts:53-100` |

**注意**：路径里 `{reqId}` 是 agent_request 主键 UUID，不是 list 路径。该 API **不支持**列表查询，前端只通过 propose 工具返回的信封拿到 `request_id` 后再单点查询。

**认证**：两个端点都要求 `bearerAuth`，且校验 `workspace.user_id === user.sub`——只能操作自己拥有的 workspace。

---

## 二、数据模型与状态机

### 2.1 表结构

DDL 位置：`control-plane/migrations/001_init.sql:287-299`

```sql
CREATE TABLE public.agent_requests (
    id            uuid        DEFAULT gen_random_uuid() NOT NULL,
    workspace_id  text        NOT NULL,
    user_id       text        NOT NULL,
    kind          text        NOT NULL,
    payload       jsonb       NOT NULL,
    status        text        DEFAULT 'pending' NOT NULL,
    reject_reason text,
    created_at    timestamptz DEFAULT now() NOT NULL,
    resolved_at   timestamptz,
    applied_at    timestamptz,
    CONSTRAINT agent_requests_status_check
        CHECK (status = ANY (ARRAY['pending', 'approved', 'rejected', 'applied']))
);
```

| 字段 | 类型 | 含义 |
|---|---|---|
| `id` | uuid | 主键，自动生成；propose 工具返回的 `request_id` 指向这里 |
| `workspace_id` | text | 所属 workspace（FK CASCADE） |
| `user_id` | text | workspace 所有者（FK CASCADE） |
| `kind` | text | 业务种类标签，如 `builder.workspace.schedule.create`；用于路由到对应 apply 实现 |
| `payload` | jsonb | 业务参数，由该 kind 对应的 zod schema 定义形状 |
| `status` | enum | `pending` / `approved` / `rejected` / `applied` |
| `reject_reason` | text | 仅 rejected 状态下有值 |
| `created_at` | timestamptz | propose 时写入 |
| `resolved_at` | timestamptz | 用户批准 / 拒绝时写入 |
| `applied_at` | timestamptz | agent apply 成功时写入 |

**索引**（`001_init.sql:1995-2002`）：
- `(user_id, created_at DESC)` — 用户级时间线查询
- `(workspace_id, created_at DESC)` — workspace 级时间线查询

### 2.2 状态机

```
                    ┌──────── approve ────────┐
                    │                          ▼
   propose ──►   pending ──────────────► approved ──(apply)──► applied
                    │
                    └──────── reject ──────────► rejected
```

- **pending**：propose 工具 INSERT 后初始态
- **approved**：用户 POST /resolve `{ decision: 'approved' }` 后进入
- **rejected**：用户 POST /resolve `{ decision: 'rejected', reason?: string }` 后进入
- **applied**：apply 工具在 approved 之后真正执行业务逻辑前 CAS 进入

**终态**：`applied` 和 `rejected` 都是终态，不可逆。

### 2.3 CAS 保护（关键 invariant）

三个写操作都带 `WHERE status = ...` 做乐观锁，防止重复 / 竞态：

| 操作 | SQL 谓词 | 含义 |
|---|---|---|
| `resolveAgentRequest` | `WHERE id = $1 AND status = 'pending'` | 防止双击重复审批 |
| `markAgentRequestApplied` | `WHERE id = $1 AND status = 'approved'` | 防止重复 apply 业务副作用 |

文件：`control-plane/src/services/db/agent-requests.ts:28-44, 52-60`

---

## 三、三阶段 HIL 流程

整个 HIL 流程由 **三个独立操作** 组成，分散在三种触发方：

| 阶段 | 触发方 | 入口 | 状态变化 |
|---|---|---|---|
| **propose** | Agent（LLM tool call） | MCP 工具 `*_propose` | `(none) → pending` |
| **resolve** | 用户（HTTP） | `POST /api/.../resolve` | `pending → approved / rejected` |
| **apply** | Agent（LLM tool call） | MCP 工具 `*_apply` | `approved → applied` |

### 3.1 propose — agent 创建请求

由 `registerBuilderAction()` 注册的 MCP 工具对中的 propose 工具实现。

文件：`control-plane/src/mcp/tools/builder-mode/define-action.ts:75-101`

```ts
async (raw) => {
  const payload = action.payload.parse(raw)           // 1. zod 校验
  const workspace = await getWorkspace(workspaceId)   // 2. workspace 存在性
  if (!workspace) return textResult('Error: workspace not found')

  const req = await createAgentRequest({              // 3. INSERT agent_requests
    workspace_id: workspaceId,
    user_id: workspace.user_id,
    kind: action.kind,
    payload,
  })

  return textResult(JSON.stringify({                  // 4. 返回信封给 agent
    request_id: req.id, kind: req.kind, label: action.label,
    payload, status: 'pending',
  }))
}
```

工具返回的是一段 JSON 文本（MCP textResult），agent 把它原样转发给前端。**信封（envelope）结构**：

```ts
interface ProposeEnvelope {
  request_id: string
  kind: string
  label: string
  payload?: unknown
  status?: 'pending' | 'approved' | 'rejected' | 'applied'
}
```

> 💡 `payload` 和 `status` 是可选的种子数据——propose 工具一定带；apply 工具的失败结果可能没有。前端用这个种子做"乐观首屏"。

### 3.2 resolve — 用户表态

HTTP API 路径（见第一节）。前端在卡片上点 Approve / Reject 触发：

```
POST /api/workspaces/{id}/agent-requests/{reqId}/resolve
Content-Type: application/json

{ "decision": "approved" }            // 或 { "decision": "rejected", "reason": "..." }
```

后端 handler 流程（`agent-requests.ts:76-100`）：

1. 鉴权：`workspace.user_id === user.sub`
2. 存在性：`getAgentRequest(reqId)` 并校验 `workspace_id` 匹配
3. 前置状态检查：`if (existing.status !== 'pending') return 409`
4. DB CAS：`resolveAgentRequest(reqId, decision, reason)` — `WHERE status = 'pending'`
5. CAS 失败 → `409 Request already resolved`（双击竞态）
6. 成功 → `200` + 更新后的请求

### 3.3 apply — agent 执行业务

由 `registerBuilderAction()` 注册的 apply 工具实现。

文件：`define-action.ts:103-139`

```ts
async ({ request_id }) => {
  const req = await getAgentRequest(request_id)
  if (!req || req.workspace_id !== workspaceId) return textResult('Error: request not found')
  if (req.kind !== action.kind)                            // 防止 apply 工具错配
    return textResult(`Error: request kind mismatch (got "${req.kind}")`)
  if (req.status !== 'approved')                          // 必须 approved 才能执行
    return textResult(`Error: request is "${req.status}", not approved`)

  const claimed = await markAgentRequestApplied(request_id)  // CAS approved → applied
  if (!claimed)                                            // 并发 apply 竞态
    return textResult('Error: request is no longer approved (already applied?)')

  const payload = action.payload.parse(req.payload)        // 用同一份 zod schema 重新解析
  const result = await action.apply({ workspaceId, userId: req.user_id, payload })
  return textResult(result)                                // 文本结果回给 agent
}
```

**关键设计**：

1. **CAS 在业务副作用之前**：`markAgentRequestApplied` 必须先成功，再调 `action.apply`，保证同一条 request 的业务副作用**最多执行一次**。
2. **payload 复用同一份 zod schema**：propose 入参和 apply 出参用同一个 `action.payload`，避免类型漂移。
3. **kind 校验**：防止 agent 把不同 action 的 apply 工具和 request 配错（理论上不会，但加一道防御）。

### 3.4 全部已注册的 action 清单

来自 `control-plane/src/mcp/tools/builder-mode/actions/*.ts`，共 13 个 kind，对应 13 对 propose/apply 工具：

| kind | propose 工具名 | scope | action 文件 |
|---|---|---|---|
| `builder.workspace.schedule.create` | `workspace_schedule_propose` | workspace | `actions/schedule.ts:19` |
| `builder.workspace.schedule.update` | `workspace_schedule_update_propose` | workspace | `actions/schedule.ts:51` |
| `builder.workspace.schedule.delete` | `workspace_schedule_delete_propose` | workspace | `actions/schedule.ts:126` |
| `builder.workspace.command.create` | `workspace_command_propose` | workspace | `actions/command.ts:42` |
| `builder.workspace.command.update` | `workspace_command_update_propose` | workspace | `actions/command.ts:62` |
| `builder.workspace.command.delete` | `workspace_command_delete_propose` | workspace | `actions/command.ts:87` |
| `builder.workspace.command.set_disabled` | `workspace_command_set_disabled_propose` | workspace | `actions/command.ts:101` |
| `builder.workspace.skill.enable` | `workspace_skill_enable_propose` | workspace | `actions/skill.ts:34` |
| `builder.workspace.skill.disable` | `workspace_skill_disable_propose` | workspace | `actions/skill.ts:59` |
| `builder.workspace.config.update` | `workspace_config_propose` | workspace | `actions/config.ts:14` |
| `builder.workspace.prompt.set` | `workspace_prompt_propose` | workspace | `actions/config.ts:56` |
| `builder.prompt_library.create` | `prompt_create_propose` | global | `actions/prompt.ts:20` |
| `builder.prompt_library.update` | `prompt_update_propose` | global | `actions/prompt.ts:41` |
| `builder.prompt_library.delete` | `prompt_delete_propose` | global | `actions/prompt.ts:72` |

**scope 区别**：`workspace` scope 的工具名为 `workspace_<resource>_*`；`global` scope 不带 `workspace_` 前缀，用于账号级资源（如用户的 prompt 库）。

---

## 四、后端实现细节

### 4.1 分层

```
HTTP 路由 (Hono + zod-openapi)
    │
    └─► DB 服务层 (services/db/agent-requests.ts)
            │
            └─► PostgreSQL (agent_requests 表)
```

**注意**：HTTP API 和 MCP 工具都直接调 DB 服务层，没有中间 service。这是因为业务逻辑足够简单——状态机推进 + CAS。

### 4.2 路由文件

`control-plane/src/routes/workspaces/agent-requests.ts` 全文 102 行，结构：

| 行号 | 内容 |
|---|---|
| 10-17 | Hono 实例 + path 参数 zod schema |
| 20-50 | GET 路由定义 + handler |
| 53-100 | POST /resolve 路由定义 + handler |

两个 handler 共享同一套鉴权流程：

```ts
const user = c.get('user')
const { id, reqId } = c.req.valid('param')
const workspace = await getWorkspace(id)
if (!workspace || workspace.user_id !== user.sub) {
  return c.json({ error: 'Workspace not found' }, 404)   // 故意把"无权"也归为 404
}
```

> ⚠️ **设计取舍**：把"无权访问"和"不存在"都返回 404，避免泄漏 workspace 存在性。但代价是前端拿不到 403 信号——如果未来要做"无权提示"，需要单独设计。

### 4.3 DB 服务层

`control-plane/src/services/db/agent-requests.ts` 全文 60 行，4 个函数：

| 函数 | 行号 | 用途 |
|---|---|---|
| `createAgentRequest(data)` | 4-16 | propose 工具调用；INSERT 后回查完整行 |
| `getAgentRequest(id)` | 18-21 | 单行查询；handler 鉴权 + apply 工具都用它 |
| `resolveAgentRequest(id, decision, reason?)` | 28-44 | POST /resolve 调用；CAS `WHERE status = 'pending'` |
| `markAgentRequestApplied(id)` | 52-60 | apply 工具调用；CAS `WHERE status = 'approved'`，返回 boolean 表示是否抢到 |

**`createAgentRequest` 实现**：

```ts
const { rows } = await pool.query(
  `INSERT INTO agent_requests (workspace_id, user_id, kind, payload)
   VALUES ($1, $2, $3, $4) RETURNING id`,
  [data.workspace_id, data.user_id, data.kind, JSON.stringify(data.payload)],
)
return (await getAgentRequest(rows[0].id))!
```

> ⚠️ 注意它只 `RETURNING id`，然后立刻再 `getAgentRequest` 回查。这是为了让 `created_at` 等数据库默认值正确填充。如果未来性能敏感，可以改成 `RETURNING *`。

### 4.4 共享类型

API schema（zod）定义在 `internal/types/api.ts:1816-1833`：

```ts
ApiAgentRequestSchema       // 响应 schema
AgentRequestResolveBodySchema // 请求体 schema
```

TS 类型在 `control-plane/src/services/db/types.ts:474-485`。前端 `web/src/lib/api/types.ts` 也镜像了 `ApiAgentRequest` 类型。

### 4.5 MCP 工具 prompt 暴露路径（教 agent 怎么用这批工具）

这一节回答两个问题：
- 工具自身的 `description` 字符串在哪儿定义？（运行时通过 MCP 协议下发给 agent）
- agent 的系统 prompt 怎么知道有这批 Builder Mode 工具、什么时候该用？

#### 4.5.1 工具自身 description（13 个 propose + 自动生成 apply）

每个 action 在 `control-plane/src/mcp/tools/builder-mode/actions/*.ts` 里定义 `proposeDescription` 字符串。所有 13 个字符串都引用 `__platform__:reference/builder-mode.md` 作为契约说明：

| action | 文件:行号 | proposeDescription 原文（节选） |
|---|---|---|
| schedule.create | `actions/schedule.ts:24` | `'Create a schedule on the current workspace. Either recurring (cron + timezone) or one-time (run_at as ISO 8601 instant in the future). See \`__platform__:reference/builder-mode.md\` for the propose/approve/apply contract.'` |
| schedule.update | `actions/schedule.ts:56` | `'Update an existing schedule. Only set the fields you want to change. Setting \`cron\` or \`run_at\` switches the schedule kind. See \`__platform__:reference/builder-mode.md\`...'` |
| schedule.delete | `actions/schedule.ts:131` | `'Delete a schedule. See \`__platform__:reference/builder-mode.md\`...'` |
| command.create | `actions/command.ts:47` | `'Create a slash command on the current workspace. See \`__platform__:reference/builder-mode.md\`...'` |
| command.update | `actions/command.ts:67` | `'Update a slash command. Only set the fields you want to change. Template-provided commands are read-only - fork them first...'` |
| command.delete | `actions/command.ts:92` | `'Delete a slash command. Template-provided commands cannot be deleted - disable them instead...'` |
| command.set_disabled | `actions/command.ts:106` | `'Enable or disable a slash command for this workspace (by name)...'` |
| config.update | `actions/config.ts:19` | `"Update the current workspace's base info (name, slug, visibility, agent_type, provider, model). Only set the fields you want to change. System prompt is managed separately by \`workspace_prompt_propose\` - do not pass system_prompt or prompt_id here..."` |
| prompt.set | `actions/config.ts:61` | `'Change the current workspace\'s system prompt source. Pass \`prompt_id\`... or \`system_prompt\` for inline text - the two are mutually exclusive...'` |
| skill.enable | `actions/skill.ts:39` | `'Attach one or more existing skills to the current workspace. Skill names come from \`list_skills\`...'` |
| skill.disable | `actions/skill.ts:64` | `'Detach one or more skills from the current workspace...'` |
| prompt_library.create | `actions/prompt.ts:26` | `"Create a new prompt in the current user's library. Defaults to private; pass \`visibility: 'public'\` only when the user has explicitly asked..."` |
| prompt_library.update | `actions/prompt.ts:47` | `'Update an existing prompt in the user library. Owner-only - team-shared / public prompts visible via \`list_prompts\` are read-only here...'` |
| prompt_library.delete | `actions/prompt.ts:78` | `'Delete a prompt from the user library. Apply hard-fails if any workspace still references this prompt...'` |

apply 工具的 `description` **不是手写**，而是在 `define-action.ts:103-108` 里自动生成：

```ts
server.registerTool(applyTool, {
  title: `Apply approved ${action.resource} request`,
  description: `Finalize a \`${proposeTool}\` request that the user has approved. Pass the \`request_id\` returned by the propose tool.`,
  inputSchema: z.object({
    request_id: z.string().describe('The agent_request id from the propose step.'),
  }),
}, ...)
```

生成格式：`Finalize a \`workspace_<resource>_propose\` request that the user has approved. Pass the \`request_id\` returned by the propose tool.`

`title` 字段同样在 `define-action.ts:71,106` 生成：
- propose: `Propose ${action.resource}`
- apply: `Apply approved ${action.resource} request`

#### 4.5.2 MCP server 注册链

工具通过 MCP 协议暴露给 agent 的注册链：

```
handleMcpRequest (control-plane/src/mcp/server.ts:64-73)
    │  每个 chat turn 创建一个新的 McpServer 实例（per-workspace 隔离）
    │
    └─► registerTools (control-plane/src/mcp/tools.ts:36)
            │
            └─► registerBuilderTools (builder-mode/index.ts:14-37)
                    │  解析 X-Builder header 里的 cap
                    │
                    ├─► registerBuilderReadTools (read.ts:32)         // 11 个只读工具（list_*, get_*）
                    │
                    ├─► registerWorkspaceCapTools (workspace.ts:25)   // 10 个 action 对（20 个工具）
                    │      └─ 10× registerBuilderAction(server, workspaceId, action)
                    │
                    └─► registerGlobalCapTools (global.ts:17)         // 3 个 action 对（6 个工具）
                           └─ 3× registerBuilderAction(server, workspaceId, action)
```

每个 action 对经 `registerBuilderAction`（`define-action.ts:59-140`）调用 `server.registerTool` 两次：一次 propose、一次 apply。

#### 4.5.3 工具命名派生

`define-action.ts:64-66`：

```ts
const prefix = action.scope === 'global' ? '' : 'workspace_'
const proposeTool = `${prefix}${action.resource}_propose`
const applyTool   = `${prefix}${action.resource}_apply`
```

| scope | action.resource | propose 工具名 | apply 工具名 |
|---|---|---|---|
| workspace | `schedule` | `workspace_schedule_propose` | `workspace_schedule_apply` |
| workspace | `schedule_update` | `workspace_schedule_update_propose` | `workspace_schedule_update_apply` |
| workspace | `schedule_delete` | `workspace_schedule_delete_propose` | `workspace_schedule_delete_apply` |
| workspace | `command` | `workspace_command_propose` | `workspace_command_apply` |
| workspace | `command_update` | `workspace_command_update_propose` | `workspace_command_update_apply` |
| workspace | `command_delete` | `workspace_command_delete_propose` | `workspace_command_delete_apply` |
| workspace | `command_set_disabled` | `workspace_command_set_disabled_propose` | `workspace_command_set_disabled_apply` |
| workspace | `config` | `workspace_config_propose` | `workspace_config_apply` |
| workspace | `prompt` | `workspace_prompt_propose` | `workspace_prompt_apply` |
| workspace | `skill_enable` | `workspace_skill_enable_propose` | `workspace_skill_enable_apply` |
| workspace | `skill_disable` | `workspace_skill_disable_propose` | `workspace_skill_disable_apply` |
| global | `prompt_create` | `prompt_create_propose` | `prompt_create_apply` |
| global | `prompt_update` | `prompt_update_propose` | `prompt_update_apply` |
| global | `prompt_delete` | `prompt_delete_propose` | `prompt_delete_apply` |

#### 4.5.4 Agent 系统级 prompt（教 LLM 怎么用这批工具）

工具自身的 description 只告诉 LLM 单个工具怎么调；**整体的 propose->approve->apply 契约**由 `__platform__` skill 的 `reference/builder-mode.md` 文档承载。

**入口**：`internal/agent-skills/platform/__platform__/SKILL.md:40-44`

```markdown
## Builder Mode

`workspace_*_propose` / `_apply` (and `<resource>_*_propose` / `_apply`
for the prompt library) let you propose configuration changes - system
prompt, schedules, slash commands, skills, model/provider, prompt library -
that the user approves via an in-chat card before they apply. Read tools
(`list_*`, `get_*`) are always on whenever any Builder cap is enabled.

-> When the user describes a configuration change in conversation rather
   than going to the UI form - see `reference/builder-mode.md` for the
   propose/approve/apply contract, scope semantics, and cross-cutting rules
   (ownership, versioning, source XOR, timezone, reference safety).
```

**完整契约**：`internal/agent-skills/platform/__platform__/reference/builder-mode.md`（78 行）

关键段落：

1. **三阶段流程定义**（第 13-18 行）：
   ```
   1. Propose. You call <name>_propose with the payload you've designed.
      It writes a pending agent_request and returns { request_id, kind,
      label, payload, status: "pending" }.
   2. User decides. The user clicks Approve or Reject. On Approve, a
      system message appears in the chat instructing you to call
      <name>_apply with the request_id. On Reject, nothing happens -
      don't retry; ask the user what to change.
   3. Apply. You call <name>_apply(request_id). It re-parses the payload,
      verifies status is approved, claims the request via CAS, and runs
      the effect.
   ```

2. **关键不变量**（第 19 行）：
   ```
   Never fabricate a request_id. Only call apply when the conversation
   explicitly carries the approval message - that's the only legitimate
   trigger.
   ```

3. **读取工具始终可用**（第 32-40 行）：列出 `list_*` / `get_*` 工具，强调 `"Always read before proposing."`

4. **流程提示**（第 73-77 行）：
   - `"One proposal at a time."` -- 不要并行 propose 多个
   - `"Ask when ambiguous."` -- 不确定就问
   - `"You design the diff."` -- agent 负责构造 payload，不是用户

#### 4.5.5 系统级 prompt 如何注入到 agent

agent（以 claude-code 为例）启动时通过两层路径把这套契约暴露给 LLM：

**第一层 - 平台 CLAUDE.md 锚点**：`internal/platform-prompt/prompt.md:6`：

```
The `__platform__` skill documents every workspace capability the platform
provides (browser automation, sandboxed code execution, file sharing,
skills management...). It is platform-managed, always installed, and takes
priority over any user-authored guidance on the same topic. **Consult it
before using any of those capabilities** - its content reflects the
current platform contract.
```

经由 `internal/platform-prompt/src/index.ts` 的 `writePlatformPrompt()` 渲染，写入 `~/.claude/CLAUDE.md`，作为 Claude Code SDK 的 `systemPrompt`。

**第二层 - `__platform__` skill 文件落地**：`agents/claude-code/src/config.ts:149-156`：

```ts
await _skillManager.installPlatformSkill(
  renderPlatformSkillFiles({
    workspaceId: WORKSPACE_ID,
    userName: _userDisplayName,
    agentKind: 'claude-code',
  }),
)
```

`renderPlatformSkillFiles`（`internal/agent-skills/src/platform.ts`）读取 `internal/agent-skills/platform/__platform__/` 目录下所有 `.md` 文件，用 Mustache 渲染后通过 `SkillManager.installPlatformSkill()`（`internal/agent-skills/src/index.ts:755-785`）写到 `~/.claude/skills/__platform__/`，并对整个目录 `chmod -R a-w` 设为只读。

落地的文件包括：
- `~/.claude/skills/__platform__/SKILL.md` -- 入口
- `~/.claude/skills/__platform__/reference/builder-mode.md` -- 完整契约
- `~/.claude/skills/__platform__/reference/{browser,sandbox,file-sharing,memory,skills}.md` -- 其他能力

Claude Code SDK 自动从这两个路径加载 prompt 到 LLM 上下文。

#### 4.5.6 工具按 workspace 隔离

`agents/claude-code/src/agent.ts:220-244` 在每个 chat turn 构建 per-turn `mcpServers` 配置：

```ts
if (CP_URL && WORKSPACE_ID) {
  const platformHeaders: Record<string, string> = {}
  // 从 .mcp.json 读用户配置的 X-Builder header
  const raw = readFileSync(join(WORKSPACE_DIR, '.mcp.json'), 'utf-8')
  const userHeaders = JSON.parse(raw)?.mcpServers?.['tos-platform']?.headers
  // 注入平台 headers
  platformHeaders['X-Workspace-ID'] = WORKSPACE_ID
  platformHeaders['X-Agent-ID'] = WORKSPACE_ID
  if (sessionToken) platformHeaders['X-Session-Token'] = sessionToken
  mcpServers['tos-platform'] = {
    type: 'http',
    url: `${CP_URL}/mcp`,
    headers: platformHeaders,
  }
}
```

- **X-Workspace-ID** 让 `handleMcpRequest`（`control-plane/src/mcp/server.ts:13`）能拿到 workspace 隔离上下文，传给 `registerBuilderAction` 作为 DB 查询 scope。
- **X-Builder** header（从 `.mcp.json` 读）控制 cap 启用：`workspace` / `global` / 各细分开关。
- 每个 chat turn 创建独立 MCP server 实例，**不共享**。

#### 4.5.7 完整暴露路径图

```
┌──────────────────────────────────────────────────────────────────────────────┐
│                              control-plane                                    │
│                                                                               │
│  POST /mcp  (per-turn, per-workspace)                                         │
│     │  读取 X-Workspace-ID, X-Builder headers                                  │
│     │                                                                         │
│     ▼  handleMcpRequest (server.ts:64)                                        │
│        └─► new McpServer('tos-platform')                                      │
│             └─► registerTools (tools.ts:36)                                  │
│                  └─► registerBuilderTools (builder-mode/index.ts:14)         │
│                       ├─► 11 个 read 工具                                     │
│                       ├─► 10 个 workspace action 对 (20 个工具)              │
│                       └─► 3 个 global action 对 (6 个工具)                   │
│                            每个工具带 proposeDescription / 生成 description   │
│                                                                               │
│  返回工具列表（含 description / inputSchema）给 agent                         │
└────────────────────────────────────────┬──────────────────────────────────────┘
                                         │ MCP 协议
                                         ▼
┌──────────────────────────────────────────────────────────────────────────────┐
│                              agent (claude-code)                              │
│                                                                               │
│  agent.ts:220-244  每个 chat turn 构建 mcpServers['tos-platform']              │
│     ├─ url: CP_URL + '/mcp'                                                  │
│     └─ headers: X-Workspace-ID, X-Agent-ID, X-Session-Token, X-Builder        │
│                                                                               │
│  Claude Code SDK 连接 MCP server                                             │
│  -> 拿到工具列表（含 description）                                            │
└────────────────────────────────────────┬──────────────────────────────────────┘
                                         │ 工具列表 + system prompt
                                         ▼
┌──────────────────────────────────────────────────────────────────────────────┐
│                          Agent 系统提示（启动时 / 配置重载）                    │
│                                                                               │
│  config.ts:loadConfig()                                                       │
│     ├─ writePlatformPrompt() -> ~/.claude/CLAUDE.md                           │
│     │     "The __platform__ skill documents... Consult it..."                 │
│     │                                                                         │
│     └─ loadSkills()                                                           │
│         └─ installPlatformSkill(renderPlatformSkillFiles(...))                │
│              ├─ ~/.claude/skills/__platform__/SKILL.md                         │
│              │     "Builder Mode: workspace_*_propose / _apply..."            │
│              └─ ~/.claude/skills/__platform__/reference/builder-mode.md       │
│                    完整 propose -> approve -> apply 契约                       │
│                                                                               │
│  Claude Code SDK 把这两份 prompt 加入 LLM 上下文                              │
└──────────────────────────────────────────────────────────────────────────────┘
```

> 💡 **设计要点**：MCP 工具的 `description` 字段是**运行时动态下发**的（每次 chat turn 都新创建 McpServer 并注册工具）；而 `__platform__` skill 是**启动时静态落地**到 `~/.claude/skills/`。两者配合：description 告诉 LLM 单个工具怎么调，skill 文档告诉 LLM 整体流程怎么走。

---

## 五、前端实现细节

### 5.0 propose 工具的识别机制（关键）

前端是怎么从 SSE 流里识别出"这是一条 propose 工具调用"的？答案是**显式枚举 13 个工具名 + 精确 Map 查表**，不是后缀模糊匹配。

#### 5.0.1 启动时一次性注册

`web/src/plugins/index.ts:21-48`：

```ts
const AGENT_REQUEST_PROPOSE_TOOLS = [
  'workspace_schedule_propose', 'workspace_schedule_update_propose',
  'workspace_schedule_delete_propose', 'workspace_command_propose',
  'workspace_command_update_propose', 'workspace_command_delete_propose',
  'workspace_skill_enable_propose', 'workspace_skill_disable_propose',
  'workspace_config_propose', 'workspace_prompt_propose',
  'prompt_create_propose', 'prompt_update_propose', 'prompt_delete_propose',
]

export function registerBuiltinPlugins(): void {
  ...
  for (const name of AGENT_REQUEST_PROPOSE_TOOLS) {
    registerToolRenderer(name, agentRequestProposeRenderer)
  }
}
```

调用时机：`web/src/main.tsx:31` 在 `boot()` 中调 `registerBuiltinPlugins()`。

#### 5.0.2 `registerToolRenderer` 实现

`internal/ui-sdk/src/tool-renderers/plugin-registry.ts:13-29`：

```ts
const renderers = new Map<string, ToolRendererDef>()
const listeners = new Set<() => void>()
let version = 0

export function registerToolRenderer(name, def) {
  if (renderers.has(name)) { console.warn(...); return }
  renderers.set(name, def)
  version++
  for (const fn of listeners) fn()    // 通知订阅者重渲染
}
```

#### 5.0.3 渲染时的查找链路

每条 tool_call 都被 `internal/ui-sdk/src/components/ToolCallBlock.tsx:41-141` 渲染：

```
SSE 来一条 tool_call:
  name   = "mcp__tos-platform__workspace_schedule_propose"
  input  = { cron: "0 9 * * *", message: "站会" }
  result = '{"request_id":"...","kind":"...","label":"...","payload":{...},"status":"pending"}'
       │
       ▼
ToolCallBlock 渲染
  (ToolCallBlock.tsx:41)
       │
   1.  unwrapExecuteDispatchName(name, input)
       (registry.ts:186) -- Codex 才拆 execute({tool,server,arguments}) 包；CC 直通
       │
   2.  getToolDisplayName(name)
       (registry.ts:164-178) -- 三种正则剥离 MCP 前缀
         • CC:    mcp__server__tool_name  -> tool_name  (命中的正则: /^mcp__(.+?)__(.+)$/)
         • Codex: Tool: server/tool_name  -> tool_name  (命中的正则: /^Tool:\s*.+\/(.+)$/)
         • Goose: server__tool_name       -> tool_name  (命中的正则: /^(.+?)__(.+)$/)
       "mcp__tos-platform__workspace_schedule_propose"
         -> "workspace_schedule_propose"
       │
   3.  resolveRenderer(tool, agentType)
       (registry.ts:199-210) -- 优先级链
         1. toolRenderers[displayName]            built-in 表（registry.ts:58-142）
         2. getPluginToolRenderer(displayName)     插件表 ← propose 工具在这里命中
         3. agentFallbacks[agentType]              agent-type fallback
         4. null                                    fallback to DefaultResult
       │
   4.  renderer = agentRequestProposeRenderer
       (web/src/components/chat/tool-renderers/mcp/agent-request.tsx:53)
       │
   5.  renderer.renderResult(tool)
       -> parseEnvelope(tool)  解析 result JSON 信封
       -> <AgentRequestCard envelope={env} />
```

#### 5.0.4 为什么 propose renderer 不放在 built-in 表里

`internal/ui-sdk/src/tool-renderers/registry.ts:112-115` 有明确注释：

```ts
// NOTE: the Builder Mode *_propose tools (workspace_*_propose,
// prompt_*_propose) carry an interactive Approve/Reject card that reaches
// into app state, so their renderer is not bundled here - the host app
// registers it via registerToolRenderer() at boot.
```

**原因**：审批卡片要 reach into app state（调 `api.getAgentRequest` / `api.resolveAgentRequest` / `actions.sendMessage`），而 UI SDK 是平台无关的包，不能依赖 host app 的 API client 和 session store。所以把 renderer 留给 host app 自己注册。

#### 5.0.5 设计取舍

| 设计点 | 选择 | 理由 |
|---|---|---|
| 匹配方式 | **显式枚举 13 个工具名**，不做 `_propose` 后缀模糊匹配 | 防止未来其它 MCP 工具以 `_propose` 结尾被误识别；显式列表便于审计 |
| 注册时机 | **app 启动时一次性** | 渲染时 O(1) Map 查找；避免每条 tool_call 重新解析 |
| 订阅机制 | `useSyncExternalStore` 订阅 renderer 注册变化 | 让 lazy 注册的 renderer 落地后，已渲染的 skeleton 自动切回真实卡片 |
| 跨 agent 兼容 | `getToolDisplayName` 用三种正则统一剥离前缀 | CC / Codex / Goose 三种 MCP 命名格式都能识别 |
| Codex 拆包 | `unwrapExecuteDispatchName` 单独处理 `execute` wrapper | Codex 把 MCP 调用全包成 `execute({tool, server, arguments})`，需要先拆 |

> ⚠️ **维护警告**：新增 Builder Mode action 时，必须**同步**把对应 propose 工具名加到 `AGENT_REQUEST_PROPOSE_TOOLS` 数组（`web/src/plugins/index.ts:21-35`），否则前端会 fallback 到 `DefaultResult` 显示原始 JSON，审批卡片不出现。

### 5.1 API client

文件：`web/src/lib/api/client.ts`

```ts
// 第 1730-1731 行
async getAgentRequest(workspaceId, reqId): Promise<ApiAgentRequest>

// 第 1734-1744 行
async resolveAgentRequest(workspaceId, reqId, decision, reason?): Promise<ApiAgentRequest>
```

**注意**：client 在构造时已设置 `baseUrl = '/api'`（`client.ts:120`），所以方法内的路径 `/workspaces/...` 实际请求 `/api/workspaces/...`。

### 5.2 Tool Renderer：审批卡片

文件：`web/src/components/chat/tool-renderers/mcp/agent-request.tsx`

这是整个前端的核心。MCP 工具调用结果渲染走插件系统：

1. `web/src/plugins/index.ts:21-35` 把 `agentRequestProposeRenderer` 注册到 13 个 propose 工具名上
2. 当对话流里出现任意 propose 工具的 `tool_result`，UI 自动调 `agentRequestProposeRenderer.renderResult(tool)`
3. `renderResult` 调 `parseEnvelope(tool)`（`agent-request.tsx:39-51`）解析 MCP 文本结果里的 JSON 信封
4. 渲染 `<AgentRequestCard envelope={env} />`

**两种模式**（`agent-request.tsx:81-88`）：

```tsx
function AgentRequestCard({ envelope }) {
  const hasSessionProvider = useHasAgentSessionProvider()
  return hasSessionProvider
    ? <AgentRequestCardInteractive envelope={envelope} />  // 有会话上下文：可交互
    : <AgentRequestCardStatic envelope={envelope} />       // 无会话上下文（如 SharePage）：只读
}
```

> 💡 **设计要点**：分享页（无 AgentSessionProvider）也要能渲染历史审批卡片，但绝不能调 session hook（会抛错）。所以拆成两个组件，Static 完全用信封里的种子数据，不 fetch。

### 5.3 交互卡片状态机

`AgentRequestCardInteractive`（`agent-request.tsx:121-307`）：

| 状态 | UI | 行为 |
|---|---|---|
| 加载中（无 seed + 未 fetch 完） | "loading..." 占位 | `useEffect` 调 `api.getAgentRequest` |
| 加载失败（无 seed + fetch 错误） | 红色错误卡 | 显示 `loadFailed` 文案 |
| 加载失败（有 seed + fetch 错误） | 继续显示 seed 卡 | **降级不报错**——避免 CP 抖动时整卡片消失 |
| `pending` | Approve / Reject 按钮 | 点击 Reject 展开输入 reason 的表单 |
| `approved` | success-soft 徽章 | 只读 |
| `rejected` | destructive-soft 徽章 + reason | 只读 |
| `applied` | accent-soft 徽章 + applied_at | 只读 |

### 5.4 会话回流（关键）

resolve 成功后，前端**必须**往会话里发一条 `agent-sys` 系统消息（`agent-request.tsx:169-191`）：

```ts
async function resolve(decision, rejectReason?) {
  // ...
  const updated = await api.resolveAgentRequest(workspaceId, envelope.request_id, decision, rejectReason)
  setReq(updated)
  const body = decision === 'approved'
    ? `Request ${envelope.request_id} has been approved. Please call the matching apply tool with request_id="${envelope.request_id}" to finalize.`
    : `Request ${envelope.request_id} has been rejected.${reasonSuffix} Do not call the apply tool.`
  actions.sendMessage(`<agent-sys>${body}</agent-sys>`)
}
```

**为什么需要这一步**：agent 本身不知道用户什么时候表态了。前端通过往对话流里塞一条系统消息，触发 agent 的下一轮 LLM 调用，agent 读到这条指令后才会去调对应的 apply 工具。这条消息是 **HIL 流程的人工续跑信号**。

> ⚠️ **Review 检查点**：未来如果有其它触发 agent 续跑的机制（如 webhook），这条 `agent-sys` 消息可能变成重复触发。需要约定：每次 approved 状态只会被 apply 一次（靠 DB CAS 兜底）。

### 5.5 请求体详情渲染

`renderAgentRequestBody(req)`（`agent-request.tsx:209`）按 `req.kind` 分发到子渲染器：

文件：`web/src/components/chat/tool-renderers/mcp/agent-request-bodies.tsx`

每个 kind 有定制的可视化（schedule 显示 cron 表达式 + message，command 显示 name + template 等），未识别的 kind fallback 到 JSON preview。

---

## 六、端到端时序（含完整示例）

### 6.1 示例场景

用户对 agent 说："每天早上 9 点提醒我站会" → agent 调 propose → 用户在前端批准 → agent 调 apply → schedule 真正写入。

### 6.2 完整时序

```
┌──────┐  ┌──────────┐  ┌────────────┐  ┌──────────┐  ┌────────────┐  ┌──────┐
│ User │  │ Frontend │  │ CP (HTTP)  │  │ CP (MCP) │  │ CP (DB)    │  │ LLM  │
└──┬───┘  └────┬─────┘  └─────┬──────┘  └────┬─────┘  └─────┬──────┘  └──┬───┘
   │           │              │              │              │            │
   │ 1. "每天9点提醒站会" ──────────────────────────────────────────────►│
   │           │              │              │              │            │
   │           │              │              │              │      2. 决定调   │
   │           │              │              │              │      workspace_ │
   │           │              │              │              │      schedule_  │
   │           │              │              │              │      propose   │
   │           │              │              │              │            │   │
   │           │              │              │ 3. tool call ────────────►│
   │           │              │              │  workspace_schedule_propose │
   │           │              │              │  {cron:"0 9 * * *", msg:"站会"} │
   │           │              │              │              │            │   │
   │           │              │              │ 4. parse + createAgentRequest   │
   │           │              │              │              │           │   │
   │           │              │              │              │ 5. INSERT │   │
   │           │              │              │              │  status=  │   │
   │           │              │              │              │  'pending'│   │
   │           │              │              │              │ ◄─────────┤   │
   │           │              │              │              │            │   │
   │           │              │              │ 6. textResult ───────────►│
   │           │              │              │   {request_id, kind, label,  │
   │           │              │              │    payload, status:pending} │
   │           │              │              │              │            │   │
   │           │  7. tool_result 流式渲染（agent 把工具结果转发给前端）   │   │
   │           │ ◄───────────────────────────────────────────────────── │   │
   │           │              │              │              │            │   │
   │           │ 8. renderer 命中 workspace_schedule_propose              │   │
   │           │    parseEnvelope → <AgentRequestCardInteractive>         │   │
   │           │              │              │              │            │   │
   │           │ 9. useEffect → api.getAgentRequest                       │   │
   │           │ ─────────────►│             │              │            │   │
   │           │               │ 10. GET /api/workspaces/{id}/           │   │
   │           │               │     agent-requests/{reqId}             │   │
   │           │               │              │              │           │   │
   │           │               │ ──────────────────────────────────────► │   │
   │           │               │              │              │ SELECT *  │   │
   │           │               │              │              │ ◄─────────┤   │
   │           │               │ ◄─────────────────────────────────────  │   │
   │           │ ◄─────────────│              │              │           │   │
   │           │              │              │              │            │   │
   │ 11. 显示审批卡片（payload 详情 + Approve/Reject）                      │
   │ ◄─────────│              │              │              │            │   │
   │           │              │              │              │            │   │
   │ 12. 点击 Approve          │              │              │            │   │
   │ ─────────►│              │              │              │            │   │
   │           │ 13. api.resolveAgentRequest('approved')                    │   │
   │           │ ─────────────►│             │              │            │   │
   │           │               │ 14. POST /api/.../resolve                  │   │
   │           │               │   {decision:"approved"}                   │   │
   │           │               │              │              │           │   │
   │           │               │ 15. 鉴权 + 存在性 + status==pending 检查  │   │
   │           │               │              │              │           │   │
   │           │               │ 16. resolveAgentRequest CAS              │   │
   │           │               │ ──────────────────────────────────────► │   │
   │           │               │              │              │ UPDATE    │   │
   │           │               │              │              │ status=   │   │
   │           │               │              │              │ approved  │   │
   │           │               │              │              │ WHERE     │   │
   │           │               │              │              │ pending   │   │
   │           │               │              │              │ ◄─────────┤   │
   │           │               │ ◄─────────────────────────────────────  │   │
   │           │               │ 17. 返回更新后的 row 200                  │   │
   │           │ ◄─────────────│              │              │           │   │
   │           │              │              │              │            │   │
   │           │ 18. sendMessage(<agent-sys>Request approved. Call apply.  │   │
   │           │              │     </agent-sys>)                            │   │
   │           │ ──────────────────────────────────────────────────────────►│
   │           │              │              │              │            │   │
   │           │              │              │              │    19. agent │
   │           │              │              │              │     读到 sys │
   │           │              │              │              │     消息，决 │
   │           │              │              │              │     定调     │
   │           │              │              │              │     workspace_│
   │           │              │              │              │     schedule_ │
   │           │              │              │              │     apply    │
   │           │              │              │              │            │   │
   │           │              │              │ 20. tool call ────────────►│
   │           │              │              │  workspace_schedule_apply  │
   │           │              │              │  {request_id:"..."}        │
   │           │              │              │              │            │   │
   │           │              │              │ 21. getAgentRequest        │   │
   │           │              │              │     kind 校验              │   │
   │           │              │              │     status==approved 校验  │   │
   │           │              │              │              │           │   │
   │           │              │              │ 22. markAgentRequestApplied│   │
   │           │              │              │ ──────────────────────────►│   │
   │           │              │              │              │ CAS UPDATE│   │
   │           │              │              │              │ status=   │   │
   │           │              │              │              │ applied   │   │
   │           │              │              │              │ WHERE     │   │
   │           │              │              │              │ approved  │   │
   │           │              │              │              │ ◄─────────┤   │
   │           │              │              │              │            │   │
   │           │              │              │ 23. action.apply({         │   │
   │           │              │              │       workspaceId,         │   │
   │           │              │              │       userId,              │   │
   │           │              │              │       payload             │   │
   │           │              │              │     })                     │   │
   │           │              │              │     → 写 schedules 表     │   │
   │           │              │              │              │            │   │
   │           │              │              │ 24. textResult "Schedule   │   │
   │           │              │              │     created (id:xxx)" ───►│   │
   │           │              │              │              │            │   │
   │           │  25. apply 工具结果流式渲染（显示成功）                       │
   │           │ ◄───────────────────────────────────────────────────── │   │
   │           │              │              │              │            │   │
   │ 26. agent 回复："已为你创建每天 9 点的站会提醒"                          │
   │ ◄──────────────────────────────────────────────────────────────────── │   │
```

### 6.3 关键文件清单

**后端 HTTP 路由**：
- `control-plane/src/index.ts:63,313` — 路由注册入口
- `control-plane/src/routes/workspaces/agent-requests.ts` — 全部 HTTP 路由 + handler
- `control-plane/src/services/db/agent-requests.ts` — DB CRUD + CAS
- `control-plane/src/services/db/types.ts:474-485` — TS 类型
- `control-plane/migrations/001_init.sql:287-299` — DDL
- `internal/types/api.ts:1816-1833` — 共享 zod schema

**后端 MCP 工具**：
- `control-plane/src/mcp/tools/builder-mode/define-action.ts` — 核心：propose/apply 工具对注册
- `control-plane/src/mcp/tools/builder-mode/actions/schedule.ts` — schedule 域 action
- `control-plane/src/mcp/tools/builder-mode/actions/command.ts` — command 域 action
- `control-plane/src/mcp/tools/builder-mode/actions/config.ts` — config + prompt set 域
- `control-plane/src/mcp/tools/builder-mode/actions/skill.ts` — skill 域
- `control-plane/src/mcp/tools/builder-mode/actions/prompt.ts` — prompt library 域（global scope）
- `internal/types/builder.ts` — kind 常量定义

**前端**：
- `web/src/lib/api/client.ts:1730-1744` — API client 两个方法
- `web/src/lib/api/types.ts` — `ApiAgentRequest` 类型
- `web/src/components/chat/tool-renderers/mcp/agent-request.tsx` — 审批卡片（交互 + 只读）
- `web/src/components/chat/tool-renderers/mcp/agent-request-bodies.tsx` — 按 kind 渲染请求体
- `web/src/plugins/index.ts:21-35` — 插件注册（绑定 13 个 propose 工具名）

---

## 七、Review 检查点

### 7.1 设计边界

| 边界 | 说明 | 实现位置 |
|---|---|---|
| HTTP API 不触发业务副作用 | 只有 apply 工具的 `action.apply()` 回调真正写业务表 | `define-action.ts:133` |
| 业务副作用最多一次 | `markAgentRequestApplied` CAS 必须先于 `action.apply` | `define-action.ts:127-133` |
| 审批只能由 workspace 所有者操作 | `workspace.user_id === user.sub` 双端点都校验 | `agent-requests.ts:41,82` |
| 无权访问与不存在都返回 404 | 避免泄漏 workspace 存在性 | `agent-requests.ts:42,83` |
| pending 不可重复审批 | DB CAS `WHERE status = 'pending'` + handler 前置 409 | `agent-requests.ts:38,90-92` |

### 7.2 不变量（invariants）

1. **`status = applied` ⇒ 存在对应的业务副作用**：CAS 在副作用之前，所以 `applied` 状态等价于业务已执行。
2. **`kind` 在 request 生命周期内不变**：propose 写入后，apply 工具会重新校验 `req.kind === action.kind`，防止工具错配。
3. **`payload` 形状由 `kind` 决定**：propose 和 apply 用同一个 `action.payload` zod schema，存入和读取各 parse 一次，类型一致。
4. **`workspace_id` 在 request 生命周期内不变**：apply 工具校验 `req.workspace_id === workspaceId`，防止跨 workspace 越权。

### 7.3 已知风险与权衡

| 风险 | 权衡 | 当前选择 |
|---|---|---|
| 用户审批后 agent 不续跑 | 前端通过 `agent-sys` 系统消息触发 agent 下一轮 | 依赖前端正确发消息；如果前端崩了，请求停在 `approved` 状态，需要手动清理 |
| agent 重复调 apply | DB CAS 兜底，第二次返回 "already applied" 错误 | 已处理 |
| 双击 Approve | handler 前置 status 检查 + DB CAS 双保险 | 已处理 |
| 长时间停在 pending | 没有自动过期机制 | 当前靠用户主动 reject；如果未来需要 TTL，要加后台 job |
| payload schema 变更 | 旧 request 的 payload 可能不符合新 schema | apply 时 zod parse 失败会报错；用户需要 reject 后重新 propose |
| MCP 工具结果文本协议 | envelope 是非正式 JSON 协议，没有版本号 | 前端 `parseEnvelope` 容错性强；未来格式变更需双向兼容 |

### 7.4 排查清单

| 现象 | 可能原因 | 排查路径 |
|---|---|---|
| 卡片一直显示 loading | `api.getAgentRequest` 失败 + 无 seed | 看 Network 请求；检查 CP 日志 |
| Approve 后 agent 不动 | `agent-sys` 消息未发送 | 看前端 console；检查 `actions.sendMessage` 调用 |
| apply 报 "kind mismatch" | apply 工具和 propose 工具不是同一 action 注册的 | 检查 propose 工具名 → 反查 action kind → 比对 apply 工具 |
| apply 报 "no longer approved" | 已被其它 agent 调用 apply 了（并发） | 查 `agent_requests.applied_at` |
| Approve 返回 409 | 已被另一个客户端 approve / reject | 查 `agent_requests.resolved_at` 和 `status` |
| 卡片无按钮、只有徽章 | 没有 `AgentSessionProvider`（如 SharePage） | 这是设计行为，不是 bug |

### 7.5 扩展指引：新增一个 action

1. 在 `internal/types/builder.ts` 加 `BUILDER_KIND_*` 常量
2. 在 `control-plane/src/mcp/tools/builder-mode/actions/<resource>.ts` 用 `defineBuilderAction()` 定义新 action（payload schema + apply 回调）
3. 在对应 scope 的注册入口（`workspace.ts` / `global.ts`）调 `registerBuilderAction(server, workspaceId, action)`
4. （可选）在前端 `agent-request-bodies.tsx` 加按 kind 渲染的子组件
5. 在 `web/src/plugins/index.ts:21-35` 的 `AGENT_REQUEST_PROPOSE_TOOLS` 列表里追加新 propose 工具名

> 💡 **不需要改的**：HTTP 路由、DB 服务层、API client、审批卡片组件。所有 HIL 通用逻辑都被 `registerBuilderAction` 封装好了。

---

## 八、与现有文档的关系

| 文档 | 覆盖范围 | 与本文的关系 |
|---|---|---|
| `docs/wiki/control-plane/src/routes/workspaces/agent-requests.md` | 后端路由层（简略） | 本文是其超集，并补齐前后端链路 |
| `docs/wiki/control-plane/src/services/db/agent-requests.md` | DB 服务层（4 个函数） | 本文第 4.3 节与此重叠但更聚焦流程 |
| `skills/nap-api/references/operations/get-api-workspaces-id-agent-requests-reqId.md` | API 参考（GET） | 字段表权威来源，本文引用 |
| `skills/nap-api/references/operations/post-api-workspaces-id-agent-requests-reqId-resolve.md` | API 参考（POST /resolve） | 同上 |

本文定位为 **HIL 流程的端到端索引**，适合做集成或排查时从总览进入，需要字段子集细节时再点开 API 参考文档。
