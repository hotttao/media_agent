# Agent MCP 配置链路

本文以 Codex agent 为例，说明工作区 MCP 配置从 control-plane 写入、数据库保存，到 agent 启动后下载、转换、落盘和运行时挂载的过程。这里的“上传”指用户或上层系统通过 control-plane API 写入 workspace 的 agent 配置，不涉及上传 MCP server 二进制包。

## 总体流程

1. 用户在创建或更新 workspace 配置时提交 `mcp_config`。
2. control-plane 把 `mcp_config` 作为 workspace 配置的一部分保存到数据库。
3. Codex agent 启动或 reload config 时，从 `/_cp/workspaces/:id/config` 拉取配置。
4. agent 解析 `mcp_config`，落盘到 Codex 能读取的位置。
5. 创建或加载 ACP session 时，agent 把 HTTP MCP 作为 `mcp_servers` 参数传给 `codex-acp`；stdio MCP 则写入 Codex 原生 `config.toml`。

## 上传到 Control Plane

用户侧入口主要是：

- `POST /api/workspaces/`：创建 workspace 时可携带 `mcp_config`。
- `PUT /api/workspaces/:id/config`：更新已有 workspace 的 agent 配置。
- `PUT /_cp/workspaces/:id/config`：内部入口，走同一套 `applyWorkspaceConfigUpdate` 写入逻辑。

`mcp_config` 在 API schema 中是字符串字段，通常内容是 JSON：

```json
{
  "mcpServers": {
    "example-http": {
      "url": "https://example.com/mcp",
      "headers": { "X-Custom": "value" }
    },
    "local-tool": {
      "command": "node",
      "args": ["server.js"],
      "env": { "FOO": "bar" }
    }
  }
}
```

control-plane 不在写入阶段把 MCP 配置拆成多张业务表；workspace 级 MCP 配置整体保存在 `workspace_config.mcp_config`。如果 workspace 正在运行，`applyWorkspaceConfigUpdate` 会尝试热通知 agent reload；如果 `agent_type` 等变更导致镜像变化，则走重建。

## 数据库存储

核心表是 `workspace_config`：

| 字段 | 含义 |
| --- | --- |
| `workspace_id` | workspace 主键，也是 agent 拉配置时使用的 ID。 |
| `agent_type` | agent 类型，例如 `codex`，决定使用哪个 agent 镜像/适配器。 |
| `mcp_config` | workspace MCP 配置，文本字段，内容通常是 JSON 字符串；支持顶层 `mcpServers`，也兼容直接以 server name 为 key 的对象。 |
| `agent_settings` | agent 私有设置；Codex 场景会作为额外 TOML 片段追加到 `~/.codex/config.toml`。 |
| `model` / `provider_type` / `base_url` / `api_key` / `small_model` | 模型供应商配置，和 MCP 无直接关系，但会随同一份 config 下发。 |
| `system_prompt` / `prompt_id` | prompt 配置，和 MCP 一起构成 agent 启动配置。 |
| `template_id` / `template_version` | 如果 workspace 使用模板，读取时可从 `template_versions` 继承 `mcp_config`。 |
| `updated_at` | 配置更新时间。 |

相关辅助表：

- `mcp_catalog`：平台内置或推荐 MCP 目录，字段包括 `id`、`label`、`description`、`url`、`saas_url`、`group`、`required`、`params`、`hooks`。它是 UI/目录来源，不是 workspace 实际挂载清单。
- `mcp_oauth_clients`：按 `server_origin` 保存 MCP OAuth client 注册信息和 discovery metadata。
- `mcp_oauth_tokens`：按 `(user_id, server_origin)` 保存用户对某个 MCP origin 的 OAuth token，用于 CP proxy 请求上游 MCP 时注入 `Authorization`。
- `session_tokens`：运行时上下文表，字段包括 `token`、`workspace_id`、`session_id`、`resolved_at`。agent 把 `token` 作为 `X-Session-Token` 带到 MCP 请求里，control-plane 再解析成 workspace/session 上下文。

## Agent 下载和转换

Codex agent 启动时会读取环境变量中的 `CP_URL` 和 `WORKSPACE_ID`，然后请求：

```text
GET {CP_URL}/_cp/workspaces/{WORKSPACE_ID}/config
```

control-plane 在这个内部接口返回配置前，会对 `mcp_config` 做一次服务端加工：

- 对所有带 `url` 的 MCP server 注入 `X-Workspace-ID` 和 `X-Agent-ID`，值都是 workspace id。
- 如果 MCP server 对应的 origin 有用户 OAuth token，则把原始 URL 改写成 control-plane 内部代理地址：`/_cp/mcp/:userId/:encodedOrigin/*`。后续 agent 访问的是 CP proxy，proxy 再向真实上游 MCP server 注入 OAuth bearer token。
- 不再在这里静态注入 `tos-platform`；平台 MCP 由 agent sidecar 在每个 session 动态挂载，这样可以附带随 session 变化的 token/task 上下文。

Python 版当前实现中，`CpClient.fetch_workspace_config()` 拉取这份 payload，`CodexHooks.load_config()` 负责解析和落盘：

- `parse_mcp_servers()` 将 `mcp_config` 解析为 `name -> config`。
- `write_mcp_json()` 写 `{"mcpServers": ...}`。
- `_write_codex_toml()` 把 Codex 原生配置写成 TOML，并区分 HTTP MCP 和 stdio MCP。

`D:\Code\koala\agent-platform\agents\codex` 下的 TS 实现做的是同一件事：`loadConfig()` 拉 CP 配置，解析 `mcp_config`，写 `~/.codex/mcp.json` 和 `~/.codex/config.toml`；`loadAcpMcpServers()` 在 ACP session 层构造 HTTP MCP 列表。

## 本地落盘位置

以容器内默认路径为准：

| 路径 | 内容 |
| --- | --- |
| `/workspace/AGENTS.md` | 当前 workspace 的 prompt 内容。 |
| `/workspace/runtime.json` | `model`、`provider_type`、`base_url`、`small_model` 等运行时模型信息。 |
| `~/.codex/mcp.json` | 原始/规范化后的 workspace MCP JSON，形如 `{ "mcpServers": ... }`。 |
| `~/.codex/config.toml` | Codex 原生配置。stdio MCP 会写入 `[mcp_servers.<name>]`；HTTP MCP 不走这里的静态挂载。 |
| `~/.codex/auth.json` | OpenAI-compatible API key，格式为 `{ "OPENAI_API_KEY": "..." }`。 |

Codex 对 MCP 的关键分流是：

- HTTP MCP：不写入 `config.toml` 的 `[mcp_servers]`，而是在 ACP `new_session/load_session` 时通过 `mcp_servers` 参数动态传入。这允许每个 session 注入不同的 `X-Session-Token`。
- stdio MCP：写入 `~/.codex/config.toml` 的 `[mcp_servers.<name>]`，包含 `command`、`args`、`env`。它是本地进程，不经过 CP proxy，也不需要 session token。

如果任一用户 MCP 是 HTTP 类型，Codex TOML 还会设置 `experimental_use_rmcp_client = true`。

## 运行时挂载

Codex adapter 基于 ACP 管理 session。每个 session 会创建或复用一个 `codex-acp` bridge：

1. `AcpServer.create_session()` 或 `AcpServer.load_session()` 启动 bridge。
2. 调用 `hooks.load_mcp_servers(session_token)` 构造本次 session 的 HTTP MCP 列表。
3. `AcpBridge.create_session()` / `load_session()` 把 `mcp_servers` 传给 ACP SDK。
4. `codex-acp` 在该 session 内把这些 MCP server 暴露给模型。

`load_mcp_servers()` 总是优先加入平台 MCP：

```text
name = tos-platform
type = http
url  = {CP_URL}/mcp
headers = X-Workspace-ID, X-Agent-ID, X-Session-Token
```

随后加入用户配置中的 HTTP MCP。对用户 HTTP MCP，agent 会保留用户自定义 headers，并在有 session token 时追加 `X-Session-Token`。这个 token 不直接暴露真实 `session_id`；CP 通过 `session_tokens` 反查后，在平台 `/mcp` 或 `/_cp/mcp/...` proxy 中还原 workspace/session/task 上下文。

运行时请求路径有两类：

- 平台 MCP：agent 调用 `{CP_URL}/mcp`，control-plane 根据 `X-Workspace-ID` 和可选 `X-Session-Token` 注册平台工具。
- 第三方 HTTP MCP：如果需要 OAuth，agent 实际访问的是 `/_cp/mcp/:userId/:encodedOrigin/*`；CP proxy 解析用户 token、刷新 OAuth token、注入 `Authorization`，并把 `X-Session-Token` 翻译成上游可读的 `X-Workspace-Id` / `X-Session-Id`。

## 关键结论

- workspace 实际 MCP 清单的唯一主存储是 `workspace_config.mcp_config`，不是 `mcp_catalog`。
- `mcp_catalog` 是目录/推荐源；`mcp_oauth_*` 只服务 OAuth 连接和代理鉴权。
- Codex HTTP MCP 是 session 级动态挂载，目的是注入 `X-Session-Token` 并支持 CP proxy/OAuth；stdio MCP 是 Codex 配置文件级静态挂载。
- agent 本地会同时保留 `~/.codex/mcp.json` 和 `~/.codex/config.toml`：前者是规范化 JSON 备份/共享格式，后者是 Codex 原生配置入口。
- `tos-platform` 平台 MCP 不从数据库静态注入，而是 agent 每次 session 构造时主动挂到 `{CP_URL}/mcp`。
