---
name: end-to-end-trace
description: 实现一个完整"自定义 MCP"的 trace；自底向上实现 MCP server、平台注册 catalog、UI 插件；每一步贴出 HTTP req/res；register* 函数在第一次用到它时即时讲解
created: 2026-07-02T08:12:01Z
updated: 2026-07-02T14:25:00Z
---

# 端到端 trace：自实现一个 "天气查询" MCP，从零到 UI 渲染

> 上一份 `how-to-custom-ui.md` 讲了**怎么在已有 MCP 的基础上**写 UI 插件。这份讲**整个链路** —— 你自己写 MCP server，平台把它接进来，UI 把它画漂亮，每一步 HTTP 长什么样都贴出来。
>
> 阅读顺序：**自底向上**。先看最底层的 MCP server（Python 写），再让平台知道它，最后写前端把它画出来。

---

## 0. 整张地图（开始之前先看）

我们这次要做的完整链路：

```
┌──────────────────────────────────────────────────────────────────────────┐
│  Layer 1 — 你的 MCP server（Python/Node 任意）                             │
│   实现 JSON-RPC over HTTP/SSE 协议，提供一个工具：get_weather(city)        │
├──────────────────────────────────────────────────────────────────────────┤
│  Layer 2 — 平台后端                                                      │
│   平台把你的 MCP 加到 McpCatalog 暴露给用户；用户启用后注入 agent          │
├──────────────────────────────────────────────────────────────────────────┤
│  Layer 3 — 前端 plugin                                                   │
│   注册面板（快速查询天气按钮）+ 注册工具卡片渲染器（把 tool_call 画漂亮）   │
├──────────────────────────────────────────────────────────────────────────┤
│  Layer 4 — 运行时                                                        │
│   用户发问 → agent 决定调你的工具 → 平台→你的 server → result 回流 → UI    │
└──────────────────────────────────────────────────────────────────────────
```

涉及的 `register*` 入口（**先列出全貌，下面第一次用到时再细讲**）：

| 入口 | 谁调它 | 何时调 | 作用 |
|---|---|---|---|
| `registerTool(server, name, def, schema)` | MCP server 进程 | 启动时 | 把工具暴露给 MCP 协议 |
| `POST /api/admin/mcp-catalog` | 平台 admin API | 平台侧一次性 | 把 MCP server 加到 catalog |
| `tos.registerToolRenderer(name, def)` | 浏览器 IIFE 插件 | 插件加载时 | 自定义某工具的 UI 渲染 |
| `tos.registerPanel(panelDef)` | 浏览器 IIFE 插件 | 插件加载时 | 添加一个面板 tab |
| `tos.registerPlugin(plugin)` | 浏览器 IIFE 插件 | 插件加载时 | 注册一组副作用 + 面板 |

下面开始**从下往上写**。

---

## 1. Layer 1 — 写一个 MCP server

MCP 协议（Model Context Protocol）是一个**开放标准**，跟 OpenAI 的 tool calling 是远房表亲。它本质上要求你的 server 暴露 HTTP 端点，使用 **JSON-RPC 2.0** over HTTP（或 SSE）。**这个 server 跟平台后端是独立的进程**，跑在另一台机器也行，跑在 localhost 也行。平台侧只是个"代理 + 路由器"。

### 1.1 协议契约（这就是你需要实现的全部）

| 方法 | 端点 | 用途 |
|---|---|---|
| `initialize` | `POST /message` | 握手，告诉客户端你能提供哪些工具 |
| `tools/list` | `POST /message` | 列出所有工具的 schema |
| `tools/call` | `POST /message` | 实际调用某个工具，返回结果 |

> **Python 类比**：跟你写过 FastAPI/Flask 没区别。MCP 协议就是规定了一组 JSON-RPC 方法的 schema。

### 1.2 Python 实现（FastAPI 风格）

```python
# weather_mcp/server.py
"""
一个最小可跑的 MCP server。
- 协议：JSON-RPC 2.0 over HTTP  (POST /message)
- 暴露工具：get_weather(city: str) -> {city, temperature, condition, updated_at}
"""

from fastapi import FastAPI, Request
from fastapi.responses import JSONResponse
import time, random

app = FastAPI()

# MCP 协议的"我能提供什么"声明 —— 类似 OpenAPI schema
TOOLS = [
    {
        "name": "get_weather",
        "description": "查某城市当前天气（mock 数据）",
        "inputSchema": {
            "type": "object",
            "properties": {
                "city": {"type": "string", "description": "城市名，比如 '北京'"}
            },
            "required": ["city"],
        },
    }
]


@app.post("/message")
async def mcp_endpoint(request: Request):
    """
    这就是 MCP 协议的入口。所有 method（initialize / tools/list / tools/call）
    都从这个端点进，按 JSON-RPC 2.0 的格式分发。
    """
    rpc = await request.json()
    method = rpc.get("method")
    rpc_id = rpc.get("id")

    # ── initialize：握手，告诉客户端你是谁 ─────────────────────
    if method == "initialize":
        return JSONResponse({
            "jsonrpc": "2.0",
            "id": rpc_id,
            "result": {
                "protocolVersion": "2024-11-05",
                "serverInfo": {"name": "weather-mcp", "version": "1.0.0"},
                "capabilities": {"tools": {}},
            },
        })

    # ── tools/list：列出我能做的工具 ──────────────────────────
    # 平台会在 agent 初始化时调这个，拿到工具 schema 后塞给 LLM
    if method == "tools/list":
        return JSONResponse({
            "jsonrpc": "2.0",
            "id": rpc_id,
            "result": {"tools": TOOLS},
        })

    # ── tools/call：真调工具 ─────────────────────────────────
    if method == "tools/call":
        params = rpc.get("params", {})
        tool_name = params.get("name")
        args = params.get("arguments", {})

        if tool_name != "get_weather":
            return JSONResponse({
                "jsonrpc": "2.0", "id": rpc_id,
                "error": {"code": -32601, "message": f"tool '{tool_name}' not found"},
            }, status_code=200)  # JSON-RPC 错误用 200 + error 字段，不要 4xx

        city = args.get("city", "未知城市")
        # mock 数据 —— 真实场景这里调第三方天气 API
        result = {
            "city": city,
            "temperature": random.randint(-10, 35),
            "condition": random.choice(["晴", "多云", "小雨", "阴"]),
            "updated_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
        }

        # MCP 规范：call 的 result.content 必须是 [{type:'text', text:'...'}, ...] 数组
        return JSONResponse({
            "jsonrpc": "2.0",
            "id": rpc_id,
            "result": {
                "content": [
                    {"type": "text", "text": str(result)}    # 真实场景建议 json.dumps
                ],
                "isError": False,
            },
        })

    # 未知 method
    return JSONResponse({
        "jsonrpc": "2.0", "id": rpc_id,
        "error": {"code": -32601, "message": f"method '{method}' not implemented"},
    }, status_code=200)


# 本地调试：uvicorn weather_mcp.server:app --host 0.0.0.0 --port 8765
```

### 1.3 你的 server 自己跑 curl 测一下

```bash
$ curl -s -X POST http://localhost:8765/message \
    -H 'Content-Type: application/json' \
    -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}'
```

**Req**：
```http
POST /message HTTP/1.1
Host: localhost:8765
Content-Type: application/json

{"jsonrpc":"2.0","id":1,"method":"tools/list"}
```

**Res**（mock）：
```http
HTTP/1.1 200 OK
Content-Type: application/json

{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "tools": [
      {
        "name": "get_weather",
        "description": "查某城市当前天气（mock 数据）",
        "inputSchema": {"type":"object","properties":{"city":{"type":"string"}}, ...}
      }
    ]
  }
}
```

这一刻你得记住：你的 MCP server 是**独立的进程、独立的协议、独立的部署**。和平台后端是两件事。**下面才轮到平台登场**。

---

## 2. Layer 2 — 平台后端怎么知道你这个 server

### 2.1 三大状态：catalog / workspace-config / runtime-agent

平台需要 3 层信息才能把你的 server 接入 agent：

| 层 | 是什么 | 谁负责 | 存在哪 |
|---|---|---|---|
| **Catalog** | 平台已知的 MCP server 清单（**全局共享**） | 平台管理员 | PostgreSQL 表 `public.mcp_catalog` |
| **Workspace Config** | 当前 workspace 启用了哪些 catalog 条目 | 用户 | PostgreSQL 表 `workspace_config.mcp_config`（**JSON 字符串**） |
| **Runtime** | 实际 HTTP 代理到你的 server | 控制面 cp | 内存 + 短连接 + 写文件到 agent pod |

**`register*` 进入舞台**：这里的"注册"是 **CRUD 数据库**，不是浏览器那个 `tos.register*`。名字撞了但语义不同——这是会让人混淆的最大坑。下面所有"注册"我都会带上"服务器/客户端"前缀。

#### 2.1.1 真 schema：`mcp_catalog`（DDL 直引 `migrations/001_init.sql:352-365`）

```sql
CREATE TABLE public.mcp_catalog (
    id              text NOT NULL,                            -- PRIMARY KEY
    label           text NOT NULL,                            -- 用户看到的中文/英文名
    description     text DEFAULT '' NOT NULL,
    url             text NOT NULL,                            -- MCP server 入口
    saas_url        text,                                     -- SaaS 变体 URL，自部署时为 null
    "group"         text DEFAULT 'Other' NOT NULL,            -- 分类："Builtin" / "Custom" / "Integration" 等
    ui_panel        text,                                     -- ★ 与前端 panel.id 关联
    required        boolean DEFAULT false NOT NULL,           -- 强制启用，用户关不掉
    params          jsonb DEFAULT '[]' NOT NULL,              -- 表单字段定义（[{header,label,type,default}]）
    created_at      timestamptz DEFAULT now() NOT NULL,
    updated_at      timestamptz DEFAULT now() NOT NULL,
    hooks           jsonb DEFAULT '{}' NOT NULL               -- 目前只有 {on_delete?: "string"}
);
ALTER TABLE ONLY public.mcp_catalog ADD CONSTRAINT mcp_catalog_pkey PRIMARY KEY (id);
```

#### 2.1.2 真 schema：`workspace_config.mcp_config`（DDL `001_init.sql:979`）

```sql
-- workspace_config 表里有这一列：
mcp_config text DEFAULT '{}'::jsonb NOT NULL  -- 字符串，反序列化就是 JSON
```

**反序列化后的真实结构**（从 `routes/internal.ts:286-317` 反向抠出来的真东西）：

```typescript
// workspace_config.mcp_config 是字符串，反序列化后：
{
  "mcpServers": {
    "<mcp_catalog.id>": {       -- key 必须等于 mcp_catalog.id
      "type": "http",            -- 传输方式："http"（默认）/ "stdio" / "streamable-http"
      "url": "...",              -- HTTP 入口（http 模式才有）
      "headers": {               -- HTTP 请求头（用户填的表单值 + cp 注入的上下文）
        "X-Weather-Token": "sk-xxx",
        "X-Workspace-ID": "ws_abc"  -- 这条是 cp 装配时注入的，DB 里没有
      },
      "enabled": true            -- ★ 决定是否真正启用（持久化字段）
    },
    "calendar": {
      "type": "stdio",           -- stdio 模式（subprocess）
      "command": "/usr/local/bin/cal-mcp",
      "args": ["--port", "9001"],
      "env": { "CAL_TOKEN": "..." },
      "enabled": false
    }
  }
}
```

> **注意**：mcpServers[id] 里**没有 `enabled` 字段**。是否启用 = 这个 key 是否存在于 mcpServers 对象里。`required: true` 的 catalog 行永远存在。
> **DB 里持久化的字段**只有 `type / url / command / args / env / headers`（由用户表单生成），`X-Workspace-ID` / `X-Agent-ID` 是 cp 装配时**临时注入**的，不会写回 DB。


#### 2.1.3 两张表的关系（一句话）

> **`mcp_catalog` 是平台**类目表**（"有这个 server 存在"）；`workspace_config.mcp_config.mcpServers.<id>` 是 workspace 实例对它的**勾选**。前者先存在，后者才能勾。**`id` 严格相等**才能关联。

### 2.2 把 server 写进 catalog（一次性，admin 调用）

**HTTP Req**：

```http
POST /api/admin/mcp-catalog HTTP/1.1
Host: app.example.com
Cookie: session=...
Content-Type: application/json

{
  "id": "weather",
  "label": "Weather (custom)",
  "description": "查天气，自部署 MCP server",
  "url": "http://localhost:8765/message",
  "saas_url": null,
  "group": "Custom",
  "ui_panel": "weather_panel",                    ← 关键！这个字段把 MCP 跟你的 UI 面板关联起来
  "required": false,
  "params": [                                     ← ★ 不是 config_schema，是 params (jsonb 数组)
    {
      "header": "X-Weather-Token",
      "label": "API Token",
      "type": "text",
      "default": ""
    }
  ]
}
```

**为什么 `ui_panel: "weather_panel"` 这么重要**：
- platform 启动时会扫这个 catalog
- 找到 `ui_panel == "weather_panel"` → 在 workspace 配置里允许用户启用 weather server → 同时**只在 catalog 里出现这个 server 的 workspace 才在 launcher 里显示 weather_panel 这个 tab**。
- `ui_panel` 就是平台和你的前端插件之间的**锚点**——没它你的面板永远不会被显示。

**HTTP Res**：

```http
HTTP/1.1 201 Created
Content-Type: application/json

{"ok": true, "id": "weather"}
```

#### 2.2.1 DB 落表实况

`POST /api/admin/mcp-catalog` → `routes/admin/...` → `db/mcp-catalog.ts:32 upsertMcpCatalogEntry()`：

```sql
INSERT INTO public.mcp_catalog (id, label, description, url, saas_url, "group", ui_panel, required, params, hooks)
VALUES ($1, $2, ..., $10)
ON CONFLICT (id) DO UPDATE SET
  label = EXCLUDED.label,
  description = EXCLUDED.description,
  url = EXCLUDED.url,
  ...
  updated_at = NOW();
```

`hooks` 是 platform 给 MCP 删除时埋的 hook（比如某个 catalog server 被删时，平台去清掉所有引用了它的 workspace_config.mcp_config 里的对应 key）。目前实现里只有 `on_delete` 这一个钩子。

### 2.3 用户在 workspace 里启用这个 server

UI 上是"添加 MCP server"对话框，但底层就一次 PATCH：

**HTTP Req**（注意 `mcp_config` 内部是 `mcpServers` 嵌套结构 —— 见 §2.1.2）：

```http
PATCH /api/workspaces/{wsId}/config HTTP/1.1
Content-Type: application/json
Cookie: session=...

{
  "mcp_config": {                                  ← 序列化后存进 workspace_config.mcp_config 列
    "mcpServers": {
      "weather": {                                  ← key = mcp_catalog.id
        "enabled": true,
        "config": {                                 ← 用户填的表单值（来自 catalog.params）
          "X-Weather-Token": "sk-xxx"
        }
      }
    }
  }
}
```

**HTTP Res**：

```http
HTTP/1.1 200 OK
{
  "mcp_config": "{\"mcpServers\":{\"weather\":{\"enabled\":true,\"config\":{...}}}}"
  ↑ 注意：mcp_config 字段在 GET 时返回的是 JSON 字符串
}
```

**DB 落表实况**（`routes/workspaces/write.ts:172` → `db/workspaces.ts`）：

```sql
UPDATE workspace_config
   SET mcp_config = $1,
       updated_at = NOW()
 WHERE workspace_id = $2;
```

**`PATCH /api/workspaces/{wsId}/config` 的字段子集**（`write.ts:167-173` 抠出来）：

```typescript
const configPatch = {}
if (body.api_key)         configPatch.api_key = body.api_key
if (body.model)           configPatch.model = body.model
if (body.prompt_id)       configPatch.prompt_id = body.prompt_id
if (body.system_prompt)   configPatch.system_prompt = body.system_prompt
if (body.mcp_config)      configPatch.mcp_config = body.mcp_config     // ← 我们这条
if (body.agent_settings)  configPatch.agent_settings = body.agent_settings
// → UPDATE workspace_config SET ... = ... WHERE workspace_id = ...
```

> 注意：`mcp_config` 是 `string` 类型（jsonb 序列化结果），不是 object。**PATCH body 里你可以传对象，平台会序列化入库；GET 出来时拿到的是字符串**。

### 2.4 Workspace 启动 / chat 调用前的工具初始化

平台控制面 cp 在启动 agent 时会通过 HTTP 帮你"打招呼"，把工具拉下来：

**HTTP Req**（cp → 你的 server）：

```http
POST http://localhost:8765/message
Content-Type: application/json

{"jsonrpc":"2.0","id":7,"method":"initialize",
 "params":{"protocolVersion":"2024-11-05","clientInfo":{"name":"nap-cp"}}}
```

**HTTP Res**：

```json
{"jsonrpc":"2.0","id":7,"result":{"protocolVersion":"2024-11-05","serverInfo":{...},"capabilities":{"tools":{}}}}
```

紧跟着 cp 拉工具列表：

**Req**：

```json
{"jsonrpc":"2.0","id":8,"method":"tools/list"}
```

**Res**：

```json
{"jsonrpc":"2.0","id":8,"result":{"tools":[
  {"name":"get_weather","description":"...","inputSchema":{...}}
]}}
```

**`registerTool` 在这里才登场**——这俩 `initialize` + `tools/list` 完成的本质就是平台"列出了你通过 `registerTool` 注册的工具"，只不过这里的 register 在你 Python server 的代码里，是上面 §1.2 那张 `TOOLS = [...]` 表（如果你用了官方 Python MCP SDK，那就调 `server.registerTool("get_weather", handler, schema)`）。

#### 2.4.5 cp 装配 mcp_config → 写文件给 agent pod（`routes/internal.ts:282-318` 真代码）

在 `initialize` 之前，cp 已经在内部组装好了一份"装配过"的 mcp.json，**这一步是 platform 真正"接通" catalog+workspace_config 的时刻**：

```javascript
// 控制面 cp 拿到 workspace_config.mcp_config 后的装配过程：
let mcpConfig = config.mcp_config                            // 字符串
const parsed = JSON.parse(mcpConfig)
if (!parsed.mcpServers) parsed.mcpServers = {}                // 兼容空配置

for (const server of Object.values(parsed.mcpServers)) {
  // 1) 注入 workspace 上下文（agent 那边就能在 MCP 工具调用时拿到 workspace_id）
  server.headers = { ...server.headers, 'X-Workspace-ID': id, 'X-Agent-ID': id }

  // 2) 如果这个 server 在 catalog 里挂了 OAuth，URL 重写成 cp 代理 URL
  //    （cp 代理在转发时会自动注入 OAuth token，避免 user_token 落到 agent pod）
  if (userHasOAuthToken) {
    server.url = `${CP_INTERNAL_URL}/_cp/mcp/${userId}/${origin}${path}`
  }
}
mcpConfig = JSON.stringify(parsed)
// 再把这段 mcpConfig 写到 agent pod 的 mcp.json 文件
```

> **关键不变量**：
> - cp 不会"读你的工具"——它只负责搬运。
> - `enabled: true` 的 server 才会被写进 mcp.json。
> - agent pod 自己的 MCP client 进程读 mcp.json → 决定要不要去连。
> - OAuth 改写 + workspace header 注入都是 cp 在这一步完成的。

### 2.5 用户发问 → agent 真的调你的工具

用户在平台聊天输入框打字："北京今天天气怎么样？"

**HTTP Req**（浏览器 → cp）：

```http
POST /api/workspaces/ws_abc/chat HTTP/1.1
Content-Type: application/json
Accept: text/event-stream

{
  "session_id": "sess_xyz",
  "content": "北京今天天气怎么样？"
}
```

平台 SSE 流回的第一个分片：

```
event: message
data: {"type":"session.ack","session_id":"sess_xyz","workspace_id":"ws_abc"}

event: message
data: {"type":"turn.start","turn_id":"t_001"}

event: message
data: {"type":"content_block_start","block":{"type":"tool_use","id":"toolu_01","name":"get_weather","input":{"city":"北京"}}}

event: message
data: {"type":"tool.executing","tool":"get_weather","input":{"city":"北京"}}
```

**此刻 cp 在干什么**：发现 agent 决定调 `get_weather` → 把这条调用**作为一个独立的 HTTP 请求转发给你的 server**：

**HTTP Req**（cp → 你的 server）：

```http
POST http://localhost:8765/message
Content-Type: application/json
X-Forwarded-For: cp.internal

{"jsonrpc":"2.0","id":42,"method":"tools/call",
 "params":{"name":"get_weather","arguments":{"city":"北京"}}}
```

**HTTP Res**（你的 server → cp）：

```json
{
  "jsonrpc": "2.0",
  "id": 42,
  "result": {
    "content": [{"type":"text","text":"{'city': '北京', 'temperature': 18, 'condition': '晴', 'updated_at': '2026-07-02T08:00:00Z'}"}],
    "isError": false
  }
}
```

cp 把这个 result **包成一个 SSE 事件** 推回浏览器：

```
event: message
data: {"type":"tool.result","tool":"get_weather","call_id":"toolu_01",
       "result":{"content":[{"type":"text","text":"..."}],
                 "isError":false}}
```

接下来 agent 拿到工具结果继续生成自然语言回答……

**到这里 Layer 2 的链路就闭环了**：用户消息 → cp → LLM → 决定调工具 → cp 转发到你的 server → 拿回结果 → SSE 流回浏览器。**Layer 2 完全没有自定义 UI**；它只负责把工具调用"如实记录到 transcript 数据流里"。

---

## 3. Layer 3 — 浏览器 IIFE 插件怎么"接管"显示

Layer 2 把工具调用塞进 SSE 流，浏览器原生会按"默认 JSON 渲染器"展示，丑。下面我们写 plugin 让它变漂亮。

### 3.1 Plugin 加载时序

**Step 3.1.1：浏览器启动**

```
GET /api/plugins                              HTTP/1.1  →  [{id:"weather-plugin", url:"/plugins/weather.js"}, ...]
```

**Step 3.1.2**：依次注入 `<script src="/plugins/weather.js">`，浏览器下载并**立即执行**该 IIFE。

**Step 3.1.3**：IIFE 跑起来 → 调 `tos.registerToolRenderer` 和 `tos.registerPanel`。

#### 🔑 第一次关键：`registerToolRenderer` 在这一刻发生

> **`tos.registerToolRenderer(toolName, rendererDef)`** — 进入"工具 → 自定义 React 组件"的关联表。
>
> - **Python 类比**：Django `admin.site.register(Model, AdminClass)` —— 把"对象类型"绑到"展示方式"。
> - **签名**：
> ```js
> window.tos.registerToolRenderer('get_weather', {
>   getPreview: (toolCall) => string,             // 收起的卡片标题
>   renderInput: (toolCall) => ReactNode | null, // "调用了什么参数"展示
>   renderResult: (toolCall) => ReactNode | null,// "返回了什么结果"展示
>   defaultExpanded?: boolean                    // true=自动展开（带 Approve/Reject 的卡片才用）
> })
> ```
> - **返回 `null` 的含义**：渲染失败回退到默认 JSON 展示（**记住：写自定义渲染器必须返回 null 在错误分支，否则用户体验很差**）。
> - **存哪**：`tos_renderers` Map（`internal/ui-sdk/src/tool-renderers/plugin-registry.ts:9`）。Map 用 `Map<string, ToolRendererDef>`，监听器 + version 用来支持 lazy loaded bundle——bundle 晚到也能 push 进来，不需要刷新整个 app。

#### 🔑 第二次关键：`registerPanel` 在这一刻发生

> **`tos.registerPanel(panelDef)`** — 把一个 React 组件塞进 workspace 的"可选 tab"池。
>
> - **Python 类比**：Django `admin.site.register(Model)` 但不做 admin，用 model 关联一个侧栏 component。
> - **签名**：
> ```js
> window.tos.registerPanel({
>   id: 'weather_panel',                    // 严格匹配 catalog.ui_panel
>   label: () => 'Weather Quick Lookup',    // 每次渲染时调用，支持 i18n 切换
>   component: WeatherPanelComponent,       // 标准 React 组件，props = { workspaceId, instanceId }
> })
> ```
> - **什么时候出现在 launcher 里**：`useWsApps` 钩子轮询 catalog，找到 `ui_panel === 'weather_panel'` 的 catalog 条目且**当前 workspace 启用了对应的 MCP server** → 显示 tab。
> - **runtime 调用**：`panel-registry.ts:30`，模块级 Map + listeners + version，作用跟 renderer registry 是同一个套路。

### 3.2 完整 plugin 代码（dev 模式加载，可直接复制）

```javascript
/**
 * weather-plugin.js —— 自定义 get_weather 工具的 UI 卡片 + 加一个 weather_panel 面板
 *
 * 加载方式：把本文件放到 web/dist/dev-plugins/weather.js，
 *          并在 web/dist/dev-plugins/manifest.json 里加 entry。
 */
(function () {
  const tos = window.tos
  if (!tos) {
    console.error('[weather-plugin] window.tos not available — load too early?')
    return
  }

  const React = tos.React
  const { useState } = React
  const h = React.createElement  // <-- 简写，参考 §1.1 how-to-custom-ui.md

  // ── ① 工具渲染器：把 get_weather 的 tool_call 画成一张天气卡片 ──
  //
  // 上面 STEP 3.1 已经讲过 registerToolRenderer 在这里就是"把这个 React 组件
  // 跟工具名绑起来"。一旦绑定，§2.5 里那段 SSE 的 tool.result 事件落到前端
  // store 里时，渲染器就会被自动调用。
  const weatherRenderer = {
    getPreview(tool) {
      // input 长得像 {city:"北京"}；getPreview 是"卡片收起来时的标题"。
      const { city = '?' } = tos.tools.unwrapMcpInput(tool.input)
      return `查天气：${city}`
    },

    renderInput(tool) {
      const input = tos.tools.unwrapMcpInput(tool.input)
      return h(
        'div',
        { style: { fontFamily: 'monospace', fontSize: 12, color: '#666' } },
        'city = ',
        h('strong', null, JSON.stringify(input.city || '?'))
      )
    },

    renderResult(tool) {
      // MCP result 格式见 §1.2：{content:[{type:'text', text:'...'}], isError}
      const text = tos.tools.getMcpText(tool)   // 帮你从两种 MCP 格式里抠出 text
      if (!text) return h('em', null, '(empty)')

      // 你的 server 在 §1.2 里 str(dict) 了一下，真实场景应该用 json.dumps 这里是演示
      let data
      try { data = JSON.parse(text.replace(/'/g, '"')) }   // 魔改一下假装是 JSON
      catch { return h('pre', null, text) }

      return h(
        'div',
        {
          style: {
            border: '1px solid #e5e7eb',
            borderRadius: 8,
            padding: 12,
            marginTop: 8,
            background: '#f9fafb',
            display: 'flex',
            gap: 16,
            alignItems: 'center',
          },
        },
        h('div', { style: { fontSize: 32 } }, data.condition === '晴' ? '☀️' : '☁️'),
        h('div', null,
          h('div', { style: { fontSize: 18, fontWeight: 'bold' } }, data.city),
          h('div', { style: { color: '#666' } }, `${data.temperature}°C · ${data.condition}`),
          h('div', { style: { fontSize: 11, color: '#999' } }, `updated ${data.updated_at}`)
        )
      )
    },
  }

  // 真正注册到平台 —— 模块级副作用，立刻生效
  tos.registerToolRenderer('get_weather', weatherRenderer)
  console.log('[weather-plugin] registerToolRenderer: get_weather')

  // ── ② 面板：weather_panel —— 让用户在不进入聊天的情况下直接查天气 ──
  function WeatherPanel({ workspaceId }) {
    const [city, setCity] = useState('北京')
    const [busy, setBusy] = useState(false)

    function ask() {
      // 这里拿到的 actions 就是聊天输入框用的同一个 sendMessage —— 见 how-to-custom-ui.md §5
      // 相当于：在聊天框里替你输入了"帮我查一下 XXX 的天气"
      const actions = tos.workspace.useAgentSessionActions()
      setBusy(true)
      try {
        actions.sendMessage(`请帮我查一下 ${city} 现在的天气`)
      } finally {
        setBusy(false)
      }
    }

    return h(
      'div',
      { style: { padding: 16 } },
      h('h3', null, 'Weather Quick Lookup'),
      h('p', { style: { color: '#666', fontSize: 13 } },
        '按下面按钮等于在聊天框输入 "请帮我查一下 北京的天气"'
      ),
      h(
        'div',
        { style: { display: 'flex', gap: 8, marginTop: 12 } },
        h('input', {
          value: city,
          onChange: (e) => setCity(e.target.value),
          style: { flex: 1, padding: 6, border: '1px solid #ddd', borderRadius: 4 },
        }),
        h(
          'button',
          { onClick: ask, disabled: busy, style: { padding: '6px 12px' } },
          busy ? '查询中…' : '查询'
        )
      )
    )
  }

  tos.registerPanel({
    id: 'weather_panel',
    label: () => 'Weather',
    component: WeatherPanel,
  })
  console.log('[weather-plugin] registerPanel: weather_panel')

  // ── ③ 完整插件（可选）—— 把上述两个 + 一组副作用一次性注册 ──
  //
  // registerPanel 和 registerToolRenderer 各自独立调用的版本对 dev 已经够用。
  // 但如果你想"附加工具完成后的副作用"（比如"工具调完 → 弹 toast 提醒"），
  // 用 registerPlugin 包成一个 unit：
  tos.registerPlugin({
    id: 'weather-plugin',
    toolResultHandlers: [
      {
        id: 'weather-plugin.notify',
        match: ({ toolName }) => toolName === 'get_weather',
        debounceMs: 300,
        onMatch: ({ toolOutput }) => {
          // 这里能拿到完整的 tool_result，可以 push toast、写 store、发事件……
          console.log('[weather-plugin] tool result received:', toolOutput)
        },
      },
    ],
    // panels 也可以在这里集中声明，等价于上面的 tos.registerPanel(...)：
    panels: [],
  })

  console.log('[weather-plugin] all registered')
})()
```

**JSX 版本**（如果你用构建工具就改成这样写）：

```tsx
// src/WeatherPanel.tsx
import { useState } from 'react'

export function WeatherPanel({ workspaceId }: { workspaceId: string }) {
  const [city, setCity] = useState('北京')
  const [busy, setBusy] = useState(false)
  return (
    <div style={{ padding: 16 }}>
      <h3>Weather Quick Lookup</h3>
      <input value={city} onChange={(e) => setCity(e.target.value)} />
      <button
        disabled={busy}
        onClick={() => {
          const actions = window.tos.workspace.useAgentSessionActions()
          actions.sendMessage(`请帮我查一下 ${city} 现在的天气`)
        }}
      >
        {busy ? '查询中…' : '查询'}
      </button>
    </div>
  )
}
```

### 3.3 Dev 加载配置

**`web/dist/dev-plugins/manifest.json`**：

```json
{
  "plugins": [
    {
      "id": "weather-plugin",
      "label": "Weather (dev)",
      "entry": "/dev-plugins/weather.js",
      "ui_panel": "weather_panel"
    }
  ]
}
```

**`web/dist/dev-plugins/weather.js`**：上面那段 IIFE 文件。

---

## 4. 全链路 trace —— 把 §2 §3 的 HTTP 都串起来

下面是从用户开页面到看到自定义天气的**端到端**时序。所有 `→` 都是 HTTP req/res，所有 `══>` 都是浏览器 event。

```
T=0   浏览器进程启动
      ┌─→ GET /api/plugins
      │     ← 200 [{id:"weather-plugin", url:"/dev-plugins/weather.js"}, ...]
      │
      │   <script src="/dev-plugins/weather.js"> 注入
      │     浏览器下载 → 立即执行 IIFE
      │       → tos.registerToolRenderer('get_weather', ...)     ✅ 模块级 Map 写入
      │       → tos.registerPanel({id:'weather_panel', ...})      ✅ 模块级 Map 写入
      │       → tos.registerPlugin({...})                         ✅
      └─ IIFE 末尾 console.log 都打出来

T=1   用户打开 workspace ws_abc
      ┌─→ GET /api/mcp-catalog
      │     ← 200 [{id:"weather", ui_panel:"weather_panel", ...}, ...]
      │
      │   useWsApps() 选中 weather，因为 ui_panel==='weather_panel'
      │   weather_panel tab 在 launcher 中可点击
      └─ 渲染 workspace shell

T=2   用户点击 weather_panel tab
      ┌─<WeatherPanel/> 挂载（用 lazy 注册方式：第一次点击触发 bundle 真加载——这里已经 eager 所以直接挂）
      │
      │   用户在 input 输入 "上海" 点查询
      └─> 调用 tos.workspace.useAgentSessionActions().sendMessage(...)

T=3   sendMessage 触发
      ┌─> POST /api/workspaces/ws_abc/chat
      │     {session_id, content:"请帮我查一下 上海 现在的天气"}
      │     ← 200 + SSE（Content-Type: text/event-stream）

      SSE  事件序列（这里只列关键几个）：
      │
      │   session.ack        → 建立 SSE 通道，告诉前端 session_id
      │   turn.start         → 开始一轮 assistant 回复
      │   content_block_start{type:tool_use, name:"get_weather", input:{city:"上海"}}
      │   ─── 此刻 cp 已把这条 tool_use 转 HTTP 发给你的 server：
      │
      │     ┌─> POST http://localhost:8765/message
      │     │   {jsonrpc:"2.0", id:42, method:"tools/call",
      │     │    params:{name:"get_weather", arguments:{city:"上海"}}}
      │     │
      │     │  ← 200 {result:{content:[{type:"text", text:"..."}], isError:false}}
      │     └─ cp 拿到结果 → 推回 SSE：
      │
      │   tool.result{call_id:"toolu_01", name:"get_weather", result:{...}}
      │     ─── 此刻前端 store 里多了一个 tool_call block，
      │         写 store 的精确代码：
      │         web/src/stores/agent-session-store.ts:530-555
      │         （findIndex 找 call_id 匹配的 tool block，
      │          把 result / isError / resultAt 写进去）
      │           ↓
      │         React re-render：MessageBubble.tsx:137-138 检测
      │         block.type === 'tool' → 渲染 <ToolCallBlock tool={block.tool}/>
      │           ↓
      │         ToolCallBlock.tsx:46  const renderer = resolveRenderer(tool.name, agentType)
      │
      │     在 registry 里走 4 级优先级（registry.ts:154）：
      │       1) 内置精确匹配       ← get_weather 不在内置表
      │       2) 插件注册渲染器     ← T=0 已经注册在这里 ✅
      │          (plugin-registry.ts:9 的 Map)
      │       3) agent 类型回退     ← 没匹配
      │       4) 返回 null          ← 不会再到这一步
      │
      │     命中 weatherRenderer：
      │       ToolCallBlock.tsx:123  renderer?.renderInput(tool)  →  "city = 北京"
      │       ToolCallBlock.tsx:125  renderer?.renderResult(tool) →  "☁️ 北京 18°C 晴" ← 你的卡
      │     ─── React 把节点塞进 <CollapsibleContent> 渲染出来 ✅
      │
      │   content_block_delta{...}     → assistant 开始打字
      │   message.end{turn_complete}   → 整轮结束
      └─ SSE 关闭
```

#### §4 关键易混点：渲染位置 = 聊天 transcript 内，不是 panel tab

`registerToolRenderer` 渲染的卡片位置 = **agent 消息内部**（跟 tool_use block 同一行）。
`registerPanel` 注册的面板位置 = **workspace launcher / 侧栏 tab**（独立窗格）。

它们**完全独立、互不相关**，但经常被搞混。完整位置对照：

| 入口 | 渲染在哪 | React 组件入口 |
|---|---|---|
| `tos.registerToolRenderer` | chat transcript（assistant 消息流里） | `MessageBubble` → `ToolCallBlock` |
| `tos.registerPanel` | workspace launcher 内的 tab | `useWsApps` → `panel-registry` 里的 React 组件 |

**关键不变量**：从 §3.IIFE 注册 → §4.T=0 写入 renderer registry，到 §4.tool.result 事件回来 → 命中渲染器，**整个过程没有任何 HTTP 之外的"桥"**。**渲染器注册是浏览器内纯客户端行为**；工具调用是 server-to-server HTTP；中间只通过 SSE 流协议连接。

---

## 5. 一图看懂"两个 register* 的语义"

```
浏览器侧 tos.register*                    平台后端 catalog / config register*
（dev manifest 加载时跑一次）              （admin API 一次性 / 用户每次启用改）
            │                                          │
            ▼                                          ▼
   web/src/lib/panel-registry.ts            数据库 mcp_catalog 表
   internal/ui-sdk/plugin-registry.ts       workspace.config.mcp_config
            │                                          │
            │        platform runtime                    │
            │     ┌────────────────────────┐            │
            └────▶│  工具调用 = HTTP       │◀───────────┘
                  │  工具展示 = React Map  │
                  │  面板挂载 = catalog 里 │
                  │         有 ui_panel    │
                  └────────────────────────┘
```

**易混淆点提醒**：
- 你写 MCP server 时调的 `server.registerTool(...)`（如果是 Python SDK）：**是你 server 进程内的注册**，跟平台无关；
- 你写前端 plugin 时调的 `tos.registerToolRenderer(...)`：**是浏览器内的注册**，跟 catalog 无关；
- 平台做的是"配置 + 路由 + 协议转换"，**它不做任何插件代码的注册**——它只让"谁启用"成为配置事实。

---

## 6. 调试 checklist（按顺序打钩）

| # | 期望 | 怎么验 |
|---|---|---|
| 1 | MCP server 独立跑通 | `curl -X POST localhost:8765/message -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}'` 拿到 tools 列表 |
| 2 | Catalog 已经收录 | `GET /api/mcp-catalog` 看到 `id:"weather", ui_panel:"weather_panel"` |
| 3 | 用户启用成功 | `PATCH /api/workspaces/{wsId}/config` 把 `mcp_config.weather.enabled=true`，200 OK |
| 4 | Plugin bundle 被浏览器加载 | DevTools → Network 看到 `weather.js` 200，且 console 出现 `[weather-plugin] all registered` |
| 5 | weather_panel tab 出现 | Workspace launcher 点开看到 "Weather" 标签 |
| 6 | 工具被注册到渲染器表 | 浏览器 console 跑 `console.log(window.tos.workspace... -> 然后到 internal/ui-sdk/src/tool-renderers/plugin-registry.ts 的 Map 里查`——或者更简单：直接触发 tool_call 看到自定义卡片 |
| 7 | sendMessage 后右侧聊天出现 user message | Developer Tools → Network 看到 `POST /api/workspaces/{wsId}/chat` 200 + SSE |
| 8 | cp 真的转发到你的 server | 在你 server 的控制台打 log：`tools/call`, city=... 应该出现 |
| 9 | tool.result 自定义渲染 | 聊天 transcript 里 `get_weather` 那张卡片是 ☁️ + "上海 18°C 晴" 而不是 default JSON |
| 10 | reload 后 tab 还在 | 刷新页面，回到 step 5 |

**任何一步没通过**：回去看对应小节，**不要跳**。颗粒度不到位就跳 = 闭眼写代码 = 复盘里只能写"莫名其妙不工作"。

---

## 7. 横向拓展：同一机制的不同用例

| 想做的事 | 用 `tos.register*` 哪个 | UI surface |
|---|---|---|
| 自定义**某个工具的卡片样式** | `registerToolRenderer(name, def)` | chat transcript 里的卡片 |
| 加一个**新面板 tab** | `registerPanel({id, label, component})` | workspace launcher |
| 工具完成时**自动做副作用**（刷数据、发事件） | `registerPlugin({toolResultHandlers})` | 不渲染 UI，跑回调 |
| **多个工具**共享同一渲染器 | 多次 `registerToolRenderer`，同一个 def 对象 | — |
| 共享**side effects** + **一组面板** | 一个 `registerPlugin` 同时声明 panels + handlers | — |
| **工具卡片里点链接跳到自定义 panel**（⭐见 §7.1） | `SlotContext.activate()` | chat transcript 卡片 → launcher tab |

> 注意：到目前为止，`tos.registerPanel` / `registerToolRenderer` 都是**名字 + 实例** 注册，没有"批量注册"。如果你要注册 50 个 MCP tool 的渲染器，就写 50 次——但其实更好的姿势是写一个 for 循环读 manifest 数组。

### 7.1 工具结果含链接 → 点击跳到自定义 panel

**问题**：你写的 MCP tool 返回一个 URL，agent 调用后，前端卡片里这个 URL 能不能被点击直接**跳到你的 registerPanel 那个 tab**，并把参数带过去？

**答：能。** 平台已经有现成的机制：`SlotContext.activate(slotId, instanceId)`，平台自己的 agent 文件链接跳转（`web/src/hooks/useWorkspaceFileLink.ts:73-86`）用的就是同一个。

#### 7.1.1 协议设计：tool.result 里的"链接"长啥样

你**自己定义**协议。最简单的一种：

```json
{
  "city": "上海",
  "temperature": 18,
  "condition": "晴",
  "_action": {
    "type": "open_panel",
    "appId": "weather",          // ★ mcp_catalog.id，不是 ui_panel
    "params": { "city": "上海" } // ★ 写进 instance persistent state
  }
}
```

#### 7.1.2 渲染器里处理点击（IIFE 插件代码片段）

```javascript
const weatherRenderer = {
  renderResult(tool) {
    const text = tos.tools.getMcpText(tool)
    const data = JSON.parse(text)         // 实际场景 safeParse
    const slotCtx = tos.workspace.useSlotContext()

    function openInPanel() {
      if (!slotCtx) return
      // 1) 找到或创建 weather 这个 app 的实例
      //    ★ appId = 'weather' = mcp_catalog.id (不是 'weather_panel')
      //    useWsApps.ts:235 注册 app 时就是 id: entry.id
      const { slotId, instanceId } = slotCtx.ensureInstance(data._action.appId)

      // 2) 把 params 写进 panel 的 persistent state
      //    panel 里用 useInstancePersistentState('city') 就能读到
      tos.workspace.useInstancePersistentState(...)   // 写 API 走 setPersistentInstanceStateMany
      // （setPersistentInstanceStateMany 在 stores/instance-state-store.ts；
      //  host.ts 没直接暴露 set 函数，需要 IIFE 通过 zustand store 调用，
      //  或者把 set 函数也挂到 tos 上 —— 这是 platform host 可能的扩展点）

      // 3) 切换到这个 tab
      slotCtx.activate(slotId, instanceId)
    }

    return h('div', null,
      h('div', null, `${data.city} ${data.temperature}°C`),
      h('button', { onClick: openInPanel }, '在 Weather 面板里看历史')
    )
  }
}
```

#### 7.1.3 平台对应物（file:line）

| API | 位置 | 作用 |
|---|---|---|
| `slotCtx.ensureInstance(appId)` | `web/src/contexts/SlotContext.tsx:264-283` | 找到/创建 app 实例 |
| `slotCtx.activate(slotId, instanceId)` | `web/src/contexts/SlotContext.tsx:257-262` | 切到前台 |
| `slotCtx` 通过 host 暴露 | `web/src/lib/host.ts:55` (`tos.workspace.useSlotContext`) | IIFE 拿得到 |
| 平台已在用同款机制 | `web/src/hooks/useWorkspaceFileLink.ts:73-86` | agent 输出文件链接 → 跳 Files panel |
| `useWsApps` 注册 app 时 `id: entry.id` | `web/src/hooks/useWsApps.ts:235` | **appId = mcp_catalog.id，不是 ui_panel** |

#### 7.1.4 ⚠️ 易踩的坑

- **`appId` 用 `mcp_catalog.id`（"weather"），不是 `ui_panel`（"weather_panel"）**。`useWsApps.ts:235` 是确凿证据。
- 写入 `instance persistent state` 的 `setPersistentInstanceStateMany` **不在 `window.tos` 表面**。最简单的 workaround：panel 内部通过 `useInstancePersistentState` 读，但写需要从渲染器端调 zustand store —— 等于绕开 host 直接用平台 store。当前 IIFE 没有简单 API，需要 plugin 通过 React 渲染触发时自然由 panel 自己 re-fetch。
- 另一种更简洁的姿势：让 MCP tool **不发链接**，直接通过 `actions.sendMessage` 触发一段文字，agent 自己的回复里调 `get_weather_history(city)` 拉更多数据，**省掉 panel 跳转**。这一招更省事，但失去了"细粒度 UI 控制"。

#### 7.1.5 完整链路图

```
你的 get_weather 工具返回 { city, ..., _action: {appId, params} }
   ↓
SSE tool.result → store → renderResult
   ↓
你的卡片渲染出 <button>「在 Weather 面板里看历史」
   ↓
用户点击
   ↓
slotCtx.ensureInstance('weather')    ← 在 launcher 里找到/创建 tab
   ↓
写 instance persistent state { city: '上海' }
   ↓
slotCtx.activate(slotId, instanceId) ← 切到前台
   ↓
panel 挂载 → useInstancePersistentState('city') 读到 '上海'
   ↓
panel 渲染详细历史 / 趋势图 / 任何想看的
```

---

## 8. 常见误区速查（这一节是免费送的）

| 症状 | 真正原因 | 解法 |
|---|---|---|
| tab 没出现但 console 打出了 `registerPanel done` | `mcp_catalog.ui_panel` 没写你的 id | curl `/api/mcp-catalog` 看 ui_panel 字段 |
| tab 出现但灰色不可点 | 当前 workspace 没启用对应 catalog server | `PATCH /api/workspaces/{wsId}/config` |
| 卡片还是默认 JSON | render path 上没匹配上 | 浏览器 console 跑 `__debug_renderers__`（如果平台暴露了）或在 `plugin-registry.ts` 打 log |
| SSE 流里有 tool_call 但 cp 没转发到 server | cp 跟你的 server 网络不通 / URL 错 | `curl -X POST http://<catalog-url>`，404/超时说明配置错 |
| `tos is undefined` 报错 | IIFE 加载时序早于 host.ts | 检查 index.html，确保 plugin bundle 在 `main.ts` 之后或用 lazy 模式 |
| `registerPanel: duplicate id` warning | 同 id 重复注册，第二次被静默拒收 | 检查是否 eager + dev 双列，或别人用了同样的 ui_panel |
| agent 不调工具 | LLM 不知道怎么用 | 在 `tools/list` 返回的 schema 写得清楚些，description 用动词开头 |
| dev manifest 改了没生效 | 浏览器缓存 / 平台没重新拉 | DevTools 勾上 Disable cache；或改完 manifest 关 tab 重开 |

---

## 9. 小结

- **底层逻辑**：整个定制流程分两个隔离层 —— **平台 layer 管"调用"（catalog + config + HTTP 转发）**，**浏览器 layer 管"展示"（注册渲染器 + 面板）**。两层之间**只通过 SSE 流协议通信**。
- **顶层设计**：当你打开浏览器看到自定义 UI，本质是 4 个 HTTP 边界（`/api/plugins` 拉清单、`/dev-plugins/weather.js` 加载 IIFE、`/api/mcp-catalog` 取工具元信息、`/api/workspaces/{id}/chat` 走 SSE）+ 2 个模块级 Map（panel-registry / plugin-registry）。
- **抓手**：`ui_panel` 是平台和 plugin 的**唯一耦合点**——它必须同时写在 `mcp_catalog` 和 `tos.registerPanel` 里。
- **闭环**：每个 register* 函数都在它**第一次真实发生**的那一刻才解释（不是事前或事后的死规则）。
- **颗粒度**：调一个工具的 UI 显示完整链路上有 **9 个 HTTP 边界**（§4 列了），漏一个 tab 都不会出现。
- **拉通**：MCP server 的 `registerTool`（Python）— catalog 的 `register*`（admin API）— 插件的 `tos.register*`（IIFE）—— 名字撞了，但**作用域不同**，看清楚就行。
