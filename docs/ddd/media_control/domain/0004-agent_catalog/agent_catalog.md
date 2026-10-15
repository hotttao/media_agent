---
name: agent_catalog
description: Agent 可复用控制资源的定义与版本
dependencies:
    - media_auth/organization
    - media_auth/team
related_workflow:
    - agent_session_startup
    - runtime_context_assembly
    - skill_content_publication
---

## 一句话职责

Agent Catalog 管理 Agent Configuration 可引用的 Prompt、Agent Template、Skill、Provider、Credential Reference 和 Memory Store。

## 边界

### 做什么
- 分别维护六类独立聚合及其状态和版本。
- Prompt、Skill 归属企业资源库，可被同企业的多个 Workspace 选择使用；不强制归属单个 Workspace。
- Prompt、Skill 默认 private，可按部门授予 viewer/editor；public 仅允许所属企业内读取，不跨企业公开。
- 分发 Skill 内容引用与 Provider 能力描述。
- 提供 Skill 的 ZIP/tar.gz 上传、Git 导入与手动同步、在线文件编辑、草稿发布及历史版本查看和下载。
- 目录内容与 Agent 已选版本分别管理，不自动更新 Agent 的已选版本；运行更新必须由用户在 Control 明确确认。
- 保存 Credential 的外部 Secret 引用，不保存明文 Secret。

### 不做什么
- 不管理业务 Plugin/MCP Definition。
- Prompt 聚合持有 Configured Prompt 的内容与版本，不拥有 Gateway 的 Platform Prompt 模板与正文生成。
- 不读取 Credential 明文，不执行模型或 Skill。

## 能力清单

| 能力 | 说明 | 对外接口/函数 |
|:---|:---|:---|
| 管理 Prompt | 维护提示词定义与版本 | `PromptService` |
| Prompt 历史与回滚 | 查询历史内容，选择历史内容形成新的当前版本 | `PromptService` |
| 管理 Agent Template | 维护 Agent 配置模板 | `AgentTemplateService` |
| 管理 Skill | 管理元数据、内容草稿、导入和不可变发布版本 | `SkillCatalogService` 与 `skill_content_publication` |
| 管理资源访问 | 管理 Prompt、Skill 的可见性和部门授权，返回有效资源权限 | 资源所属服务执行最终授权 |
| 管理 Provider | 维护模型 Provider 描述 | `ProviderService` |
| 管理凭据引用 | 维护 Secret Manager 引用 | `CredentialReferenceService` |
| 管理 Memory Store | 维护 Memory Store 与 Workspace 绑定 | `MemoryStoreService` |

## 数据模型

### `prompts`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| id | text PK | Prompt ID |
| organization_id | text | 所属企业，创建后不可变；不表示平台级公开 |
| owner_user_id | text | 创建用户 ID |
| visibility | text | private/team/public；public 仅限所属企业 |
| name | text | 名称 |
| content | text | Prompt 内容 |
| status | text | active/archived |
| version | bigint | 内容版本 |
| created_at | timestamptz | 创建时间 |
| updated_at | timestamptz | 最后更新时间 |

### `agent_templates`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| id | text PK | Agent Template ID |
| workspace_id | text nullable | Workspace 归属；空表示平台级 |
| owner_user_id | text | 创建用户 ID |
| name | text | 名称 |
| configuration | jsonb | Agent 配置模板 |
| status | text | active/archived |
| version | bigint | 模板版本 |
| created_at | timestamptz | 创建时间 |
| updated_at | timestamptz | 最后更新时间 |

### `skills`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| id | text PK | Skill ID |
| organization_id | text | 所属企业，创建后不可变；不表示平台级公开 |
| owner_user_id | text | 创建用户 ID |
| name | text | Skill 名称 |
| visibility | text | private/team/public；public 仅限所属企业 |
| content_path | text | Storage 中的内容路径 |
| checksum | text | 内容校验值 |
| status | text | active/disabled/archived |
| version | bigint | Skill 版本 |
| created_at | timestamptz | 创建时间 |
| updated_at | timestamptz | 最后更新时间 |

### `providers`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| id | text PK | Provider ID |
| name | text | Provider 名称 |
| provider_type | text | model/generation/asr 等能力类型 |
| endpoint | text | 内部或外部服务地址 |
| capabilities | jsonb | 模型和能力描述 |
| status | text | active/disabled |
| version | bigint | 定义版本 |
| created_at | timestamptz | 创建时间 |
| updated_at | timestamptz | 最后更新时间 |

### `credential_references`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| id | text PK | Credential Reference ID |
| workspace_id | text nullable | Workspace 归属；空表示平台级 |
| owner_user_id | text | 创建用户 ID |
| name | text | 引用名称 |
| provider_id | text | 对应 Provider ID |
| secret_ref | text | Secret Manager 引用，不含明文 Secret |
| status | text | active/revoked |
| version | bigint | 引用版本 |
| created_at | timestamptz | 创建时间 |
| updated_at | timestamptz | 最后更新时间 |

### `memory_stores`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| id | text PK | Memory Store ID |
| workspace_id | text | 所属 Workspace |
| name | text | 名称 |
| store_type | text | 存储实现类型 |
| configuration | jsonb | 非敏感连接配置 |
| credential_reference_id | text nullable | 访问凭据引用 |
| status | text | active/disabled |
| version | bigint | 配置版本 |
| created_at | timestamptz | 创建时间 |
| updated_at | timestamptz | 最后更新时间 |

### UUID 生成规则

所有 Catalog 聚合由 Control 生成 ULID，跨服务只传 ID 与版本。

## 能力详细说明

### Catalog 写入

1. 按资源类型校验 schema 与调用者权限。
2. 更新使用 expected_version，删除前检查本地 Agent Configuration 引用。
3. 状态或版本变化发布对应 Catalog 事件。

### Prompt 编辑与历史

资源库 Prompt 支持创建、编辑、查看历史版本和回滚。内容实际变化形成新版本，历史内容不可原地修改；回滚读取指定历史内容并形成新的当前内容版本，不删除其间历史。资源读取与编辑分别遵守资源权限，Workspace 的固定引用不跟随当前指针自动变化。

Agent Configuration 可选择不设置 Prompt、直接填写正文或引用一份库 Prompt 固定版本。直接填写正文属于配置，不能强制用户先创建资源库条目；引用与直接填写同时生效的组合不受支持。配置确认和运行更新见 [Agent Configuration](../0003-agent_configuration/agent_configuration.md)。

### Prompt、Skill 资源授权与引用

- Prompt 与 Skill 是独立聚合；各自的部门 Grant 与所属资源在本地一致性边界内维护，不共用可变的聚合状态。
- 资源所属服务保存部门 viewer/editor Grant；授权部门必须属于资源同一企业，成员和企业有效性事实由 Auth 提供。
- owner 管理资源可见性及 Grant；editor 可编辑资源内容，viewer 可读取资源。可见列表、详情和内容读取执行相同的资源权限判定，不沿用仅创建者可访问的旧规则。
- 用户将某一资源版本配置给 Agent 时，同时校验该资源的读取权限、资源与 Workspace 的企业一致性及 Workspace manage 权限。单独具有资源编辑权限不能修改 Workspace 配置。
- 资源编辑不改变 Agent 已选版本；用户明确确认后才形成新的 Metadata Release，已有 Session 的固定版本规则保留。
- 原 `workspace_id` 可空且以空值表示平台级的 Prompt、Skill 模型不再是目标模型；历史数据迁移不能猜测所属企业，迁移方案在实现规格中明确。

资源授权后的跨 Workspace 选择流程见全局 [元数据更新工作流](../../../workflow/0010-agent-metadata-update/agent_metadata_update.md)。

### Skill 内容与发布

- Skill 聚合包含可编辑草稿和发布版本的元数据；实际文件通过产物引用保存。现有 `content_path/checksum` 两个字段不足以表达完整内容管理能力。
- 上传支持 ZIP、tar.gz，文件集合包含入口 `SKILL.md` 以及可选脚本和参考资料。Git 导入选择仓库、分支和子目录，并支持用户手动同步。
- 在线编辑支持浏览文件树、读取、创建、修改和删除文件；未发布的草稿不出现在 Agent 可选版本中。
- 发布时校验完整文件集合和 `SKILL.md`，生成不可变版本；已发布版本的文件、校验值和内容引用不能原地修改。
- 可以查看历史版本并下载版本包；选择历史版本供 Agent 使用仍通过既有显式确认流程，不覆盖发布历史。
- 编辑、导入、同步和发布均校验资源编辑权限；内容读取、历史查看和下载校验资源读取权限，企业边界保持一致。
- 资源发布和 Agent 配置确认是两个独立动作；发布或同步不通知 Agent 自动升级。

文件产物保存与发布事实提交由 [Skill 内容发布工作流](../../workflow/0003-skill_content_publication/skill_content_publication.md) 协调。
