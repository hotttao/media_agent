# Codex agent 其他内容配置链路

本文只说明 Codex agent 除 MCP 和 Skills 以外的内容如何进入 control-plane、数据库如何保存、agent 如何下载并落盘。MCP 与 Skills 的详细链路不在本文展开，只在涉及同一个 config payload 时点到字段。

## 总体流程

Codex 相关内容的主路径是：

1. 用户或系统通过 control-plane 的用户 API 创建/更新 workspace、provider、prompt、credential 等配置。
2. control-plane 将配置写入 PostgreSQL，主要落在 `workspaces`、`workspace_config`、`model_providers`、`prompts`、`user_credentials`、`user_credential_workspaces`、`workspace_usage_events` 等表。
3. 运行中的 agent 由 control-plane 通知 reload，或在容器启动时主动拉取配置。
4. Codex agent 调用 `GET /_cp/workspaces/:id/config` 和 `GET /_cp/workspaces/:id/credentials`，把配置转换成本地文件、环境变量和 Codex ACP 启动参数。
5. 使用量不是 agent 启动配置的一部分。control-plane 定时或会话结束后从 agent 的 `/usage` 端点拉取本地 transcript usage，写入使用量账本表。

Python 版 `media_agents/codex` 当前实现与 Koala 侧 Node 版 Codex agent 的配置落盘语义基本一致；差异是 Python 版 `load_skills()` 仍是 skip/not implemented。

## Workspace 与运行配置

### 上传/更新入口

创建 workspace 使用 `POST /api/workspaces`。不使用 template 时，可直接带这些 Codex 相关字段：

- `agent_type`：agent 镜像/类型，Codex 使用 `codex`。
- `provider_id`：引用已保存的模型 provider。
- `provider_type`、`base_url`、`api_key`、`model`、`small_model`：直接内联 provider/model 配置。
- `prompt_id`、`system_prompt`：系统提示词来源。
- `agent_settings`：传给具体 agent 的附加设置。Codex 侧期望 TOML 片段；JSON 风格设置会被跳过，不追加到 `config.toml`。
- `compute_resources`：资源规格。
- `template_id`：使用模板时，agent/provider/prompt/settings/compute_resources 等从 template version 解析，workspace 自己的字段可覆盖部分值。

更新已有 workspace 配置使用 `PUT /api/workspaces/:id/config`。空的 `api_key` 会被视为“不更新”，避免把已保存密钥清空。更新后 `applyWorkspaceConfigUpdate()` 会先写库；如果 `agent_type` 变化导致镜像需要变化，会重建容器，否则对运行中的 workspace 发送 `reload-config`，让 agent 热加载 config。

还有内部入口 `PUT /_cp/workspaces/:id/config`，用于内部系统或 builder-mode 工具走同一套 `applyWorkspaceConfigUpdate()`。

### 数据库存储

基础 workspace 身份在 `workspaces`：

- `id`：workspace id，也是 agent 拉配置时的 `WORKSPACE_ID`。
- `user_id`：拥有者，用于 provider、credential、usage 归属。
- `name`、`slug`、`visibility`：展示名、调用/分享可见性。
- `status`：`running`、`starting`、`stopped` 等，决定更新配置后是否需要通知 agent。
- `runtime_version`：运行时模板版本，用来判断是否需要 rebuild。

agent 运行配置集中在 `workspace_config`：

- `workspace_id`：关联 `workspaces.id`。
- `agent_type`：选择 agent 类型/镜像。Codex 对应 `codex`。
- `provider_id`：可选，引用 `model_providers.id`。存在时读取 provider 表中的 `provider_type`、`base_url`、`api_key`。
- `provider_type`：provider 类型。Codex agent 最终只按 OpenAI-compatible 环境变量处理。
- `base_url`：OpenAI-compatible API base URL。为空时 Codex 配置使用内置 `openai` provider；非空时写 custom provider。
- `api_key`：workspace 内联 API key。若 `provider_id` 存在，`getWorkspaceConfig()` 会优先取 provider 的 key。
- `model`、`small_model`：主模型与小模型。Codex 启动配置实际写入 `model`，`small_model` 只进入共享 `runtime.json`。
- `system_prompt`：内联系统提示词。
- `prompt_id`、`prompt_version`：引用 prompt library。agent-facing config 会额外返回 `prompt_content`，Codex 优先使用它。
- `agent_settings`：Codex 的额外 TOML 设置。默认值是字符串 `'{}'`。
- `compute_resources`：JSONB，资源请求/限制与存储规格。
- `auto_start`：停止状态 workspace 收到聊天请求时是否自动启动。
- `template_id`、`template_version`：使用模板时，从 `template_versions` 解析默认 agent/provider/prompt/settings。

`getWorkspaceConfig()` 的解析规则比较关键：如果 workspace 使用模板，`template_versions` 上的 `agent_type`、`provider_id`、`model`、`small_model`、`system_prompt`、`agent_settings`、`compute_resources` 等会作为默认值；workspace 自己配置了非空值时覆盖模板值。`prompt_content` 是运行时联表 `prompts` / `prompt_versions` 得到的内容，不是 `workspace_config` 的物理字段。

Provider library 在 `model_providers`：

- `id`、`name`：provider 标识与展示名。
- `provider_type`：如 OpenAI-compatible、Anthropic 等类型。Codex 侧只消费 OpenAI-compatible 风格。
- `base_url`、`api_key`：provider 连接信息。
- `user_id`、`visibility`、`is_public`：所有者与可见性。
- `description`、`created_at`、`updated_at`：元数据。

Prompt library 在 `prompts`：

- `id`、`name`、`content`：提示词标识、名称、当前内容。
- `current_version`：当前版本号；历史版本另有 prompt version 表。
- `user_id`、`visibility`、`is_public`：所有者与可见性。

## Agent 下载与本地落盘

Codex agent 启动时由 `CodexHooks.load_config()` 调 `CpClient.fetch_workspace_config()`，请求：

```text
GET {CP_URL}/_cp/workspaces/{WORKSPACE_ID}/config
```

control-plane 返回 `ApiWorkspaceConfig`，agent 做以下落盘：

- `/workspace/AGENTS.md`：写入系统提示词。优先用 `prompt_content`，否则用 `system_prompt`。这是 Codex 读取 workspace 指令的主文件。
- `/workspace/runtime.json`：写入共享运行时信息：
  - `model`
  - `provider_type`
  - `base_url`
  - `small_model`
- `~/.codex/config.toml`：写入 Codex CLI/ACP 配置：
  - `model_provider = "openai"` 或 `"custom"`，取决于 `base_url` 是否为空。
  - `model = "<model>"`。
  - `disable_response_storage = true`。
  - 默认补 `sandbox_mode = "danger-full-access"` 与 `approval_policy = "never"`，除非 `agent_settings` 已显式包含同名 key。
  - `base_url` 非空时追加 `[model_providers.custom]`，设置 `name = "custom"`、`base_url`、`wire_api = "responses"`。
  - 追加 `[features] memories = false`，平台托管 memory，不启用 Codex 内置 memory。
  - 如果 `agent_settings` 是非空、非 `'{}'`、且不是 JSON 起始的文本，则作为 TOML 片段追加到文件末尾。
- `~/.codex/auth.json`：当 config 中有 `api_key` 时写入 `{"OPENAI_API_KEY": "<api_key>"}`。

随后 `restart_bridge()` 会读取 `/workspace/runtime.json`，并结合内存中的 `_api_key` 调用 provider env 注入逻辑。Codex ACP 实际消费的是：

- `OPENAI_API_KEY`
- `OPENAI_BASE_URL`，仅当 `base_url` 非空时设置。

同时会清理 Anthropic 相关 env 与旧的 OpenAI/Codex env，避免 provider 切换后残留。

Codex ACP bridge 启动时使用：

- program：默认 `codex-acp`。
- cwd：workspace 目录，默认 `/workspace`。
- env：`INITIAL_AGENT_MODE=agent-full-access`，保证容器内 workspace、AFS、memory 等挂载路径按平台约定可写。

agent server 还会启动辅助进程：

- `ttyd`：Web terminal，工作目录为 `/workspace`。
- `dufs` workspace 文件浏览：服务 `/workspace`。
- `dufs` AFS 文件浏览：服务 `/mnt/afs`。

## Credentials

### 上传/更新入口

凭据使用用户级 API：

```text
PUT /api/credentials/:name
DELETE /api/credentials/:name
GET /api/credentials
```

`PUT` body 的核心字段：

- `value`：凭据明文，只在写入时提交。
- `inject`：`env` 或 `file`。
- `path`：`inject=file` 时必须提供，支持 `~` 展开到 agent home。
- `mode`：文件权限，字符串八进制，如 `0600`。
- `scope`：`global` 或 `selected`。
- `workspace_ids`：`scope=selected` 时的 workspace 白名单。

`inject=env` 时，`:name` 必须是合法环境变量名。写入或删除凭据后，control-plane 会通知该用户所有运行中的 workspace reload credentials。

### 数据库存储

凭据元数据与密文保存在 `user_credentials`：

- `user_id`、`name`：联合主键语义，同一用户下凭据名唯一。
- `encrypted_value`：使用 `CREDENTIAL_ENCRYPTION_KEY` 经 `pgp_sym_encrypt` 加密后 base64 存储。
- `inject`：注入方式，`env` 或 `file`。
- `path`：文件注入目标路径。
- `mode`：文件权限。
- `scope`：`global` 或 `selected`。
- `status`：`active` 或 `deleting`。
- `updated_at`：更新时间。

`user_credential_workspaces` 存 selected scope 的关联：

- `user_id`
- `credential_name`
- `workspace_id`

agent-facing 的 `GET /_cp/workspaces/:id/credentials` 会根据 workspace owner 取：

- 所有 `scope='global'` 的 active credential。
- 所有 `scope='selected'` 且关联到当前 workspace 的 active credential。

返回给 agent 时会解密 `value`；普通用户侧 `GET /api/credentials` 只返回 metadata，不返回明文。

### Agent 下载与本地注入

Codex agent 启动或 reload credentials 时请求：

```text
GET {CP_URL}/_cp/workspaces/{WORKSPACE_ID}/credentials
```

本地处理规则：

- `inject=env`：写入当前 agent 进程环境变量，并生成 `~/.agent-credentials.env`，内容是 `export NAME='value'`。
- `inject=file`：写入指定 `path`；`~/` 会展开到 `HOME`；默认权限 `0600`，可由 `mode` 覆盖。
- `status=deleting`：删除对应 env 或文件。当前 agent-facing 查询只返回 active 凭据；deleting 主要用于 reload/hard-delete 协调。
- 自动确保 `~/.ssh` 存在并设置 `0700`；若 `~/.ssh/config` 不存在，会写入 `StrictHostKeyChecking accept-new`。
- 若有 env 凭据，会设置 `BASH_ENV=~/.agent-credentials.env`，并在 `~/.bashrc`、`~/.profile` 中追加 source 行，让新开的 terminal shell 也能拿到凭据。

## Prompt 与 platform prompt

Codex 有两层提示词文件：

- `/workspace/AGENTS.md`：workspace 级系统提示词，由 `prompt_content || system_prompt` 写入。当前 Python 实现就是这个逻辑。
- `~/.codex/AGENTS.md`：Koala Node 版还会通过 `writePlatformPrompt()` 渲染 platform prompt，包含 agent kind、workspace id、用户显示名、memory attachments 等平台说明。Python 版当前只写 workspace 内的 `AGENTS.md`，未看到对应 platform prompt 渲染逻辑。

control-plane 的 `GET /_cp/workspaces/:id/config` 会额外返回：

- `user_display_name`：来自 workspace owner 的 display name 或 username。
- `memory_attachments`：当前挂载的 memory store 列表，包含 `store_id`、`store_name`、`store_description`、`access`、`instructions`，以及 `/MEMORY.md` 的 `index_content` 快照。

这些字段在 Koala Node 版用于 platform prompt。它们不是本文的 MCP/Skills 内容，但属于 config payload 的一部分。

## Usage

Codex 使用量不通过 workspace config 下发，而是反向采集：

1. Codex agent 暴露 `POST /usage`。
2. control-plane 的 reconcile loop 或 session ended 事件调用 agent `/usage`。
3. agent 扫描本地 transcript usage，结合 `runtime.json` 中的 `model` 作为 fallback model，返回增量 records 和 cursor。
4. control-plane 用 `insertUsageRecords()` 写入 append-only ledger。

使用量表是 `workspace_usage_events`：

- `workspace_id`、`user_id`：归属快照。
- `session_id`：对应会话，可为空。
- `source`：usage 来源。
- `model`：模型名。
- `input_tokens`、`output_tokens`：输入/输出 token。
- `cache_read_tokens`、`cache_creation_tokens`、`cache_creation_5m_tokens`、`cache_creation_1h_tokens`：缓存相关 token。
- `reasoning_output_tokens`：reasoning token。
- `web_search_requests`：web search 次数。
- `speed`：速度/档位标记。
- `fields_incomplete`：该条记录字段是否不完整。
- `ts`：transcript 里的发生时间。
- `dedup_key`：去重键，唯一冲突时跳过，保证重复拉取幂等。
- `created_at`：入库时间。

`workspace_usage_cursor` 保存每个 workspace 的采集 cursor，避免每次全量扫描；删除 workspace 时会删除 cursor，但 usage ledger 本身保留。

## 关键结论

- Codex 的运行配置以 `workspace_config` 为中心，`provider_id`、`prompt_id`、`template_id/version` 会在 `getWorkspaceConfig()` 中解析成 agent-facing 的完整 payload。
- Codex agent 真正落盘的核心文件是 `/workspace/AGENTS.md`、`/workspace/runtime.json`、`~/.codex/config.toml`、`~/.codex/auth.json`。
- `prompt_content` 优先级高于 `system_prompt`；前者来自 prompt library 联表结果，不是 `workspace_config` 物理字段。
- `api_key` 对用户 API 是 write-only；agent-facing internal API 会返回明文，用于写 `auth.json` 和注入 `OPENAI_API_KEY`。
- credentials 是用户级资源，可全局或 selected workspace 生效；数据库只存加密值，agent-facing 拉取时才解密。
- 使用量是 control-plane pull 模式，不是 agent 主动上传；落在 append-only 的 `workspace_usage_events`，靠 `dedup_key` 保证幂等。
