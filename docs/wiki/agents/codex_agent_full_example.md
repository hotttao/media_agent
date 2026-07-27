# Codex Agent 全链路示例

本文用一个具体工作区把 Codex agent 的配置链路串起来：workspace 配置、MCP、Skills、provider、prompt、credentials 如何进入 control-plane 和数据库，agent 启动或 reload 时如何拉取并落盘，ACP session 如何挂 MCP，Codex 如何看到 Skills，以及 usage 如何被 control-plane 反向采集。

示例中的固定值：

```text
user_id       = user_7f3a
workspace_id  = ws_9xq2
workspace     = 小红书选题助手
agent_type    = codex
cp_url         = http://nap-cp.default.svc.cluster.local:3000
workspace_dir = /workspace
home          = /root
provider_id   = prv_openai_proxy_01
prompt_id     = prm_xhs_editor_01
skill_id      = 9f4b0b5d-2e6c-4c0c-9c2d-2c19d9b4f111
session_id    = sess_2f8c
session_token = 4bd0a38a-4d44-4d5f-b09a-3b1f31bc8410
```

## 一次完整配置

用户先创建可复用 provider：

```http
POST /api/providers/
Authorization: Bearer tos_user_token
Content-Type: application/json

{
  "name": "TOS OpenAI Proxy",
  "description": "公司统一 OpenAI-compatible 网关",
  "provider_type": "openai",
  "base_url": "https://llm-gateway.example.com/v1",
  "api_key": "sk-prod-abc123",
  "visibility": "private"
}
```

响应不会回显密钥：

```json
{
  "id": "prv_openai_proxy_01",
  "name": "TOS OpenAI Proxy",
  "provider_type": "openai",
  "base_url": "https://llm-gateway.example.com/v1",
  "api_key": "",
  "user_id": "user_7f3a",
  "visibility": "private",
  "is_owner": true
}
```

再创建 prompt：

```http
POST /api/prompts/
Authorization: Bearer tos_user_token
Content-Type: application/json

{
  "name": "小红书爆款选题助手",
  "content": "你是小红书内容策划助手。输出必须包含选题、标题、封面文案和风险提醒。",
  "visibility": "private"
}
```

响应：

```json
{
  "id": "prm_xhs_editor_01",
  "name": "小红书爆款选题助手",
  "content": "你是小红书内容策划助手。输出必须包含选题、标题、封面文案和风险提醒。",
  "current_version": 1,
  "visibility": "private"
}
```

上传一个 Skill 包。control-plane 的 `/api/skills` 只负责编排和 ACL，实际 `skills`、`skill_sources`、`skill_versions` 写入由 skills-content-service 完成：

```http
POST /api/skills?name=xhs-research&description=小红书调研工作流&visibility=private&category=research
Authorization: Bearer tos_user_token
Content-Type: application/gzip

<xhs-research.tar.gz>
```

发布后，工作区只挂 skill UUID，不靠 name：

```json
{
  "id": "9f4b0b5d-2e6c-4c0c-9c2d-2c19d9b4f111",
  "name": "xhs-research",
  "active_version_id": "6a6d57e1-0f35-4b7a-a84c-9b2df70f5d90"
}
```

再保存用户凭据。示例里一个走 env，一个走 file：

```http
PUT /api/credentials/GITHUB_TOKEN
Authorization: Bearer tos_user_token
Content-Type: application/json

{
  "value": "ghp_exampletoken",
  "inject": "env",
  "scope": "selected",
  "workspace_ids": ["ws_9xq2"]
}
```

```http
PUT /api/credentials/SSH_DEPLOY_KEY
Authorization: Bearer tos_user_token
Content-Type: application/json

{
  "value": "-----BEGIN OPENSSH PRIVATE KEY-----\n...\n-----END OPENSSH PRIVATE KEY-----\n",
  "inject": "file",
  "path": "~/.ssh/id_ed25519",
  "mode": "0600",
  "scope": "selected",
  "workspace_ids": ["ws_9xq2"]
}
```

最后创建 Codex workspace，同时写入 MCP、provider、prompt、Skills 挂载关系：

```http
POST /api/workspaces/
Authorization: Bearer tos_user_token
Content-Type: application/json

{
  "name": "小红书选题助手",
  "agent_type": "codex",
  "provider_id": "prv_openai_proxy_01",
  "model": "gpt-5",
  "small_model": "gpt-5-mini",
  "prompt_id": "prm_xhs_editor_01",
  "mcp_config": "{\"mcpServers\":{\"xhs-search\":{\"url\":\"https://mcp.example.com/xhs/mcp\",\"headers\":{\"X-Tenant\":\"brand-a\"}},\"local-report\":{\"command\":\"node\",\"args\":[\"/workspace/tools/report-mcp.js\"],\"env\":{\"REPORT_DIR\":\"/workspace/reports\"}},\"tos-platform\":{\"headers\":{\"X-Builder\":\"enabled\"}}}}",
  "agent_settings": "sandbox_mode = \"danger-full-access\"\napproval_policy = \"never\"\n",
  "skill_ids": ["9f4b0b5d-2e6c-4c0c-9c2d-2c19d9b4f111"],
  "compute_resources": {
    "cpu_request": "1",
    "memory_request": "2Gi",
    "storage": "20Gi"
  }
}
```

创建时 `workspaces` 与 `workspace_config` 在同一事务里初始化；如果没有 template，`write.ts` 会把请求里的 provider/prompt/model/MCP/settings 写入 `workspace_config`，并把 `skill_ids` 写入 `workspace_skills`。随后 placement 记录期望运行位置，workspace 状态进入 `starting`。

## 数据库快照

`workspaces` 保存工作区身份和运行状态：

```json
{
  "id": "ws_9xq2",
  "user_id": "user_7f3a",
  "name": "小红书选题助手",
  "status": "starting",
  "visibility": "private",
  "is_system": false,
  "runtime_version": "codex-2026-07-20"
}
```

`workspace_config` 是 Codex 启动配置的主表：

```json
{
  "workspace_id": "ws_9xq2",
  "agent_type": "codex",
  "provider_id": "prv_openai_proxy_01",
  "provider_type": "",
  "base_url": "",
  "api_key": "",
  "model": "gpt-5",
  "small_model": "gpt-5-mini",
  "prompt_id": "prm_xhs_editor_01",
  "system_prompt": "",
  "mcp_config": "{\"mcpServers\":{\"xhs-search\":{\"url\":\"https://mcp.example.com/xhs/mcp\",\"headers\":{\"X-Tenant\":\"brand-a\"}},\"local-report\":{\"command\":\"node\",\"args\":[\"/workspace/tools/report-mcp.js\"],\"env\":{\"REPORT_DIR\":\"/workspace/reports\"}},\"tos-platform\":{\"headers\":{\"X-Builder\":\"enabled\"}}}}",
  "agent_settings": "sandbox_mode = \"danger-full-access\"\napproval_policy = \"never\"\n",
  "compute_resources": {
    "cpu_request": "1",
    "memory_request": "2Gi",
    "storage": "20Gi"
  },
  "auto_start": true
}
```

`getWorkspaceConfig()` 读取时会联表解析 `provider_id` 和 `prompt_id`，所以 agent-facing payload 里会出现 provider 的 `provider_type/base_url/api_key` 和 prompt 的 `prompt_content`。这些不是都物理存回 `workspace_config`。

`model_providers`：

```json
{
  "id": "prv_openai_proxy_01",
  "name": "TOS OpenAI Proxy",
  "provider_type": "openai",
  "base_url": "https://llm-gateway.example.com/v1",
  "api_key": "sk-prod-abc123",
  "user_id": "user_7f3a",
  "visibility": "private"
}
```

`prompts` 和 `prompt_versions`：

```json
{
  "prompts": {
    "id": "prm_xhs_editor_01",
    "user_id": "user_7f3a",
    "name": "小红书爆款选题助手",
    "content": "你是小红书内容策划助手。输出必须包含选题、标题、封面文案和风险提醒。",
    "current_version": 1,
    "visibility": "private"
  },
  "prompt_versions": {
    "id": "prmv_01",
    "prompt_id": "prm_xhs_editor_01",
    "version": 1,
    "content": "你是小红书内容策划助手。输出必须包含选题、标题、封面文案和风险提醒。"
  }
}
```

Skills 资源由 SCS 写内容表，control-plane 写挂载关系：

```json
{
  "skill_sources": {
    "id": "src_3b7b",
    "user_id": "user_7f3a",
    "kind": "native",
    "draft_package": null
  },
  "skills": {
    "id": "9f4b0b5d-2e6c-4c0c-9c2d-2c19d9b4f111",
    "source_id": "src_3b7b",
    "active_version_id": "6a6d57e1-0f35-4b7a-a84c-9b2df70f5d90",
    "name": "xhs-research",
    "user_id": "user_7f3a",
    "visibility": "private"
  },
  "skill_versions": {
    "id": "6a6d57e1-0f35-4b7a-a84c-9b2df70f5d90",
    "skill_id": "9f4b0b5d-2e6c-4c0c-9c2d-2c19d9b4f111",
    "package": "<tar.gz bytea>",
    "content_hash": "sha256:6b3d..."
  },
  "workspace_skills": {
    "workspace_id": "ws_9xq2",
    "skill_id": "9f4b0b5d-2e6c-4c0c-9c2d-2c19d9b4f111"
  }
}
```

Credentials 只在 agent-facing internal API 里解密返回；普通用户 API 不返回明文：

```json
{
  "user_credentials": [
    {
      "user_id": "user_7f3a",
      "name": "GITHUB_TOKEN",
      "encrypted_value": "base64(pgp_sym_encrypt(...))",
      "inject": "env",
      "path": null,
      "mode": null,
      "scope": "selected",
      "status": "active"
    },
    {
      "user_id": "user_7f3a",
      "name": "SSH_DEPLOY_KEY",
      "encrypted_value": "base64(pgp_sym_encrypt(...))",
      "inject": "file",
      "path": "~/.ssh/id_ed25519",
      "mode": "0600",
      "scope": "selected",
      "status": "active"
    }
  ],
  "user_credential_workspaces": [
    {
      "user_id": "user_7f3a",
      "credential_name": "GITHUB_TOKEN",
      "workspace_id": "ws_9xq2"
    },
    {
      "user_id": "user_7f3a",
      "credential_name": "SSH_DEPLOY_KEY",
      "workspace_id": "ws_9xq2"
    }
  ]
}
```

MCP 目录和 OAuth 表不代表实际挂载清单：

```json
{
  "mcp_catalog": {
    "id": "xhs-search",
    "label": "XHS Search",
    "url": "https://mcp.example.com/xhs/mcp",
    "required": false,
    "params": []
  },
  "mcp_oauth_tokens": {
    "user_id": "user_7f3a",
    "server_origin": "https://mcp.example.com",
    "access_token": "oauth_access_token",
    "refresh_token": "oauth_refresh_token"
  }
}
```

真正的 workspace MCP 清单仍然在 `workspace_config.mcp_config`。如果某个 HTTP MCP origin 有 OAuth token，`GET /_cp/workspaces/:id/config` 返回前会把该 MCP URL 改写到 CP proxy。

## Agent 启动拉取

Pod 启动时通常带这些环境变量：

```text
CP_URL=http://nap-cp.default.svc.cluster.local:3000
WORKSPACE_ID=ws_9xq2
WORKSPACE_DIR=/workspace
HOME=/root
```

Codex agent 首先请求配置：

```http
GET /_cp/workspaces/ws_9xq2/config
Host: nap-cp.default.svc.cluster.local:3000
```

control-plane 返回给 agent 的 payload 已经是解析后的运行视图：

```json
{
  "agent_type": "codex",
  "provider_id": "prv_openai_proxy_01",
  "prompt_id": "prm_xhs_editor_01",
  "prompt_name": "小红书爆款选题助手",
  "prompt_content": "你是小红书内容策划助手。输出必须包含选题、标题、封面文案和风险提醒。",
  "provider_type": "openai",
  "model": "gpt-5",
  "base_url": "https://llm-gateway.example.com/v1",
  "api_key": "sk-prod-abc123",
  "small_model": "gpt-5-mini",
  "system_prompt": "",
  "mcp_config": "{\"mcpServers\":{\"xhs-search\":{\"url\":\"http://nap-cp.default.svc.cluster.local:3000/_cp/mcp/user_7f3a/aHR0cHM6Ly9tY3AuZXhhbXBsZS5jb20=/xhs/mcp\",\"headers\":{\"X-Tenant\":\"brand-a\",\"X-Workspace-ID\":\"ws_9xq2\",\"X-Agent-ID\":\"ws_9xq2\"}},\"local-report\":{\"command\":\"node\",\"args\":[\"/workspace/tools/report-mcp.js\"],\"env\":{\"REPORT_DIR\":\"/workspace/reports\"}},\"tos-platform\":{\"headers\":{\"X-Builder\":\"enabled\"}}}}",
  "agent_settings": "sandbox_mode = \"danger-full-access\"\napproval_policy = \"never\"\n",
  "compute_resources": {
    "cpu_request": "1",
    "memory_request": "2Gi",
    "storage": "20Gi"
  },
  "auto_start": true,
  "user_display_name": "Alice",
  "memory_attachments": []
}
```

这里有三点容易混淆：

- `prompt_content` 优先于 `system_prompt`，来自 `prompts` / `prompt_versions` 联表。
- `api_key` 对用户 API 是 write-only，但 internal config 会返回明文给 agent，用于启动模型进程。
- `tos-platform` 不由 CP 静态注入；如果用户配置了同名 headers，agent 会把这些 headers 合并进动态平台 MCP。

## 本地文件落盘

TS Codex 和当前 Python Codex 都会写这些核心文件。

`/workspace/AGENTS.md`：

```md
你是小红书内容策划助手。输出必须包含选题、标题、封面文案和风险提醒。
```

`/workspace/runtime.json`：

```json
{
  "model": "gpt-5",
  "provider_type": "openai",
  "base_url": "https://llm-gateway.example.com/v1",
  "small_model": "gpt-5-mini"
}
```

`/root/.codex/mcp.json`：

```json
{
  "mcpServers": {
    "xhs-search": {
      "url": "http://nap-cp.default.svc.cluster.local:3000/_cp/mcp/user_7f3a/aHR0cHM6Ly9tY3AuZXhhbXBsZS5jb20=/xhs/mcp",
      "headers": {
        "X-Tenant": "brand-a",
        "X-Workspace-ID": "ws_9xq2",
        "X-Agent-ID": "ws_9xq2"
      }
    },
    "local-report": {
      "command": "node",
      "args": ["/workspace/tools/report-mcp.js"],
      "env": {
        "REPORT_DIR": "/workspace/reports"
      }
    },
    "tos-platform": {
      "headers": {
        "X-Builder": "enabled"
      }
    }
  }
}
```

`/root/.codex/config.toml`：

```toml
model_provider = "custom"
model = "gpt-5"
disable_response_storage = true
sandbox_mode = "danger-full-access"
approval_policy = "never"
experimental_use_rmcp_client = true

[model_providers.custom]
name = "custom"
base_url = "https://llm-gateway.example.com/v1"
wire_api = "responses"

[mcp_servers.local-report]
command = "node"
args = ["/workspace/tools/report-mcp.js"]
env = { REPORT_DIR = "/workspace/reports" }

[features]
memories = false
```

HTTP MCP 不写入 `[mcp_servers]`，只在 ACP session 创建/加载时动态传给 `codex-acp`；stdio MCP 写入 `config.toml`，由 Codex 原生配置静态启动。

`/root/.codex/auth.json`：

```json
{"OPENAI_API_KEY":"sk-prod-abc123"}
```

`loadCredentials()` 再请求：

```http
GET /_cp/workspaces/ws_9xq2/credentials
```

响应包含解密后的值：

```json
[
  {
    "name": "GITHUB_TOKEN",
    "value": "ghp_exampletoken",
    "inject": "env",
    "path": null,
    "mode": null,
    "scope": "selected",
    "status": "active"
  },
  {
    "name": "SSH_DEPLOY_KEY",
    "value": "-----BEGIN OPENSSH PRIVATE KEY-----\n...\n-----END OPENSSH PRIVATE KEY-----\n",
    "inject": "file",
    "path": "~/.ssh/id_ed25519",
    "mode": "0600",
    "scope": "selected",
    "status": "active"
  }
]
```

落盘和环境注入结果：

```text
process.env.GITHUB_TOKEN=ghp_exampletoken
process.env.BASH_ENV=/root/.agent-credentials.env
/root/.agent-credentials.env
/root/.ssh/config
/root/.ssh/id_ed25519
```

`/root/.agent-credentials.env`：

```sh
export GITHUB_TOKEN='ghp_exampletoken'
```

`/root/.ssh/config`：

```sshconfig
StrictHostKeyChecking accept-new
```

## Skills 下载与 Codex 可见性

TS Codex agent 启动时会在 `loadConfig()` 后执行 `loadSkills()`：

```http
GET /_cp/workspaces/ws_9xq2/skills
```

响应：

```json
{
  "skills": [
    {
      "id": "9f4b0b5d-2e6c-4c0c-9c2d-2c19d9b4f111",
      "name": "xhs-research",
      "editable": true,
      "gitSource": false
    }
  ]
}
```

agent 按 UUID 下载 active version：

```http
GET /_cp/skills/9f4b0b5d-2e6c-4c0c-9c2d-2c19d9b4f111/package
If-None-Match: "sha256:6b3d..."
```

若版本未变，SCS 经 CP 透传 `304`；若有新包，agent 解压到 staging，再原子替换：

```text
/tmp/skill-xhs-research/
  SKILL.md
  references/research-workflow.md
/tmp/.skill-etag-xhs-research
/tmp/.skill-managed-xhs-research
/root/.codex/skills/xhs-research -> /tmp/skill-xhs-research
/root/.codex/skills/__platform__/
  SKILL.md
```

示例 `SKILL.md`：

```md
# xhs-research

当用户要求调研小红书内容趋势、竞品笔记或爆款标题时使用本 skill。

## 流程

1. 先明确品牌、品类、目标人群。
2. 使用 MCP 或浏览工具收集候选笔记。
3. 输出选题、标题、封面文案、风险提醒。
```

Codex 不需要 ACP capability 显式声明 Skills。Koala TS server 的 capability 里 `skills: false`，因为 Codex CLI/ACP 会按自身规则扫描 `/root/.codex/skills` 并读取每个目录的 `SKILL.md`。

当前 Python `media_agents/codex` 的状态不同：`CodexHooks.load_skills()` 只记录 `Skills load skipped (not implemented)` 并返回成功，`CpClient` 也没有 Skills list/package API。因此 Python 版目前不会下载、解压或生成 `/root/.codex/skills`；如果目标运行时是 Python Codex，要补齐 Skills 同步后才能达到上面 TS Codex 的效果。

## ACP Session 挂 MCP

用户发起聊天时，control-plane 会给 agent `/chat` 带上 `session_token`。Python 侧 `Chat` schema 里这个字段是不透明 token，不直接暴露真实 session id：

```http
POST http://ws_9xq2-agent:3001/chat
Content-Type: application/json

{
  "message": "帮我调研 20 个小红书露营咖啡爆款选题",
  "session_id": null,
  "session_token": "4bd0a38a-4d44-4d5f-b09a-3b1f31bc8410"
}
```

agent 在 `AcpServer.create_session()` 中启动一个新的 `codex-acp` bridge，然后调用 `hooks.load_mcp_servers(session_token)`。传给 ACP `new_session` 的 MCP 列表类似：

```json
[
  {
    "type": "http",
    "name": "tos-platform",
    "url": "http://nap-cp.default.svc.cluster.local:3000/mcp",
    "headers": {
      "X-Builder": "enabled",
      "X-Workspace-ID": "ws_9xq2",
      "X-Agent-ID": "ws_9xq2",
      "X-Session-Token": "4bd0a38a-4d44-4d5f-b09a-3b1f31bc8410"
    }
  },
  {
    "type": "http",
    "name": "xhs-search",
    "url": "http://nap-cp.default.svc.cluster.local:3000/_cp/mcp/user_7f3a/aHR0cHM6Ly9tY3AuZXhhbXBsZS5jb20=/xhs/mcp",
    "headers": {
      "X-Tenant": "brand-a",
      "X-Workspace-ID": "ws_9xq2",
      "X-Agent-ID": "ws_9xq2",
      "X-Session-Token": "4bd0a38a-4d44-4d5f-b09a-3b1f31bc8410"
    }
  }
]
```

同时 stdio MCP `local-report` 已在 `/root/.codex/config.toml` 中，`codex-acp` 按 Codex 原生配置启动本地进程。

运行时请求分两类：

- `tos-platform` 请求 `POST /mcp`，CP 根据 `X-Workspace-ID` 与 `X-Session-Token` 解析平台工具上下文。
- 第三方 HTTP MCP 请求 `/_cp/mcp/:userId/:encodedOrigin/*`，CP proxy 用 `mcp_oauth_tokens` 刷新并注入上游 `Authorization`，同时把 session token 解析为 workspace/session 上下文。

`session_tokens` 示例：

```json
{
  "token": "4bd0a38a-4d44-4d5f-b09a-3b1f31bc8410",
  "workspace_id": "ws_9xq2",
  "session_id": "sess_2f8c",
  "resolved_at": "2026-07-25T03:12:10.000Z"
}
```

## Reload 时序

如果用户后来改 workspace 配置：

```http
PUT /api/workspaces/ws_9xq2/config
Authorization: Bearer tos_user_token
Content-Type: application/json

{
  "model": "gpt-5.1",
  "mcp_config": "{\"mcpServers\":{\"xhs-search\":{\"url\":\"https://mcp.example.com/xhs/mcp\",\"headers\":{\"X-Tenant\":\"brand-b\"}},\"local-report\":{\"command\":\"node\",\"args\":[\"/workspace/tools/report-mcp.js\"],\"env\":{\"REPORT_DIR\":\"/workspace/reports\"}}}}"
}
```

control-plane 执行 `applyWorkspaceConfigUpdate()`：

```text
1. getWorkspace(ws_9xq2)
2. updateWorkspaceConfig(ws_9xq2, patch)
3. 如果 workspace 不在 running，结束
4. reconcileWorkspacePod(ws_9xq2)
5. 若 agent_type/runtime 镜像需要变更，rebuild 并把 status 改为 starting
6. 否则 notifyAgentReload(ws_9xq2, ["config"])
```

agent 收到：

```http
POST /reload-config
Content-Type: application/json

{
  "scope": ["config"]
}
```

Python adapter 的 reload 流程：

```text
1. hooks.load_config()
2. hooks.restart_bridge()
3. AcpServer.destroy_idle()
4. busy session 标记 pending_destroy，当前 turn 结束后 drain_pending 再销毁
5. 新建 bridge 会继承新的 OPENAI_API_KEY/OPENAI_BASE_URL 与新的 MCP session 列表
```

Provider 更新也会通知正在使用该 provider 的运行中 workspace reload `config`；prompt 更新会通知引用该 prompt 的运行中 workspace reload；credential 写入或删除会通知该用户所有运行中 workspace reload `credentials`；Skill 发布或切 active version 会通过 reload fanout 通知挂载它的运行中 workspace reload `skills`。

## Usage 回传与采集

Codex 使用量不是 agent 主动上报到 CP，而是 CP pull。

Codex 本地 transcript 示例文件：

```text
/root/.codex/sessions/2026/07/25/rollout-sess_2f8c.jsonl
```

其中 token_count 事件类似：

```json
{"timestamp":"2026-07-25T03:15:20.000Z","type":"event_msg","payload":{"type":"token_count","info":{"model":"gpt-5","total_token_usage":{"input_tokens":1200,"cached_input_tokens":300,"output_tokens":500,"reasoning_output_tokens":80,"total_tokens":1780}}}}
```

agent 暴露：

```http
POST /usage
Content-Type: application/json

{
  "cursors": {},
  "maxFiles": 50
}
```

Python `sweep_usage()` 扫描 `/root/.codex/sessions/**/rollout-*.jsonl`，把累计 token 计数转换为增量 records：

```json
{
  "records": [
    {
      "source": "codex",
      "session_id": "sess_2f8c",
      "model": "gpt-5",
      "ts": "2026-07-25T03:15:20.000Z",
      "input_tokens": 900,
      "output_tokens": 500,
      "cache_read_tokens": 300,
      "cache_creation_tokens": 0,
      "cache_creation_5m_tokens": 0,
      "cache_creation_1h_tokens": 0,
      "reasoning_tokens": 80,
      "web_search_requests": 0,
      "speed": null,
      "fields_incomplete": false,
      "dedup_key": "codex:sess_2f8c:1780"
    }
  ],
  "cursors": {
    "/root/.codex/sessions/2026/07/25/rollout-sess_2f8c.jsonl": {
      "dev": 45,
      "ino": 78910,
      "size": 4096,
      "mtimeMs": 1784950000000
    }
  },
  "hasMore": false
}
```

control-plane 的 `pullWorkspaceUsage()` 会：

```text
1. 读取 workspace_usage_cursor.cursor
2. POST ws agent /usage，body 带 cursors 和 maxFiles
3. insertUsageRecords(workspace_id, user_id, records)
4. setUsageCursor(workspace_id, cursors)
5. 如果 hasMore=true，继续下一批，直到批次数上限
```

`workspace_usage_events` 是 append-only ledger，用 `dedup_key` 保证重复 pull 幂等：

```json
{
  "workspace_id": "ws_9xq2",
  "user_id": "user_7f3a",
  "session_id": "sess_2f8c",
  "source": "codex",
  "model": "gpt-5",
  "input_tokens": 900,
  "output_tokens": 500,
  "cache_read_tokens": 300,
  "cache_creation_tokens": 0,
  "cache_creation_5m_tokens": 0,
  "cache_creation_1h_tokens": 0,
  "reasoning_output_tokens": 80,
  "web_search_requests": 0,
  "speed": null,
  "fields_incomplete": false,
  "ts": "2026-07-25T03:15:20.000Z",
  "dedup_key": "codex:sess_2f8c:1780"
}
```

`workspace_usage_cursor` 保存下一次扫描的文件指纹：

```json
{
  "workspace_id": "ws_9xq2",
  "cursor": {
    "/root/.codex/sessions/2026/07/25/rollout-sess_2f8c.jsonl": {
      "dev": 45,
      "ino": 78910,
      "size": 4096,
      "mtimeMs": 1784950000000
    }
  }
}
```

## 执行时序汇总

```text
用户/API
  ├─ POST /api/providers/                         -> model_providers
  ├─ POST /api/prompts/                           -> prompts + prompt_versions
  ├─ POST /api/skills                             -> SCS 写 skill_sources/skills/skill_versions
  ├─ PUT  /api/credentials/:name                  -> user_credentials + user_credential_workspaces
  └─ POST /api/workspaces/                        -> workspaces + workspace_config + workspace_skills

control-plane
  ├─ placement/env-runner 创建或更新 workspace pod
  ├─ GET /_cp/workspaces/ws_9xq2/config           -> 返回联表后的 agent-facing config
  ├─ GET /_cp/workspaces/ws_9xq2/credentials      -> 返回解密后的 active credentials
  ├─ GET /_cp/workspaces/ws_9xq2/skills           -> 返回 workspace 挂载的 skill UUID/name
  └─ GET /_cp/skills/:id/package                  -> 透传 SCS tar.gz/ETag

Codex agent
  ├─ load_config                                  -> AGENTS.md/runtime.json/mcp.json/config.toml/auth.json
  ├─ load_skills                                  -> TS 写 /root/.codex/skills；Python 当前跳过
  ├─ load_credentials                             -> env/file/ssh/shell source
  ├─ bridge_factory                               -> codex-acp cwd=/workspace INITIAL_AGENT_MODE=agent-full-access
  ├─ /chat + session_token                        -> ACP new_session/load_session 动态挂 HTTP MCP
  └─ /usage                                       -> 扫 rollout JSONL 返回 records/cursors

usage pull
  ├─ CP session.ended 或 reconcile sweep 调 pullWorkspaceUsage
  ├─ POST agent /usage
  ├─ INSERT workspace_usage_events ON CONFLICT DO NOTHING
  └─ UPSERT workspace_usage_cursor
```

## 关键结论

- Codex workspace 的主配置存储是 `workspace_config`；provider、prompt、template 在 `getWorkspaceConfig()` 中解析成 agent-facing payload。
- workspace 实际 MCP 清单存在 `workspace_config.mcp_config`；`mcp_catalog` 是目录，`mcp_oauth_*` 是 OAuth/proxy 辅助状态。
- HTTP MCP 是 ACP session 级动态挂载，用来注入 `X-Session-Token`；stdio MCP 是 Codex `config.toml` 级静态挂载。
- TS Codex 已完整支持 Skills 下载、ETag、解压、symlink 和 `__platform__` skill；当前 Python Codex 还没有 Skills 同步实现。
- Codex 看到 Skills 的方式是本地文件系统 `/root/.codex/skills/*/SKILL.md`，不是 ACP capability。
- Credentials 是用户级资源，数据库只存加密值；agent-facing internal API 才返回明文并注入 env/file。
- Usage 是 CP pull 模式：agent 解析本地 transcript，CP 写 append-only `workspace_usage_events`，并用 `workspace_usage_cursor` 与 `dedup_key` 保证增量和幂等。
