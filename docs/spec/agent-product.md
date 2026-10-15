# Agent 可用产品功能规格

本规格补充现有 Agent Runtime：交付可由用户操作的资源管理、配置和正式 Agent 对话链路。隔离、扩缩容、故障接管及 SRE 专项建设延期，已有必要接口和业务权限保留。

## 已确认产品契约

- Prompt、Skill 是企业资源库资源，可跨同企业 Workspace 复用。默认 private，按部门授予 viewer/editor；public 仅企业内读取。owner 管理 ACL，editor 编辑内容，viewer 读取和选用；配置给 Agent 还要求 Workspace manage。
- Prompt 支持创建、编辑、历史读取和回滚。Agent 配置采用不设置、直接填写或引用一份库固定版本三种互斥方式，Platform Prompt 仍独立生成。内容实际变化创建历史，回滚保留历史。
- Skill 支持 ZIP/tar.gz、Git 导入及手动同步、在线文件树和增删改、草稿发布、历史查看和下载。发布版本不可变，草稿不能选给 Agent。
- 平台管理员维护平台 MCP；企业管理员可登记本企业外部 MCP。普通成员按授权选用，Workspace 配置要求 manage，Agent 统一连接 Gateway。目录事实在 Gateway，绑定和具体实现仍归 Plugin。
- 资源编辑、Git 同步或发布不自动升级 Agent。用户明确确认配置后保存不可变 Release 并通知，旧 Session/Turn 固定版本。

## Prompt 资源管理接口

沿用 `/api/v1/prompts`；新增必需企业上下文，旧 `workspace_id` 不再表示资源归属。创建接受 organization_id、name、content、visibility 和部门 grants；owner 从认证主体取得。列表只返回指定企业中可见资源，详情返回 my_permission。

更新正文/名称要求 owner/editor 和 expected_version；ACL 更新要求 owner 和 expected_version。历史列表与指定版本读取需要当前资源可读；回滚需要 owner/editor 和 expected_version，读取历史正文形成新的当前版本。归档要求 owner，保留已有 Release 需要的历史。

Auth 通过既有内部企业成员与部门权限接口提供当前事实；资源服务保存 Grant 并最终判定资源权限，不能仅依赖客户端或 JWT 中的企业、角色声明。企业已停用、非成员、跨企业及部门已停用均不能扩大资源权限；Auth 故障返回服务不可用。

迁移为当前资源增加企业归属及可见性、部门 Grant 表。已有 Workspace 资源仅从其真实 Workspace 回填企业；无法确定企业的历史资源保留数据并明确列为待归属，不猜测或自动公开。不得因删除原 Workspace 级联删除企业资源。

| 接口 | 行为 |
|:---|:---|
| POST /api/v1/prompts | organization_id、name、content、visibility、grants 创建企业资源 |
| GET /api/v1/prompts?organization_id=… | 当前用户在该企业可见的有效资源及 my_permission |
| GET/PUT /api/v1/prompts/{id} | 授权读取 / expected_version 更新名称与正文 |
| PUT /api/v1/prompts/{id}/access | owner 使用 expected_version 修改 visibility 与完整 grants |
| GET /api/v1/prompts/{id}/versions[/{version}] | 当前资源可读用户查询历史 |
| POST /api/v1/prompts/{id}/rollback | target_version、expected_version；新版本采用历史正文，名称与 ACL 保持当前值 |
| POST /api/v1/prompts/{id}/archive | owner 使用 expected_version 归档，保留历史 |
| GET /api/v1/prompts/unassigned | 仅原 owner 可见的未归属历史资源 |
| POST /api/v1/prompts/{id}/assign-organization | 原 owner 指定 organization_id、expected_version，一次性归入本人所属有效企业 |

private 不接受部门 Grant；team 至少需要一个 Grant；public 允许额外指定部门 editor，公共读取仍限制在企业内。Grant 数量最多100，部门必须 active 且属于同一 active 企业。资源当前版本同时作为内容/ACL 写入 CAS；历史读取以当前资源 ACL 判断，不恢复历史 ACL。旧历史缺少可信记录时间时返回 created_at=0，不伪造日期。

Agent 配置确认沿用 POST `/api/v1/workspaces/{id}/agent-metadata:confirm`，增加 `prompt={mode: none|custom|library, content, resource_id, resource_version}`。none 不接受正文或引用，custom 只接受正文，library 只接受固定 ID/版本；冲突输入拒绝。旧客户端可继续在 resources 中传单个 Prompt 引用，等价 library；不能与显式 none/custom 混用。

GET `/api/v1/workspaces/{id}/agent-metadata` 返回已确认来源、固定正文/引用与当前配置 CAS version；无已确认配置返回404。浏览器读取库来源正文仍要求资源可读，不能靠 Workspace view 泄露私有 Prompt。资源归档后已有发布保留，不能将归档资源选入新的配置。完整运行响应和消息格式保持兼容，Prompt 方式只进入 Control 的非秘密 Release，不要求修改 Agent 协议。

## 后续切片接口范围

### Skill 内容切片

Skill 当前 `version` 是资源 CAS，控制名称、ACL、草稿、Git 来源和发布；`published_version` 是不可变内容版本，供 Agent 选择，两者分别返回。`skills` 增加企业、visibility、description、draft_checksum、published_version、Git 来源和加密访问令牌，`skill_grants` 保存部门权限，`skill_versions` 保存发布内容引用、入口名称、描述和发布者。旧目录历史中有效的 sha256 包引用按原版本迁移，无法推断企业的资源保留为原 owner 的待归属项。

草稿和发布均存储规范化 ZIP（根目录 SKILL.md）；上传接受 ZIP/tar.gz 并去掉单层包裹目录，Git 选定目录也归一化为相同格式。包8MiB、单文件8MiB、展开32MiB、最多1024文件；拒绝逃逸路径和链接。编辑可暂时保存不完整 SKILL.md，发布时完整校验 YAML frontmatter 的 name/description。文件响应采用 bytes/base64，UI 对文本编辑、对二进制预览/下载。规范化包保留脚本可执行标志，Agent 安装后赋予文件所有者执行权限。

创建可提供压缩包或在线编写起始内容；Git 导入指定 HTTP(S) 仓库、分支及子目录，支持写入式可选 access_token。令牌使用 Control 现有认证加密机制保存，只返回 configured 标志；不进入文件包、历史、日志、运行快照或 API 回显。Git 仅手动同步，无后台同步服务；来源和凭据变更要求资源 owner，内容编辑和发布允许 editor。

内容读写由 Skill 内容发布 Workflow 协调，服务层执行聚合 CAS 与权限，文件产物保存和 Git 访问在数据库事务外。上传/同步更新草稿，发布将完整固定草稿保存为新的内容版本；发布不修改任何 Workspace 的选择。Agent 确认选择改为查询 Skill 发布历史与当前资源 ACL，不把编辑中的 catalog CAS 版本当作已发布内容。

资源接口沿用 `/api/v1/skills`（organization_id 创建/列表、name/description 编辑、access 管理、status 启停、archive），新增 `/import-git`、`/{id}/git:sync`、`/{id}/source`、`/{id}/draft`、`/{id}/files`、`/{id}/file`、`/{id}/publish`、`/{id}/versions`、`/{id}/versions/{version}/package`，以及与 Prompt 对称的未归属列表和归属确认。所有写操作使用 expected_version；内容读取和下载执行当前 ACL。

Skill 扩展现有 CRUD 为内容管理与发布 API；MCP 扩展现有 Gateway 目录 API 并实现 Plugin 的绑定及固定配置查询。具体数据契约在对应实现切片前补充，保持以上已确认语义。管理界面使用真实 API，不以编辑内部存储路径作为上传功能，不以模型替身测试作为正式模型可用性证据。

正式使用入口包括企业资源库、Agent 配置（含显式更新）、会话创建、消息发送、流式输出、历史和工具权限确认；Docker 仅负责创建一个可用 Agent。

## MCP 管理与认证切片

Gateway Definition 增加不可变 organization_id，空值属于平台目录。平台管理员维护平台定义；企业外部定义只允许该企业当前 admin 创建和修改，普通企业成员读取本企业及平台目录。企业停用或移除成员后立即失去企业目录权限。公开查询需要当前身份，内部只读使用服务凭据；Workspace 配置及运行调用还要核对真实 Workspace 企业归属。Definition ID 全局唯一，重名返回版本冲突，禁止通过更新转移企业归属。

外部认证支持 none、bearer、api_key、oauth。bearer/API Key 由 Plugin 按 Workspace 加密保存，API 只返回是否配置，用户替换或清除时显式提交，历史快照不得包含明文。API Key 支持指定合法 HTTP Header；拒绝 Host、Cookie、代理身份及逐跳协议 Header。OAuth 由 Auth 负责用户授权、连接与可用 access token，Plugin 代理使用，Agent 不持有凭据。平台 session_context 类型仅限平台维护的可信服务，企业不可注册该类型。

Definition 更新、绑定编辑、OAuth 连接及凭据替换不自动升级 Agent。用户明确确认 MCP 选择后才进入 Control 的不可变配置 Release；旧会话继续引用固定 Definition/Binding 版本。当前目录禁用或权限撤销仍阻止后续调用。
