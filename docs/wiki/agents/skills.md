# Agent Skills 流程

本文以 Codex agent 为例，说明 Skills 从用户上传到 control-plane、数据库持久化、agent 下载落盘，以及运行时被 Codex 使用的完整链路。

## 总览

Skills 是一组按目录打包的能力说明文件，核心入口通常是 `SKILL.md`。平台侧把 Skill 当作可版本化资源管理；工作区只保存“启用哪些 skill”的挂载关系。agent 启动或 reload 时，根据工作区挂载关系从 control-plane 下载当前 active version 的 tar.gz 包，解压到本地，再暴露给 Codex 运行时读取。

当前可完整跑通 Skills 下载/落盘的是 Koala `agent-platform/agents/codex` 的 TypeScript 实现；本仓库 Python `media_agents/codex` 目前 `CodexHooks.load_skills()` 仍是 `Skills load skipped (not implemented)`，只复用了配置、MCP、凭证和 ACP bridge 生命周期，还没有实现 Skills 同步。

## 上传和发布

control-plane 入口在 `control-plane/src/index.ts` 挂载 `app.route('/api/skills', skillsRoutes)`。用户侧的 Skills 管理 API 在 `control-plane/src/routes/skills.ts`：

- `POST /api/skills?name=...`：上传一个完整 tar.gz 包，metadata 通过 query 传入，包括 `name`、`description`、`visibility`、`category`。
- `POST /api/skills/from-git`：从 Git 仓库导入某个 subpath，生成 skill 和首个版本。
- `POST /api/skills/{id}/publish`：把 native source 的 draft 发布为新版本，并切换为 active version。
- `PUT /api/skills/{id}/active-version`：切换当前 active version。
- `PATCH /api/skills/{id}`：更新名称、描述、可见性、分类等 metadata。

control-plane 本身不直接 materialize tarball，也不直接写 `skills` / `skill_sources` / `skill_versions`。这些内容写入由 skills-content-service 负责，control-plane 通过 `services/skills-content.ts` 调用 `SKILLS_CONTENT_URL`，例如：

- `scsUploadSkill()` -> `POST {SKILLS_CONTENT_URL}/skills/upload`
- `scsPublishSkill()` -> `POST {SKILLS_CONTENT_URL}/skills/{id}/publish`
- `scsSetActiveVersion()` -> `PUT {SKILLS_CONTENT_URL}/skills/{id}/active-version`

control-plane 的职责是解析 HTTP 请求、做 ACL/可见性校验、维护工作区挂载关系、触发 reload。这个边界在 `services/skill-repository.ts` 里也有明确约定：SCS 拥有 `skills`、`skill_sources`、`skill_versions` 的写入；CP 拥有 `workspace_skills`、`skill_grants`、`template_version_skills` 等关系表写入。

## 数据库存储

核心表来自 `control-plane/migrations/001_init.sql`。

`skill_sources` 表描述 Skill 的来源：

- `id`：source UUID。
- `user_id`：source 所属用户。
- `kind`：`native` 或 `git`。
- `git_type`、`git_url`、`git_host`、`git_owner`、`git_repo`、`git_ref`、`credential_name`：Git source 的仓库信息和凭证名。
- `last_commit_sha`、`last_synced_at`：最近一次 Git sync 的提交和时间。
- `draft_package`：native source 的未发布草稿包，bytea。
- `created_at`、`updated_at`：审计时间。

`skills` 表是 Skill metadata 和 active version 指针：

- `id`：skill UUID。现在对外和 agent 下载都以 UUID 定位，不能只靠 name。
- `source_id`：关联 `skill_sources.id`。
- `active_version_id`：当前发布版本，关联 `skill_versions.id`。创建中可短暂为空。
- `name`：展示名，同一 `user_id` 下唯一。
- `subpath`：Git monorepo 中该 skill 所在子目录；native 通常为空。
- `description`：说明。
- `user_id`：owner。
- `visibility`：`private`、`team`、`public`。
- `is_public`：旧兼容字段，当前主要看 `visibility`。
- `category`：分类。
- `created_at`、`updated_at`：审计时间。

`skill_versions` 表保存每次发布的不可变内容快照：

- `id`：version UUID。
- `skill_id`：所属 skill。
- `source_id`：发布时来自哪个 source。
- `package`：tar.gz 包内容，bytea。
- `content_hash`：由 `package` 生成的 sha256，用于去重/幂等。
- `commit_sha`：Git source 对应提交，native 可为空。
- `note`：发布说明。
- `published_at`、`published_by`：发布审计信息。

`workspace_skills` 表保存工作区启用关系：

- `workspace_id`：工作区 ID。
- `skill_id`：启用的 skill UUID。
- 主键是 `(workspace_id, skill_id)`，表示一个工作区挂载一组 skills。

此外，`skill_grants` 控制 team 共享权限，`template_version_skills` 控制模板版本内置的 Skills 依赖。

## 挂载到工作区

工作区启用哪些 Skills 走 internal API：

- `PUT /_cp/workspaces/{id}/skills`：body 为 `{ "skills": ["skill-uuid", ...] }`，完整替换工作区挂载关系。
- `SkillsService.attachToWorkspace()` 会先读取 workspace owner，再用 `findSkillIdsNotVisibleToUser()` 校验这些 skill 对 owner 可见。
- 校验通过后，`PgSkillRepository.setWorkspaceSkills()` 在事务中删除旧 `workspace_skills`，再插入新的 `(workspace_id, skill_id)`。
- 如果工作区正在运行，control-plane 会调用 agent reload，要求刷新 `skills`。

Skill 发布、sync、切 active version 时，`SkillsService` 会把受影响 workspace 的 reload 放入队列。后台 worker 再调用 `/_cp/skills/{id}/reload-fanout`，枚举正在使用该 skill 的运行中工作区，并通知各 agent reload。这样热门 Skill 的更新不会阻塞发布请求。

## Agent 下载和落盘

Koala TypeScript Codex agent 在 `agents/codex/src/index.ts` 启动时依次执行：

1. `loadConfig()`：从 `/_cp/workspaces/{WORKSPACE_ID}/config` 拉模型、prompt、MCP 等配置，写入 `AGENTS.md`、`runtime.json`、`~/.codex/config.toml`、`~/.codex/auth.json`。
2. `loadSkills()`：创建 `SkillManager` 并执行 `SkillManager.load()`。
3. `loadCredentials()`：拉凭证并注入环境变量或文件。
4. 启动 `codex-acp` bridge、ttyd、dufs。

Codex 的 `SkillManager` 初始化关键参数：

- `skillsDir = join(home, '.codex', 'skills')`：Codex 运行时最终看到的目录。
- `localBase = '/tmp'`：发布版 skill 的解压目录在 tmpfs，避免 NFS 小文件开销。
- `draftBase = join(WORKSPACE_DIR, '.skills-draft')`：编辑中的 draft 放在工作区持久卷，避免 pod rebuild 丢失。
- `useSymlink = true`：`~/.codex/skills/{name}` 是指向实际内容目录的 symlink。
- `filesBrowsePath = '/.home/.codex/skills'`：前端文件浏览器使用的路径前缀。

下载过程在 `internal/agent-skills/src/index.ts`：

1. `GET {CP_URL}/_cp/workspaces/{WORKSPACE_ID}/skills` 获取当前工作区启用列表。返回项包含 `id`、`name`、`editable`、`gitSource`。
2. agent 记录 `name -> id`，目录名仍使用展示名 `name`，下载 URL 使用 UUID。
3. 对每个启用的 skill，调用 `GET {CP_URL}/_cp/skills/{id}/package` 下载 active version 的 tar.gz。
4. 如果本地有上次 ETag，会带 `If-None-Match`；CP 透传给 SCS，SCS 可返回 `304`，agent 就跳过解压。
5. 成功下载后，agent 写入 staging 目录，执行 `tar xzf` 解包，再把 staging rename 成 `/tmp/skill-{name}`。
6. 在 symlink 模式下，刷新 `~/.codex/skills/{name} -> /tmp/skill-{name}`。
7. ETag 写到 `/tmp/.skill-etag-{name}`，managed 标记写到 `/tmp/.skill-managed-{name}`。

失败策略是“保留旧内容”：单个 skill 下载或解压失败时，不会删除已有的可用目录。启动阶段如果有失败项，Codex agent 会退出，让 kubelet 重启重试，避免长时间运行在空 skills 目录上。

平台还会在下载用户 Skills 后安装一个保留 skill：`__platform__`。它由 agent 本地渲染写入，目录在 `~/.codex/skills/__platform__`，写完后 chmod 为只读；这个名字在 SkillManager 里是 reserved，不走用户编辑/发布生命周期。

## 运行时如何使用

Codex 运行时不需要通过 ACP capability 显式暴露 Skills。Koala Codex server 的 capability 里 `skills: false`，原因是 Skills 已经被 materialize 到 Codex 自己约定的本地目录：`~/.codex/skills`。

启动 `codex-acp` 时，bridge 的工作目录是 `WORKSPACE_DIR`，环境变量强制 `INITIAL_AGENT_MODE=agent-full-access`。Codex CLI/ACP 进程会按 Codex 自身规则扫描 `~/.codex/skills` 下的目录，并读取每个 skill 的 `SKILL.md` 及其附属文件。对 agent 来说，CP 和 SCS 都不在推理路径上；推理时看到的是本地文件系统中的 Skills。

编辑态和发布态路径不同：

- 发布态：内容在 `/tmp/skill-{name}`，`~/.codex/skills/{name}` 是 symlink。
- 草稿态：内容在 `{WORKSPACE_DIR}/.skills-draft/skill-{name}`，`~/.codex/skills/{name}` 指向草稿目录，并带 `.editing` 锁文件。
- agent reload 遇到 `.editing` 会跳过该 skill，避免覆盖用户正在编辑的草稿。

## 当前 Python 实现状态

本仓库 `media_agents/codex` 复用了同一类 agent 生命周期，但 Skills 部分还没接上：

- `media_agents/adapter/agent_lifespan.py` 会在启动时调用 `hooks.load_config()`、`hooks.load_skills()`、`hooks.load_credentials()`。
- `media_agents/codex/config.py` 的 `CodexHooks.load_config()` 已实现：写 `AGENTS.md`、`runtime.json`、`~/.codex/mcp.json`、`~/.codex/config.toml`、`~/.codex/auth.json`。
- `CodexHooks.load_skills()` 当前只记录日志并返回成功，没有请求 `/_cp/workspaces/{id}/skills`，也没有下载、解压或写 `~/.codex/skills`。
- `media_agents/adapter/clients/cp.py` 目前只有 `fetch_workspace_config()` 和 `fetch_credentials()`，没有 Skills list/package API。
- `media_agents/codex/main.py` 启动的是 FastAPI ACP adapter、Codex ACP bridge、ttyd、dufs；未注册 TypeScript 版里的 `/skills` 本地管理路由。

因此，如果要让 Python Codex agent 达到 Koala TS Codex 的 Skills 行为，需要补齐 CP client 的 Skills list/package 下载、ETag、tar 解压、`~/.codex/skills` symlink、draft 目录和本地 `/skills` 管理接口。
