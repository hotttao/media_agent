---
title: Builder Mode 实现原理（架构师向新人讲解版）
description: 配套 builder-mode 使用文档，按「代码地图 → 抓手拆解 → 端到端时序」逐层揭开 propose/apply 审批框架、cap 机制与 12 个 action 的实现细节。
created: 2026-07-07T09:57:12Z
updated: 2026-07-07T09:57:12Z
---

> 配套阅读：[docs/wiki/workspace/builder-mode.md](builder-mode.md)（产品视角，使用文档）。

为了方便新人从代码层面建立认知，本文按「**先骨架、再细节、最后拉通**」的顺序展开。先把 Builder Mode 的代码地图摊开看清每个模块住在哪个目录，再逐一拆解 cap 分发、`define-action` 框架、审批状态机、读工具集四个抓手，最后用端到端时序把「Agent 提议 → 用户批准 → 后端落地」串成一条完整的链路。

---

## 一、代码结构地图（顶层设计）

Builder Mode 不是某一个文件，而是一组**按 MCP 工具组织的纵向切片**——每个业务动作（schedule / command / skill / config / prompt）都有一个 action 描述符，后端把它编成一对 MCP 工具（`*_propose` + `*_apply`）。先把"骨架"看清楚。

### 1. 后端代码地图

| 角色 | 路径 | 作用 |
| --- | --- | --- |
| MCP 工具入口 | `control-plane/src/mcp/tools.ts` | 调度所有 MCP 工具注册，调用 `registerBuilderTools` |
| Builder 调度 | `control-plane/src/mcp/tools/builder-mode/index.ts` | 解析 `X-Builder` header，按 cap 分发到 workspace/global 注册器 |
| Action 框架 | `control-plane/src/mcp/tools/builder-mode/define-action.ts` | `defineBuilderAction` 描述符 + `registerBuilderAction` 把描述符展开成 propose+apply 两个 MCP 工具 |
| Workspace 工具集 | `control-plane/src/mcp/tools/builder-mode/workspace.ts` | 注册 12 个 workspace 范围内的 mutation action |
| Global 工具集 | `control-plane/src/mcp/tools/builder-mode/global.ts` | 注册 3 个账户级别的 mutation action（prompt library） |
| 读工具集 | `control-plane/src/mcp/tools/builder-mode/read.ts` | 注册 11 个纯读工具（`list_prompts` / `list_schedules` / `list_sessions` 等） |
| Action 描述符 | `control-plane/src/mcp/tools/builder-mode/actions/{schedule,command,skill,config,prompt}.ts` | 每个文件描述一组业务动作（create/update/delete 等） |
| 共享类型/Schema | `internal/types/builder.ts` | cap 枚举、kind 字符串常量、每个 action 的 zod payload schema（前后端共用） |
| 持久化层 | `control-plane/src/services/db/agent-requests.ts` | `agent_requests` 表的 CRUD + 状态机 CAS（pending → approved → applied） |
| HTTP 审批路由 | `control-plane/src/routes/workspaces/agent-requests.ts` | `GET /:id/agent-requests/:reqId`、`POST .../resolve`（前端卡片调用） |
| 业务服务层 | `control-plane/src/services/db/{schedules,commands,prompts,workspaces,...}.ts` | 各业务实体的 CRUD，被 action 的 `apply` 函数调用 |
| 任务队列 | `control-plane/src/lib/jobs.ts` | schedule action 通过 pgboss 注册/取消定时任务 |

### 2. 前端代码地图

| 角色 | 路径 | 作用 |
| --- | --- | --- |
| Cap 编辑器 | `web/src/components/workspace/BuilderCapsEditor.tsx` | workspace 设置里勾选 `workspace` / `global` 两个 cap |
| 审批卡片壳 | `web/src/components/chat/tool-renderers/mcp/agent-request.tsx` | 渲染每条 propose 工具结果，包含 Approve/Reject 按钮 + 状态切换 |
| Per-kind 渲染器 | `web/src/components/chat/tool-renderers/mcp/agent-request-bodies.tsx` | 按 `kind` 把 payload 拆成字段展示（schedule create/update、prompt set 等各有专属展示） |
| API client | `web/src/lib/api/*.ts`（自动生成） | `getAgentRequest` / `resolveAgentRequest` 调上述 HTTP 路由 |

### 3. 数据库表（`control-plane/migrations/001_init.sql`）

`agent_requests` 是整个 Builder Mode 的"暂存 + 状态机"中枢，每一行就是一笔待审批的改动：

| 列 | 类型 | 说明 |
| --- | --- | --- |
| `id` | uuid | 主键，propose 时返回给 Agent，apply 时回传 |
| `workspace_id` | uuid | 改动的归属 workspace（防跨 workspace apply） |
| `user_id` | uuid | 谁触发的 propose |
| `kind` | text | 业务分类标签，如 `builder.workspace.schedule.create` |
| `payload` | jsonb | **原始改动数据**——apply 时不再读 Agent 的输入，只从这里读 |
| `status` | text | `pending` / `approved` / `rejected` / `applied` |
| `reject_reason` | text | 拒绝时用户填的原因 |
| `created_at` / `resolved_at` / `applied_at` | timestamptz | 三个里程碑时间点 |

> 顶层设计一句话：**Builder Mode = 一组 MCP 工具（按 cap 分组注册）+ 一张 agent_requests 表（审批状态机）+ 一套 action 描述符（每个业务动作展成 propose/apply 双工具）**。Agent 看到的只是工具；用户看到的只是审批卡片；中间的全部靠这张表串起来。

---

## 二、核心抓手拆解（底层逻辑）

骨架看完，现在拆四个抓手。每个抓手都是一组相对独立的机制，理解它们就理解了 Builder Mode 80% 的代码。

### 抓手 1：Cap 分发——"按 X-Builder header 决定暴露哪些工具"

**位置**：`mcp/tools/builder-mode/index.ts` + `internal/types/builder.ts:parseBuilderHeader`

Builder Mode 不是一刀切——workspace owner 在 UI 勾选"我能改什么"（`workspace` / `global`），这个勾选状态以**逗号分隔字符串**的形式塞进 MCP server 的 `X-Builder` header。Agent 每次连 MCP，cp 端都读这个 header，按 cap 加载对应的工具集。

```typescript
// mcp/tools/builder-mode/index.ts:14
export function registerBuilderTools(server, workspaceId, headers) {
  const caps = parseBuilderHeader(headers.get('x-builder'))
  if (caps.length === 0) return  // 未启用 builder mode，一个工具都不注册

  // 任何 cap 开启都能用读工具——propose 之前 Agent 必须先 list_*
  registerBuilderReadTools(server, workspaceId)

  for (const cap of caps) {
    if (cap === 'workspace') registerWorkspaceCapTools(server, workspaceId)
    if (cap === 'global') registerGlobalCapTools(server, workspaceId)
  }
}
```

**关键设计**：
- **读工具先于 cap 注册**：只要 Builder Mode 开启，`list_prompts` / `list_sessions` 等 11 个纯读工具就到位——它们给 Agent 提供 propose 之前必需的上下文。
- **cap 是 token 级而非全局**：同一个 workspace 对不同 session / token 可以暴露不同 cap（实现上 cap 在 token 注入的 MCP 配置里）。
- **未知 cap 静默丢弃**：`parseBuilderHeader` 对未知 token 直接 `continue`，stale header 不会让整个工具集加载失败。

读工具的集合在 `read.ts:32 registerBuilderReadTools` 里集中声明——文件底部用 helper 把 list/get 的输出格式统一成 `<id> | "<name>" | <meta>` 的纯文本行（不用 JSON，节省 token；详细 schema 见各 helper 的 zod inputSchema）。

### 抓手 2：Action 描述符——"一个定义，展成两个工具"

**位置**：`mcp/tools/builder-mode/define-action.ts`（**整个 Builder Mode 最核心的 140 行**）

每个业务动作（创建 schedule、改 prompt、删除 skill）都按"**写一次，展两枚**"的模式定义：

```typescript
// 一个 action 描述符 = 一份业务定义
export const scheduleCreateAction = defineBuilderAction({
  kind: 'builder.workspace.schedule.create',  // DB tag，前端用它派发 per-kind 渲染器
  resource: 'schedule',                        // 工具名 slug（拼上前缀后 = workspace_schedule_propose）
  scope: 'workspace',                         // 或 'global'，决定要不要 workspace_ 前缀
  payload: ScheduleCreatePayloadSchema,        // zod schema，propose 和 apply 共用，**apply 会再次 parse**
  label: 'Create schedule',                   // 工具结果里的简短一行（rich 渲染交给 web）
  proposeDescription: '...',                   // 只对 *_propose 工具生效
  apply: async ({ workspaceId, userId, payload }) => { /* 真正的业务 */ },
})
```

注册器 `registerBuilderAction` 把这一份描述符展开成两个 MCP 工具：

```
workspace_schedule_propose   input = ScheduleCreatePayloadSchema
workspace_schedule_apply     input = { request_id: string }
```

**两个工具的核心约定**：

| 工具 | 干啥 | 关键约束 |
| --- | --- | --- |
| `*_propose` | 把 payload 写到 `agent_requests` 表（status=pending），返回 `request_id` | 写完就返回，没有副作用；返回里**附带 payload 镜像**让前端不用查 DB 就能渲染 |
| `*_apply` | 拿 `request_id` 查回原行，校验状态、CAS 占位、**用同一个 zod schema 重新 parse payload**，再跑 `apply` 函数 | 状态机是原子 CAS；payload 不接受 Agent 传，只接受 DB 里那一份 |

propose 流程（`define-action.ts:75`）：

```
raw input → payload.parse(raw)         // 第一次 schema 校验（防垃圾进库）
         → getWorkspace(workspaceId)
         → createAgentRequest({         // INSERT INTO agent_requests ... status='pending'
             workspace_id, user_id,
             kind: action.kind,
             payload,
           })
         → 返回 { request_id, kind, label, payload, status: 'pending' }
```

apply 流程（`define-action.ts:112`）——**这一段是审批安全模型的核心**：

```
raw { request_id }
  → getAgentRequest(id)                                  // 拿原行
  → if !req || req.workspace_id !== workspaceId → 404
  → if req.kind !== action.kind → "kind mismatch"        // 跨 action 防呆
  → if req.status !== 'approved' → "not approved"
  → markAgentRequestApplied(id)                          // CAS: approved → applied（占位）
    → if !claimed → "already applied?"                   // 并发只允许一个胜出
  → action.payload.parse(req.payload)                    // 第二次 schema 校验（防库被绕过）
  → action.apply({ workspaceId, userId, payload })       // 业务真正落地
  → 返回业务结果文本
```

**两段重复的 parse 不是冗余，是纵深防御**：第一次拦 Agent 工具调用，第二次拦"DB 数据被改/被回放/被注入"。Agent 永远不能在 apply 时塞一份不同的 payload 进去——它能传的只是一个 ID。

### 抓手 3：审批状态机——"一张表上的 CAS 锁"

**位置**：`services/db/agent-requests.ts` + `routes/workspaces/agent-requests.ts`

审批状态机只有四个状态：`pending → approved | rejected → applied`。每次跳转都用 SQL 的 `WHERE status = '<expected>'` 原子条件，避免"读 → 判断 → 写"的窗口期被并发抢。

```sql
-- resolveAgentRequest（pending → approved/rejected）
UPDATE agent_requests
SET status = $2, reject_reason = $3, resolved_at = now()
WHERE id = $1 AND status = 'pending'  -- CAS
RETURNING id;

-- markAgentRequestApplied（approved → applied）
UPDATE agent_requests
SET status = 'applied', applied_at = now()
WHERE id = $1 AND status = 'approved'  -- CAS
RETURNING id;
```

如果 `UPDATE` 影响的行数是 0，说明状态已经不是 expected（被并发改了或已经走完了），返回 null 让路由层把 `409 Conflict` 抛给前端。

**两段审批路径**：
- **前端卡片** → `POST /api/workspaces/:id/agent-requests/:reqId/resolve { decision, reason? }` → 调 `resolveAgentRequest`。
- **Agent 调 apply** → 内部 `markAgentRequestApplied` 抢占"applied"。两者都是 SQL CAS，但作用不同：前端的是"用户决定批不批"，Agent 的是"业务已落地"。

**关键设计**：
- `agent_requests.workspace_id` 在 apply 路由里强制比对——Agent 拿到的 `request_id` 必须属于它当前 workspace，否则 404。这阻止了"用别人 workspace 提的 ID 来 apply"的攻击面。
- `reject_reason` 只在拒绝时记录，批准为 null。前端用这个字段给 Agent 反馈为什么没批。
- `applied_at` 是审计字段；超管可以在前端看"谁、什么时候、批了哪笔"。

### 抓手 4：业务 apply 函数——"Action 描述符里唯一有副作用的代码"

**位置**：`mcp/tools/builder-mode/actions/*.ts`

每个 action 文件就是一组 `defineBuilderAction({ ... apply: async (...) => { ... } })`。apply 函数是被调用最频繁的"业务代码层"，但它的边界很清晰：**只接受 `(workspaceId, userId, payload)`，只跑该资源自己的 CRUD**，**不要**做跨资源编排。

值得抄笔记的几个设计细节：

**a) cron XOR run_at 的 schema 约束**（`builder.ts:81`）

```typescript
.refine((v) => Boolean(v.cron) !== Boolean(v.run_at), {
  message: 'Exactly one of `cron` or `run_at` must be provided.',
})
```

zod 的 `.refine` 在 propose 时先拦住，在 apply 时又拦一次（`action.payload.parse(req.payload)`）——结构上的保证，不是文档约定。

**b) Schedule apply 的"建后立即取消"回滚**（`actions/schedule.ts:39`）

```typescript
const schedule = await createSchedule({...})
try {
  const pgbossJobId = await jobs.enqueueScheduleTimer(schedule)
  if (pgbossJobId) await updateSchedule(schedule.id, { pgboss_job_id: pgbossJobId })
} catch (e) {
  await deleteSchedule(schedule.id)   // pgBoss 失败就回滚 DB
  throw new Error(`failed to register schedule timer (${e.message})`)
}
```

事务边界在数据库层不够（涉及 pgboss），就用"先建后删"补一层。

**c) Workspace 自有命令的 read-only 拦截**（`actions/command.ts:27 requireLocalCommand`）

template 提供的命令（`source === 'template'`）不是 workspace 自有的行，update/delete 必须显式拦截并提示用户去 disable / fork：

```typescript
const tpl = (await listWorkspaceCommands(workspaceId)).find(
  (c) => c.id === id && c.source === 'template',
)
if (tpl) {
  throw new Error(
    `"/${tpl.name}" is a template-provided command and is read-only. ...`,
  )
}
```

而 `command_set_disabled` 是这条规则的"逃生口"——它专门为 template command 服务，调的是 `setTemplateCommandDisabled`（一个独立表存的 workspace 级别偏好）。

**d) Skill enable 的去重 + 差异最小化**（`actions/skill.ts:40`）

```typescript
const requested = Array.from(new Set(payload.names))    // 先 dedupe
const { ids, missing } = await resolveNamesForWorkspace(workspaceId, requested)
if (missing.length > 0) return `No change — unknown skill name(s): ...`  // 名称错就明确告诉用户
const requestedIds = Array.from(ids.values())
const current = await skillRepo.getWorkspaceSkillIds(workspaceId)
const added = requestedIds.filter((id) => !current.includes(id))        // 算 diff
if (added.length === 0) return `No change — all ... already attached.`
const merged = Array.from(new Set([...current, ...requestedIds]))        // 保留原有 + 新增
await skillsService.attachToWorkspace(workspaceId, merged)
```

不直接覆盖全集，是为了**审批模型安全**——Agent 提议里只写了三个名字，apply 落地的就是这三个加原有，而不是覆盖。审批-落地一致性。

**e) Config update 的 "agent_type 触发容器重建"**（`actions/config.ts:47`）

```typescript
const { rebuilt } = await applyWorkspaceConfigUpdate(workspaceId, configPatch)
if (rebuilt) {
  return `Workspace config updated (${changed.join(', ')}); container rebuilding for agent_type change.`
}
```

`applyWorkspaceConfigUpdate` 不是简单 UPDATE，它会在 `agent_type` 变化时主动 rebuild 工作区的 runtime 容器——这是 Builder Mode 唯一一个会触发容器重建的动作，所以 web 端的审批卡片要在 prompt 里提醒用户。

---

## 三、端到端时序（拉通）

骨架和抓手都拆完，现在把"用户说一句话 → 改动落地"的全链路串起来。

### Step 1. 用户在对话里说"把刚才这套提问做成 `/review` 命令"

Agent 收到这条 prompt，模型先决定走哪条路——Builder Mode 是它的工具集之一，模型会调合适的 list_* 工具摸现状：

```
agent pod → POST /api/agents/<ws>/mcp
  header: X-Session-Token, X-Builder: workspace

mcp/server.ts:handleMcpRequest
  ├── resolveToken(token) → sessionId
  ├── registerTools(server, { workspaceId, sessionId, ..., headers })
  │     └── registerBuilderTools(server, workspaceId, headers)
  │           ├── parseBuilderHeader("workspace") → ["workspace"]
  │           ├── registerBuilderReadTools(server, workspaceId)   ← 抓手 1
  │           └── registerWorkspaceCapTools(server, workspaceId)  ← 注册 12 个 action
  └── agent pod 拿到工具清单 + list/get 系列

agent LLM → list_commands({ search: "review" })
           ← "no command named review in this workspace"
agent LLM → 决定调 workspace_command_propose
```

### Step 2. Agent 调 propose，落 agent_requests

```
agent LLM → workspace_command_propose
              { name: "review", type: "plain", prompt: "请基于以下对话生成 review：..." }

define-action.ts:75 registerBuilderAction.propose handler
  ├── CommandCreatePayloadSchema.parse(raw)              ← 抓手 2 第一次 parse
  ├── getWorkspace(workspaceId)
  └── createAgentRequest({
        workspace_id, user_id,
        kind: "builder.workspace.command.create",
        payload: { name, type, prompt },
      })
      └── INSERT INTO agent_requests (..., status='pending') RETURNING id

  → 返回 { request_id, kind: "builder.workspace.command.create",
           label: "Create slash command", payload, status: "pending" }
```

工具结果是一个**结构化 envelope**，前端识别 `kind` 后从 `agent-request-bodies.tsx` 选 `CommandCreateBody` 把 payload 拆成"命令名 / 类型 / prompt 内容"渲染：

```
┌─ Create slash command ──────── [Pending] ─┐
│  /review                                   │
│  type: plain                               │
│  prompt:                                   │
│  > 请基于以下对话生成 review：...           │
│                                           │
│  [Approve]  [Reject]                       │
└───────────────────────────────────────────┘
```

### Step 3. 用户点 Approve

```
web AgentRequestCard → POST /api/workspaces/<ws>/agent-requests/<reqId>/resolve
                        { decision: "approved" }

routes/workspaces/agent-requests.ts:76
  ├── workspace ownership check
  ├── existing.status === 'pending' 校验
  └── resolveAgentRequest(reqId, "approved")
        └── UPDATE agent_requests SET status='approved', resolved_at=now()
              WHERE id=$1 AND status='pending'
              RETURNING id

  ← { id, kind, payload, status: "approved", resolved_at: "..." }

卡片立刻显示 [Approved] 徽标，Agent 在对话流里看到这条工具结果。
```

### Step 4. Agent 调 apply（同一个 turn 或下一个 turn）

模型根据工具结果里 `status === 'approved'` 调对应的 apply：

```
agent LLM → workspace_command_apply { request_id: "..." }

define-action.ts:112 registerBuilderAction.apply handler
  ├── getAgentRequest(request_id)
  ├── req.workspace_id === workspaceId        ← 抓手 3 跨 workspace 防呆
  ├── req.kind === "builder.workspace.command.create"  ← 跨 action 防呆
  ├── req.status === "approved"
  ├── markAgentRequestApplied(id)             ← CAS approved → applied（抢占）
  │     └── UPDATE ... WHERE id=$1 AND status='approved'
  │     RETURNING id
  │     ← 1 row affected → claimed=true
  ├── CommandCreatePayloadSchema.parse(req.payload)   ← 抓手 2 第二次 parse
  └── commandCreateAction.apply({ workspaceId, userId, payload })
        └── createWorkspaceCommand({ workspace_id, user_id, name, type, prompt_id: null, content })
              └── INSERT INTO workspace_commands ...

  → 返回 "Slash command "/review" created (id=<id>)."

agent_requests.status 现在是 'applied'，前端卡片可显示 [Applied]
```

如果用户在 Step 3 之后**又调了一次** apply（手抖 / 网络重试），CAS 会让第二次返回 `claimed=false`，路由层把"already applied?"错误文本吐回 Agent，不会重复建命令。

### Step 5. Reject 路径（流程对照）

```
用户点 Reject
  → POST .../resolve { decision: "rejected", reason: "名字改成 /review-pr" }

resolveAgentRequest(id, "rejected", reason)
  └── UPDATE ... SET status='rejected', reject_reason='名字改成 /review-pr'
  └── 卡片显示 [Rejected] + 拒绝原因
  └── Agent 看到工具结果里的 reject_reason，改 payload 再 propose 一次
```

reject 之后整笔 request 永远不会再被 apply——状态机没有 `rejected → approved` 的转换，`apply` 路由会直接 409。

### 时序图

```
┌──────┐    ┌─────┐    ┌──────┐    ┌──────┐    ┌─────┐    ┌──────┐
│Agent │    │ cp  │    │ pg   │    │ web  │    │ user│    │ pgb  │
└──┬───┘    └──┬──┘    └──┬───┘    └──┬───┘    └──┬──┘    └──┬───┘
   │  list_*   │          │           │           │          │
   │──────────►│──────────►           │           │          │
   │           │          │           │           │          │
   │  propose  │          │           │           │          │
   │──────────►│ INSERT pending ─────►│           │          │
   │           │          │           │           │          │
   │           │          │           │  卡片渲染  │          │
   │           │          │           │◄──────────│          │
   │           │          │           │           │          │
   │           │          │           │  approve  │          │
   │           │          │           │◄──────────│          │
   │           │          │  UPDATE approved ────►│          │
   │           │          │           │           │          │
   │  apply    │          │           │           │          │
   │──────────►│ SELECT + CAS applied ─►          │          │
   │           │          │           │           │          │
   │           │  INSERT workspace_commands ─────►│          │
   │           │  enqueueScheduleTimer（仅 schedule 类）─────►│
   │           │          │           │           │          │
   │  text     │          │           │  [Applied]│          │
   │◄──────────│           │           │◄──────────│          │
```

---

## 四、常见踩坑与防御点速查

新人 review 时可以重点看这几条——它们都是出 bug 的高发位置：

| 位置 | 易错点 | 代码层防御 |
| --- | --- | --- |
| `define-action.ts:75` propose | 忘了 parse 直接写库 → 垃圾进 `payload` | 强制 `action.payload.parse(raw)` 在 createAgentRequest 之前 |
| `define-action.ts:112` apply | Agent 想偷换 payload | 只读 `req.payload`，不接受 Agent 输入；schema 二次 parse |
| `agent-requests.ts:52` markAgentRequestApplied | 并发 apply 双倍执行 | `UPDATE ... WHERE status='approved'` CAS；rowCount==0 视为失败 |
| `routes/workspaces/agent-requests.ts:90` resolve | 用户多点几次按钮 | 路由层先 `existing.status !== 'pending'` 检查；DB 层再 CAS |
| `actions/schedule.ts:39` | pgboss 注册失败留下空 schedule | try/catch + 失败时 `deleteSchedule(schedule.id)` 回滚 |
| `actions/command.ts:27` requireLocalCommand | Agent 想去改 template command | 显式查找 template 命令并抛"read-only / fork instead"错误 |
| `actions/skill.ts:42` resolveNamesForWorkspace | 名字拼错静默忽略 | `missing` 数组必须非空才 return；否则 return 明确错误信息 |
| `actions/config.ts:21` provider 校验 | provider_id 传错/不可见 | 调 `getProviderForUser(id, userId)` 校验可见性，否则 throw |
| `read.ts:535` get_session_export_urls | URL token 长期有效泄露 session | 默认 TTL 600s（10 分钟），最大 1 小时；URL 即 bearer token |

---

## 五、给新人 review 时的一句话总结

> **Builder Mode 的本质 = 把"Agent 在工具调用里塞 raw payload 直接生效"换成"Agent 塞 ID + 后端读 DB 里已审批的原 payload 再生效"。**
>
> 整个代码的核心——`define-action.ts` 那 140 行——就是这个替换的实现：propose 时把 payload 冻结到 `agent_requests.payload`，apply 时只接受 ID，然后用 zod schema 二次校验 + CAS 抢占落地。读工具是给 Agent 提供 propose 前上下文的辅助层；cap header 是给 owner 暴露工具集粒度的开关；per-kind 渲染器是把审批卡片做成"字段化的字段"，而不是吐一段裸 JSON 给用户看。
>
> 加一个新 Builder Mode 动作的标准步骤（按这个顺序改三处就够了）：
> 1. `internal/types/builder.ts` 新增 `BUILDER_KIND_XXX` + `XxxPayloadSchema`
> 2. `mcp/tools/builder-mode/actions/xxx.ts` 写 `defineBuilderAction({...})`
> 3. 在 `workspace.ts` 或 `global.ts` 里 `registerBuilderAction(server, workspaceId, xxxAction)`
> 4.（可选）在 `agent-request-bodies.tsx` 加 per-kind 渲染器，让审批卡片字段化
>
> 不用改状态机、不用改审批路由、不用改数据库——这就是这套框架的扩展性边界。