---
name: agent_catalog
description: Agent 可复用控制资源的定义与版本
dependencies: []
related_workflow:
    - agent_session_startup
    - runtime_context_assembly
---

## 一句话职责

Agent Catalog 管理 Agent Configuration 可引用的 Prompt、Agent Template、Skill、Provider、Credential Reference 和 Memory Store。

## 边界

### 做什么
- 分别维护六类独立聚合及其状态和版本。
- 分发 Skill 内容引用与 Provider 能力描述。
- 资源编辑产生目录新版本，不自动更新 Agent 的已选版本；运行更新必须由用户在 Control 明确确认。
- 保存 Credential 的外部 Secret 引用，不保存明文 Secret。

### 不做什么
- 不管理业务 Plugin/MCP Definition。
- Prompt 聚合持有 Configured Prompt 的内容与版本，不拥有 Gateway 的 Platform Prompt 模板与正文生成。
- 不读取 Credential 明文，不执行模型或 Skill。

## 能力清单

| 能力 | 说明 | 对外接口/函数 |
|:---|:---|:---|
| 管理 Prompt | 维护提示词定义与版本 | `PromptService` |
| 管理 Agent Template | 维护 Agent 配置模板 | `AgentTemplateService` |
| 管理 Skill | 维护 Skill 元数据与内容路径 | `SkillCatalogService` |
| 管理 Provider | 维护模型 Provider 描述 | `ProviderService` |
| 管理凭据引用 | 维护 Secret Manager 引用 | `CredentialReferenceService` |
| 管理 Memory Store | 维护 Memory Store 与 Workspace 绑定 | `MemoryStoreService` |

## 数据模型

### `prompts`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| id | text PK | Prompt ID |
| workspace_id | text nullable | 私有 Prompt 所属 Workspace；空表示平台级 |
| owner_user_id | text | 创建用户 ID |
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
| workspace_id | text nullable | Workspace 归属；空表示平台级 |
| owner_user_id | text | 创建用户 ID |
| name | text | Skill 名称 |
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
