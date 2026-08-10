---
title: Teamwork 实现原理（架构师向新人讲解版）
description: 配套 teamwork 使用文档，先走一个端到端示例让读者看清"teamwork 到底在做什么"，再回头拆任务级可见性 / 自动共享目录 / MCP 反查 / 协作时间线四个抓手的实现。
created: 2026-07-07T09:50:59Z
updated: 2026-07-18T06:30:00Z
---

> 产品视角使用文档尚未迁移到本目录。

本文先带你走一个**具体示例**，让你直观感受 teamwork 在做什么、解决了什么问题；示例走完后，我们再回头拆背后的数据模型与四个核心实现抓手。

---

## 一、先讲一个示例：Alice 协调 Bob 和 Carol 完成季度报告

### 场景设定

- **Alice** 是团队领导，她有一个自己的 workspace（agent）叫 `alice-coord`，是这次协作的「协调员」。
- **Bob** 是研究员，workspace 叫 `bob-research`，私有。
- **Carol** 是数据分析师，workspace 叫 `carol-analysis`，私有。
- Bob 和 Carol 平时互相看不见对方的 workspace（私有隔离）。
- 目标：让 Alice 的协调员 agent 临时能"看见"Bob 和 Carol，分派子任务，最后汇总成一份 Q3 季度报告。

> ⚠️ 关键问题：Alice 的协调员 agent 平时只能看见自己 workspace 的资源；Bob / Carol 的 workspace 是私有的；三人之间没有共享文件目录。teamwork 就是来解决这三件事的。

### Step 1. Alice 创建一个 teamwork 任务

Alice 在 UI 上点「新建任务」，填入：

```
name: "Q3 季度报告"
brief: "让 Bob 调研行业动态，Carol 分析内部数据，汇总成报告"
coordinator_workspace_id: alice-coord
```

请求：`POST /api/teamwork` -> `routes/teamwork.ts:152`

系统做了三件事：
1. **写 task 主表**：`teamwork_tasks(id, owner_user_id=alice, coordinator_workspace_id=alice-coord, afs_share_id=null)`
2. **创建一个共享 AFS 目录**：路径形如 `/mnt/afs/team-<taskId>/`，所有参与者都会挂载到这个目录
3. **把这个目录挂到 Alice 自己的 workspace**：从此 Alice 的 agent 能在 `/mnt/afs/team-<taskId>/` 读写文件

返回 `ApiTeamworkTask`，前端跳到任务详情页。

### Step 2. Alice 添加 Bob 和 Carol 作为成员

Alice 在详情页加成员：

```
POST /api/teamwork/:id/participants { workspace_id: bob-research }
POST /api/teamwork/:id/participants { workspace_id: carol-analysis }
```

每次添加成员，系统做两件事（`routes/teamwork.ts:294-351`）：

1. **写成员表**：`teamwork_participants(task_id, workspace_id)` -- 这是"task 能看见谁"的权威表
2. **自动把共享目录挂到成员 workspace**：Bob 的 workspace 现在也有 `/mnt/afs/team-<taskId>/`，权限 `read_write`

成功后，三人共享同一份目录。这是 teamwork 的"自动共享目录"能力。

### Step 3. Alice 跟协调员 agent 聊天，下达任务

Alice 在任务详情页的协调员会话面板发消息：

```
"让 Bob 调研行业动态，让 Carol 分析内部销售数据，结果都放到 /mnt/afs/team-xxx/ 下，最后你综合两份报告写一个总结。"
```

请求：`POST /api/teamwork/:id/chat { message, mode: 'stream' }` -> `routes/teamwork.ts:483-509`

这个请求**复用普通 chat 流**，但多传一个 `taskId`（`dispatchChatTurn.ts:38` 透传）：

```
dispatchChatTurn({ workspace, body, acceptHeader, callerUserId, taskId })
  -> executeChat({ ..., taskId })
     ├── ensureWorkspaceRunning(alice-coord)
     ├── session 已存在 -> ensureTokenForSession；否则 mintToken
     └── createInterceptedSSEResponse
           ├── 持久化 session + messages
           └── onNewSession(taskId) -> addTeamworkSession(taskId, sid, 'coordinator', null)
                ↑ 把"协调员 session 属于这个 task"记下来
```

> 💡 这里就开始回答「Agent 怎么知道自己在哪个 task 里」-- 答案是 session 创建时就在 `teamwork_sessions` 表里登记了 `(task_id, session_id, role='coordinator')`。后续 MCP 调用靠这张表反查。

### Step 4. 协调员 agent 调 call_agent 分派给 Bob 的 agent

Alice 的协调员 agent（在 SSE 流里）看到这条指令后，决定调 `call_agent` 工具：

```json
{
  "tool": "call_agent",
  "input": {
    "slug": "bob-research",
    "prompt": "请调研 Q3 行业动态，把报告写到 /mnt/afs/team-xxx/bob-report.md",
    "mode": "async"
  }
}
```

这条 tool_call 通过 MCP 协议发到 control-plane。**关键步骤在 `mcp/server.ts:12-62`**：

```
x-session-token
  -> resolveToken                  (lib/session-token.ts:52)
  -> 比对 token 绑定的 workspace 与 X-Workspace-ID  (防 token 跨用户冒用)
  -> sessionId
  -> findTaskBySession(sessionId)  (db/teamwork.ts:250)
  -> taskId
  -> getTeamworkTask(taskId) 校验 coordinator_workspace_id === workspaceId
  -> registerTools(server, { workspaceId, sessionId, taskId, ... })
```

注册工具时把 `taskId` 透传给 `call_agent` 工具。这就是 MCP 反查 task context 的完整链路。

进入 `call_agent` 实现（`agent.ts:566`）：

```ts
let target = await resolveWorkspaceBySlug(slug, callerWorkspace.user_id)
// ↑ 普通查找：只在 Alice 自己拥有的 workspace 里找。Bob 的私有 ws 找不到。

if (!target && taskId) {
  target = await resolveRosterMemberBySlug(slug, callerWorkspace.user_id, taskId)
  // ↑ 二次回退：在 task 的成员表里找。Bob 在 teamwork_participants 里，命中。
}
```

这就是 teamwork 的"任务级可见性" -- **不是字段，是查找时多走一步 SQL**。Bob 的 workspace 平时对 Alice 不可见，但在 task 上下文里，Alice 能通过 slug 找到它。

然后 control-plane 在 Bob 的 workspace 上发起一次新 chat，把 prompt 传过去。Bob 的 agent 开始跑。

### Step 5. Bob 的 agent 在共享目录里写报告

Bob 的 workspace `bob-research` 现在挂了 `/mnt/afs/team-<taskId>/`。Bob 的 agent 跑的时候：

- 读：`/mnt/afs/team-<taskId>/` 下的文件（如果 Carol 先写了的话）
- 写：把调研报告写到 `/mnt/afs/team-<taskId>/bob-report.md`

由于所有成员都挂了同一份 AFS share，Bob 的写入会通过 fuse / 远端 runner 同步给所有人。这是跨成员协作的文件层。

> ⚠️ **跨环境（BYOI）情况**：如果 Bob 的 workspace 在另一个 k8s 集群，AFS 操作走 `afs.ts:30 callRemote`，逻辑在 `env-runner-k8s/src/afs-control.ts`。对成员来说透明。

### Step 6. Alice 让协调员综合两份报告

Bob 和 Carol 都跑完后，他们的 `call_agent` tool 返回结果给 Alice 的协调员 session（以 tool_result 形式）。

Alice 看到 Bob 和 Carol 的 sub-session tab（由 `TeamworkSection.tsx:1021` 的 `deriveMemberSessions` 从 tool_result 里提取 `session_id` 渲染出来）。她可以点开每个 tab 看完整的 sub-session 对话。

如果她想让协调员综合，继续在协调员会话里发：

```
"两份报告都好了，你读 /mnt/afs/team-xxx/bob-report.md 和 carol-report.md，综合写一个最终总结到 final.md"
```

协调员 agent 读两个文件（fuse 直接读，因为挂载在 Alice workspace） -> 写 final.md -> 完成。

### Step 7. Alice 删除任务

```
DELETE /api/teamwork/:id -> routes/teamwork.ts:238
```

系统做几件事：

1. `teardownTeamworkShare(shareId)`：
   - `revokeDir(env, dir.id, access)` -- 强制 unmount 所有成员的挂载
   - `deleteAfsShare(shareId)` -- 删 AFS share 行
2. `deleteTeamworkTask(taskId)` -- CASCADE 清掉 `teamwork_participants` 和 `teamwork_sessions`

效果：
- Alice / Bob / Carol workspace 里的 `/mnt/afs/team-<id>/` 全部失效
- AFS 卷回收
- **Bob / Carol 的 session 行还在**（sessions 表保留），他们可以继续在自己 workspace 聊
- 下次该 session 再请求 MCP 时 `findTaskBySession` 返回 `null` -> 自动回退到普通 workspace 模式

---

## 二、示例带出来的 4 个核心问题

走完一遍示例，你应该能感觉到 teamwork 要解决的就是这四件事：

| # | 问题 | 一句话答案 | 实现抓手 |
|---|---|---|---|
| Q1 | Alice 怎么"临时"看见 Bob 的私有 workspace？ | 不是改字段，是 `call_agent` 查找失败时多走一步 SQL 回退到 task 成员表 | 任务级可见性 |
| Q2 | Alice / Bob / Carol 怎么交换文件？ | 任务创建时自动 provision 一个 AFS share，加成员时自动挂到成员 ws，删任务时强制回收 | AFS 生命周期 |
| Q3 | Agent 调工具时怎么知道自己在哪个 task？ | session_token -> session_id -> task_id 三 hop 反查，注册工具时透传 taskId | MCP 反查 |
| Q4 | Alice 怎么看到协作进度？ | 不订阅 member session 实时事件，从协调员 session 里的 tool_call / tool_result 反推泳道 | 时间线 UI |

下面四节就是这四个抓手的实现细节。每节都标了对应示例里的 Step。

---

## 三、数据模型（三张表 + 一个引用）

DDL 在 `control-plane/migrations/001_init.sql`，全文三张表：

```
teamwork_tasks ──┬── afs_share_id (FK) ──> afs_shares
                 └── coordinator_workspace_id (FK) ──> workspaces

teamwork_participants:
  (task_id, workspace_id) ──> 成员多对多

teamwork_sessions:
  task_id (FK) ──> teamwork_tasks
  session_id (FK) ──> sessions   ← 注意顺序敏感
  role: 'coordinator' | 'member'
  parent_session_id ──> sessions (可空，暂未使用)
```

### 三个容易踩坑的细节

- **FK 顺序敏感**：`teamwork_sessions.session_id` 同时是 PK 与 FK，指向 `sessions(id)`。所以 INSERT `teamwork_sessions` 必须发生在 sessions 行 commit 之后。这就是 `agent.ts:267-272` 里 `onSessionPersisted` 钩子挂在 `createSession` 队列任务之后的原因。
- **cascade 方向**：`teamwork_tasks` 删除 -> `teamwork_participants`、`teamwork_sessions` 都 `CASCADE`；`afs_share_id` 用 `SET NULL`。删 task 时数据库自动清成员与会话登记，但 AFS 卷由应用层单独事务回收（见 §四.2）。
- **`parent_session_id` 当前未填充**：`agent.ts:649` 注释「we don't carry caller session_id by design」。未来要画「coord -> member -> sub-member」递归树时再回填。

---

## 四、四个抓手的实现细节

### 抓手 1：任务级可见性 -- `call_agent` 的二次回退

**对应示例**：Step 4，Alice 的协调员 agent 通过 slug `bob-research` 找到 Bob 的私有 workspace。

**入口**：`mcp/tools/agent.ts:519` 的 `registerAgentTools(server, workspaceId, taskId)`。

`taskId` 的来路（`mcp/server.ts:12-62`）：

```
x-session-token
  ↓ resolveToken                          ← lib/session-token.ts:52
session_id
  ↓ findTaskBySession                     ← db/teamwork.ts:250
task_id
  ↓ getTeamworkTask (校验 coordinator_workspace_id === workspaceId)
  ↓                                       ← mcp/server.ts:51-60
registerTools(server, { workspaceId, sessionId, taskId, ... })
```

回退逻辑（`agent.ts:580-585`）：

```ts
let target = await resolveWorkspaceBySlug(slug, callerWorkspace.user_id)
if (!target && taskId) {
  target = await resolveRosterMemberBySlug(slug, callerWorkspace.user_id, taskId)
}
```

`resolveRosterMemberBySlug` 的边界（`db/teamwork.ts:164-196`）：

- 只匹配 caller 自己的 workspace
- 只匹配 `teamwork_participants` 表里有 `task_id` 的
- 别人的私有 workspace 永远不会通过这条放宽

> 💡 这就是「任务级可见性」的全部真相：**不是字段，是查找时多走一步 SQL**。两条 defensive check 必须记住：`mcp/server.ts:35` 防 token 跨用户冒用；`mcp/server.ts:51-60` 防 stale task 状态。

### 抓手 2：自动共享目录 -- AFS 生命周期

**对应示例**：Step 1 自动挂载给 Alice，Step 2 自动挂载给 Bob / Carol，Step 7 自动回收。

`services/teamwork.ts` 的 4 个函数对应 4 种动作：

| 函数 | 调用点 | 动作 |
|---|---|---|
| `provisionTeamworkShare(task)` | `routes/teamwork.ts:152` 创建任务后 | 建 AFS 目录、写 afs_shares、挂到 coordinator、记 share_id |
| `mountTeamworkShareForMember(shareId, taskId, wsId)` | `routes/teamwork.ts:339` 添加成员后 | 挂到目标 workspace、写成员关系 |
| `unmountTeamworkShareForMember(...)` | `routes/teamwork.ts:377` 移除成员 | 清成员关系、unmount workspace |
| `teardownTeamworkShare(shareId)` | `routes/teamwork.ts:260` 删除任务 | revokeDir（强制 unmount）、删 afs_shares 行 |

**命名前缀固定** `team-<taskId>`（`services/teamwork.ts:23`），所有成员挂载路径一致。

**3 个 rollback 行为**：

- 创建 task 时 `provisionTeamworkShare` 失败 -> `routes/teamwork.ts:172` 直接 `deleteTeamworkTask`，不留半成品
- 添加成员时挂载失败 -> `routes/teamwork.ts:342` 立即回滚刚插入的 participant 行
- 移除成员时 unmount 抛错 -> 仍然吃掉（`teamwork_participants` 是「能否看见 share」的权威；workspace pod 已停、手动 unmount 等场景 fuse 抛错没意义，幂等尝试才正确）

**跨环境（BYOI）**：`afsEnvForWorkspace(workspaceId)` 返回 `isBuiltin=false` 时，`afs.ts:30 callRemote` 把操作 ship 到目标环境的 runner（`env-runner-k8s/src/afs-control.ts`）。Coordinator / member 可能分布在不同环境。

### 抓手 3：MCP 反查 task context -- session_token 是钥匙

**对应示例**：Step 4，agent 调 call_agent 时 control-plane 通过 session_token 自动拿到 taskId。

整条链有三段：

**段 1：mint token（dispatch 侧）** -- `lib/session-token.ts:27`

- 新会话：`mintToken({ workspaceId })` -> `session_id` 留空，等 SDK `session.started` 触发 `bindSession`
- 续会话：`ensureTokenForSession(workspaceId, sessionId)` -> 沿用旧 token，跨 turn 稳定
- sub-agent 调 `call_agent` 时再 `mintToken({ workspaceId: target.id })` 一次 -- 这是为 sub-session 自己开的，与 caller 的 token 互不相关

**段 2：threading（agent pod 内）** -- `chat/executeChat.ts:98` 把 `sessionToken` 写进 `/chat` body，agent 透传到 MCP 客户端，作为 `X-Session-Token` header。

**段 3：reverse resolve（MCP handler）** -- `mcp/server.ts:30-62`

```
x-session-token
  -> resolveToken
  -> 比对 workspace_id 与 X-Workspace-ID（防 token 跨用户冒用）
  -> sessionId
  -> findTaskBySession(sessionId)
  -> getTeamworkTask(taskId) - 校验 coordinator 关系
  -> 注册工具时把 taskId 透传
```

> 💡 **为什么这样设计**：Agent SDK 不需要懂「session 属于哪个 task」；agent 重启也不丢 task 身份（MCP 入口处再校一次）。用「中间表示」（session_token）解耦 agent 和 task 的强耦合。

**段 4：登记 member session**（`agent.ts:647-655`）-- `runSubAgentTurn` 时把 sub-session 写进 `teamwork_sessions`：

```ts
onSessionPersisted: taskId
  ? (sid) => addTeamworkSession(taskId, sid, 'member', null)
  : undefined
```

`addTeamworkSession` 不能早于 `createSession`，所以钩子挂在 `session.started` 处理里的 `queue.run(... createSession ...)` **之后**调用（`agent.ts:267-272`）。FK 顺序敏感，§三已提过。

### 抓手 4：协作时间线 -- 从 tool 调用反推泳道

**对应示例**：Step 6，Alice 在详情页看到 Bob / Carol 的 sub-session tab 和俯瞰泳道图。

**入口**：`web/src/components/teamwork/TeamworkTimeline.tsx:159` 的 `deriveOrchestrationModel`。

**输入**：

- `coordMessages`：协调员 session 的消息流（含 `tool_call` 和 `tool_result` block）
- `candidates`：roster 候选名单（解决 slug -> 工作区显示名）

**提取规则**：

- 工具名正则 `/(^|[^A-Za-z0-9_])call_agent$/` -- 容忍多种 MCP 命名变体（`mcp__tos-platform__call_agent`、`Tool: tos-platform/call_agent` 等）
- 同理匹配 `get_agent_result`，用来把 background 的响应 backfill 到时间线
- `args.session_id` 存在即 `isContinuation`，与同 session 前一次事件 share 同一行
- `parsed.session_id` 用来开新 lane，slug 用来 lookup 显示名

**输出**：`OrchestrationModel { columns, laneOrder, laneLabels, events, connectors }`，喂给 SVG 的水平 lane + 垂直 connector。

> ⚠️ **一个易错的设计选择**：时间线**不订阅成员 session 实时事件**，只走「coord 聊过的 tool 调用」这一条线。成员 session 文本通过 `MemberSessionPanel`（`TeamworkSection.tsx:1311`，标记 `readonly`）独立展示。这是「不要把多 Agent 协作画成节点连线」原则的体现。**详情页右侧就是若干个独立的 chat tab**，时间线只是俯瞰；想看具体一次 sub-agent 的对话，切到对应 tab 读真消息。

---

## 五、端到端时序（拉通对齐）

把上面四块拼起来走一次完整生命周期。每步都指明文件 + 行号，新人可以照着在 IDE 里点开。**注意这个时序就是 §一示例的技术化版本，两相对照看**。

### Step 1. 创建任务

```
Web UI (CreateTaskDialog)
  -> POST /api/teamwork
       body: { name, brief, coordinator_workspace_id }
  -> routes/teamwork.ts:152
       ├── getWorkspace 检查 owner + status=running
       ├── createTeamworkTask -> DB INSERT
       └── provisionTeamworkShare(task)
              ├── afsEnvForWorkspace(coordinator)
              ├── ensureDefaultFs(env)
              ├── createDir(env) -> { id, accessKey }
              ├── createAfsShare(name='team-<id>', dir, access)
              ├── mountAtWorkspace(coordinator, dir, access, name)
              ├── addAfsShareMember(coordinator, 'read_write')
              └── updateTeamworkTask(id, { afs_share_id })
       ← ApiTeamworkTask
```

### Step 2. 加成员（自动 mount）

```
POST /api/teamwork/:id/participants { workspace_id }
  -> routes/teamwork.ts:294-351
       ├── task ownership check
       ├── ws 必须 callable（自己 / 别人 public）
       ├── ws.status === 'running'
       ├── addTeamworkParticipant(taskId, wsId)
       │     ← INSERT ... ON CONFLICT DO NOTHING
       └── mountTeamworkShareForMember(shareId, taskId, wsId)
              ├── getAfsShareById
              ├── mountAtWorkspace(env, wsId, dir, access, name)
              └── addAfsShareMember(shareId, wsId, 'read_write')
```

成功后成员工作区出现 `/mnt/afs/team-<id>/`。

### Step 3. 协调员发起 chat（建立 task context）

```
Web UI (CoordSessionPanel)
  -> POST /api/teamwork/:id/chat  body: { message, mode? }
  -> routes/teamwork.ts:483-509
       ├── task ownership + coord 工作区存在性
       └── dispatchChatTurn({ workspace, body, acceptHeader, callerUserId, taskId })

dispatchChatTurn.ts:38
  -> resolveChatMode + executeChat({ ..., taskId })

executeChat.ts:59
  ├── ensureWorkspaceRunning（cold start 预算 ~90s，503 on fail）
  ├── session 已存在 -> ensureTokenForSession；否则 -> mintToken({ workspaceId })
  ├── POST agent /chat (24h hard cap)
  └── createInterceptedSSEResponse
        ├── 持久化 session + messages
        ├── onNewSession(taskId) -> addTeamworkSession(taskId, sid, 'coordinator', null)
        └──   ↑ 仅在 sid 首次 reveal 时触发，幂等
```

> ⚠️ 旧会话没注册过的，executeChat 还会补一刀（`executeChat.ts:168-175`）-- 这样即便 cp 节点重启、session 已经被别的容器接住，task 关联也能恢复。

### Step 4. 调用 call_agent 时 task scope 自动生效

```
agent pod -> POST /api/agents/<ws>/mcp
  header: X-Workspace-Id, X-Session-Token

mcp/server.ts:12 handleMcpRequest
  ├── resolveToken(token) -> sessionId
  ├── if sessionId -> findTaskBySession(sessionId) -> taskId
  ├── 校验 coordinator 关系
  └── registerTools(server, { workspaceId, sessionId, taskId, ... })

agent.ts:566 call_agent(slug, prompt, mode)
  ├── resolveWorkspaceBySlug -> target
  ├── if !target && taskId -> resolveRosterMemberBySlug -> target   ← 抓手 1
  ├── session 已存在 -> ensureTokenForSession(target.id, sid)
  ├── session 不存在 -> mintToken({ workspaceId: target.id })
  ├── POST target /chat { message, session_id?, session_token }
  └── runSubAgentTurn
        ├── onSessionPersisted（taskId ?）：
        │     在 sessions 行 commit 后才 INSERT teamwork_sessions(role='member')
        │     ← FK 顺序敏感点
        ├── broadcastPlugin ── cp-reconnect 仍可观察 sub-session
        └── resultPromise -> tool result 回 coord
```

前端拿到 `tool.result = JSON.stringify({ session_id, status, text })`，由 `TeamworkSection.tsx:1021` 的 `deriveMemberSessions` 把 `session_id` 转成一个 member tab。

### Step 5. 协同写文件 -- AFS 层

所有成员 workspace 都挂了 `/mnt/afs/team-<taskId>/`。

- 写：`/mnt/afs/team-<taskId>/report.md`
- 其他成员的 sub-agent 用同一绝对路径读；fuse 把写请求广播到源 share 所在环境

跨环境（BYOI）走 `afs.ts:30 callRemote`，逻辑在 `env-runner-k8s/src/afs-control.ts`。

### Step 6. 任务结束

```
DELETE /api/teamwork/:id
  -> routes/teamwork.ts:238
       ├── task ownership
       ├── if task.afs_share_id -> teardownTeamworkShare(shareId)
       │     ├── revokeDir(env, dir.id, access) ── 强制 unmount 所有成员
       │     └── deleteAfsShare(shareId)
       └── deleteTeamworkTask(taskId) ── CASCADE 清 participants + sessions
```

效果：

1. 成员 workspace 的 `/mnt/afs/team-<id>` 路径失效
2. AFS 卷回收
3. `sessions` 行保留（session 仍可在原 workspace 继续聊），但 `teamwork_sessions` 的登记行消失
4. 下次该 session 再请求 MCP 时 `findTaskBySession` 返回 `null` -> 自动回退到普通 workspace 模式

---

## 六、要点小结（owner 视角的检查清单）

到这里你应该能讲清楚下面这些事。建议拿来做新人 review：

| # | 要点 | 关键文件 |
|---|---|---|
| 1 | 数据模型三张表 + 一个 `afs_shares` 引用，cascade 方向要记得住 | `migrations/001_init.sql:867-904` |
| 2 | 可见性的真正生效点是 `call_agent` 的二次回退，不是 visibility 字段 | `mcp/tools/agent.ts:580` |
| 3 | session 关联 task 的时机：sessions 行 commit 之后才注册 `teamwork_sessions` | `mcp/tools/agent.ts:243-272` |
| 4 | 共享目录的「自动」入口：4 个函数对应 4 种生命周期动作 | `services/teamwork.ts:31-101` |
| 5 | 时间线是反推的：从 coord 的 tool 调用观察，不订阅 member session | `web/src/components/teamwork/TeamworkTimeline.tsx:159` |
| 6 | 跨用户 private 永远不放开：只在 `resolveRosterMemberBySlug` 的 SQL 边界里允许 | `db/teamwork.ts:164-196` |
| 7 | MCP token 是反向解析的钥匙：`x-session-token -> session_id -> task_id`，三 hop 都有 defensive 校验 | `mcp/server.ts:30-62` |
| 8 | 任务删后 member session 仍可在原 ws 继续聊：task 关系消失但 session 行保留 | `routes/teamwork.ts:238-264` + `migrations/001_init.sql:2863` |

## 接下来

- 产品视角使用文档尚未迁移到本目录。
- 想理解共享目录的底层 gRPC/AFS，看 [control-plane/src/services/afs.ts](../../control-plane/src/services/afs.ts) 与 `proto/afs.proto`
- 想跟一次端到端，建议本地开一个两 workspace 的账号，从建任务 -> 加成员 -> call_agent 走一遍，看 [TeamworkSection.tsx](../../web/src/components/teamwork/TeamworkSection.tsx) 里三层 React 渲染
- 想做修改，先确认改动是否触及 §四.抓手 4 的「不订阅成员 session」原则 -- 这条破坏了就回退不了
