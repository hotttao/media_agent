export const domains = [
  {
    "id": "config",
    "label": "配置主线",
    "description": "workspace 到 agent-facing config 的核心表。"
  },
  {
    "id": "skills",
    "label": "Skills",
    "description": "Skill 来源、版本、workspace 挂载和本地下载。"
  },
  {
    "id": "mcp",
    "label": "MCP",
    "description": "MCP 目录、workspace 配置、OAuth token 和代理访问。"
  },
  {
    "id": "credentials",
    "label": "Credentials",
    "description": "用户凭据、selected scope 和 agent 注入。"
  },
  {
    "id": "oauth",
    "label": "OAuth 登录",
    "description": "CP 作为 OAuth Provider，Sandbox/Browser 作为 OAuth Client。"
  },
  {
    "id": "runtime",
    "label": "运行时",
    "description": "session token、usage 账本和增量 cursor。"
  },
  {
    "id": "memory",
    "label": "Memory",
    "description": "memory store、workspace 挂载、FUSE 读写和版本审计。"
  },
  {
    "id": "afs",
    "label": "AFS",
    "description": "跨 workspace 共享文件、controller capability 和 FUSE sidecar 挂载。"
  }
]

export const tables = {
  "users": {
    "domain": "oauth",
    "title": "用户主体",
    "purpose": "保存平台登录用户。workspace、provider、prompt、skill、credential、OAuth code/token 都会通过 user_id 归属到用户。",
    "readWhen": "看 owner 关系、OAuth 登录校验、资源可见性时先看它。",
    "importantFor": [
      "config",
      "skills",
      "credentials",
      "oauth"
    ],
    "fields": [
      [
        "id",
        "text",
        "PK",
        "用户唯一 ID。很多表的 user_id 都指向它。",
        true
      ],
      [
        "username",
        "text",
        "unique",
        "登录名。OAuth 登录时用它查用户，userinfo 也会返回。",
        true
      ],
      [
        "display_name",
        "text",
        "",
        "展示名。agent 侧通常不直接依赖，但前端和 userinfo 会用。",
        true
      ],
      [
        "password_hash",
        "text",
        "sensitive",
        "密码哈希，只在 CP 登录接口校验。",
        false
      ],
      [
        "last_login_at",
        "timestamptz",
        "",
        "最近登录时间，登录成功后更新。",
        true
      ],
      [
        "created_at",
        "timestamptz",
        "",
        "用户创建时间。",
        false
      ]
    ],
    "sample": {
      "id": "u_alice",
      "username": "alice",
      "display_name": "Alice",
      "last_login_at": "2026-07-26T10:10:00Z"
    }
  },
  "workspaces": {
    "domain": "config",
    "title": "工作区实例",
    "purpose": "表示一个 agent 运行空间。agent 启动时拿 WORKSPACE_ID，control-plane 用它找到 owner、状态和配置。",
    "readWhen": "任何 /_cp/workspaces/:id/* 请求的第一站。",
    "importantFor": [
      "skills",
      "credentials",
      "runtime"
    ],
    "fields": [
      [
        "id",
        "text",
        "PK",
        "workspace 唯一 ID，也是 agent 启动环境变量 WORKSPACE_ID 的值。",
        true
      ],
      [
        "user_id",
        "text",
        "FK users.id",
        "这个 workspace 属于哪个用户；后续查 credentials/MCP token 都靠它。",
        true
      ],
      [
        "name",
        "text",
        "",
        "人类可读名称。",
        true
      ],
      [
        "status",
        "text",
        "",
        "running/stopped/deleted 等状态。agent 请求配置前通常会校验。",
        true
      ],
      [
        "agent_type",
        "text",
        "",
        "codex、browser 等。决定 agent 侧写哪些配置文件。",
        true
      ],
      [
        "created_at",
        "timestamptz",
        "",
        "创建时间。",
        false
      ],
      [
        "updated_at",
        "timestamptz",
        "",
        "更新时间。",
        false
      ]
    ],
    "sample": {
      "id": "ws_codex_demo",
      "user_id": "u_alice",
      "name": "源码阅读工作区",
      "status": "running",
      "agent_type": "codex"
    }
  },
  "workspace_config": {
    "domain": "config",
    "title": "工作区配置入口",
    "purpose": "workspace 的配置总线。它不直接等于 agent 最终配置，而是先看 workspace 自己的覆盖项，再回退到 template_versions，最后再 JOIN provider/prompt 得到 agent-facing config。",
    "readWhen": "agent 请求 /_cp/workspaces/:id/config 时重点看。",
    "importantFor": [
      "mcp"
    ],
    "fields": [
      [
        "workspace_id",
        "text",
        "PK/FK workspaces.id",
        "配置所属 workspace。",
        true
      ],
      [
        "provider_id",
        "text",
        "FK model_providers.id",
        "选中的模型供应商。CP 会 JOIN 出 base_url/api_key。",
        true
      ],
      [
        "prompt_id",
        "text",
        "FK prompts.id",
        "workspace 自己覆盖的 prompt。存在时优先级高于 template_versions.prompt_id。",
        true
      ],
      [
        "template_id",
        "text",
        "FK templates.id",
        "这个 workspace 基于哪个模板创建/继承。为空表示没有模板继承，只看 workspace 自己的配置。",
        true
      ],
      [
        "template_version",
        "integer",
        "join template_versions.version",
        "锁定使用模板的哪个版本；与 template_id 一起 JOIN template_versions。",
        true
      ],
      [
        "model",
        "text",
        "",
        "agent 最终使用的模型名。",
        true
      ],
      [
        "mcp_config",
        "jsonb",
        "",
        "启用的 MCP server 清单。OAuth MCP 启动时会被 CP 改写成 proxy URL 给 agent。",
        true
      ],
      [
        "tools_config",
        "jsonb",
        "",
        "工具开关、权限、审批策略等扩展配置。",
        false
      ],
      [
        "updated_at",
        "timestamptz",
        "",
        "配置更新时间。agent 可用它判断是否 reload。",
        false
      ]
    ],
    "sample": {
      "workspace_id": "ws_codex_demo",
      "template_id": "tpl_research",
      "template_version": 3,
      "provider_id": null,
      "prompt_id": "prompt_codex",
      "model": "gpt-5",
      "mcp_config": {
        "mcpServers": {
          "docs": {
            "url": "https://docs.example.com/mcp"
          }
        }
      }
    }
  },
  "model_providers": {
    "domain": "config",
    "title": "模型供应商",
    "purpose": "保存 OpenAI-compatible provider、base_url、api_key 等。workspace_config 只存 provider_id，真正请求模型需要 JOIN 这里。",
    "readWhen": "agent-facing config 里出现 base_url/api_key/model 异常时看。",
    "fields": [
      [
        "id",
        "text",
        "PK",
        "provider 唯一 ID。",
        true
      ],
      [
        "user_id",
        "text",
        "FK users.id",
        "个人 provider 属于哪个用户。",
        true
      ],
      [
        "name",
        "text",
        "",
        "显示名。",
        true
      ],
      [
        "provider_type",
        "text",
        "",
        "openai/openai-compatible/anthropic 等。",
        true
      ],
      [
        "base_url",
        "text",
        "",
        "模型 API 根地址。",
        true
      ],
      [
        "api_key",
        "text",
        "sensitive",
        "模型 key，返回给 agent 前应按安全策略处理。",
        true
      ],
      [
        "created_at",
        "timestamptz",
        "",
        "创建时间。",
        false
      ]
    ],
    "sample": {
      "id": "provider_company",
      "user_id": "u_alice",
      "name": "Company LLM Proxy",
      "provider_type": "openai-compatible",
      "base_url": "https://llm.example.com/v1"
    }
  },
  "prompts": {
    "domain": "config",
    "title": "Prompt 模板",
    "purpose": "保存 agent 启动时注入的 prompt。workspace_config 通过 prompt_id 选择一个版本。",
    "readWhen": "agent 行为不像预期、AGENTS.md/system prompt 内容不对时看。",
    "fields": [
      [
        "id",
        "text",
        "PK",
        "prompt 唯一 ID。",
        true
      ],
      [
        "user_id",
        "text",
        "FK users.id",
        "自定义 prompt 的 owner。",
        true
      ],
      [
        "name",
        "text",
        "",
        "显示名。",
        true
      ],
      [
        "content",
        "text",
        "",
        "真正注入 agent 的文本。",
        true
      ],
      [
        "created_at",
        "timestamptz",
        "",
        "创建时间。",
        false
      ]
    ],
    "sample": {
      "id": "prompt_codex",
      "user_id": "u_alice",
      "name": "Codex Teacher",
      "content": "You are Codex, explain before editing..."
    }
  },
  "prompt_versions": {
    "domain": "config",
    "title": "Prompt 版本快照",
    "purpose": "保存 prompt 每次升级后的内容快照。template_versions.prompt_id + prompt_version 会 JOIN 到这里，从而固定模板版本使用的 prompt 内容。",
    "readWhen": "担心模板版本因为 prompt 更新而变化时看。",
    "fields": [
      [
        "id",
        "text",
        "PK",
        "prompt version 唯一 ID。",
        true
      ],
      [
        "prompt_id",
        "text",
        "FK prompts.id",
        "属于哪个 prompt 稳定身份。",
        true
      ],
      [
        "version",
        "integer",
        "unique with prompt_id",
        "prompt 的版本号。template_versions.prompt_version 锁定它。",
        true
      ],
      [
        "content",
        "text",
        "",
        "这一版 prompt 的不可变内容快照。",
        true
      ],
      [
        "created_at",
        "timestamptz",
        "",
        "版本创建时间。",
        false
      ]
    ],
    "sample": {
      "id": "prompt_research_v2",
      "prompt_id": "prompt_research",
      "version": 2,
      "content": "Research carefully, cite files..."
    }
  },
  "templates": {
    "domain": "config",
    "title": "Workspace 模板",
    "purpose": "模板的稳定身份。workspace_config.template_id 指向这里；真正可继承的参数在 template_versions 里。",
    "readWhen": "workspace 是从模板创建、需要判断模板是否有新版本、或者要看 owner/visibility 时看。",
    "fields": [
      [
        "id",
        "text",
        "PK",
        "模板唯一 ID。",
        true
      ],
      [
        "name",
        "text",
        "",
        "模板显示名。",
        true
      ],
      [
        "description",
        "text",
        "",
        "模板说明。",
        false
      ],
      [
        "owner_id",
        "text",
        "FK users.id",
        "模板 owner。",
        true
      ],
      [
        "latest_version",
        "integer",
        "",
        "当前模板最新版本号；workspace_config.template_version 可小于它，表示有更新可用。",
        true
      ],
      [
        "visibility",
        "text",
        "",
        "private/team/public，决定谁能使用模板。",
        true
      ]
    ],
    "sample": {
      "id": "tpl_research",
      "name": "Research Agent",
      "owner_id": "u_alice",
      "latest_version": 4,
      "visibility": "team"
    }
  },
  "template_versions": {
    "domain": "config",
    "title": "模板版本配置",
    "purpose": "模板在某个版本上的参数快照。workspace_config 按 template_id + template_version JOIN 它，并把空的 workspace 字段回退到这里。",
    "readWhen": "想知道 workspace 没填的 model/provider/prompt/mcp_config 从哪里继承时看。",
    "fields": [
      [
        "id",
        "text",
        "PK",
        "模板版本 ID。",
        true
      ],
      [
        "template_id",
        "text",
        "FK templates.id",
        "属于哪个模板。",
        true
      ],
      [
        "version",
        "integer",
        "",
        "版本号，与 workspace_config.template_version 匹配。",
        true
      ],
      [
        "agent_type",
        "text",
        "",
        "模板默认 agent 类型。workspace_config.agent_type 非空时覆盖它。",
        true
      ],
      [
        "provider_id",
        "text",
        "FK model_providers.id",
        "模板默认 provider。workspace_config.provider_id 优先。",
        true
      ],
      [
        "model",
        "text",
        "",
        "模板默认模型。workspace_config.model 非空时覆盖它。",
        true
      ],
      [
        "system_prompt",
        "text",
        "",
        "模板内联 prompt。workspace_config.system_prompt 非空时覆盖它。",
        true
      ],
      [
        "prompt_id",
        "text",
        "FK prompts.id",
        "模板默认引用的 prompt 稳定身份。workspace_config.prompt_id 优先。",
        true
      ],
      [
        "prompt_version",
        "integer",
        "join prompt_versions.version",
        "锁定 prompt 的具体内容版本；为空时读取 prompts.content 当前内容，会随 prompt 更新漂移。",
        true
      ],
      [
        "mcp_config",
        "jsonb/text",
        "",
        "模板默认 MCP 配置。workspace_config.mcp_config 不是 {} 时覆盖它。",
        true
      ],
      [
        "agent_settings",
        "jsonb/text",
        "",
        "模板默认 agent 设置。workspace_config.agent_settings 不是 {} 时覆盖它。",
        false
      ],
      [
        "compute_resources",
        "jsonb",
        "",
        "模板默认资源规格。workspace_config.compute_resources 优先。",
        false
      ]
    ],
    "sample": {
      "id": "tpl_research_v3",
      "template_id": "tpl_research",
      "version": 3,
      "provider_id": "provider_company",
      "model": "gpt-5-mini",
      "prompt_id": "prompt_research",
      "prompt_version": 2,
      "mcp_config": {
        "mcpServers": {
          "search": {
            "url": "https://search.example.com/mcp"
          }
        }
      }
    }
  },
  "template_version_skills": {
    "domain": "skills",
    "title": "模板版本内置 Skills",
    "purpose": "记录某个 template version 默认包含哪些 skill。它维护的是“skill 集合快照”：这个模板版本包含哪些 skill_id；但它不保存 skill_version_id，所以不固定 skill 包内容。",
    "readWhen": "判断模板到底固定了什么时重点看。结论：template_version_skills 固定 skill_id 列表；如果 skills.active_version_id 更新，后续解析到的实际 package 会跟着变。",
    "importantFor": [
      "config"
    ],
    "fields": [
      [
        "template_version_id",
        "text",
        "PK/FK template_versions.id",
        "属于哪个模板版本。",
        true
      ],
      [
        "skill_id",
        "uuid",
        "PK/FK skills.id",
        "模板版本包含哪个 skill 稳定身份。注意这里没有 skill_version_id，后续下载仍会走 skills.active_version_id。",
        true
      ]
    ],
    "notes": [
      "template_version_skills does not lock package: 表里没有 skill_version_id。",
      "模板版本只固定“包含 skill_media 这个 skill”。",
      "如果 skill_media 的 skills.active_version_id 从 ver_media_1 更新到 ver_media_2，template_version_skills 这一行不变，但 agent 后续下载会解析到 ver_media_2.package。"
    ],
    "sample": {
      "template_version_id": "tpl_research_v3",
      "skill_id": "skill_media",
      "before_publish": {
        "skills_active_version_id": "ver_media_1",
        "agent_downloads": "ver_media_1.package"
      },
      "after_publish": {
        "skills_active_version_id": "ver_media_2",
        "agent_downloads": "ver_media_2.package",
        "template_version_skills": "UNCHANGED"
      }
    }
  },
  "oauth_clients": {
    "domain": "oauth",
    "title": "CP OAuth 客户端",
    "purpose": "CP 作为 OAuth Provider 时，登记允许接入的客户端，例如 sandbox-service、browser-service。redirect_uris 必须精确匹配。",
    "readWhen": "登录报 invalid_client 或 invalid_redirect_uri 时看。",
    "fields": [
      [
        "id",
        "text",
        "PK",
        "client_id。Sandbox 登录时传 sandbox-service。",
        true
      ],
      [
        "name",
        "text",
        "",
        "授权页展示名称。",
        true
      ],
      [
        "redirect_uris",
        "text[]",
        "",
        "允许回调 URL 列表，必须和请求参数完全一致。",
        true
      ],
      [
        "scopes",
        "text[]",
        "",
        "允许申请的 scope，例如 profile。",
        true
      ],
      [
        "created_at",
        "timestamptz",
        "",
        "seed 创建时间。",
        false
      ]
    ],
    "sample": {
      "id": "sandbox-service",
      "name": "Sandbox",
      "redirect_uris": [
        "http://127.0.0.1:30086/api/auth/callback"
      ],
      "scopes": [
        "profile"
      ]
    }
  },
  "oauth_authorization_codes": {
    "domain": "oauth",
    "title": "OAuth 一次性授权码",
    "purpose": "用户点击 Authorize 后写入。Sandbox callback 用 code 换 token 时，CP 校验 PKCE 后删除它。",
    "readWhen": "授权页成功但 callback 换 token 失败时看。",
    "fields": [
      [
        "code",
        "text",
        "PK",
        "一次性 code。只应使用一次。",
        true
      ],
      [
        "client_id",
        "text",
        "FK oauth_clients.id",
        "哪个 OAuth client 申请的。",
        true
      ],
      [
        "user_id",
        "text",
        "FK users.id",
        "授权的是哪个用户。",
        true
      ],
      [
        "redirect_uri",
        "text",
        "",
        "换 token 时必须再次匹配。",
        true
      ],
      [
        "code_challenge",
        "text",
        "",
        "PKCE challenge。",
        true
      ],
      [
        "scope",
        "text",
        "",
        "授权范围。",
        false
      ],
      [
        "expires_at",
        "timestamptz",
        "",
        "过期时间，通常很短。",
        true
      ]
    ],
    "sample": {
      "code": "code_43_chars",
      "client_id": "sandbox-service",
      "user_id": "u_alice",
      "redirect_uri": "http://127.0.0.1:30086/api/auth/callback",
      "expires_at": "+10min"
    }
  },
  "oauth_refresh_tokens": {
    "domain": "oauth",
    "title": "OAuth Refresh Token",
    "purpose": "code 换 token 成功后写入。access_token 是 CP 签发的短期 JWT，不落库；refresh_token 只保存 hash。",
    "readWhen": "登录后长期会话、刷新 token、撤销授权时看。",
    "fields": [
      [
        "id",
        "text",
        "PK",
        "refresh token 记录 ID。",
        true
      ],
      [
        "client_id",
        "text",
        "FK oauth_clients.id",
        "属于哪个 client。",
        true
      ],
      [
        "user_id",
        "text",
        "FK users.id",
        "属于哪个用户。",
        true
      ],
      [
        "token_hash",
        "text",
        "sensitive",
        "refresh_token 的哈希值，不保存明文。",
        true
      ],
      [
        "expires_at",
        "timestamptz",
        "",
        "过期时间。",
        true
      ],
      [
        "revoked_at",
        "timestamptz",
        "",
        "撤销时间。",
        false
      ]
    ],
    "sample": {
      "id": "rt_001",
      "client_id": "sandbox-service",
      "user_id": "u_alice",
      "token_hash": "sha256(refresh_token)",
      "expires_at": "+30d"
    }
  },
  "mcp_catalog": {
    "domain": "mcp",
    "title": "MCP 目录",
    "purpose": "给前端展示可连接的 MCP。url 是 MCP endpoint，saas_url 是产品主页或管理页，group 用于 UI 分类。",
    "readWhen": "用户不理解某个 MCP 从哪里来、为什么在 UI 分组里时看。",
    "fields": [
      [
        "id",
        "text",
        "PK",
        "目录项 ID，例如 google-drive。",
        true
      ],
      [
        "label",
        "text",
        "",
        "前端显示名称。",
        true
      ],
      [
        "url",
        "text",
        "",
        "真正的 MCP endpoint。agent 或 CP proxy 最终访问的是它或它的代理 URL。",
        true
      ],
      [
        "saas_url",
        "text",
        "",
        "第三方 SaaS 的主页/控制台地址，用于人类跳转查看账号状态，不等于 MCP endpoint。",
        true
      ],
      [
        "group",
        "text",
        "",
        "UI 分类，例如 Office、Code、Search。只影响展示，不影响 agent 启动。",
        true
      ],
      [
        "required",
        "boolean",
        "",
        "是否建议/强制默认启用。",
        false
      ],
      [
        "params",
        "jsonb",
        "",
        "连接该 MCP 需要用户填写的参数 schema。",
        true
      ],
      [
        "hooks",
        "jsonb",
        "",
        "特殊处理开关，例如 oauth: true 表示需要 OAuth 连接流程。",
        true
      ]
    ],
    "sample": {
      "id": "google-drive",
      "label": "Google Drive",
      "url": "https://drive-mcp.example.com/mcp",
      "saas_url": "https://drive.google.com",
      "group": "Office",
      "params": [
        {
          "name": "root_folder",
          "type": "text"
        }
      ],
      "hooks": {
        "oauth": true
      }
    }
  },
  "mcp_oauth_clients": {
    "domain": "mcp",
    "title": "上游 MCP OAuth Client",
    "purpose": "CP 连接第三方 MCP 时，自己作为 OAuth client 在上游注册/复用的 client 信息。真实表只把 server_origin、metadata、client_id/client_secret 作为物理字段；OAuth endpoint 放在 metadata jsonb 里。",
    "readWhen": "MCP OAuth authorize 跳不过去、client_id 不对、metadata endpoint 不对时看。",
    "fields": [
      [
        "server_origin",
        "text",
        "PK",
        "上游 MCP 站点 origin，例如 https://docs.example.com；mcp_oauth_tokens 通过它外键关联。",
        true
      ],
      [
        "metadata",
        "jsonb",
        "",
        "OAuth discovery metadata，例如 authorization_endpoint、token_endpoint、registration_endpoint、scopes_supported。不是独立列。",
        true
      ],
      [
        "client_id",
        "text",
        "",
        "CP 在上游动态注册或配置得到的 OAuth client_id。",
        true
      ],
      [
        "client_secret",
        "text",
        "sensitive nullable",
        "上游分配的 client_secret；public client 或 PKCE 场景可能为空。",
        true
      ],
      [
        "created_at",
        "timestamptz",
        "",
        "创建时间。",
        false
      ],
      [
        "updated_at",
        "timestamptz",
        "",
        "更新时间。metadata/client 信息刷新后变化。",
        false
      ]
    ],
    "sample": {
      "server_origin": "https://docs.example.com",
      "metadata": {
        "authorization_endpoint": "https://docs.example.com/oauth/authorize",
        "token_endpoint": "https://docs.example.com/oauth/token",
        "registration_endpoint": "https://docs.example.com/oauth/register"
      },
      "client_id": "mcp-client-123",
      "client_secret": "encrypted-or-null",
      "updated_at": "2026-07-26T12:00:00Z"
    }
  },
  "mcp_oauth_tokens": {
    "domain": "mcp",
    "title": "用户的 MCP 授权 Token",
    "purpose": "保存用户授权 CP 访问某个上游 MCP 的 token。agent 不直接拿上游 token，而是访问 CP proxy。",
    "readWhen": "MCP OAuth 成功后 agent 仍访问失败、token 过期、proxy 401 时看。",
    "fields": [
      [
        "user_id",
        "text",
        "FK users.id",
        "哪个用户授权的。",
        true
      ],
      [
        "server_origin",
        "text",
        "FK mcp_oauth_clients.server_origin",
        "授权给哪个上游 MCP。",
        true
      ],
      [
        "access_token",
        "text",
        "sensitive",
        "上游 access token，CP proxy 使用。",
        true
      ],
      [
        "refresh_token",
        "text",
        "sensitive",
        "上游 refresh token。",
        true
      ],
      [
        "expires_at",
        "timestamptz",
        "",
        "access token 过期时间。",
        true
      ],
      [
        "scope",
        "text",
        "",
        "上游授权范围。",
        false
      ]
    ],
    "sample": {
      "user_id": "u_alice",
      "server_origin": "https://docs.example.com",
      "access_token": "encrypted_access",
      "refresh_token": "encrypted_refresh",
      "expires_at": "+1h"
    }
  },
  "skill_sources": {
    "domain": "skills",
    "title": "Skill 来源",
    "purpose": "描述 skill 从哪里来，例如内置、GitHub、上传包。skills 表引用它。",
    "readWhen": "想知道 skill 包来源、同步策略时看。",
    "fields": [
      [
        "id",
        "text",
        "PK",
        "来源 ID。",
        true
      ],
      [
        "type",
        "text",
        "",
        "builtin/github/upload 等。",
        true
      ],
      [
        "uri",
        "text",
        "",
        "仓库、对象存储或本地路径。",
        true
      ],
      [
        "created_at",
        "timestamptz",
        "",
        "创建时间。",
        false
      ]
    ],
    "sample": {
      "id": "src_media",
      "type": "github",
      "uri": "https://github.com/company/media-skills"
    }
  },
  "skills": {
    "domain": "skills",
    "title": "Skill 元数据",
    "purpose": "一个 skill 的稳定身份。active_version_id 指向当前启用版本。编辑中的 .editing 逻辑会避免覆盖用户正在改的本地 skill。",
    "readWhen": "workspace_skills 指向某个 skill 后，继续查这里确认 active version。",
    "fields": [
      [
        "id",
        "text",
        "PK",
        "skill 唯一 ID。",
        true
      ],
      [
        "source_id",
        "text",
        "FK skill_sources.id",
        "来源。",
        true
      ],
      [
        "name",
        "text",
        "",
        "agent 本地目录名。",
        true
      ],
      [
        "display_name",
        "text",
        "",
        "前端显示名。",
        true
      ],
      [
        "active_version_id",
        "text",
        "FK skill_versions.id",
        "当前应该下载/挂载的版本。",
        true
      ],
      [
        "created_at",
        "timestamptz",
        "",
        "创建时间。",
        false
      ]
    ],
    "sample": {
      "id": "skill_media",
      "source_id": "src_media",
      "name": "media-helper",
      "display_name": "Media Helper",
      "active_version_id": "ver_media_2"
    }
  },
  "skill_versions": {
    "domain": "skills",
    "title": "Skill 版本包",
    "purpose": "保存具体版本的包地址、hash 和 manifest。agent 下载 active version 后解压到本地 skills 目录。",
    "readWhen": "skill 内容不更新、hash 不匹配、下载失败时看。",
    "fields": [
      [
        "id",
        "text",
        "PK",
        "版本 ID。",
        true
      ],
      [
        "skill_id",
        "text",
        "FK skills.id",
        "属于哪个 skill。",
        true
      ],
      [
        "version",
        "text",
        "",
        "语义版本或内部版本号。",
        true
      ],
      [
        "package",
        "bytea",
        "sensitive/large",
        "tar.gz 压缩包二进制内容。SCS 直接写入 Postgres，不是对象存储 URL；CP/agent 下载时由 SCS 从 DB 读出并以 application/gzip 返回。",
        true
      ],
      [
        "content_hash",
        "text",
        "",
        "内容 hash，用于缓存和一致性校验。",
        true
      ],
      [
        "manifest",
        "jsonb",
        "",
        "SKILL.md 摘要、入口、权限等。",
        false
      ]
    ],
    "sample": {
      "id": "ver_media_2",
      "skill_id": "skill_media",
      "content_hash": "a1b2c3",
      "package": "<bytea: media-helper.tar.gz>",
      "commit_sha": "abc123"
    }
  },
  "workspace_skills": {
    "domain": "skills",
    "title": "Workspace 启用的 Skills",
    "purpose": "workspace 和 skill 的多对多关系。agent 启动时只下载这个 workspace 启用的 skills。",
    "readWhen": "某个 skill 为什么在 agent 里出现或没出现时看。",
    "fields": [
      [
        "workspace_id",
        "text",
        "FK workspaces.id",
        "哪个 workspace 启用。",
        true
      ],
      [
        "skill_id",
        "text",
        "FK skills.id",
        "启用了哪个 skill。",
        true
      ],
      [
        "enabled",
        "boolean",
        "",
        "是否启用。",
        true
      ],
      [
        "created_at",
        "timestamptz",
        "",
        "启用时间。",
        false
      ]
    ],
    "sample": {
      "workspace_id": "ws_codex_demo",
      "skill_id": "skill_media",
      "enabled": true
    }
  },
  "memory_stores": {
    "domain": "memory",
    "title": "Memory Store",
    "purpose": "一个独立的记忆存储空间。agent 容器里看到的 /mnt/memory/{store_id}，最后一段就是 memory_stores.id。一个 store 可以挂到多个 workspace。",
    "readWhen": "判断某个 memory 挂载点来自哪里、归属哪个用户、是否被归档时看。",
    "importantFor": [
      "config"
    ],
    "fields": [
      [
        "id",
        "text",
        "PK",
        "store 唯一 ID，也是 FUSE 挂载路径 /mnt/memory/{id} 的 {id}。",
        true
      ],
      [
        "owner_user_id",
        "text",
        "FK users.id",
        "store 的拥有者。",
        true
      ],
      [
        "name",
        "text",
        "",
        "人类可读名称，UI 展示用。",
        true
      ],
      [
        "description",
        "text",
        "default ''",
        "store 说明。",
        false
      ],
      [
        "archived_at",
        "timestamptz",
        "nullable",
        "归档时间。",
        false
      ],
      [
        "created_at",
        "timestamptz",
        "",
        "创建时间。",
        false
      ],
      [
        "updated_at",
        "timestamptz",
        "",
        "更新时间。",
        false
      ]
    ],
    "sample": {
      "id": "mem_project",
      "owner_user_id": "u_alice",
      "name": "Project KB",
      "description": "项目知识库"
    }
  },
  "workspace_memory_attachments": {
    "domain": "memory",
    "title": "Workspace Memory 挂载关系",
    "purpose": "定义某个 workspace 挂载哪些 memory store，以及挂载权限。memory-fuse 启动时首先读取这张表，而不是读取 workspace_config。",
    "readWhen": "agent 容器里为什么出现或没有出现 /mnt/memory/{store_id} 时先看它。",
    "importantFor": [
      "config"
    ],
    "fields": [
      [
        "workspace_id",
        "text",
        "PK/FK workspaces.id",
        "哪个 workspace 要挂载 memory store。",
        true
      ],
      [
        "store_id",
        "text",
        "PK/FK memory_stores.id",
        "挂载哪个 store；同时决定 /mnt/memory/{store_id} 路径。",
        true
      ],
      [
        "access",
        "text",
        "check read_only/read_write",
        "挂载权限。read_only 拒绝写入；read_write 才能 commit-on-close。",
        true
      ],
      [
        "instructions",
        "text",
        "default ''",
        "给 agent 的说明文本，例如建议优先读哪个文件；挂载本身不靠它完成。",
        true
      ],
      [
        "created_at",
        "timestamptz",
        "",
        "挂载关系创建时间。",
        false
      ]
    ],
    "notes": [
      "真实表名是 workspace_memory_attachments，不是 workspace_memory_store_attachments。",
      "它不在 getWorkspaceConfig() 的 SQL 里；memory-fuse 用 /_cp/workspaces/:id/memory-attachments 单独读取。"
    ],
    "sample": {
      "workspace_id": "ws_codex_demo",
      "store_id": "mem_project",
      "access": "read_write",
      "instructions": "优先阅读 /notes/overview.md"
    }
  },
  "memories": {
    "domain": "memory",
    "title": "Memory 当前文件",
    "purpose": "store 内当前可见的 memory 文件。数据库里是扁平 path 记录，memory-fuse 根据 path 前缀动态合成目录树。",
    "readWhen": "看某个 /mnt/memory/{store}/notes/foo.md 文件的当前内容、sha、大小时看。",
    "fields": [
      [
        "id",
        "text",
        "PK",
        "memory 文件唯一 ID。",
        true
      ],
      [
        "store_id",
        "text",
        "FK memory_stores.id",
        "属于哪个 store。",
        true
      ],
      [
        "path",
        "text",
        "unique with store_id",
        "绝对路径，例如 /notes/overview.md；同一 store 内唯一。",
        true
      ],
      [
        "content",
        "text",
        "",
        "文件正文。Read 需要内容时才由 memory-fuse 通过 GetMemory 拉取。",
        true
      ],
      [
        "content_sha256",
        "text",
        "CAS",
        "内容 hash。写入/删除时作为 if_match_sha256 乐观锁。",
        true
      ],
      [
        "size_bytes",
        "integer",
        "",
        "UTF-8 字节数，用于列表和 inode 大小。",
        true
      ],
      [
        "description",
        "text",
        "default ''",
        "memory 描述。",
        false
      ],
      [
        "mem_type",
        "text",
        "nullable",
        "memory 类型标签。",
        false
      ],
      [
        "created_at",
        "timestamptz",
        "",
        "创建时间。",
        false
      ],
      [
        "updated_at",
        "timestamptz",
        "",
        "更新时间。",
        false
      ]
    ],
    "sample": {
      "id": "mem_file_1",
      "store_id": "mem_project",
      "path": "/notes/overview.md",
      "content_sha256": "sha_old",
      "size_bytes": 512,
      "content": "---\nname: overview\ndescription: project overview\nmetadata:\n  type: project\n---\n..."
    }
  },
  "memory_versions": {
    "domain": "memory",
    "title": "Memory 版本审计",
    "purpose": "每次 create/update/delete/rename/migrate 都写一条版本记录。当前内容看 memories，历史和回滚看 memory_versions。",
    "readWhen": "解释谁改了 memory、什么时候改、能否回滚时看。",
    "fields": [
      [
        "id",
        "text",
        "PK",
        "版本记录 ID。",
        true
      ],
      [
        "store_id",
        "text",
        "FK memory_stores.id",
        "属于哪个 store。",
        true
      ],
      [
        "memory_id",
        "text",
        "nullable",
        "对应 memories.id；delete 后当前 memories 行可能不存在，但版本历史保留。",
        true
      ],
      [
        "path",
        "text",
        "",
        "当时操作的路径。",
        true
      ],
      [
        "operation",
        "text",
        "check",
        "create/update/delete/rename/migrate。",
        true
      ],
      [
        "content",
        "text",
        "nullable",
        "该版本内容；delete 时为 null。",
        true
      ],
      [
        "content_sha256",
        "text",
        "nullable",
        "该版本内容 hash；delete 时为 null。",
        true
      ],
      [
        "actor_kind",
        "text",
        "check",
        "user/agent/reflect/migrate。memory-fuse 写入时是 agent。",
        true
      ],
      [
        "actor_id",
        "text",
        "nullable",
        "操作者 ID。memory-fuse 通过 internal route 写入时 actor_id=workspace_id。",
        true
      ],
      [
        "created_at",
        "timestamptz",
        "",
        "版本产生时间。",
        false
      ]
    ],
    "sample": {
      "id": "mem_ver_2",
      "store_id": "mem_project",
      "memory_id": "mem_file_1",
      "path": "/notes/overview.md",
      "operation": "update",
      "actor_kind": "agent",
      "actor_id": "ws_codex_demo",
      "content_sha256": "sha_new"
    }
  },
  "afs_shares": {
    "domain": "afs",
    "title": "AFS Share",
    "purpose": "control-plane 侧的共享文件夹记录。它把人类可读 name、owner workspace、controller 颁发的 afs_dir_id/access_key 绑定起来。",
    "readWhen": "看某个 /mnt/afs/<name> 是谁创建的、对应哪把 controller 钥匙、授权时会把哪个 access_key 交给目标 sidecar。",
    "fields": [
      [
        "id",
        "text",
        "PK",
        "CP 侧 share ID，供 HTTP/MCP 管理接口使用。",
        true
      ],
      [
        "owner_workspace_id",
        "text",
        "FK workspaces.id",
        "创建这个 share 的 workspace。name 的幂等性通常在 owner workspace 范围内判断。",
        true
      ],
      [
        "name",
        "text",
        "unique with owner",
        "人类可读共享名，也是挂载路径 /mnt/afs/{name} 的最后一段；controller 不知道这个名字。",
        true
      ],
      [
        "afs_dir_id",
        "text",
        "controller dirs.id",
        "controller 颁发的 capability ID；FuseService.MountRequest.id 使用它。",
        true
      ],
      [
        "access_key",
        "text",
        "sensitive",
        "controller 颁发的 bearer secret；CP 留底，因为授权新成员或 pod 重启恢复挂载时还要交给 sidecar。",
        true
      ],
      [
        "created_at",
        "timestamptz",
        "",
        "创建时间。",
        false
      ]
    ],
    "notes": [
      "这张表是 CP PostgreSQL 表。controller SQLite 里没有 share name，也没有 owner_workspace_id。",
      "AFS 不是 workspace 自身 /workspace 的持久化；它只解决跨 workspace 共享文件。"
    ],
    "sample": {
      "id": "shr_req_001",
      "owner_workspace_id": "ws_a",
      "name": "req",
      "afs_dir_id": "8f3a-c91",
      "access_key": "ak_live_xxx"
    }
  },
  "afs_share_members": {
    "domain": "afs",
    "title": "AFS Share Member",
    "purpose": "记录哪些 workspace 被授权挂载某个 AFS share，以及挂载权限。",
    "readWhen": "目标 workspace 为什么能看到 /mnt/afs/<name>，或者授权/撤销是否生效时看。",
    "fields": [
      [
        "share_id",
        "text",
        "PK/FK afs_shares.id",
        "被授权的 share。",
        true
      ],
      [
        "workspace_id",
        "text",
        "PK/FK workspaces.id",
        "被授权挂载的 workspace。",
        true
      ],
      [
        "permission",
        "text",
        "read_only/read_write",
        "read_only 会让 sidecar 以只读方式挂载；read_write 允许写。",
        true
      ],
      [
        "mounted_at",
        "timestamptz",
        "",
        "成员挂载或授权时间。",
        true
      ]
    ],
    "sample": {
      "share_id": "shr_req_001",
      "workspace_id": "ws_b",
      "permission": "read_only",
      "mounted_at": "2026-07-27T10:00:00Z"
    }
  },
  "afs_controller_filesystems": {
    "domain": "afs",
    "title": "Controller filesystems",
    "purpose": "AFS controller SQLite 表，保存后端存储配置别名。它不是 share，也不是物理目录；它把 name=shared 映射到 fs_type=local 和 config.base_path=/data/afs。",
    "readWhen": "想知道 sidecar ValidateToken 后为什么能创建 LocalStorage(/data/afs)，或者 CreateDir(fs_name=shared) 为什么能成功时看。",
    "fields": [
      [
        "name",
        "text",
        "PK",
        "filesystem 名，例如 shared；CreateDirRequest.fs_name 引用它。",
        true
      ],
      [
        "fs_type",
        "text",
        "",
        "后端类型，当前常见 local，也支持 nfs。",
        true
      ],
      [
        "config",
        "text/json",
        "",
        "后端配置。local 使用 { base_path: '/data/afs' }。",
        true
      ],
      [
        "created_at",
        "timestamp",
        "",
        "注册时间。",
        false
      ]
    ],
    "notes": [
      "这是 controller.db 的 SQLite 表，不是 CP PostgreSQL 表。",
      "初始化方式：CP/runner 在创建 share 前调用 Controller.RegisterFs(name=shared, fs_type=local, config={base_path:/data/afs})。",
      "使用方式：dirs.fs_name 引用 filesystems.name；ValidateToken join dirs + filesystems 后把 fs_type/fs_config 返回给 sidecar。",
      "与 afs_storage 的关系：sidecar 根据 fs_config 创建 StorageBackend。local backend 的真实根目录就是 config.base_path。"
    ],
    "sample": {
      "name": "shared",
      "fs_type": "local",
      "config": {
        "base_path": "/data/afs"
      }
    }
  },
  "afs_controller_dirs": {
    "domain": "afs",
    "title": "Controller dirs",
    "purpose": "AFS controller 颁发的 capability 记录。dir 不是人类文件夹名，而是 id + access_key 这把钥匙。",
    "readWhen": "确认 ValidateToken 为什么通过或失败、RevokeDir 是否让 key 失效时看。",
    "fields": [
      [
        "id",
        "text",
        "PK",
        "dir capability ID；CP 存到 afs_shares.afs_dir_id。",
        true
      ],
      [
        "access_key",
        "text",
        "sensitive",
        "挂载凭证；sidecar Mount 前会拿它找 controller ValidateToken。",
        true
      ],
      [
        "permission",
        "text",
        "READ_ONLY/READ_WRITE",
        "controller 侧最高权限。请求 readonly=true 可以降权，不能越权升权。",
        true
      ],
      [
        "fs_name",
        "text",
        "FK filesystems.name",
        "指向后端 filesystem，例如 shared。",
        true
      ],
      [
        "status",
        "text",
        "active/deleted",
        "active 才能 ValidateToken 成功；delete/revoke 后失效。",
        true
      ],
      [
        "created_at",
        "timestamp",
        "",
        "创建时间。",
        false
      ]
    ],
    "sample": {
      "id": "8f3a-c91",
      "access_key": "ak_live_xxx",
      "permission": "READ_WRITE",
      "fs_name": "shared",
      "status": "active"
    }
  },
  "afs_controller_sessions": {
    "domain": "afs",
    "title": "Controller sessions",
    "purpose": "AFS controller SQLite 表，记录已经挂载成功的一次 mount 会话。这里的 session 不是用户登录 session，而是“某个 sidecar 把某个 dir 挂到某个 mountpoint”。",
    "readWhen": "执行 revoke/force unmount 后，想知道 controller 如何找到对应 sidecar 和挂载点时看。",
    "fields": [
      [
        "session_id",
        "text",
        "PK",
        "一次 mount 登记的 ID。",
        true
      ],
      [
        "dir_id",
        "text",
        "FK dirs.id",
        "被挂载的 capability dir。",
        true
      ],
      [
        "stream_id",
        "text",
        "",
        "sidecar 与 controller 的长连接 ID；controller 通过它把 ForceUnmount 发回对应 sidecar。",
        true
      ],
      [
        "mountpoint",
        "text",
        "",
        "sidecar pod 内的挂载点，例如 /mnt/afs/req。",
        true
      ],
      [
        "created_at",
        "timestamp",
        "",
        "mount 成功并注册 session 的时间。",
        false
      ]
    ],
    "notes": [
      "stream_id：一个 afs-fuse sidecar 进程到 controller 的 SessionStream 长连接 ID；同一个 sidecar 上多个 mount 共享它。",
      "session_id：一次具体 mount 的登记 ID；连接断开或 pod 重启后旧 session_id 可能被删除，新挂载会生成新 session_id。",
      "mountpoint：sidecar pod 内路径，例如 /mnt/afs/req。",
      "controller.sessions 是在线挂载索引，不是恢复事实源；恢复事实源是 CP 的 afs_shares/afs_share_members。",
      "revoke 时：controller 按 dir_id 查 sessions，再按 stream_id 找在线连接推 ForceUnmount；如果 stream 不在线，旧 session 会被清理，无法推给旧 pod。"
    ],
    "sample": {
      "session_id": "sess_001",
      "dir_id": "8f3a-c91",
      "stream_id": "stream_fuse_b",
      "mountpoint": "/mnt/afs/req"
    }
  },
  "user_credentials": {
    "domain": "credentials",
    "title": "用户凭据",
    "purpose": "保存用户的 API token、OAuth token、文件型凭据等。workspace 是否能用取决于 scope 和关联表。",
    "readWhen": "agent 环境变量/文件凭据没注入时看。",
    "fields": [
      [
        "user_id",
        "text",
        "FK users.id",
        "凭据 owner。",
        true
      ],
      [
        "name",
        "text",
        "PK part",
        "凭据名，例如 GITHUB_TOKEN。",
        true
      ],
      [
        "kind",
        "text",
        "",
        "env/file/json 等。",
        true
      ],
      [
        "scope",
        "text",
        "",
        "all 或 selected。selected 需要 user_credential_workspaces 授权。",
        true
      ],
      [
        "encrypted_value",
        "text",
        "sensitive",
        "加密后的值。CP 返回给 agent 前解密。",
        true
      ],
      [
        "inject",
        "text",
        "",
        "注入方式：env 或 file。",
        true
      ]
    ],
    "sample": {
      "user_id": "u_alice",
      "name": "GITHUB_TOKEN",
      "kind": "env",
      "scope": "selected",
      "inject": "env"
    }
  },
  "user_credential_workspaces": {
    "domain": "credentials",
    "title": "凭据授权到 Workspace",
    "purpose": "selected scope 的凭据通过这张表授权给具体 workspace。",
    "readWhen": "凭据存在但当前 workspace 不能使用时看。",
    "fields": [
      [
        "user_id",
        "text",
        "FK users.id",
        "凭据 owner。",
        true
      ],
      [
        "credential_name",
        "text",
        "FK user_credentials.name",
        "凭据名。",
        true
      ],
      [
        "workspace_id",
        "text",
        "FK workspaces.id",
        "被授权 workspace。",
        true
      ],
      [
        "created_at",
        "timestamptz",
        "",
        "授权时间。",
        false
      ]
    ],
    "sample": {
      "user_id": "u_alice",
      "credential_name": "GITHUB_TOKEN",
      "workspace_id": "ws_codex_demo"
    }
  },
  "session_tokens": {
    "domain": "runtime",
    "title": "Agent 会话 Token",
    "purpose": "运行时会话凭据。不同实现里可能用于 agent 访问 CP 内部接口的校验。",
    "readWhen": "agent 调 CP 401/403、session 过期时看。",
    "fields": [
      [
        "token_hash",
        "text",
        "PK/sensitive",
        "会话 token hash。",
        true
      ],
      [
        "workspace_id",
        "text",
        "FK workspaces.id",
        "绑定 workspace。",
        true
      ],
      [
        "expires_at",
        "timestamptz",
        "",
        "过期时间。",
        true
      ],
      [
        "created_at",
        "timestamptz",
        "",
        "创建时间。",
        false
      ]
    ],
    "sample": {
      "token_hash": "sha256(session_token)",
      "workspace_id": "ws_codex_demo",
      "expires_at": "+12h"
    }
  },
  "workspace_usage_events": {
    "domain": "runtime",
    "title": "Workspace Usage 明细账本",
    "purpose": "append-only 的 usage ledger。control-plane 从 agent /usage 拉回标准化 UsageRecord 后写这里；重复拉取靠 dedup_key 唯一约束去重，不靠 cursor 指向某一行。",
    "readWhen": "账单、token 统计、Codex/Claude/Goose 用量重复或漏算时先看。",
    "fields": [
      [
        "id",
        "bigint",
        "PK",
        "自增主键。",
        true
      ],
      [
        "workspace_id",
        "text",
        "indexed",
        "归属 workspace；由 pullWorkspaceUsage(workspaceId) 写入。",
        true
      ],
      [
        "user_id",
        "text",
        "indexed with ts",
        "workspace owner 快照，用于用户维度统计。",
        true
      ],
      [
        "session_id",
        "text",
        "nullable",
        "agent 侧 session；Claude subagent usage 会归到父 session。",
        true
      ],
      [
        "source",
        "text",
        "claude/codex/goose",
        "usage 来源 parser。",
        true
      ],
      [
        "model",
        "text",
        "",
        "本条 usage 使用的模型；缺失时使用 agent fallbackModel。",
        true
      ],
      [
        "input_tokens",
        "bigint",
        "default 0",
        "非 cache-read 输入 token；Codex 会把 cached_input_tokens 从 input 中拆出去。",
        true
      ],
      [
        "output_tokens",
        "bigint",
        "default 0",
        "输出 token。",
        true
      ],
      [
        "cache_read_tokens",
        "bigint",
        "default 0",
        "cache read token。Codex 来自 cached_input_tokens；Goose 当前不可得。",
        true
      ],
      [
        "cache_creation_tokens",
        "bigint",
        "default 0",
        "cache write token 总数。Claude 会调和 flat 与 nested cache_creation 字段。",
        true
      ],
      [
        "cache_creation_5m_tokens",
        "bigint",
        "default 0",
        "Claude ephemeral_5m cache write 子集。",
        false
      ],
      [
        "cache_creation_1h_tokens",
        "bigint",
        "default 0",
        "Claude ephemeral_1h cache write 子集。",
        false
      ],
      [
        "reasoning_output_tokens",
        "bigint",
        "default 0",
        "Codex reasoning output token；Claude/Goose 当前一般为 0。",
        true
      ],
      [
        "web_search_requests",
        "integer",
        "default 0",
        "Claude server_tool_use.web_search_requests。",
        false
      ],
      [
        "speed",
        "text",
        "nullable",
        "Claude speed：standard/fast/null。",
        false
      ],
      [
        "fields_incomplete",
        "boolean",
        "default false",
        "某些来源无法提供 cache/reasoning/speed 等细分字段时置 true，例如 Goose。",
        true
      ],
      [
        "ts",
        "timestamptz",
        "nullable",
        "usage 原始发生时间。",
        true
      ],
      [
        "dedup_key",
        "text",
        "UNIQUE",
        "全局幂等键。Claude=message.id，Codex=session+累计 total，Goose=session+累计 total 或行号。",
        true
      ],
      [
        "created_at",
        "timestamptz",
        "default now()",
        "入库时间。",
        false
      ]
    ],
    "notes": [
      "insertUsageRecords 使用 INSERT ... ON CONFLICT (dedup_key) DO NOTHING。",
      "admin_token_daily_stats、admin_token_user_stats、admin_token_workspace_stats 从这张表聚合。",
      "cursor 只决定哪些 transcript 文件要重新解析；真正防重复的是 dedup_key。"
    ],
    "sample": {
      "id": 101,
      "workspace_id": "ws_codex_demo",
      "user_id": "u_alice",
      "session_id": "sess_codex_1",
      "source": "codex",
      "model": "gpt-5",
      "input_tokens": 800,
      "output_tokens": 120,
      "cache_read_tokens": 3000,
      "reasoning_output_tokens": 40,
      "dedup_key": "codex:sess_codex_1:3960",
      "ts": "2026-07-26T10:20:00Z"
    }
  },
  "workspace_usage_cursor": {
    "domain": "runtime",
    "title": "Workspace Usage Cursor",
    "purpose": "每个 workspace 一行，保存 agent-usage sweep 的文件 fingerprint map。它不是 last_event_id，也不和 workspace_usage_events 建外键。",
    "readWhen": "想知道下一次 /usage 会跳过哪些 transcript、为什么某个文件会整文件重读时看。",
    "fields": [
      [
        "workspace_id",
        "text",
        "PK",
        "归属 workspace。control-plane 拉这个 workspace 的用量时使用。",
        true
      ],
      [
        "cursor",
        "jsonb",
        "default {}",
        "SweepCursors：{ transcriptFilePath: { dev, ino, size, mtimeMs } }。",
        true
      ],
      [
        "updated_at",
        "timestamptz",
        "default now()",
        "最后保存 cursor 的时间。",
        true
      ]
    ],
    "notes": [
      "增量粒度是文件 fingerprint，不是 JSONL 行号。",
      "文件没变：保留 cursor 并跳过。文件变了：整文件重读，返回新 fingerprint。",
      "如果 Claude 文件尾部 assistant 可能还在写，parser 会暂时跳过尾部并删除该文件 cursor，确保下次继续重读。"
    ],
    "sample": {
      "workspace_id": "ws_codex_demo",
      "cursor": {
        "/home/oai/.codex/sessions/2026/05/30/rollout-abc.jsonl": {
          "dev": 2049,
          "ino": 81122,
          "size": 4096,
          "mtimeMs": 1780123456789
        },
        "/home/oai/.claude/projects/-workspace/sess-a.jsonl": {
          "dev": 2049,
          "ino": 81123,
          "size": 8192,
          "mtimeMs": 1780123456799
        },
        "/home/oai/.acp-usage/sess-goose.jsonl": {
          "dev": 2049,
          "ino": 81124,
          "size": 512,
          "mtimeMs": 1780123456800
        }
      },
      "updated_at": "2026-07-26T10:21:00Z"
    }
  },
  "service_tokens": {
    "domain": "runtime",
    "title": "Service Token",
    "purpose": "机器/API 调用 CP 或其他内部服务时使用的 Bearer token。普通 token 只保存 token_hash；is_platform=true 的平台 token 额外保存明文 token，方便 CP 代表 workspace owner 调 Browser/Sandbox/Channel Gateway 等服务。",
    "readWhen": "排查 Authorization: Bearer tos_xxx、服务间调用、browser/sandbox/channel gateway 代用户调用 CP 时看。注意不要和 session_tokens 混淆。",
    "fields": [
      [
        "id",
        "text",
        "PK。service token 记录 id。"
      ],
      [
        "name",
        "text",
        "token 名称。用户创建的 token 使用用户输入名称；平台 token 固定为 __platform__。"
      ],
      [
        "token_hash",
        "text",
        "SHA-256(token)。CP 全局 auth middleware 对 Bearer token 做 hash 后按此字段查询。"
      ],
      [
        "token",
        "text nullable",
        "明文 token。普通 service token 不保存明文；is_platform=true 的平台 token 会保存明文，供 CP 后端转发给内部服务。"
      ],
      [
        "created_by",
        "text nullable FK users.id",
        "token 所属用户。Bearer 命中后 CP 把 currentUser.sub 设置为 created_by。"
      ],
      [
        "created_at",
        "timestamptz",
        "创建时间。"
      ],
      [
        "revoked_at",
        "timestamptz nullable",
        "撤销时间。查询有效 token 时必须 revoked_at IS NULL。"
      ],
      [
        "is_platform",
        "boolean",
        "是否平台自动维护 token。每个用户最多一个 active platform token；普通 revoke API 不允许撤销 is_platform=true。"
      ]
    ],
    "indexes": [
      "PRIMARY KEY (id)",
      "UNIQUE INDEX idx_service_tokens_platform_user ON (created_by) WHERE is_platform=true AND revoked_at IS NULL",
      "FOREIGN KEY created_by REFERENCES users(id)"
    ],
    "gotchas": [
      "service_tokens 是 API/服务间认证，不是 agent 启动拉 /_cp/config 的凭据。",
      "普通 token 创建响应只返回一次明文，数据库只保存 token_hash。",
      "platform token 特殊：明文保存在 token 列，因为 CP 需要把它交给 Browser Service、Sandbox、Channel Gateway 等内部服务。",
      "不要和 session_tokens 混淆：session_tokens 是 chat session 归属 token，走 X-Session-Token，不是 Authorization Bearer。"
    ],
    "sample": {
      "id": "st_platform_alice",
      "name": "__platform__",
      "token_hash": "sha256(tos_platform_alice)",
      "token": "tos_platform_alice",
      "created_by": "u_alice",
      "is_platform": true,
      "revoked_at": null
    }
  },
  "sessions": {
    "domain": "runtime",
    "title": "Agent 会话",
    "purpose": "记录一个 workspace 里的 agent 会话身份和当前对话状态。Question 流程里最关键的是 chat_status：agent 正在生成时是 agent，agent 把问题交回人时切到 human，结束后回到 idle。",
    "readWhen": "排查“页面为什么在等用户回答”“为什么不能继续发消息”“SSE 是否已经结束”时先看这张表。",
    "importantFor": [
      "runtime"
    ],
    "fields": [
      [
        "id",
        "text / uuid",
        "PK",
        "会话 ID。前端后续 pending-question、respond、reconnect 都带它，CP proxy 也用它把请求路由到对应 agent replica。",
        true
      ],
      [
        "workspace_id",
        "text",
        "FK -> workspaces.id",
        "会话属于哪个 workspace。ACL、agent 地址解析、UI 会话列表都会以 workspace 为边界。",
        true
      ],
      [
        "chat_status",
        "text",
        "state",
        "会话当前由谁接手。question.requested 到达 CP 后会写成 human；reconnect 或继续流式输出时写成 agent；session.ended 或流结束后写成 idle。",
        true
      ],
      [
        "provider_session_id",
        "text",
        "",
        "底层 agent/SDK 的会话标识。Claude Code resume 时会用 session_id 继续同一个 SDK 会话。",
        true
      ],
      [
        "created_at",
        "timestamptz",
        "",
        "会话创建时间。",
        false
      ],
      [
        "updated_at",
        "timestamptz",
        "",
        "状态或元数据更新时刷新。",
        false
      ]
    ],
    "notes": [
      "Question 的问题正文通常不靠这张表持久化；当前 Claude Code adapter 把 pending question 存在 agent 进程内存里。",
      "这张表保存的是“会话状态”，不是“用户回答内容”。用户回答通过 /respond 回到 agent 进程后唤醒 SDK。"
    ],
    "sample": {
      "id": "sess_9x7",
      "workspace_id": "ws_claude_demo",
      "chat_status": "human",
      "provider_session_id": "claude_sess_9x7"
    }
  },
  "messages": {
    "domain": "runtime",
    "title": "会话消息",
    "purpose": "保存会话里的用户输入、assistant 输出或工具结果摘要。它帮助 UI 重放历史，但 Question 的 pending promise 不在这里。",
    "readWhen": "排查页面历史消息、assistant 输出是否落库、刷新后历史能否恢复时看这张表。",
    "importantFor": [
      "runtime"
    ],
    "fields": [
      [
        "id",
        "text / uuid",
        "PK",
        "消息行 ID。",
        true
      ],
      [
        "session_id",
        "text",
        "FK -> sessions.id",
        "消息属于哪个会话。",
        true
      ],
      [
        "role",
        "text",
        "",
        "user、assistant、tool 等角色。",
        true
      ],
      [
        "content",
        "jsonb / text",
        "",
        "消息正文或结构化内容。",
        true
      ],
      [
        "created_at",
        "timestamptz",
        "",
        "消息创建时间。",
        false
      ]
    ],
    "notes": [
      "用户对 AskUserQuestion 的 answer 是工具输入的补充，当前路径重点是唤醒 agent 内存里的 pendingResponses，不是新增一条审批记录。",
      "如果 UI 额外把问答渲染成消息，那属于前端展示或消息持久化策略，不能把它等同于 agent.ts 里的 pending question 状态。"
    ],
    "sample": {
      "id": "msg_user_1",
      "session_id": "sess_9x7",
      "role": "user",
      "content": "帮我部署，缺参数就问我"
    }
  },
  "session_events": {
    "domain": "runtime",
    "title": "会话事件流",
    "purpose": "保存或辅助观察 SSE 事件级别的生命周期，例如 session.started、item.delta、question.requested、session.ended。具体落库粒度取决于 SSE persist plugin 的实现。",
    "readWhen": "排查“事件是否从 agent 发出”“CP 是否在广播前完成状态转换”“断线后 UI 如何补事件”时看这类事件表或对应日志。",
    "importantFor": [
      "runtime"
    ],
    "fields": [
      [
        "id",
        "text / uuid",
        "PK",
        "事件行 ID。",
        true
      ],
      [
        "session_id",
        "text",
        "FK -> sessions.id",
        "事件属于哪个会话。",
        true
      ],
      [
        "type",
        "text",
        "",
        "事件类型，例如 question.requested。",
        true
      ],
      [
        "payload",
        "jsonb",
        "",
        "原始事件 payload。Question 事件里会包含 request_id 和 questions。",
        true
      ],
      [
        "created_at",
        "timestamptz",
        "",
        "事件创建时间。",
        false
      ]
    ],
    "notes": [
      "源码里能确定的是 sse.ts 收到 question.requested 会把 sessions.chat_status 切到 human，并延迟广播到 DB 状态更新之后。",
      "不要把 session_events 当成 pending question 的权威存储；Claude Code adapter 的可恢复 pending question 来自进程内 pendingQuestionsBySession。"
    ],
    "sample": {
      "id": "evt_question_1",
      "session_id": "sess_9x7",
      "type": "question.requested",
      "payload": {
        "request_id": "req_abc",
        "questions": [
          {
            "id": "env",
            "label": "部署到哪个环境？"
          }
        ]
      }
    }
  }
}

export const edges = [
  [
    "users",
    "workspaces",
    "users.id -> workspaces.user_id"
  ],
  [
    "workspaces",
    "workspace_config",
    "workspaces.id -> workspace_config.workspace_id"
  ],
  [
    "workspace_config",
    "model_providers",
    "provider_id -> model_providers.id"
  ],
  [
    "workspace_config",
    "prompts",
    "prompt_id -> prompts.id"
  ],
  [
    "workspace_config",
    "templates",
    "template_id -> templates.id"
  ],
  [
    "workspace_config",
    "template_versions",
    "template_id + template_version -> template_versions"
  ],
  [
    "templates",
    "template_versions",
    "templates.id -> template_versions.template_id"
  ],
  [
    "template_versions",
    "model_providers",
    "provider_id -> model_providers.id"
  ],
  [
    "template_versions",
    "prompts",
    "prompt_id -> prompts.id"
  ],
  [
    "template_versions",
    "prompt_versions",
    "prompt_id + prompt_version -> prompt_versions"
  ],
  [
    "prompt_versions",
    "prompts",
    "prompt_id -> prompts.id"
  ],
  [
    "template_versions",
    "template_version_skills",
    "id -> template_version_skills.template_version_id"
  ],
  [
    "template_version_skills",
    "skills",
    "skill_id -> skills.id"
  ],
  [
    "oauth_clients",
    "oauth_authorization_codes",
    "client_id"
  ],
  [
    "users",
    "oauth_authorization_codes",
    "user_id"
  ],
  [
    "oauth_clients",
    "oauth_refresh_tokens",
    "client_id"
  ],
  [
    "users",
    "oauth_refresh_tokens",
    "user_id"
  ],
  [
    "mcp_catalog",
    "workspace_config",
    "mcp_config 引用 catalog/url"
  ],
  [
    "mcp_oauth_clients",
    "mcp_oauth_tokens",
    "server_origin"
  ],
  [
    "users",
    "mcp_oauth_tokens",
    "user_id"
  ],
  [
    "skill_sources",
    "skills",
    "source_id"
  ],
  [
    "skills",
    "skill_versions",
    "skill_id"
  ],
  [
    "skills",
    "workspace_skills",
    "skill_id"
  ],
  [
    "workspaces",
    "workspace_skills",
    "workspace_id"
  ],
  [
    "users",
    "user_credentials",
    "user_id"
  ],
  [
    "user_credentials",
    "user_credential_workspaces",
    "user_id + credential_name"
  ],
  [
    "workspaces",
    "user_credential_workspaces",
    "workspace_id"
  ],
  [
    "workspaces",
    "session_tokens",
    "workspace_id"
  ],
  [
    "workspaces",
    "workspace_usage_events",
    "workspace_id"
  ],
  [
    "workspaces",
    "workspace_usage_cursor",
    "workspace_id"
  ],
  [
    "users",
    "service_tokens",
    "created_by"
  ],
  [
    "workspaces",
    "sessions",
    "workspaces.id -> sessions.workspace_id"
  ],
  [
    "sessions",
    "messages",
    "sessions.id -> messages.session_id"
  ],
  [
    "sessions",
    "session_events",
    "sessions.id -> session_events.session_id"
  ]
]

export const walkthroughs = {
  "oauthLogin": {
    "title": "OAuth 登录：从点击登录到数据库变化",
    "intro": "这条链路来自 Koala docs/wiki/auth/oauth.md。观察重点是浏览器 URL 如何变成 authorization code，code 如何被消费，refresh token 为什么会新增一行。",
    "steps": [
      {
        "title": "部署 seed OAuth client",
        "actor": "install.sh / seed job",
        "request": "INSERT oauth_clients",
        "url": "N/A: deployment seed job writes oauth_clients; no HTTP request",
        "implementation": [
          "处理位置：部署脚本/seed job，不属于用户点击登录时的请求链路。",
          "参数来源：SANDBOX_PUBLIC_URL、SANDBOX_OAUTH_REDIRECT_URI 等安装变量被拼成 redirect_uris。",
          "实现结果：写入 oauth_clients，后续 CP 只信任这里登记过的 client_id 和 redirect_uri。"
        ],
        "db": [
          {
            "table": "oauth_clients",
            "op": "INSERT/UPSERT",
            "values": {
              "id": "sandbox-service",
              "redirect_uris": [
                "http://127.0.0.1:30086/api/auth/callback"
              ],
              "name": "Sandbox"
            }
          }
        ],
        "response": "无 HTTP 接口返回。部署/seed job 直接写 oauth_clients；如果是脚本执行，成功表现为 seed 过程无错误。",
        "note": "这一步只在部署或域名变化后执行。redirect_uri 后续必须严格匹配，否则登录阶段报 invalid_redirect_uri。"
      },
      {
        "title": "用户访问 Sandbox，点击登录",
        "actor": "Browser -> Sandbox",
        "request": "GET /api/auth/login",
        "url": "http://127.0.0.1:30086/api/auth/login",
        "redirectTo": "http://127.0.0.1:30080/oauth/authorize?client_id=sandbox-service&redirect_uri=http%3A%2F%2F127.0.0.1%3A30086%2Fapi%2Fauth%2Fcallback&response_type=code&scope=profile&code_challenge=sha256_verifier&code_challenge_method=S256&state=random_state",
        "params": {
          "client_id": "sandbox-service",
          "redirect_uri": "http://127.0.0.1:30086/api/auth/callback",
          "response_type": "code",
          "scope": "profile",
          "code_challenge_method": "S256",
          "state": "random_state"
        },
        "paramNotes": [
          "client_id：OAuth client 身份，这里是 sandbox-service；CP 用它查 oauth_clients。",
          "redirect_uri：授权完成后 code 回到哪里；必须和 oauth_clients.redirect_uris 完全匹配。",
          "response_type=code：声明使用 Authorization Code Flow。",
          "scope：客户端请求的权限范围，这里 profile 表示只要用户基础信息。",
          "code_challenge_method=S256：PKCE 算法，表示 code_challenge 是 code_verifier 的 SHA256。",
          "state：防 CSRF 和关联登录上下文；callback 时必须原样带回。"
        ],
        "implementation": [
          "处理位置：Sandbox 前端先显示登录按钮；用户点击后浏览器请求 Sandbox 后端 /api/auth/login。",
          "参数来源：client_id、redirect_uri、scope 来自 Sandbox 的 OAuth client 配置；state 和 PKCE code_verifier/challenge 由 Sandbox 后端临时生成。",
          "跳转实现：Sandbox 后端返回 HTTP 302，Location 指向 CP 的 /oauth/authorize?...，浏览器自动跳过去。"
        ],
        "db": [],
        "response": {
          "status": 302,
          "headers": {
            "Location": "http://127.0.0.1:30080/oauth/authorize?..."
          },
          "body": "empty"
        },
        "note": "此时没有 DB 写入。Sandbox 只生成 PKCE code_verifier/challenge 和 state，然后 302 到 CP /oauth/authorize。"
      },
      {
        "title": "CP 未登录，跳转登录页",
        "actor": "Browser -> CP",
        "request": "GET /oauth/authorize?...",
        "url": "http://127.0.0.1:30080/oauth/authorize?client_id=sandbox-service&redirect_uri=http%3A%2F%2F127.0.0.1%3A30086%2Fapi%2Fauth%2Fcallback&response_type=code&scope=profile&code_challenge=sha256_verifier&code_challenge_method=S256&state=random_state",
        "redirectTo": "http://127.0.0.1:30080/login?next=%2Foauth%2Fauthorize%3Fclient_id%3Dsandbox-service%26redirect_uri%3Dhttp%253A%252F%252F127.0.0.1%253A30086%252Fapi%252Fauth%252Fcallback%26response_type%3Dcode%26scope%3Dprofile%26state%3Drandom_state",
        "params": {
          "next": "/oauth/authorize?client_id=sandbox-service&redirect_uri=..."
        },
        "paramNotes": [
          "next：用户登录 CP 成功后要回到的原始授权页地址。里面保留 client_id、redirect_uri、scope、state、code_challenge 等 OAuth 参数。"
        ],
        "implementation": [
          "处理位置：CP 后端全局 auth middleware。",
          "参数来源：浏览器刚才访问 /oauth/authorize?... 时 URL 上已有 client_id、redirect_uri、state、code_challenge。middleware 把完整原始路径编码进 next。",
          "跳转实现：因为没有 CP 的 token cookie，后端返回 HTTP 302 到 /login?next=encoded(/oauth/authorize?...）。"
        ],
        "db": [],
        "response": {
          "status": 302,
          "headers": {
            "Location": "http://127.0.0.1:30080/login?next=encoded_authorize_url"
          },
          "body": "empty"
        },
        "note": "CP middleware 发现没有 CP 登录 cookie，302 到 /login?next=...。这里 /oauth/authorize 是前端路由，不是 /api/oauth/authorize。"
      },
      {
        "title": "用户提交账号密码",
        "actor": "Browser -> CP",
        "request": "POST /api/auth/login",
        "url": "http://127.0.0.1:30080/api/auth/login",
        "redirectTo": "http://127.0.0.1:30080/oauth/authorize?client_id=sandbox-service&redirect_uri=http%3A%2F%2F127.0.0.1%3A30086%2Fapi%2Fauth%2Fcallback&response_type=code&scope=profile&state=random_state",
        "params": {
          "username": "alice",
          "password": "******"
        },
        "paramNotes": [
          "username：用户在 CP 登录页输入的账号，用来查 users。",
          "password：用户输入的明文密码，只用于本次校验；后端和 password_hash 比对，不应该落库。"
        ],
        "implementation": [
          "处理位置：CP 前端 LoginPage 收集表单输入，调用 fetch(\"/api/auth/login\")；CP 后端 auth route 校验用户名密码。",
          "参数来源：username/password 来自登录表单；next 不是后端生成的，它来自当前页面 URL 的 ?next=... 查询参数，由 LoginPage 读出来保存在前端。",
          "跳转实现：/api/auth/login 成功后通常返回 JSON 并 Set-Cookie: token=CP_JWT；前端拿到成功结果后调用 React Router navigate(next)，所以不是后端 302。"
        ],
        "db": [
          {
            "table": "users",
            "op": "SELECT",
            "values": {
              "username": "alice",
              "password_hash": "argon2 hash"
            }
          },
          {
            "table": "users",
            "op": "UPDATE",
            "values": {
              "id": "u_alice",
              "last_login_at": "now()"
            }
          }
        ],
        "response": {
          "status": 200,
          "headers": {
            "Set-Cookie": "token=CP_JWT; HttpOnly; Path=/; ..."
          },
          "body": {
            "user": {
              "id": "u_alice",
              "username": "alice",
              "display_name": "Alice"
            }
          }
        },
        "note": "CP 登录成功后给浏览器写 CP 自己的 token cookie。这个 cookie 只证明用户已登录 CP。"
      },
      {
        "title": "登录成功后回到授权页路由",
        "actor": "CP React Router",
        "request": "client-side navigate /oauth/authorize?...",
        "url": "http://127.0.0.1:30080/oauth/authorize?client_id=sandbox-service&redirect_uri=http%3A%2F%2F127.0.0.1%3A30086%2Fapi%2Fauth%2Fcallback&response_type=code&scope=profile&code_challenge=sha256_verifier&code_challenge_method=S256&state=random_state",
        "params": {
          "client_id": "sandbox-service",
          "redirect_uri": "http://127.0.0.1:30086/api/auth/callback",
          "state": "random_state"
        },
        "paramNotes": [
          "client_id：授权页需要知道是哪个客户端在请求授权，用来展示 client name 并校验。",
          "redirect_uri：授权确认后 code 要发回的地址；后端会再次精确校验。",
          "state：从登录前保存到登录后，保证回调能对应同一次登录发起。"
        ],
        "implementation": [
          "处理位置：CP 前端 React Router。",
          "参数来源：next 中保存的 /oauth/authorize?... 被 navigate(next) 放回浏览器地址栏，OAuthAuthorizePage 从 URLSearchParams 读取 client_id、redirect_uri、state、code_challenge。",
          "跳转实现：这是 SPA 客户端路由切换，不是 /api 请求，也不写数据库。"
        ],
        "db": [],
        "response": "无后端接口返回。React Router 在浏览器内切换到 /oauth/authorize?...，随后页面组件再发 GET /api/oauth/authorize。",
        "note": "你说得对：人看到的授权页地址栏是 /oauth/authorize，没有 /api。登录页调用 navigate(next)，这是 SPA 路由切换，本身不读数据库。"
      },
      {
        "title": "授权页组件校验 OAuth client 和 redirect_uri",
        "actor": "CP React page -> CP API",
        "request": "GET /api/oauth/authorize?...",
        "url": "http://127.0.0.1:30080/api/oauth/authorize?client_id=sandbox-service&redirect_uri=http%3A%2F%2F127.0.0.1%3A30086%2Fapi%2Fauth%2Fcallback&response_type=code&scope=profile&code_challenge=sha256_verifier&code_challenge_method=S256&state=random_state",
        "params": {
          "client_id": "sandbox-service",
          "redirect_uri": "http://127.0.0.1:30086/api/auth/callback"
        },
        "paramNotes": [
          "client_id：后端用它 SELECT oauth_clients。不存在则 invalid_client。",
          "redirect_uri：必须在 oauth_clients.redirect_uris 中；不匹配则 invalid_redirect_uri。"
        ],
        "implementation": [
          "处理位置：CP 前端 OAuthAuthorizePage 的 useEffect 发起 fetch；CP 后端 oauth-provider GET /authorize 处理。",
          "参数来源：fetch 使用当前地址栏 /oauth/authorize?... 上的 searchParams 原样拼到 /api/oauth/authorize?...。",
          "实现结果：后端 SELECT oauth_clients，校验 client_id 存在、response_type=code、redirect_uri 与 redirect_uris 完全相等，然后返回 client_name/scope 给前端渲染授权页。"
        ],
        "db": [
          {
            "table": "oauth_clients",
            "op": "SELECT",
            "values": {
              "id": "sandbox-service",
              "redirect_uris": [
                "http://127.0.0.1:30086/api/auth/callback"
              ]
            }
          }
        ],
        "response": {
          "status": 200,
          "body": {
            "client_id": "sandbox-service",
            "client_name": "Sandbox",
            "redirect_uri": "http://127.0.0.1:30086/api/auth/callback",
            "scope": "profile",
            "user": {
              "id": "u_alice",
              "username": "alice"
            }
          }
        },
        "note": "OAuthAuthorizePage 渲染后才 fetch /api/oauth/authorize。这里只读 oauth_clients，用于展示 client_name/scope 并校验 redirect_uri。"
      },
      {
        "title": "用户点击 Authorize，CP 颁发 code",
        "actor": "Browser -> CP API",
        "request": "POST /api/oauth/authorize",
        "url": "http://127.0.0.1:30080/api/oauth/authorize",
        "redirectTo": "http://127.0.0.1:30086/api/auth/callback?code=code_43_chars&state=random_state",
        "params": {
          "client_id": "sandbox-service",
          "redirect_uri": "http://127.0.0.1:30086/api/auth/callback",
          "code_challenge": "sha256(verifier)",
          "state": "random_state",
          "scope": "profile"
        },
        "paramNotes": [
          "client_id：code 归属的 OAuth client，写入 oauth_authorization_codes.client_id。",
          "redirect_uri：code 绑定的回调地址；换 token 时必须传同一个值。",
          "code_challenge：PKCE 挑战值，和后续 token 请求的 code_verifier 配对校验。",
          "state：授权完成后原样拼回 redirect_uri，客户端用它确认不是伪造回调。",
          "scope：最终授权给客户端的权限范围，会影响 access_token/userinfo 能返回什么。"
        ],
        "implementation": [
          "处理位置：CP 前端授权页按钮 onClick 发 POST；CP 后端 oauth-provider POST /authorize 生成 code。",
          "参数来源：client_id、redirect_uri、state、code_challenge 仍来自授权页 URL；用户点击 Authorize 只是确认 consent。",
          "跳转实现：后端返回 JSON { redirect_uri: sandbox_callback?code=...&state=... }；前端执行 window.location.href = redirect_uri，浏览器跨域跳回 Sandbox。"
        ],
        "db": [
          {
            "table": "oauth_authorization_codes",
            "op": "INSERT",
            "values": {
              "code": "code_43_chars",
              "client_id": "sandbox-service",
              "user_id": "u_alice",
              "redirect_uri": "http://127.0.0.1:30086/api/auth/callback",
              "expires_at": "+10min"
            }
          }
        ],
        "response": {
          "status": 200,
          "body": {
            "redirect_uri": "http://127.0.0.1:30086/api/auth/callback?code=code_43_chars&state=random_state"
          }
        },
        "note": "这是第一次关键写库。code 是一次性票据，浏览器随后跳回 Sandbox callback。"
      },
      {
        "title": "Sandbox callback 用 code 换 token",
        "actor": "Sandbox backend -> CP API",
        "request": "POST /api/oauth/token",
        "url": "http://127.0.0.1:30080/api/oauth/token",
        "params": {
          "grant_type": "authorization_code",
          "code": "code_43_chars",
          "client_id": "sandbox-service",
          "redirect_uri": "http://127.0.0.1:30086/api/auth/callback",
          "code_verifier": "original_verifier"
        },
        "paramNotes": [
          "grant_type=authorization_code：告诉 CP 这是用授权码换 token。",
          "code：一次性授权码，CP 查 oauth_authorization_codes，校验后删除。",
          "client_id：必须和 code 记录里的 client_id 一致。",
          "redirect_uri：必须和颁发 code 时绑定的 redirect_uri 一致。",
          "code_verifier：PKCE 原文；CP 重新计算 SHA256 后必须等于之前的 code_challenge。"
        ],
        "implementation": [
          "处理位置：Sandbox 后端 /api/auth/callback 接收浏览器带回来的 code/state，然后由 Sandbox 后端调用 CP /api/oauth/token。",
          "参数来源：code/state 来自 callback URL；code_verifier 来自 Sandbox 在第一步保存的 pendingOAuth 内存状态；redirect_uri/client_id 来自 Sandbox OAuth 配置。",
          "实现结果：CP 校验 code、redirect_uri、PKCE，消费 oauth_authorization_codes，并写入 oauth_refresh_tokens。"
        ],
        "db": [
          {
            "table": "oauth_authorization_codes",
            "op": "SELECT + DELETE",
            "values": {
              "code": "code_43_chars",
              "reason": "consume once"
            }
          },
          {
            "table": "oauth_refresh_tokens",
            "op": "INSERT",
            "values": {
              "client_id": "sandbox-service",
              "user_id": "u_alice",
              "token_hash": "sha256(refresh_token)",
              "expires_at": "+30d"
            }
          }
        ],
        "response": {
          "status": 200,
          "body": {
            "access_token": "CP_access_token",
            "token_type": "Bearer",
            "expires_in": 900,
            "refresh_token": "CP_refresh_token",
            "scope": "profile"
          }
        },
        "note": "access_token 是 CP 签的短期 JWT，不落库；refresh_token 落库。完整登录后 DB 净增 1 行 refresh token。"
      },
      {
        "title": "Sandbox 查询 userinfo 并签自己的 session cookie",
        "actor": "Sandbox backend",
        "request": "GET /api/oauth/userinfo + Set-Cookie sandbox_token",
        "url": "http://127.0.0.1:30080/api/oauth/userinfo",
        "redirectTo": "http://127.0.0.1:30086/",
        "params": {
          "Authorization": "Bearer CP_access_token"
        },
        "paramNotes": [
          "Authorization：Sandbox 后端把 CP access_token 放到 Bearer header；CP 验签后从 token subject 识别用户。",
          "CP_access_token：短期 JWT，不落库；过期后需要 refresh_token 换新 token。"
        ],
        "implementation": [
          "处理位置：Sandbox 后端继续调用 CP /api/oauth/userinfo，然后自己签发 sandbox_token。",
          "参数来源：Authorization Bearer 是上一步 CP 返回的 access_token；用户信息由 CP JWT 解出。",
          "跳转实现：Sandbox 后端 Set-Cookie: sandbox_token 后，把浏览器重定向回 Sandbox 首页。"
        ],
        "db": [],
        "response": {
          "userinfo_response": {
            "status": 200,
            "body": {
              "sub": "u_alice",
              "username": "alice",
              "display_name": "Alice"
            }
          },
          "sandbox_callback_response": {
            "status": 302,
            "headers": {
              "Set-Cookie": "sandbox_token=SANDBOX_JWT; Path=/; ...",
              "Location": "http://127.0.0.1:30086/"
            },
            "body": "empty"
          }
        },
        "note": "Sandbox 拿到用户信息后，用 SANDBOX_JWT_SECRET 签自己的 sandbox_token。这个 token 写浏览器 cookie，不落 CP 数据库。"
      }
    ]
  },
  "skillUpdates": {
    "title": "Skill 更新两次：为什么 source 和 skill 要分表",
    "intro": "把 skill_sources 理解成“仓库/来源”，把 skills 理解成“平台里的 skill 身份”。同一个来源可以产生多个 skill；同一个 skill 可以更新多个版本，但它的身份和 workspace 挂载关系不应该变化。编辑中的 skills 不是数据库状态，而是 agent 本地 .editing 锁保护未发布草稿。",
    "steps": [
      {
        "title": "第一次导入来源：记录这个仓库从哪里来",
        "actor": "Admin/UI -> control-plane",
        "request": "POST /api/skills/sources",
        "params": {
          "type": "github",
          "uri": "https://github.com/company/agent-skills",
          "ref": "main"
        },
        "paramNotes": [
          "type：source 类型，决定后端按 GitHub、上传包还是内置来源处理。",
          "uri：skill source 的位置，后续同步任务从这里拉取内容。",
          "ref：Git 分支/标签/commit，用来决定同步哪个版本的仓库内容。"
        ],
        "response": {
          "status": 201,
          "body": {
            "source_id": "src_company_skills",
            "type": "github",
            "uri": "https://github.com/company/agent-skills",
            "ref": "main"
          }
        },
        "implementation": [
          "处理位置：control-plane 接收用户添加 skill source 的请求。",
          "参数来源：用户在 UI 里填仓库 URL、分支或上传包地址。",
          "关键理解：这里只回答“从哪里同步”，还没有决定 workspace 用哪个 skill。"
        ],
        "db": [
          {
            "table": "skill_sources",
            "op": "INSERT",
            "values": {
              "id": "src_company_skills",
              "type": "github",
              "uri": "https://github.com/company/agent-skills",
              "ref": "main"
            }
          }
        ],
        "note": "source 是供应渠道。它像 npm registry 或 GitHub repo 地址，不等于某个具体 skill。"
      },
      {
        "title": "首次同步：发现 media-helper v1",
        "actor": "sync job -> DB",
        "request": "scan source and create skill/version",
        "params": {
          "source_id": "src_company_skills",
          "discoveredSkill": "media-helper",
          "version": "0.1.0"
        },
        "paramNotes": [
          "source_id：要同步的来源。",
          "discoveredSkill：同步任务从仓库 manifest/SKILL.md 中识别出的 skill 名称。",
          "version：这次打包出来的 skill 版本。"
        ],
        "response": {
          "status": 200,
          "body": {
            "created_skills": [
              "skill_media"
            ],
            "created_versions": [
              "ver_media_1"
            ],
            "active_version_id": "ver_media_1"
          }
        },
        "implementation": [
          "处理位置：同步任务拉取 source，读取每个 skill 的 manifest/SKILL.md。",
          "参数来源：source.uri 指向的仓库内容；name/display_name 来自 manifest；package bytea 来自打包后的 tar.gz Buffer；content_hash 是 DB 根据 package 自动算出的 sha256。",
          "关键理解：skills 保存稳定身份，skill_versions 保存这次同步出来的具体包。"
        ],
        "db": [
          {
            "table": "skills",
            "op": "INSERT",
            "values": {
              "id": "skill_media",
              "source_id": "src_company_skills",
              "name": "media-helper",
              "display_name": "Media Helper",
              "active_version_id": "ver_media_1"
            }
          },
          {
            "table": "skill_versions",
            "op": "INSERT",
            "values": {
              "id": "ver_media_1",
              "skill_id": "skill_media",
              "version": "0.1.0",
              "content_hash": "hash_v1",
              "package": "<bytea: tar.gz bytes>"
            }
          }
        ],
        "note": "这时如果 workspace 启用 skill_media，agent 会下载 ver_media_1。"
      },
      {
        "title": "用户把 skill 挂到 workspace",
        "actor": "UI -> control-plane",
        "request": "POST /api/workspaces/ws_codex_demo/skills",
        "params": {
          "workspace_id": "ws_codex_demo",
          "skill_id": "skill_media"
        },
        "paramNotes": [
          "workspace_id：当前要启用 skill 的工作区。",
          "skill_id：稳定 skill 身份，不是 skill_versions.id。"
        ],
        "response": {
          "status": 200,
          "body": {
            "workspace_id": "ws_codex_demo",
            "skill_ids": [
              "skill_media"
            ]
          }
        },
        "implementation": [
          "处理位置：前端点击启用 skill，后端只记录 workspace_id 和 skill_id。",
          "参数来源：workspace_id 来自当前工作区；skill_id 来自 skills 表。",
          "关键理解：workspace_skills 不指向 source，也不指向某个 version，而是指向稳定 skill 身份。"
        ],
        "db": [
          {
            "table": "workspace_skills",
            "op": "INSERT",
            "values": {
              "workspace_id": "ws_codex_demo",
              "skill_id": "skill_media",
              "enabled": true
            }
          }
        ],
        "note": "这就是拆表的核心收益：后续更新版本时，workspace_skills 不需要改。"
      },
      {
        "title": "第一次更新：media-helper 从 v1 升到 v2",
        "actor": "sync job -> DB",
        "request": "sync source again",
        "params": {
          "source_id": "src_company_skills",
          "changedFiles": [
            "media-helper/SKILL.md",
            "media-helper/scripts/*"
          ]
        },
        "paramNotes": [
          "source_id：仍然是同一个来源。",
          "changedFiles：说明为什么 content_hash 会变化，实际由同步任务扫描 Git diff/包内容得到。"
        ],
        "response": {
          "status": 200,
          "body": {
            "skill_id": "skill_media",
            "previous_active_version_id": "ver_media_1",
            "active_version_id": "ver_media_2"
          }
        },
        "implementation": [
          "处理位置：同步任务再次扫描同一个 source。",
          "参数来源：GitHub main 分支已有新提交，打包后 hash 改变。",
          "实现结果：新增 skill_versions 一行，再把 skills.active_version_id 从 ver_media_1 改成 ver_media_2。"
        ],
        "db": [
          {
            "table": "skill_sources",
            "op": "UNCHANGED",
            "values": {
              "id": "src_company_skills",
              "uri": "https://github.com/company/agent-skills"
            }
          },
          {
            "table": "skill_versions",
            "op": "INSERT",
            "values": {
              "id": "ver_media_2",
              "skill_id": "skill_media",
              "version": "0.2.0",
              "content_hash": "hash_v2",
              "package": "<bytea: tar.gz bytes>"
            }
          },
          {
            "table": "skills",
            "op": "UPDATE",
            "values": {
              "id": "skill_media",
              "active_version_id": "ver_media_2"
            }
          },
          {
            "table": "workspace_skills",
            "op": "UNCHANGED",
            "values": {
              "workspace_id": "ws_codex_demo",
              "skill_id": "skill_media"
            }
          }
        ],
        "note": "数据库里保留 v1 和 v2 两个版本。当前启用哪个版本只看 skills.active_version_id。"
      },
      {
        "title": "第二次更新：同一来源又新增一个 skill，并把 media-helper 升到 v3",
        "actor": "sync job -> DB",
        "request": "sync source third time",
        "params": {
          "source_id": "src_company_skills",
          "discovered": [
            "media-helper@0.3.0",
            "video-helper@0.1.0"
          ]
        },
        "paramNotes": [
          "source_id：同一个仓库来源。",
          "discovered：这次同步后识别到的 skill/version 结果。"
        ],
        "response": {
          "status": 200,
          "body": {
            "updated": [
              {
                "skill_id": "skill_media",
                "active_version_id": "ver_media_3"
              }
            ],
            "created": [
              {
                "skill_id": "skill_video",
                "active_version_id": "ver_video_1"
              }
            ]
          }
        },
        "implementation": [
          "处理位置：还是同一个 source 的同步任务。",
          "参数来源：同一个仓库里既更新了 media-helper，又新增了 video-helper 目录。",
          "实现结果：media-helper 只更新 active_version_id；video-helper 作为另一个 skills 身份插入，但 source_id 相同。"
        ],
        "db": [
          {
            "table": "skill_sources",
            "op": "UNCHANGED",
            "values": {
              "id": "src_company_skills",
              "type": "github",
              "uri": "https://github.com/company/agent-skills"
            }
          },
          {
            "table": "skill_versions",
            "op": "INSERT",
            "values": {
              "id": "ver_media_3",
              "skill_id": "skill_media",
              "version": "0.3.0",
              "content_hash": "hash_v3"
            }
          },
          {
            "table": "skills",
            "op": "UPDATE",
            "values": {
              "id": "skill_media",
              "active_version_id": "ver_media_3"
            }
          },
          {
            "table": "skills",
            "op": "INSERT",
            "values": {
              "id": "skill_video",
              "source_id": "src_company_skills",
              "name": "video-helper",
              "active_version_id": "ver_video_1"
            }
          },
          {
            "table": "skill_versions",
            "op": "INSERT",
            "values": {
              "id": "ver_video_1",
              "skill_id": "skill_video",
              "version": "0.1.0",
              "content_hash": "hash_video_1"
            }
          }
        ],
        "note": "这说明 source 和 skill 是一对多：一个 GitHub 仓库可以产出多个 skill。"
      },
      {
        "title": "用户进入编辑态：把已发布 skill 变成本地草稿",
        "actor": "Skill UI -> agent local skill server",
        "request": "POST /skills/media-helper/edit",
        "url": "http://127.0.0.1:<agent-port>/skills/media-helper/edit",
        "params": {
          "name": "media-helper",
          "current_local_version": "ver_media_2"
        },
        "paramNotes": [
          "name：来自本地 skills 列表，也就是 ~/.codex/skills/{name} 的目录名。",
          "current_local_version：不是请求参数，是为了观察方便；本地目录通常由 skills.active_version_id 对应的 package 解压而来。"
        ],
        "response": {
          "status": 200,
          "body": {
            "success": true
          }
        },
        "implementation": [
          "处理位置：agent 进程内的 skill routes 接收 /:name/edit，然后调用 SkillManager.startEditing(name)。",
          "参数来源：前端本地 skill 管理页点击编辑；后端从 URL path 取 name，不查数据库来决定 active_version。",
          "本地处理：如果配置了 draftBase，会把 tmpfs 上的已发布 skill 拷贝到持久草稿目录 {WORKSPACE_DIR}/.skills-draft/skill-media-helper，再把 ~/.codex/skills/media-helper symlink 指向草稿目录。",
          "锁文件：在该 skill 目录写入 .editing。这个文件就是“编辑中”的真实标记；它不是 skills 表字段，也不会写入 skill_versions。"
        ],
        "db": [
          {
            "table": "skill_sources",
            "op": "UNCHANGED",
            "values": {
              "id": "src_company_skills"
            }
          },
          {
            "table": "skills",
            "op": "UNCHANGED",
            "values": {
              "id": "skill_media",
              "active_version_id": "ver_media_2"
            }
          },
          {
            "table": "skill_versions",
            "op": "UNCHANGED",
            "values": {
              "id": "ver_media_2"
            }
          },
          {
            "table": "local_fs",
            "op": "WRITE",
            "values": {
              "path": "{WORKSPACE_DIR}/.skills-draft/skill-media-helper/.editing",
              "meaning": "保护这个本地草稿，reload 不要覆盖"
            }
          }
        ],
        "note": "所以“编辑中的 skill”首先是本地文件系统状态：用户正在改草稿，数据库仍然只知道当前已发布版本。"
      },
      {
        "title": "编辑期间远端又更新：DB active_version 变了，但本地草稿不被覆盖",
        "actor": "sync job + agent reload",
        "request": "GET /_cp/workspaces/ws_codex_demo/skills then load()",
        "url": "http://127.0.0.1:30080/_cp/workspaces/ws_codex_demo/skills",
        "params": {
          "workspace_id": "ws_codex_demo",
          "local_lock": ".editing exists",
          "remote_active_version_id": "ver_media_3"
        },
        "paramNotes": [
          "workspace_id：agent reload 时仍然向 CP 查询这个 workspace 启用的 skill。",
          "local_lock：不是 HTTP 参数，是 agent 本地 load() 看到的文件状态。",
          "remote_active_version_id：CP DB 里 skills.active_version_id 已经指向新发布版本。"
        ],
        "response": {
          "status": 200,
          "body": {
            "skills": [
              {
                "id": "skill_media",
                "name": "media-helper",
                "active_version_id": "ver_media_3",
                "editable": true
              }
            ]
          },
          "localLoadResult": {
            "loaded": [],
            "failed": [],
            "editing": [
              "media-helper"
            ]
          }
        },
        "implementation": [
          "处理位置：SkillManager.load() 先拉 workspace skills 列表，再遍历每个 name。",
          "关键判断：如果 isEditing(name) 为 true，直接把 name 加入 editing 数组并 return，不执行 downloadWithRetry，也不 extractAtomic。",
          "结果差异：数据库已经把 active_version_id 更新到 ver_media_3；但是当前 agent 本地 ~/.codex/skills/media-helper 仍然指向草稿目录。",
          "为什么这样做：用户正在编辑的文件优先于远端 active version，避免 reload 或 workspace config 刷新把草稿覆盖掉。"
        ],
        "db": [
          {
            "table": "skill_versions",
            "op": "INSERT by remote sync",
            "values": {
              "id": "ver_media_3",
              "skill_id": "skill_media",
              "version": "0.3.0",
              "content_hash": "hash_v3"
            }
          },
          {
            "table": "skills",
            "op": "UPDATE by remote sync",
            "values": {
              "id": "skill_media",
              "active_version_id": "ver_media_3"
            }
          },
          {
            "table": "workspace_skills",
            "op": "UNCHANGED",
            "values": {
              "workspace_id": "ws_codex_demo",
              "skill_id": "skill_media"
            }
          },
          {
            "table": "local_fs",
            "op": "UNCHANGED",
            "values": {
              "path": "{WORKSPACE_DIR}/.skills-draft/skill-media-helper/.editing",
              "local_content": "用户草稿，仍然保留"
            }
          }
        ],
        "note": "这里最容易误解：active_version_id 是平台当前发布版本；.editing 是某个 agent/workspace 的本地草稿保护状态。两者可以短时间不一致。"
      },
      {
        "title": "发布或结束编辑：本地草稿如何回到版本表",
        "actor": "Skill UI -> agent local server -> control-plane",
        "request": "POST /skills/media-helper/pack, then upload package, then POST /skills/media-helper/stop-edit",
        "url": "http://127.0.0.1:<agent-port>/skills/media-helper/pack",
        "params": {
          "name": "media-helper",
          "package": "tar.gz generated from local draft",
          "publish_as": "ver_media_4"
        },
        "paramNotes": [
          "name：要打包的本地 skill 目录。",
          "package：agent local server 用 tar 从草稿目录打出的 gzip 字节。",
          "publish_as：示例里的新版本 ID，实际由 control-plane/DB 生成。"
        ],
        "response": {
          "pack_response": {
            "status": 200,
            "headers": {
              "Content-Type": "application/gzip"
            },
            "body": "<tar.gz bytes from local draft>"
          },
          "save_draft_response": {
            "status": 200,
            "body": {
              "ok": true,
              "byte_count": 4812
            }
          },
          "publish_response": {
            "status": 200,
            "body": {
              "skill": {
                "id": "skill_media",
                "active_version_id": "ver_media_4"
              },
              "version": {
                "id": "ver_media_4",
                "content_hash": "hash_draft_v4"
              }
            }
          },
          "stop_edit_response": {
            "status": 200,
            "body": {
              "success": true
            }
          }
        },
        "implementation": [
          "处理位置一：agent local server 的 /:name/pack 调 SkillManager.pack(name)，对当前本地目录加锁读取并打包成 tar.gz；打包时排除 .editing 和 .skill.tar.gz。",
          "用户发起和 agent 发起现在分成两张多泳道时序图看：每条箭头都明确写出调用方和接收方。",
          "用户发起链路的入口是 Browser/UI -> Control-plane API；agent 发起链路的入口是 Agent -> CP MCP skill_publish。",
          "两条链路后半段相同：调用 Agent local server pack 本地草稿，然后由 SkillsService/SCS 写 draft_package、skill_versions 和 skills.active_version_id。",
          "处理位置三：skillsService.publishDraft(userId, skillId) 调 scsPublishSkill；SCS 从 draft_package 创建新的 skill_versions 行，并把 skills.active_version_id 更新为新 version。",
          "处理位置四：发布成功后，MCP 调 agent 的 /:name/stop-edit，SkillManager.stopEditing(name) 删除本地 .editing。",
          "如果编辑期间 DB 里的 skills.active_version_id 已经从 ver_media_2 更新到 ver_media_3，发布时不会自动合并 ver_media_3。当前代码没有“三方合并”“基线版本比较”“冲突检测”步骤。",
          "发布出来的 ver_media_4 是本地草稿目录的完整快照。它会覆盖 active 指针，让 active_version_id 从 ver_media_3 变成 ver_media_4；ver_media_3 仍保留在 skill_versions 历史里。",
          "下一次 load：如果 draftBase 里还有草稿但已经不处于 editing，load() 会回收草稿目录并重新 materialize 当前 active version，避免继续服务过期草稿。"
        ],
        "db": [
          {
            "table": "skills",
            "op": "BEFORE publish",
            "values": {
              "id": "skill_media",
              "active_version_id": "ver_media_3",
              "meaning": "编辑期间 DB 已经被更新到 v3"
            }
          },
          {
            "table": "local_fs",
            "op": "PACK",
            "values": {
              "path": "{WORKSPACE_DIR}/.skills-draft/skill-media-helper",
              "based_on": "ver_media_2",
              "content": "用户草稿完整目录"
            }
          },
          {
            "table": "skill_sources",
            "op": "UPDATE draft_package",
            "values": {
              "id": "src_native_media",
              "draft_package": "<tar.gz bytes from local draft>"
            }
          },
          {
            "table": "skill_versions",
            "op": "INSERT",
            "values": {
              "id": "ver_media_4",
              "skill_id": "skill_media",
              "package": "<bytea: tar.gz bytes from local draft>",
              "content_hash": "hash_draft_v4"
            }
          },
          {
            "table": "skills",
            "op": "UPDATE active pointer",
            "values": {
              "id": "skill_media",
              "active_version_id": "ver_media_4",
              "previous_active_version_id": "ver_media_3"
            }
          },
          {
            "table": "local_fs",
            "op": "DELETE",
            "values": {
              "path": "{WORKSPACE_DIR}/.skills-draft/skill-media-helper/.editing"
            }
          }
        ],
        "note": "关键结论：用户发起和 agent 发起只是入口不同；发布入库的内容都来自当前 agent 本地草稿的完整 tar.gz。草稿发布不会自动合并 DB 最新 active version。",
        "codeBlocks": [
          {
            "title": "发布时序图：多调用方泳道图",
            "code": "下面用“多调用方泳道图”看，不再画成一条线。\n读法：左边是调用方，右边是接收方，箭头后面是这次调用做了什么。\n\n────────────────────────────────────────\nA. 用户在 UI 点击发布\n────────────────────────────────────────\n\n参与方：\n  User                 人类操作者\n  Browser/UI           前端页面\n  Control-plane API    普通 HTTP API\n  Agent local server   当前 workspace agent 进程里的 /skills 路由\n  SkillsService        control-plane 业务服务\n  SCS                  skills-content-service\n  DB                   PostgreSQL\n\nsequenceDiagram\n  participant User as User\n  participant UI as Browser/UI\n  participant CP as Control-plane API\n  participant Agent as Agent local server\n  participant Service as SkillsService\n  participant SCS as Skills Content Service\n  participant DB as PostgreSQL\n\n  User->>UI: 点击“发布 media-helper”\n  UI->>CP: POST /api/skills/{skill_id}/publish 或等价发布接口\n  CP->>Agent: POST /skills/media-helper/pack\n  Agent->>Agent: SkillManager.pack(name)\ntar -C 本地草稿目录 --exclude .editing .\n  Agent-->>CP: 返回 tar.gz(draft content)\n  CP->>Service: saveDraft({ sourceId, body: tar.gz })\n  Service->>SCS: PUT/POST draft package\n  SCS->>DB: UPDATE skill_sources.draft_package = tar.gz\n  DB-->>SCS: OK\n  SCS-->>Service: saveDraft OK\n  Service-->>CP: saveDraft OK\n  CP->>Service: publishDraft(userId, skillId)\n  Service->>SCS: POST /skills/{skillId}/publish\n  SCS->>DB: SELECT skill_sources.draft_package\n  SCS->>DB: INSERT skill_versions(ver_media_4, package=draft tar.gz)\n  SCS->>DB: UPDATE skills.active_version_id = ver_media_4\n  DB-->>SCS: Published version\n  SCS-->>Service: { skill, version }\n  Service-->>CP: { skill, version }\n  CP->>Agent: POST /skills/media-helper/stop-edit\n  Agent->>Agent: SkillManager.stopEditing(name)\n删除 .editing\n  Agent-->>CP: { success: true }\n  CP-->>UI: 发布成功\n  UI-->>User: 展示成功状态\n\n这里的发起者是 User。Browser/UI 只是把人的操作变成 HTTP 请求。\n真正打包草稿的是 Agent local server；真正写版本表的是 SCS/DB。\n\n────────────────────────────────────────\nB. Agent 调 MCP 工具发布\n────────────────────────────────────────\n\n参与方：\n  Agent                执行任务的模型/工具调用方\n  MCP skill_publish    control-plane 里的 MCP 工具实现\n  Agent local server   当前 workspace agent 进程里的 /skills 路由\n  SkillsService        control-plane 业务服务\n  SkillRepo            control-plane DB repository\n  SCS                  skills-content-service\n  DB                   PostgreSQL\n\nsequenceDiagram\n  participant LLM as Agent\n  participant MCP as CP MCP skill_publish\n  participant AgentSrv as Agent local server\n  participant Repo as SkillRepo\n  participant Service as SkillsService\n  participant SCS as Skills Content Service\n  participant DB as PostgreSQL\n\n  LLM->>MCP: skill_publish({ name: \"media-helper\" })\n  MCP->>MCP: getAgentAddress()\n确定当前 workspace 的 agent 地址\n  MCP->>Repo: getWorkspace(workspaceId)\n  Repo->>DB: SELECT workspaces WHERE id = workspaceId\n  DB-->>Repo: workspace/user_id\n  Repo-->>MCP: workspace\n  MCP->>Repo: getWritableSkillByName(name, user_id)\n  Repo->>DB: SELECT skills/source/grants\n确认是 native 且可写\n  DB-->>Repo: existing skill/source\n  Repo-->>MCP: existing skill\n  MCP->>AgentSrv: POST /skills/media-helper/pack\n  AgentSrv->>AgentSrv: SkillManager.pack(name)\ntar -C 本地草稿目录 --exclude .editing .\n  AgentSrv-->>MCP: 返回 tar.gz(draft content)\n  MCP->>Service: saveDraft({ userId, sourceId, body: tar.gz })\n  Service->>DB: 校验 source 可写\nresolveNativeSourceForWrite\n  DB-->>Service: OK\n  Service->>SCS: scsPutDraft(sourceId, tar.gz)\n  SCS->>DB: UPDATE skill_sources.draft_package = tar.gz\n  DB-->>SCS: OK\n  SCS-->>Service: saveDraft OK\n  Service-->>MCP: saveDraft OK\n  MCP->>Service: publishDraft(userId, skillId)\n  Service->>DB: getSkillForUser(skillId, userId)\n校验 owner/editor\n  DB-->>Service: skill permission OK\n  Service->>SCS: POST /skills/{skillId}/publish\n  SCS->>DB: SELECT skill_sources.draft_package\n  SCS->>DB: INSERT skill_versions(ver_media_4, package=draft tar.gz)\n  SCS->>DB: UPDATE skills.active_version_id = ver_media_4\n  DB-->>SCS: Published version\n  SCS-->>Service: { skill, version }\n  Service-->>MCP: { skill, version }\n  MCP->>Repo: getWorkspaceSkillIds(workspaceId)\n  Repo->>DB: SELECT workspace_skills\n  DB-->>Repo: current skill ids\n  Repo-->>MCP: current skill ids\n  MCP->>Repo: setWorkspaceSkills(...包含 skillId)\n  Repo->>DB: INSERT/UPDATE workspace_skills\n  DB-->>Repo: OK\n  MCP->>AgentSrv: POST /skills/media-helper/stop-edit\n  AgentSrv->>AgentSrv: SkillManager.stopEditing(name)\n删除 .editing\n  AgentSrv-->>MCP: { success: true }\n  MCP-->>LLM: tool result success\n\n这里的发起者是 Agent。MCP skill_publish 是编排者：它分别调用 Agent local server、SkillRepo、SkillsService/SCS。\n\n────────────────────────────────────────\n共同结论：是否会自动合并最新 active version？\n────────────────────────────────────────\n\n不会。\n\n如果编辑期间 DB 已经从：\n  skills.active_version_id = ver_media_2\n变成：\n  skills.active_version_id = ver_media_3\n\n发布时不会出现下面这些调用：\n  MCP/CP -> SCS/DB: 下载 ver_media_3 package\n  Agent local server -> DB: 读取 ver_media_3\n  SCS -> SCS: 三方合并 ver_media_2 + local draft + ver_media_3\n\n实际发生的是：\n  Agent local server -> MCP/CP: 返回本地草稿 tar.gz\n  SCS -> DB: INSERT skill_versions(ver_media_4, package=本地草稿 tar.gz)\n  SCS -> DB: UPDATE skills.active_version_id = ver_media_4\n\n所以 ver_media_4 是“本地草稿完整快照”，不是“ver_media_3 + 草稿差异”。"
          },
          {
            "title": "发布链路源码：pack 本地草稿，publish 读取 draft_package",
            "code": "// internal/agent-skills/src/routes.ts\napp.post('/skills/:name/pack', async (c) => {\n  const name = c.req.param('name')\n  const buf = await mgr().pack(name)\n  return new Response(new Uint8Array(buf), { headers: { 'Content-Type': 'application/gzip' } })\n})\n\n// internal/agent-skills/src/index.ts:787-808\nasync pack(name: string): Promise<Buffer> {\n  return this.withNameLock(name, async () => {\n    const local = this.localDir(name)\n    await this.shell.exec('tar', [\n      'czf', tarFile,\n      '-C', local,\n      '--exclude', '.editing',\n      '--exclude', '.skill.tar.gz',\n      '.',\n    ])\n    return await this.fs.readFile(tarFile)\n  })\n}\n\n// control-plane/src/mcp/tools/skills.ts:120-141\nconst packResp = await fetch(agent + '/skills/' + name + '/pack', { method: 'POST' })\nif (existing) {\n  await skillsService.saveDraft({ userId, sourceId: existing.source_id, body })\n  const published = await skillsService.publishDraft(userId, existing.id)\n}\n\n// control-plane/src/services/skills-service.ts:329-337\nasync saveDraft(input) {\n  await this.resolveNativeSourceForWrite(input.userId, input.sourceId)\n  const result = await scsPutDraft({ sourceId: input.sourceId, body: input.body })\n  return result.value\n}\n\n// control-plane/src/services/skills-content.ts:350-367\n// publish (read source.draft_package -> create version + set active)\nexport async function scsPublishSkill(skillId, body) {\n  return doJsonPost('/skills/' + skillId + '/publish', body)\n}\n\n解释：agent pack 的输入是当前 localDir(name)，也就是本地草稿目录。CP 保存这个 tar.gz 为 draft_package 后，SCS publish 用 draft_package 创建新 skill_versions，并更新 skills.active_version_id。"
          },
          {
            "title": "不会自动合并：用版本例子看数据库结果",
            "code": "假设：\n  ver_media_2 = 用户开始编辑时的版本\n  ver_media_3 = 编辑期间 DB 被远端同步/别人发布更新出来的新 active\n  local draft = 用户基于 ver_media_2 修改后的本地目录\n\n发布前：\nskills:\n  id=skill_media active_version_id=ver_media_3\n\nskill_versions:\n  ver_media_2  content_hash=hash_v2\n  ver_media_3  content_hash=hash_v3\n\n发布后：\nskill_versions:\n  ver_media_2  content_hash=hash_v2\n  ver_media_3  content_hash=hash_v3\n  ver_media_4  content_hash=hash_draft_v4  package=<本地草稿 tar.gz>\n\nskills:\n  id=skill_media active_version_id=ver_media_4\n\n解释：ver_media_3 不会被删除，但也不会自动参与 ver_media_4 的内容合并。ver_media_4 的内容完全来自发布时 pack 出来的本地草稿。"
          }
        ]
      },
      {
        "title": "发布中断恢复：网络失败后能不能继续",
        "actor": "Agent local server -> CP MCP/API -> SkillsService -> SCS -> DB -> reload queue",
        "request": "skill_publish interrupted / retry publish",
        "url": "POST agent:/skills/:name/pack -> PUT/POST SCS /sources/:id/draft -> POST SCS /skills/:id/publish -> POST agent:/skills/:name/stop-edit",
        "params": {
          "skill_name": "media-helper",
          "source_id": "src_native_media",
          "skill_id": "skill_media",
          "draft_package": "skill_sources.draft_package，保存发布前上传到 SCS 的 tar.gz",
          "local_draft": "{WORKSPACE_DIR}/.skills-draft/skill-media-helper，agent 本地持久草稿目录",
          "publish_tx": "insert skill_versions + update skills.active_version_id + clear draft_package in one DB transaction",
          "reload_queue": "skill-reload pg-boss job，发布成功后通知依赖 workspace reload"
        },
        "paramNotes": [
          "当前代码没有 publish_job 表、没有 idempotency key，也没有“从第 N 步自动继续”的 workflow 状态机。",
          "恢复依靠两类持久化：本地 draftBase 保存草稿目录；SCS 的 skill_sources.draft_package 保存已经上传成功的 draft tar.gz。",
          "SCS publish 是原子事务：要么 version/active/draft 清理一起成功，要么一起回滚，避免 active_version_id 指向不存在的版本。",
          "重复发布同样内容时，skill_versions 有 (skill_id, content_hash) 唯一约束；相同包会返回既有 version，不会无限插入重复版本。",
          "发布成功后的 reload fanout 是队列，有 retry/dead-letter；但 stop-edit 不是队列任务，失败后需要下一次 load 或手动 stop-edit 清理本地编辑态。"
        ],
        "response": {
          "short_answer": "没有完整自动恢复 workflow；有可重试的持久化检查点。",
          "safe_part": "SCS publish 事务安全，不会写出半个版本。",
          "manual_retry": "如果中断前本地 .editing 还在，重新执行 skill_publish 会重新 pack 本地草稿并覆盖 draft_package，然后再 publish。",
          "risk": "如果 publish 已成功但 CP/MCP 在 stop-edit 前断了，DB 已经 active 到新版本，但 agent 本地仍可能处于 .editing 草稿态。"
        },
        "implementation": [
          "完整链路是顺序编排：pack 本地草稿 -> saveDraft 上传 draft_package -> publishDraft 创建版本并切 active -> setWorkspaceSkills 启用 -> stop-edit 删除 .editing。中间没有统一的 publish 状态表。",
          "如果 pack 前/pack 中失败：DB 没变化；本地草稿还在；重试 skill_publish 会重新 pack。",
          "如果 pack 成功但 saveDraft 上传中断：SCS 没完整读到 body 就不会成功更新 draft_package；DB 通常不变；重试会重新 pack/upload。",
          "如果 saveDraft 成功但 publishDraft 前中断：skill_sources.draft_package 已有 tar.gz，active_version_id 未变；重新执行 skill_publish 会重新 saveDraft 覆盖一遍，然后 publish。理论上也可以只调用 /skills/:id/publish，因为 draft_package 已存在。",
          "如果 publishDraft 在 SCS 事务中失败：withTx 会 ROLLBACK，draft_package 不会被清掉，skill_versions/active_version_id 不会半更新；可以重试 publish。",
          "如果 publishDraft 成功但 notifyAffectedWorkspaces/reload enqueue 失败：DB 已经发布成功；CP publishDraft 会抛错给上层，但下次 publish/sync/set-active 或容器重启会让 workspace 自愈。",
          "如果 setWorkspaceSkills 失败：新版本已经发布，但当前 workspace 可能没启用这个 skill；重试 skill_publish 会再次走启用逻辑。",
          "如果 stop-edit 失败：DB 已经发布，workspace 也可能已启用；但本地 .editing 还在，当前 agent reload 时会继续保护草稿，不会立刻 materialize active version。需要再次 stop-edit、重新发布成功走到 stop-edit，或下次非 editing load 回收 stale draft。",
          "结论：恢复不是后台自动续跑，而是“重试同一个 publish 命令 + SCS 事务/唯一约束/草稿持久化”共同保证大多数中断不会丢草稿或写坏版本。"
        ],
        "db": [
          {
            "table": "skill_sources",
            "op": "CHECKPOINT after saveDraft",
            "values": {
              "id": "src_native_media",
              "draft_package": "<tar.gz bytes or null>",
              "meaning": "上传成功后才有；publish 成功后在同一事务里清空"
            }
          },
          {
            "table": "skill_versions",
            "op": "INSERT during publish tx",
            "values": {
              "skill_id": "skill_media",
              "content_hash": "hash_draft_v4",
              "package": "<draft tar.gz>",
              "unique": "(skill_id, content_hash)"
            }
          },
          {
            "table": "skills",
            "op": "UPDATE during publish tx",
            "values": {
              "id": "skill_media",
              "active_version_id": "ver_media_4"
            }
          },
          {
            "table": "workspace_skills",
            "op": "ENABLE after publish",
            "values": {
              "workspace_id": "ws_codex_demo",
              "skill_id": "skill_media",
              "risk": "这步失败时版本已发布但 workspace 可能未启用"
            }
          },
          {
            "table": "local_filesystem",
            "op": "STOP EDIT last",
            "values": {
              "path": "{WORKSPACE_DIR}/.skills-draft/skill-media-helper/.editing",
              "risk": "这步失败时 DB 已发布但本地仍显示编辑中"
            }
          }
        ],
        "note": "判断是否已经发布，查 skills.active_version_id 和 skill_versions；判断是否还有未发布上传草稿，查 skill_sources.has_draft/draft_package；判断当前 agent 是否还卡在编辑态，看本地 .editing。",
        "codeBlocks": [
          {
            "title": "发布链路没有统一恢复状态机：MCP 工具是顺序 await",
            "code": "// control-plane/src/mcp/tools/skills.ts:117-160\nconst packResp = await fetch(agent + '/skills/' + name + '/pack', { method: 'POST' })\nconst body = packResp.body\n\nif (existing) {\n  await skillsService.saveDraft({ userId, sourceId: existing.source_id, body })\n  const published = await skillsService.publishDraft(ws.user_id, existing.id)\n  skillId = published.skill.id\n} else {\n  const uploaded = await skillsService.uploadSkill({ userId, name, body })\n  skillId = uploaded.skill.id\n}\n\nconst current = await skillRepo.getWorkspaceSkillIds(workspaceId)\nif (!current.includes(skillId)) {\n  await skillRepo.setWorkspaceSkills(workspaceId, [...current, skillId])\n}\n\nawait fetch(agent + '/skills/' + name + '/stop-edit', { method: 'POST' })\n\n解释：这里没有 publish_jobs 表，也没有 checkpoint 状态字段。哪一步网络失败，就从这次工具调用返回 Error。恢复方式通常是重新调用 skill_publish。"
          },
          {
            "title": "SCS publish 是单事务：不会留下半个 version",
            "code": "// skills-content-service/src/index.ts:986-1007\nconst draft = await getDraftPackage(source.id)\nif (!draft || draft.byteLength === 0) {\n  return c.json({ error: 'No draft package to publish' }, 409)\n}\n\nconst result = await withTx(async (client) => {\n  const { version } = await insertVersion(client, {\n    skillId: skill.id,\n    sourceId: source.id,\n    package: draft,\n    commitSha: null,\n    note: body.note ?? null,\n    publishedBy: body.published_by,\n  })\n  const updated = await setActiveVersion(client, skill.id, version.id)\n  await client.query(\n    'UPDATE skill_sources SET draft_package = NULL, updated_at = NOW() WHERE id = $1',\n    [source.id],\n  )\n  return { skill: updated ?? skill, version }\n})\n\n// skills-content-service/src/db.ts:251-260\nawait client.query('BEGIN')\ntry { await fn(client); await client.query('COMMIT') }\ncatch (e) { await client.query('ROLLBACK'); throw e }\n\n解释：insert skill_versions、update skills.active_version_id、clear draft_package 在一个事务里。中间失败会回滚，draft_package 不会丢。"
          },
          {
            "title": "相同内容重复 publish 的幂等边界：content_hash 唯一约束",
            "code": "// skills-content-service/src/db.ts:368-386\n// UNIQUE(skill_id, content_hash) makes re-import of identical bytes idempotent\nINSERT INTO skill_versions (\n  skill_id, source_id, package, commit_sha, note, published_by\n)\nVALUES ($1,$2,$3,$4,$5,$6)\nON CONFLICT (skill_id, content_hash) DO UPDATE SET\n  note = skill_versions.note\nRETURNING id, skill_id, source_id, content_hash, ...\n\n解释：如果网络超时让调用方“不知道 publish 是否成功”，再次发布同一个 draft 包时，DB 不会因为相同 content_hash 无限插入重复版本。它会返回已有版本，并继续把 active_version_id 指向该版本。"
          },
          {
            "title": "失败矩阵：每个断点应该观察什么",
            "code": "断点 1：pack 失败\n- DB：无变化\n- 本地：.editing 仍在，草稿还在\n- 处理：修复本地文件或网络后重试 skill_publish\n\n断点 2：saveDraft 上传失败\n- DB：draft_package 大概率无变化；如果请求没有完整到达 SCS，不会保存完整 draft\n- 本地：草稿还在\n- 处理：重试 skill_publish，重新 pack/upload\n\n断点 3：saveDraft 成功，publish 前失败\n- DB：skill_sources.draft_package 有值；skills.active_version_id 未变\n- 本地：.editing 仍在\n- 处理：重试 skill_publish；也可以从 CP/SCS publish 已存在 draft\n\n断点 4：publish 事务中失败\n- DB：ROLLBACK；draft_package 仍在；active_version_id 不变\n- 处理：重试 publish\n\n断点 5：publish 成功，reload enqueue/通知失败\n- DB：skill_versions 已有新版本；skills.active_version_id 已更新；draft_package 已清空\n- 影响：部分 agent 可能暂时没 reload\n- 处理：reload queue 有 retry/dead-letter；下次 reload 或容器重启自愈\n\n断点 6：publish 成功，setWorkspaceSkills 失败\n- DB：版本已发布；当前 workspace 可能没启用\n- 处理：重试 skill_publish 或单独启用 workspace skill\n\n断点 7：publish 成功，stop-edit 失败\n- DB：版本已发布\n- 本地：.editing 可能还在，agent 继续保护草稿\n- 处理：再次 stop-edit 或重新执行 publish 直到走到 stop-edit；否则当前 agent 可能继续使用草稿视图。"
          },
          {
            "title": "发布后的 reload 有队列恢复，但它不是 publish workflow 恢复",
            "code": "// control-plane/src/lib/skill-reload-queue.ts:48-65\nawait boss.createQueue('skill-reload', {\n  retryLimit: 3,\n  retryDelay: 15,\n  retryBackoff: true,\n  expireInSeconds: 120,\n  retentionSeconds: 7 * 24 * 3600,\n  deadLetter: DEAD_LETTER_QUEUE,\n})\n\nawait boss.send('skill-reload', { kind: 'skill-reload', skillId }, { singletonKey: skillId })\n\n解释：这是“发布成功后通知 agent reload”的恢复机制。\n它能重试 reload fanout，但不能恢复 pack/saveDraft/publish/stop-edit 这条前台发布编排。"
          }
        ]
      },
      {
        "title": "两次更新后的数据库快照",
        "actor": "DB observer",
        "request": "SELECT related skill rows",
        "params": {
          "skill_source": "src_company_skills",
          "enabledWorkspace": "ws_codex_demo"
        },
        "paramNotes": [
          "skill_source：观察哪个来源。",
          "enabledWorkspace：观察哪个 workspace 的挂载关系是否变化。"
        ],
        "response": "无业务接口返回。这一步是人为观察 SELECT 结果，用来理解 source / skill / version / workspace_skills 的关系。",
        "implementation": [
          "skill_sources 仍然只有一行，因为来源没变。",
          "skills 有两行：media-helper 和 video-helper。media-helper 的 active_version_id 指向最新 ver_media_3。",
          "skill_versions 有四行：media v1/v2/v3 加 video v1。历史版本可追溯，workspace 挂载关系不用反复改。"
        ],
        "db": [
          {
            "table": "skill_sources",
            "op": "1 row",
            "values": {
              "id": "src_company_skills",
              "uri": "https://github.com/company/agent-skills"
            }
          },
          {
            "table": "skills",
            "op": "2 rows",
            "values": [
              {
                "id": "skill_media",
                "name": "media-helper",
                "active_version_id": "ver_media_3"
              },
              {
                "id": "skill_video",
                "name": "video-helper",
                "active_version_id": "ver_video_1"
              }
            ]
          },
          {
            "table": "skill_versions",
            "op": "4 rows",
            "values": [
              {
                "id": "ver_media_1",
                "version": "0.1.0"
              },
              {
                "id": "ver_media_2",
                "version": "0.2.0"
              },
              {
                "id": "ver_media_3",
                "version": "0.3.0"
              },
              {
                "id": "ver_video_1",
                "version": "0.1.0"
              }
            ]
          },
          {
            "table": "workspace_skills",
            "op": "UNCHANGED",
            "values": {
              "workspace_id": "ws_codex_demo",
              "skill_id": "skill_media"
            }
          }
        ],
        "note": "如果把 source 和 skill 合成一张表，会混淆“仓库从哪里同步”和“workspace 启用了哪个能力”，也很难表达一个仓库多个 skill、一个 skill 多个版本、历史版本回滚这些场景。"
      }
    ]
  },
  "workspaceConfigLoad": {
    "title": "Workspace Config 加载顺序：先看业务顺序，再看 SQL",
    "intro": "这一章分清两条启动加载链路：getWorkspaceConfig(workspaceId) 负责 provider/prompt/mcp_config，也会附带 memory_attachments 摘要给 agent 主进程渲染 platform prompt；真正 FUSE 挂载仍由 memory-fuse sidecar 另调 /_cp/workspaces/:id/memory-attachments 完成。",
    "steps": [
      {
        "title": "第一步：用 workspace_id 读取 workspace_config",
        "actor": "control-plane getWorkspaceConfig",
        "request": "SELECT workspace_config WHERE workspace_id = $1",
        "params": {
          "workspace_id": "ws_codex_demo"
        },
        "paramNotes": [
          "workspace_id：getWorkspaceConfig 的唯一入口参数，来自 agent 请求路径 /_cp/workspaces/:id/config。"
        ],
        "response": {
          "internal": true,
          "partial": {
            "workspace_id": "ws_codex_demo",
            "template_id": "tpl_research",
            "template_version": 3
          }
        },
        "implementation": [
          "入口参数只有 workspaceId。agent 并不会传 provider、prompt、template 这些字段。",
          "workspace_config 是配置入口表，先拿到 workspace 自己保存的覆盖值。",
          "如果 template_id 为空，后面基本只按 workspace_config 自己的字段返回。"
        ],
        "db": [
          {
            "table": "workspace_config",
            "op": "SELECT",
            "values": {
              "workspace_id": "ws_codex_demo",
              "template_id": "tpl_research",
              "template_version": 3,
              "provider_id": null,
              "prompt_id": null,
              "model": "gpt-5",
              "mcp_config": "{}"
            }
          }
        ],
        "note": "先记住一个原则：workspace_config 是入口，不等于最终运行配置。最终返回值是 SQL 合并后的结果。"
      },
      {
        "title": "第二步：如果使用模板，锁定 template_versions",
        "actor": "control-plane SQL JOIN",
        "request": "LEFT JOIN template_versions",
        "params": {
          "template_id": "tpl_research",
          "template_version": 3
        },
        "paramNotes": [
          "template_id：workspace 选择的模板身份。",
          "template_version：workspace 锁定的模板版本，不自动跟随 templates.latest_version。"
        ],
        "response": {
          "internal": true,
          "joined": {
            "template_name": "Research Agent",
            "template_latest_version": 4,
            "template_version_row": "tpl_research_v3"
          }
        },
        "implementation": [
          "当 wc.template_id 不为空时，用 wc.template_id + wc.template_version 找到模板版本。",
          "templates.latest_version 只用于告诉前端是否有新版模板；不会自动改变当前 workspace 使用的 template_version。",
          "template_versions 提供默认 agent_type、provider_id、model、system_prompt、mcp_config、agent_settings、compute_resources。"
        ],
        "db": [
          {
            "table": "templates",
            "op": "LEFT JOIN",
            "values": {
              "id": "tpl_research",
              "latest_version": 4
            }
          },
          {
            "table": "template_versions",
            "op": "LEFT JOIN",
            "values": {
              "template_id": "tpl_research",
              "version": 3,
              "provider_id": "provider_company",
              "model": "gpt-5-mini",
              "prompt_id": "prompt_research",
              "prompt_version": 2
            }
          }
        ],
        "note": "加载配置时不会因为 latest_version=4 就自动读 v4；workspace_config.template_version=3 就读 v3。"
      },
      {
        "title": "第三步：合并普通字段，workspace 覆盖 template",
        "actor": "control-plane SQL CASE/COALESCE",
        "request": "merge scalar/json config fields",
        "params": {
          "workspace_values": {
            "model": "gpt-5",
            "mcp_config": "{}"
          },
          "template_defaults": {
            "model": "gpt-5-mini",
            "mcp_config": "{...}"
          }
        },
        "paramNotes": [
          "workspace_values：workspace_config 中的覆盖值。",
          "template_defaults：template_versions 中的默认值。",
          "空字符串和 {} 通常表示没有覆盖，需要回退模板。"
        ],
        "response": {
          "internal": true,
          "merged": {
            "model": "gpt-5",
            "mcp_config": "template mcp_config"
          }
        },
        "implementation": [
          "agent_type/model/small_model/system_prompt：workspace_config 非空值优先，否则回退 template_versions。",
          "mcp_config/agent_settings：workspace_config 不是 {} 时优先，否则回退 template_versions。",
          "compute_resources：workspace_config 有值优先，否则回退 template_versions。"
        ],
        "db": [
          {
            "table": "workspace_config",
            "op": "read override",
            "values": {
              "model": "gpt-5",
              "mcp_config": "{}"
            }
          },
          {
            "table": "template_versions",
            "op": "read default",
            "values": {
              "model": "gpt-5-mini",
              "mcp_config": {
                "mcpServers": {
                  "search": {
                    "url": "https://search.example.com/mcp"
                  }
                }
              }
            }
          }
        ],
        "note": "示例中 model 返回 gpt-5，因为 workspace_config.model 非空；mcp_config 返回模板值，因为 workspace_config.mcp_config 是 {}。"
      },
      {
        "title": "第四步：解析 provider，先决定 id，再 JOIN 详情",
        "actor": "control-plane SQL JOIN model_providers",
        "request": "workspace provider -> template provider -> inline fallback",
        "params": {
          "workspace_provider_id": null,
          "template_provider_id": "provider_company"
        },
        "paramNotes": [
          "workspace_provider_id：workspace 自己选的 provider，优先级最高。",
          "template_provider_id：模板默认 provider。",
          "provider_id 只决定身份，base_url/api_key 要再 JOIN model_providers。"
        ],
        "response": {
          "internal": true,
          "provider": {
            "provider_id": "provider_company",
            "base_url": "https://llm.example.com/v1",
            "api_key": "***"
          }
        },
        "implementation": [
          "provider_id：workspace_config.provider_id 优先；为空时用 template_versions.provider_id。",
          "provider_type/base_url/api_key：优先读 workspace provider wp；没有则读 template provider tp；最后才用 workspace_config 里的 inline 字段。",
          "这就是为什么 workspace_config 只看到 provider_id 不够，还要看 model_providers。"
        ],
        "db": [
          {
            "table": "workspace_config",
            "op": "provider override",
            "values": {
              "provider_id": null
            }
          },
          {
            "table": "template_versions",
            "op": "provider default",
            "values": {
              "provider_id": "provider_company"
            }
          },
          {
            "table": "model_providers",
            "op": "LEFT JOIN",
            "values": {
              "id": "provider_company",
              "provider_type": "openai-compatible",
              "base_url": "https://llm.example.com/v1",
              "api_key": "***"
            }
          }
        ],
        "note": "provider 的返回值不是单表字段，而是 workspace_config/template_versions/model_providers 三者合并后的结果。"
      },
      {
        "title": "第五步：解析 prompt，system_prompt 会打断模板 prompt",
        "actor": "control-plane SQL JOIN prompts/prompt_versions",
        "request": "workspace prompt -> inline system_prompt -> template prompt",
        "params": {
          "workspace_prompt_id": null,
          "workspace_system_prompt": "",
          "template_prompt_id": "prompt_research",
          "template_prompt_version": 2
        },
        "paramNotes": [
          "workspace_prompt_id：workspace 显式选择的 prompt，优先级最高。",
          "workspace_system_prompt：非空时表示使用内联 prompt，不再用模板 prompt。",
          "template_prompt_version：锁定 prompt_versions 的具体内容版本。"
        ],
        "response": {
          "internal": true,
          "prompt": {
            "prompt_id": "prompt_research",
            "prompt_content": "Research carefully...",
            "source": "prompt_versions v2"
          }
        },
        "implementation": [
          "如果 workspace_config.prompt_id 存在，直接用 workspace prompt，并返回 prompts.content。",
          "如果 workspace_config.system_prompt 非空，说明 workspace 写了内联 prompt，不再使用模板 prompt。",
          "否则才用 template_versions.prompt_id；如果 template_versions.prompt_version 存在，优先读 prompt_versions.content。",
          "prompt_versions.prompt_id 仍然外键到 prompts.id；它是具体版本内容快照。"
        ],
        "db": [
          {
            "table": "workspace_config",
            "op": "prompt override",
            "values": {
              "prompt_id": null,
              "system_prompt": ""
            }
          },
          {
            "table": "template_versions",
            "op": "prompt default",
            "values": {
              "prompt_id": "prompt_research",
              "prompt_version": 2
            }
          },
          {
            "table": "prompt_versions",
            "op": "LEFT JOIN",
            "values": {
              "prompt_id": "prompt_research",
              "version": 2,
              "content": "Research carefully..."
            }
          },
          {
            "table": "prompts",
            "op": "FK identity",
            "values": {
              "id": "prompt_research",
              "name": "Research Prompt",
              "current_version": 3
            }
          }
        ],
        "note": "template_versions.prompt_version=2 时，即使 prompts.current_version 已经是 3，这个 template version 仍读 v2 的内容。"
      },
      {
        "title": "第五点五：prompt 三字段返回值例子",
        "actor": "control-plane getWorkspaceConfig",
        "request": "resolve prompt_id/system_prompt/prompt_content",
        "params": {
          "fields": [
            "prompt_id",
            "system_prompt",
            "prompt_content"
          ],
          "workspace_id": "ws_codex_demo"
        },
        "paramNotes": [
          "prompt_id：最终选择的 prompt 身份。可能来自 workspace_config.prompt_id，也可能来自 template_versions.prompt_id，也可能是 null。",
          "system_prompt：最终内联 system prompt 文本。它来自 workspace_config.system_prompt 或 template_versions.system_prompt 的合并结果。",
          "prompt_content：通过 prompt_id JOIN prompts/prompt_versions 得到的文本。它是派生字段，不是 workspace_config 的物理字段。",
          "prompt_content 为 null 时，接口 JSON 返回的就是 null，不会自动变成空字符串。"
        ],
        "response": {
          "cases": [
            {
              "name": "workspace 显式选择 prompt",
              "input": {
                "wc_prompt_id": "prompt_codex",
                "wc_system_prompt": "",
                "tv_prompt_id": "prompt_research",
                "tv_prompt_version": 2
              },
              "output": {
                "prompt_id": "prompt_codex",
                "system_prompt": "",
                "prompt_content": "prompts(prompt_codex).content"
              },
              "meaning": "workspace prompt_id 优先，模板 prompt 被忽略。"
            },
            {
              "name": "workspace 写了内联 system_prompt",
              "input": {
                "wc_prompt_id": null,
                "wc_system_prompt": "You are a strict reviewer",
                "tv_prompt_id": "prompt_research",
                "tv_prompt_version": 2
              },
              "output": {
                "prompt_id": null,
                "system_prompt": "You are a strict reviewer",
                "prompt_content": null
              },
              "meaning": "内联 system_prompt 会打断模板 prompt。接口返回 prompt_content: null，agent 使用 system_prompt。"
            },
            {
              "name": "workspace 没覆盖，使用模板 prompt 的固定版本",
              "input": {
                "wc_prompt_id": null,
                "wc_system_prompt": "",
                "tv_prompt_id": "prompt_research",
                "tv_prompt_version": 2
              },
              "output": {
                "prompt_id": "prompt_research",
                "system_prompt": "",
                "prompt_content": "prompt_versions(prompt_research, v2).content"
              },
              "meaning": "template_versions.prompt_version 命中时，prompt_content 来自 prompt_versions 内容快照。"
            },
            {
              "name": "模板 prompt 没有 prompt_version 或版本未命中",
              "input": {
                "wc_prompt_id": null,
                "wc_system_prompt": "",
                "tv_prompt_id": "prompt_research",
                "tv_prompt_version": null
              },
              "output": {
                "prompt_id": "prompt_research",
                "system_prompt": "",
                "prompt_content": "prompts(prompt_research).content"
              },
              "meaning": "COALESCE(tpv.content, tpr.content)：版本内容没有时 fallback 到 prompts 当前 content。"
            },
            {
              "name": "没有 workspace prompt，也没有模板 prompt",
              "input": {
                "wc_prompt_id": null,
                "wc_system_prompt": "",
                "tv_prompt_id": null,
                "tv_prompt_version": null
              },
              "output": {
                "prompt_id": null,
                "system_prompt": "",
                "prompt_content": null
              },
              "meaning": "接口返回 prompt_content: null。agent 侧不会得到额外 prompt_content，只能使用其它默认 system prompt/本地配置。"
            }
          ]
        },
        "implementation": [
          "先看 workspace_config.prompt_id：有值时 prompt_id 和 prompt_content 都走 workspace prompt。",
          "再看 workspace_config.system_prompt：非空时 prompt_id 返回 NULL，prompt_content 返回 NULL。",
          "再看 template_versions.prompt_id：有值时 prompt_id 返回模板 prompt_id。",
          "prompt_content 的模板路径是 COALESCE(tpv.content, tpr.content)：先取 prompt_versions 内容，取不到再取 prompts 当前内容。",
          "如果最终 prompt_content 是 null，JSON 响应字段就是 null；这不是错误，只代表没有通过 prompt 表解析出内容。",
          "agent 启动时还会再做一次本地选择：prompt_content 是非空字符串就写入 AGENTS.md；否则回退 system_prompt；两者都没有就写空 AGENTS.md。"
        ],
        "db": [
          {
            "table": "workspace_config",
            "op": "read",
            "values": {
              "prompt_id": null,
              "system_prompt": "You are a strict reviewer"
            }
          },
          {
            "table": "template_versions",
            "op": "maybe fallback",
            "values": {
              "prompt_id": "prompt_research",
              "prompt_version": 2
            }
          },
          {
            "table": "prompt_versions",
            "op": "maybe read",
            "values": {
              "prompt_id": "prompt_research",
              "version": 2
            }
          },
          {
            "table": "prompts",
            "op": "maybe read",
            "values": {
              "id": "prompt_research"
            }
          }
        ],
        "note": "最容易混淆的是：system_prompt 是直接文本；prompt_id 是身份；prompt_content 是 JOIN 后派生出来的内容。prompt_content 可以为 null，而且接口会如实返回 null。agent 侧遇到 null 会回退 system_prompt。"
      },
      {
        "title": "第六步：对照完整 SQL，看每个返回字段怎么来",
        "actor": "control-plane SQL",
        "request": "getWorkspaceConfig SQL projection",
        "params": {
          "workspace_id": "ws_codex_demo"
        },
        "paramNotes": [
          "workspace_id：SQL WHERE wc.workspace_id = $1 的绑定参数。",
          "其余字段不是请求参数，而是通过 JOIN、CASE、COALESCE、NULLIF 推导出来的返回列。",
          "读 SQL 的顺序建议：先看 FROM/JOIN 参与哪些表，再看 SELECT 中每组字段的覆盖规则。"
        ],
        "response": {
          "internal": true,
          "returns": "WorkspaceConfig | null",
          "fields": [
            "provider_id",
            "agent_type",
            "provider_type",
            "base_url",
            "api_key",
            "model",
            "small_model",
            "system_prompt",
            "mcp_config",
            "agent_settings",
            "compute_resources",
            "prompt_id",
            "prompt_name",
            "prompt_content",
            "template_name",
            "template_latest_version"
          ]
        },
        "implementation": [
          "FROM 主表：workspace_config wc 是入口，只按 workspace_id 取一行。",
          "模板 JOIN：template_versions tv 用 wc.template_id + wc.template_version 精确匹配；templates t 只提供 template_name/latest_version。",
          "provider_id：有模板时 COALESCE(wc.provider_id, tv.provider_id)；没模板时直接 wc.provider_id。",
          "agent_type：有模板时 workspace 非空值优先，否则 tv.agent_type；没模板时 wc.agent_type。",
          "provider_type/base_url/api_key：先看 workspace provider wp 的非空值，再看 template provider tp，最后 fallback 到 wc 上的 inline 字段。",
          "model/small_model/system_prompt：有模板时 wc 非空值覆盖 tv 默认值；没模板时直接 wc。",
          "mcp_config/agent_settings：有模板时 wc 不是 {} 才覆盖，否则回退 tv；没模板时直接 wc。",
          "compute_resources：有模板时 COALESCE(wc.compute_resources, tv.compute_resources)；没模板时 wc.compute_resources。",
          "auto_start/updated_at：不参与模板合并，直接返回 wc.auto_start 和 wc.updated_at。",
          "prompt_id：workspace prompt_id 优先；如果 workspace system_prompt 非空则返回 NULL；否则用 tv.prompt_id。",
          "prompt_name：workspace prompt_id 存在时用 pr.name；否则模板且 workspace 没有 system_prompt 时用 tpr.name。",
          "prompt_content：workspace prompt_id 存在时用 pr.content；否则模板且 workspace 没有 system_prompt 时用 COALESCE(tpv.content, tpr.content)。",
          "prompt_versions：只有模板 prompt 路径才参与，JOIN 条件是 tv.prompt_id + tv.prompt_version；如果没有命中，就 fallback 到 prompts 当前 content。"
        ],
        "sql": "SELECT wc.workspace_id,\n  CASE WHEN wc.template_id IS NOT NULL THEN COALESCE(wc.provider_id, tv.provider_id)\n       ELSE wc.provider_id END AS provider_id,\n  wc.template_id, wc.template_version,\n  CASE WHEN wc.template_id IS NOT NULL THEN COALESCE(NULLIF(wc.agent_type, ''), tv.agent_type)\n       ELSE wc.agent_type END AS agent_type,\n  CASE WHEN wc.template_id IS NOT NULL THEN COALESCE(NULLIF(wp.provider_type, ''), tp.provider_type, wc.provider_type)\n       WHEN wc.provider_id IS NOT NULL THEN COALESCE(NULLIF(wp.provider_type, ''), wc.provider_type)\n       ELSE wc.provider_type END AS provider_type,\n  CASE WHEN wc.template_id IS NOT NULL THEN COALESCE(NULLIF(wp.base_url, ''), tp.base_url, wc.base_url)\n       WHEN wc.provider_id IS NOT NULL THEN COALESCE(NULLIF(wp.base_url, ''), wc.base_url)\n       ELSE wc.base_url END AS base_url,\n  CASE WHEN wc.template_id IS NOT NULL THEN COALESCE(NULLIF(wp.api_key, ''), tp.api_key, wc.api_key)\n       WHEN wc.provider_id IS NOT NULL THEN COALESCE(NULLIF(wp.api_key, ''), wc.api_key)\n       ELSE wc.api_key END AS api_key,\n  CASE WHEN wc.template_id IS NOT NULL THEN COALESCE(NULLIF(wc.model, ''), tv.model)\n       ELSE wc.model END AS model,\n  CASE WHEN wc.template_id IS NOT NULL THEN COALESCE(NULLIF(wc.small_model, ''), tv.small_model)\n       ELSE wc.small_model END AS small_model,\n  CASE WHEN wc.template_id IS NOT NULL THEN COALESCE(NULLIF(wc.system_prompt, ''), tv.system_prompt)\n       ELSE wc.system_prompt END AS system_prompt,\n  CASE WHEN wc.template_id IS NOT NULL THEN COALESCE(NULLIF(wc.mcp_config, '{}'), tv.mcp_config::text)\n       ELSE wc.mcp_config END AS mcp_config,\n  CASE WHEN wc.template_id IS NOT NULL THEN COALESCE(NULLIF(wc.agent_settings, '{}'), tv.agent_settings::text)\n       ELSE wc.agent_settings END AS agent_settings,\n  CASE WHEN wc.template_id IS NOT NULL THEN COALESCE(wc.compute_resources, tv.compute_resources)\n       ELSE wc.compute_resources END AS compute_resources,\n  wc.auto_start,\n  wc.updated_at,\n  CASE WHEN wc.template_id IS NOT NULL\n       THEN CASE WHEN wc.prompt_id IS NOT NULL THEN wc.prompt_id\n                 WHEN COALESCE(wc.system_prompt, '') <> '' THEN NULL\n                 ELSE tv.prompt_id END\n       ELSE wc.prompt_id END AS prompt_id,\n  CASE WHEN wc.prompt_id IS NOT NULL THEN pr.name\n       WHEN wc.template_id IS NOT NULL AND COALESCE(wc.system_prompt, '') = '' THEN tpr.name\n       ELSE NULL END AS prompt_name,\n  CASE WHEN wc.prompt_id IS NOT NULL THEN pr.content\n       WHEN wc.template_id IS NOT NULL AND COALESCE(wc.system_prompt, '') = '' THEN COALESCE(tpv.content, tpr.content)\n       ELSE NULL END AS prompt_content,\n  t.name AS template_name,\n  t.latest_version AS template_latest_version\nFROM workspace_config wc\nLEFT JOIN model_providers wp ON wc.provider_id = wp.id\nLEFT JOIN prompts pr ON wc.prompt_id = pr.id\nLEFT JOIN template_versions tv ON wc.template_id = tv.template_id AND wc.template_version = tv.version\nLEFT JOIN templates t ON wc.template_id = t.id\nLEFT JOIN model_providers tp ON tv.provider_id = tp.id\nLEFT JOIN prompts tpr ON tv.prompt_id = tpr.id\nLEFT JOIN prompt_versions tpv ON tv.prompt_id = tpv.prompt_id AND tv.prompt_version = tpv.version\nWHERE wc.workspace_id = $1",
        "db": [
          {
            "table": "workspace_config",
            "op": "FROM",
            "values": {
              "alias": "wc",
              "role": "配置入口；直接字段 auto_start/updated_at 也来自这里"
            }
          },
          {
            "table": "template_versions",
            "op": "LEFT JOIN",
            "values": {
              "alias": "tv",
              "role": "模板默认值：agent/model/prompt/mcp/settings/resources"
            }
          },
          {
            "table": "templates",
            "op": "LEFT JOIN",
            "values": {
              "alias": "t",
              "role": "只提供 template_name/template_latest_version"
            }
          },
          {
            "table": "model_providers",
            "op": "LEFT JOIN twice",
            "values": {
              "wp": "workspace provider",
              "tp": "template provider"
            }
          },
          {
            "table": "prompts",
            "op": "LEFT JOIN twice",
            "values": {
              "pr": "workspace prompt",
              "tpr": "template prompt identity/current"
            }
          },
          {
            "table": "prompt_versions",
            "op": "LEFT JOIN",
            "values": {
              "alias": "tpv",
              "role": "template prompt version content；未命中则 fallback tpr.content"
            }
          }
        ],
        "note": "这段 SQL 的核心不是 JOIN 多，而是 SELECT 每个字段都在决定：workspace 覆盖值、template 默认值、provider/prompt 详情三者谁优先。"
      },
      {
        "title": "第七步：memory_attachments 有两个消费者：/config 摘要 + sidecar 挂载",
        "actor": "agent 主进程 + memory-fuse sidecar -> control-plane",
        "request": "GET /_cp/workspaces/ws_codex_demo/config 和 GET /_cp/workspaces/ws_codex_demo/memory-attachments",
        "url": "agent config: http://127.0.0.1:30080/_cp/workspaces/ws_codex_demo/config；sidecar mount: http://127.0.0.1:30080/_cp/workspaces/ws_codex_demo/memory-attachments",
        "params": {
          "workspace_id": "ws_codex_demo"
        },
        "paramNotes": [
          "workspace_id：两个请求都来自同一个 WORKSPACE_ID。",
          "/config 返回的 memory_attachments：给 agent 主进程使用，Codex 在 agents/codex/src/config.ts:100 读取它并传给 writePlatformPrompt，用来把 /mnt/memory 列表和 MEMORY.md index 写进 platform reminder/AGENTS.md。",
          "/memory-attachments 返回的 attachments：给 memory-fuse sidecar 使用，只需要 store_id/access/instructions，用来决定实际 mount 哪些 /mnt/memory/{store_id}。",
          "store_id：不是请求参数；它来自 workspace_memory_attachments 查询结果。"
        ],
        "response": {
          "config_response": {
            "status": 200,
            "body": {
              "memory_attachments": [
                {
                  "store_id": "mem_project",
                  "store_name": "Project KB",
                  "store_description": "项目知识库",
                  "access": "read_write",
                  "instructions": "优先阅读 /notes/overview.md",
                  "index_content": "- [Overview](notes/overview.md) — 项目总览",
                  "index_source": "memories.content where store_id=mem_project and path=/MEMORY.md"
                }
              ]
            }
          },
          "sidecar_response": {
            "status": 200,
            "body": {
              "attachments": [
                {
                  "store_id": "mem_project",
                  "access": "read_write",
                  "instructions": "优先阅读 /notes/overview.md"
                }
              ]
            }
          }
        },
        "implementation": [
          "agent 主进程路径：agents/codex/src/config.ts fetch /_cp/workspaces/:id/config；第 100 行读取 config.memory_attachments 并传给 writePlatformPrompt。Claude/Goose agent 也有同样逻辑。",
          "control-plane /config：routes/internal.ts 先 listAttachmentsForWorkspace(id)，得到 workspace_memory_attachments JOIN memory_stores 的 store_id/store_name/store_description/access/instructions。",
          "然后它对每个 attachment 调 getMemoryByPath(a.store_id, '/MEMORY.md')；如果 memories 表里存在 path=/MEMORY.md 的当前文件，就把该行 content 放进 index_content；如果不存在，index_content=null。",
          "所以 index_content 不是 workspace_memory_attachments 字段，也不是 sidecar 返回字段，而是当前 store 根目录 MEMORY.md 的内容快照。",
          "sidecar 路径：memory-fuse 启动 bootPull() 调 cpclient.ListWorkspaceAttachments(cpURL, workspaceID)，请求 /_cp/workspaces/:id/memory-attachments。",
          "关键区别：/config 里的 memory_attachments 是提示词摘要，不负责 mount；sidecar 的 /memory-attachments 是挂载清单，才负责实际 FUSE mount。"
        ],
        "db": [
          {
            "table": "workspace_memory_attachments",
            "op": "SELECT",
            "values": {
              "workspace_id": "ws_codex_demo",
              "store_id": "mem_project",
              "access": "read_write",
              "instructions": "优先阅读 /notes/overview.md"
            }
          },
          {
            "table": "memory_stores",
            "op": "JOIN",
            "values": {
              "id": "mem_project",
              "name": "Project KB",
              "owner_user_id": "u_alice"
            }
          },
          {
            "table": "memories",
            "op": "SELECT /MEMORY.md",
            "values": {
              "store_id": "mem_project",
              "path": "/MEMORY.md",
              "content": "- [Overview](notes/overview.md) — 项目总览"
            }
          }
        ],
        "note": "所以应分清：memory_attachments 不属于 workspace_config 表字段，但属于 /config 接口返回字段；同时 sidecar 仍会单独调用 /memory-attachments 做真正挂载。"
      },
      {
        "title": "第八步：memory-fuse 根据 attachments 挂载 /mnt/memory/{store_id}",
        "actor": "memory-fuse mount manager",
        "request": "Mount(store_id, readOnly)",
        "params": {
          "store_id": "mem_project",
          "readOnly": false,
          "mountpoint": "/mnt/memory/mem_project"
        },
        "paramNotes": [
          "store_id：来自 attachments.store_id。",
          "readOnly：由 access === read_only 计算得到。",
          "mountpoint：memory-fuse 固定用 /mnt/memory/{store_id}。"
        ],
        "response": {
          "internal": true,
          "mounted": "/mnt/memory/mem_project"
        },
        "implementation": [
          "mountmgr.Mount() 创建 cpclient.Client 和 memfs.Root，然后调用 fs.Mount(mountpoint, root)。",
          "memfs.NewRoot() 会先 refreshOnce()：GET /_cp/workspaces/:wsId/memory-stores/:storeId/memories，只拉 metadata 列表，不拉每个文件 content。",
          "memfs 根据 memories.path 把扁平记录合成目录树，例如 /notes/overview.md 变成 notes/overview.md。",
          "agent 后续像读普通文件一样访问 /mnt/memory/mem_project/...，实际由 FUSE 转成 CP internal API。"
        ],
        "db": [
          {
            "table": "memories",
            "op": "SELECT list",
            "values": [
              {
                "store_id": "mem_project",
                "path": "/notes/overview.md",
                "content_sha256": "sha_old",
                "size_bytes": 512
              }
            ]
          }
        ],
        "note": "这一步才是 memory 系统真正挂到 agent 容器文件系统的地方。"
      }
    ]
  },
  "memorySystem": {
    "title": "记忆系统：从 attachment 到 Pod 内 /mnt/memory",
    "intro": "memory 系统要分三层看：数据库里 workspace_memory_attachments 决定 workspace 挂哪些 store；/config 把 memory_attachments 摘要返回给 agent 主进程写入 platform prompt；Pod 里 memory-fuse sidecar 再把这些 store 实际映射成 /mnt/memory/{store_id}。agent 后续只是读写普通文件，真正查改由 FUSE 转成 control-plane internal API。",
    "quickLinks": [
      {
        "label": "数据库结构：memory_stores",
        "moduleId": "schema",
        "domain": "memory",
        "table": "memory_stores"
      },
      {
        "label": "数据库结构：workspace_memory_attachments",
        "moduleId": "schema",
        "domain": "memory",
        "table": "workspace_memory_attachments"
      },
      {
        "label": "数据库结构：memories",
        "moduleId": "schema",
        "domain": "memory",
        "table": "memories"
      },
      {
        "label": "Config 加载：memory_attachments 两条链路",
        "moduleId": "configLoad",
        "hash": "agent-step-7",
        "table": "workspace_memory_attachments"
      }
    ],
    "steps": [
      {
        "title": "创建 memory store",
        "actor": "User/UI -> control-plane",
        "request": "POST /api/memory-stores",
        "url": "http://127.0.0.1:30080/api/memory-stores",
        "params": {
          "name": "Project KB",
          "description": "项目知识库"
        },
        "paramNotes": [
          "name：前端表单输入的 store 展示名。",
          "description：前端表单输入的说明文字。",
          "owner_user_id：后端从登录态推导，前端不应该传。",
          "store 是内容边界；不同 store 里的相同 path 不冲突。"
        ],
        "response": {
          "status": 201,
          "body": {
            "id": "mem_project",
            "owner_user_id": "u_alice",
            "name": "Project KB"
          }
        },
        "implementation": [
          "前端提交表单。",
          "control-plane 校验登录态后 INSERT memory_stores。",
          "这一步只创建库，还没有挂到任何 workspace，agent Pod 内看不到它。"
        ],
        "db": [
          {
            "table": "memory_stores",
            "op": "INSERT",
            "values": {
              "id": "mem_project",
              "owner_user_id": "u_alice",
              "name": "Project KB"
            }
          }
        ],
        "note": "先有 store，后有 attachment。"
      },
      {
        "title": "把 store 挂到 workspace",
        "actor": "User/UI -> control-plane",
        "request": "POST /api/workspaces/ws_codex_demo/memory-attachments",
        "url": "http://127.0.0.1:30080/api/workspaces/ws_codex_demo/memory-attachments",
        "params": {
          "workspace_id": "ws_codex_demo",
          "store_id": "mem_project",
          "access": "read_write",
          "instructions": "优先阅读 /notes/overview.md"
        },
        "paramNotes": [
          "workspace_id：来自当前 workspace 页面。",
          "store_id：用户选择要挂载的 memory store。",
          "access：read_only/read_write，后续决定 FUSE 是否允许写。",
          "instructions：给 agent 的使用提示，不是 mount 参数。"
        ],
        "response": {
          "status": 200,
          "body": {
            "workspace_id": "ws_codex_demo",
            "store_id": "mem_project",
            "access": "read_write"
          }
        },
        "implementation": [
          "后端 attachStore() upsert workspace_memory_attachments。",
          "重复挂载会更新 access/instructions。",
          "运行中可触发 sidecar invalidate；启动时一定会被 bootPull 重新加载。"
        ],
        "db": [
          {
            "table": "workspace_memory_attachments",
            "op": "INSERT/UPSERT",
            "values": {
              "workspace_id": "ws_codex_demo",
              "store_id": "mem_project",
              "access": "read_write",
              "instructions": "优先阅读 /notes/overview.md"
            }
          }
        ],
        "note": "attachment 是可见性关系。没有这行，Pod 内就不该出现 /mnt/memory/mem_project。"
      },
      {
        "title": "Pod 内外挂载示例：agent 与 sidecar 如何共享 /mnt/memory",
        "actor": "Kubernetes -> agent Pod",
        "request": "Pod manifest volumeMounts",
        "url": "k8s://namespace/agent-pods/ws-codex-demo",
        "params": {
          "pod": "ws-codex-demo",
          "host": "node-a",
          "volume": "memory-fuse-root",
          "agentContainerMount": "/mnt/memory",
          "sidecarContainerMount": "/mnt/memory",
          "sharedStoreId": "mem_project"
        },
        "paramNotes": [
          "pod：某一个 workspace 对应的 agent Pod 示例。",
          "host：示例宿主机名，用来说明 Pod 落在哪台 node 上。",
          "volume：Pod 内 agent 容器和 memory-fuse sidecar 共同挂载的 volume。",
          "agentContainerMount：agent 主容器内看到的 memory 根目录。",
          "sidecarContainerMount：memory-fuse sidecar 容器内使用的同一个挂载根。",
          "sharedStoreId：多个 workspace attachment 指向同一个 store_id 时，它们通过数据库读写同一份 memory store 内容。"
        ],
        "codeBlocks": [
          {
            "title": "Kubernetes manifest 关键片段",
            "code": "apiVersion: v1\nkind: Pod\nmetadata:\n  name: ws-codex-demo\nspec:\n  nodeName: node-a\n  containers:\n    - name: agent\n      image: agent-platform/codex-agent:latest\n      env:\n        - name: WORKSPACE_ID\n          value: ws_codex_demo\n        - name: CP_URL\n          value: http://control-plane:30080\n      volumeMounts:\n        - name: memory-fuse-root\n          mountPath: /mnt/memory\n          mountPropagation: HostToContainer\n    - name: memory-fuse\n      image: agent-platform/memory-fuse:latest\n      securityContext:\n        privileged: true\n      env:\n        - name: WORKSPACE_ID\n          value: ws_codex_demo\n        - name: CP_URL\n          value: http://control-plane:30080\n      volumeMounts:\n        - name: memory-fuse-root\n          mountPath: /mnt/memory\n          mountPropagation: Bidirectional\n  volumes:\n    - name: memory-fuse-root\n      hostPath:\n        path: /var/lib/agent-platform/memory-fuse/ws_codex_demo\n        type: DirectoryOrCreate"
          },
          {
            "title": "宿主机 node-a 上的示例目录结构",
            "code": "manifest 对应关系：\n- volumes[0].name = memory-fuse-root\n- volumes[0].hostPath.path = /var/lib/agent-platform/memory-fuse/ws_codex_demo\n- agent.volumeMounts[name=memory-fuse-root].mountPath = /mnt/memory\n- memory-fuse.volumeMounts[name=memory-fuse-root].mountPath = /mnt/memory\n\n所以同一个 mount 根目录在三个视角里的名字是：\n宿主机 node-a: /var/lib/agent-platform/memory-fuse/ws_codex_demo\nagent 容器:     /mnt/memory\nsidecar 容器:   /mnt/memory\n\n/var/lib/agent-platform/memory-fuse/\n├── ws_codex_demo/                 # ws-codex-demo 的 hostPath.path，只是 mount 根\n│   ├── mem_project/               # FUSE mountpoint：入口，不是普通数据目录\n│   └── mem_team/\n├── ws_claude_demo/                # 另一个 Pod 的 hostPath.path，另一块 mount 根\n│   └── mem_project/               # 另一个 FUSE mountpoint：也是入口\n└── ws_goose_demo/\n    └── mem_private/\n\n这棵目录只说明“入口在哪里”。\n/memory-fuse/ws_codex_demo/mem_project 和 /memory-fuse/ws_claude_demo/mem_project 不是两份权威内容目录；它们是两个 FUSE 入口。真正的 overview.md 权威内容在 CP 数据库：memories(store_id=mem_project, path=/notes/overview.md)。"
          },
          {
            "title": "两个 workspace 都读过 overview.md 后，缓存到底会不会有两份",
            "code": "先说结论：不是“权威内容可能两份”，而是“本地 cache 是否出现、出现几份，取决于运行条件”。\n\n权威内容永远只有一份：\ncontrol-plane DB\n└── memories\n    └── (store_id=mem_project, path=/notes/overview.md, content=..., content_sha256=sha_old)\n\n本地 cache 出现的条件：\n1. memory-fuse 启动时成功创建 ContentCache。\n   源码里默认 MEMORY_CACHE_ROOT=/var/cache/memory-fuse；cache.New(cacheRoot) 失败时会降级为无缓存。\n2. agent 真的读过文件内容。\n   只 ls/readdir 只用 metadata snapshot，不会把 overview.md content 写进磁盘 cache。\n3. 读取时 cache miss，memory-fuse 从 CP GET 到 content 后，才会 Cache.Put(store_id, path, sha, content)。\n4. cache 没有被 CP invalidate、pod 重启清空、detach 后 prune、或者 sha 不匹配丢弃。\n\n如果两个 Pod 的 memory-fuse 都满足这些条件，宿主机/容器存储里通常会有两份各自的 cache：\nnode-a kubelet/container storage（示意）\n├── pod-ws-codex-demo/\n│   └── memory-fuse container writable layer or emptyDir\n│       └── /var/cache/memory-fuse/mem_project/notes/overview.md\n│       └── /var/cache/memory-fuse/mem_project/notes/overview.md.sha\n└── pod-ws-claude-demo/\n    └── memory-fuse container writable layer or emptyDir\n        └── /var/cache/memory-fuse/mem_project/notes/overview.md\n        └── /var/cache/memory-fuse/mem_project/notes/overview.md.sha\n\n如果下面任一情况发生，就不会看到两份 cache：\n- 其中一个 workspace 从没读过 overview.md，只做过 ls。\n- 其中一个 sidecar 的 cache.New 失败，运行在 no-cache 模式。\n- cacheRoot 挂的是 emptyDir，Pod 重启后 cache 被清空。\n- 另一个 agent 写入同一文件后，CP 广播 invalidate，sidecar 清掉旧 cache。\n- snapshot 里的 content_sha256 和本地 .sha 不一致，memory-fuse 会把本地 cache 当 miss，不信任它。\n\n所以更准确的说法是：\n/mnt/memory/mem_project 是两个入口，不是两份权威内容；\n读过以后，每个 sidecar 可以各自保存一份可丢弃的本地 cache。cache 是性能优化，不是共享数据的来源。"
          },
          {
            "title": "容器内看到的目录结构",
            "code": "# ws-codex-demo Pod\nagent 容器:\n/mnt/memory/\n├── mem_project/\n│   └── notes/overview.md\n└── mem_team/\n\nmemory-fuse sidecar 容器:\n/mnt/memory/\n├── mem_project/      # sidecar 在这里 mount FUSE root\n└── mem_team/\n\n# ws-claude-demo Pod，attachment 也指向 store_id=mem_project\nagent 容器:\n/mnt/memory/\n└── mem_project/\n    └── notes/overview.md          # 和 ws-codex-demo 看到的是同一个 store 的远端内容"
          }
        ],
        "response": {
          "visibleInAgent": [
            "/mnt/memory/mem_project",
            "/mnt/memory/mem_team"
          ],
          "sharedStore": "mem_project",
          "whyAgentCanSeeIt": "agent 和 sidecar 挂载同一个 Pod volume；sidecar 用 Bidirectional mountPropagation 在 /mnt/memory/{store_id} 建立 FUSE mount，agent 容器通过 HostToContainer 看到这个 mount。"
        },
        "implementation": [
          "同一个 Pod 里 agent 容器和 memory-fuse sidecar 都把 memory-fuse-root volume 挂到 /mnt/memory。",
          "sidecar 启动后请求 /_cp/workspaces/:id/memory-attachments，拿到 store_id 列表。",
          "sidecar 对每个 store 调 Mount(storeID)，在 /mnt/memory/{store_id} 建立 FUSE mount。这个 mount 操作发生在 memory-fuse-root 这个共享 volume 里。",
          "mountPropagation 的作用：sidecar 使用 Bidirectional，表示 sidecar 容器里新建的 mount 会传播回宿主机对应 hostPath；agent 使用 HostToContainer，表示宿主机/共享 volume 上新增的 mount 会传播进 agent 容器。组合起来就是：sidecar mount 后，agent 容器能看到 /mnt/memory/{store_id}。",
          "FUSE 原理：/mnt/memory/mem_project 看起来像普通目录，但内核把 open/read/write/readdir 等文件系统操作转发给 memory-fuse 进程；memory-fuse 再调用 control-plane internal API，最终读写 memories 和 memory_versions。",
          "agent 容器读 /mnt/memory/mem_project/notes/overview.md 时，不是在读 hostPath mountpoint 里的普通文件，而是触发 FUSE Read；FUSE 先看 sidecar 内存 buffer/磁盘 cache 是否命中，必要时再调用 CP 的 memory API。",
          "多个 workspace 如果都 attach store_id=mem_project，各自 Pod 里会有自己的 /mnt/memory/mem_project mountpoint；这些 mountpoint 都代理到同一个 store_id 的后端数据库内容，所以看到的是同一份远端权威内容。",
          "但 cache 要单独理解：memory-fuse 有 MEMORY_CACHE_ROOT，默认 /var/cache/memory-fuse。只有 sidecar 成功创建 cache、agent 读过文件内容、且 cache 未被 invalidate/prune/重启清掉时，才会在该 sidecar 的 cache root 下看到 overview.md 和 overview.md.sha。两个 Pod 都满足这些条件时，会有两份各自的 cache。",
          "cache 是否存在取决于是否成功启用 ContentCache、是否真的读取过 content、cache 是否仍然新鲜。只执行 ls/readdir 不会写 content cache，因为目录列表只来自 metadata snapshot。"
        ],
        "db": [
          {
            "table": "workspace_memory_attachments",
            "op": "SELECT later",
            "values": [
              {
                "workspace_id": "ws_codex_demo",
                "store_id": "mem_project"
              },
              {
                "workspace_id": "ws_claude_demo",
                "store_id": "mem_project"
              },
              {
                "workspace_id": "ws_goose_demo",
                "store_id": "mem_private"
              }
            ]
          }
        ],
        "note": "关键点：/mnt/memory/{store_id} 的 mountpoint 不是数据副本；CP 数据库里的 memories 行才是权威内容。两个 workspace 都读过同一文件后，只有在各自 sidecar cache 启用且未被清理时，宿主机上才会有两份可丢弃的 cache。"
      },
      {
        "title": "sidecar 启动加载 attachment 列表",
        "actor": "memory-fuse sidecar -> control-plane",
        "request": "GET /_cp/workspaces/ws_codex_demo/memory-attachments",
        "url": "http://127.0.0.1:30080/_cp/workspaces/ws_codex_demo/memory-attachments",
        "params": {
          "workspace_id": "ws_codex_demo"
        },
        "paramNotes": [
          "workspace_id：来自 sidecar 环境变量 WORKSPACE_ID。",
          "store_id：不是请求参数；来自接口返回的 workspace_memory_attachments 查询结果。",
          "access：不是请求参数；返回后转换成 readOnly flag。"
        ],
        "response": {
          "status": 200,
          "body": {
            "attachments": [
              {
                "store_id": "mem_project",
                "access": "read_write",
                "instructions": "优先阅读 /notes/overview.md"
              }
            ]
          }
        },
        "implementation": [
          "cmd/memory-fuse/main.go 的 bootPull() 调 ListWorkspaceAttachments。",
          "control-plane internal route 调 listAttachmentsForWorkspace(id)，但返回给 sidecar 的 JSON 只保留 store_id/access/instructions；store name、description、index_content 是 /config 里给 agent prompt 用的摘要字段。",
          "每个 attachment 交给 mountmgr.Mount(storeID, readOnly)。"
        ],
        "db": [
          {
            "table": "workspace_memory_attachments",
            "op": "SELECT",
            "values": {
              "workspace_id": "ws_codex_demo",
              "store_id": "mem_project",
              "access": "read_write"
            }
          },
          {
            "table": "memory_stores",
            "op": "JOIN",
            "values": {
              "id": "mem_project",
              "name": "Project KB"
            }
          }
        ],
        "note": "这是 memory_attachments 的 load 点；它不在 getWorkspaceConfig() 的 SQL 返回值里。"
      },
      {
        "title": "挂载为 /mnt/memory/{store_id} 并首次拉 metadata 列表",
        "actor": "memory-fuse memfs -> control-plane",
        "request": "GET /_cp/workspaces/ws_codex_demo/memory-stores/mem_project/memories",
        "url": "http://127.0.0.1:30080/_cp/workspaces/ws_codex_demo/memory-stores/mem_project/memories",
        "params": {
          "store_id": "mem_project"
        },
        "paramNotes": [
          "store_id：当前要列出文件的 memory store，来自上一步 attachment.store_id。",
          "mountpoint：不是 HTTP 参数；它是 sidecar 本地根据 store_id 算出来的路径，规则是 /mnt/memory/{store_id}，所以这里是 /mnt/memory/mem_project。",
          "metadata：保存在 memories 表里，列表接口只返回 id/store_id/path/content_sha256/size_bytes/description/mem_type/created_at/updated_at，不返回 content。"
        ],
        "response": {
          "status": 200,
          "body": {
            "memories": [
              {
                "id": "mem_idx",
                "store_id": "mem_project",
                "path": "/MEMORY.md",
                "content_sha256": "sha_idx",
                "size_bytes": 180,
                "description": "store index"
              },
              {
                "id": "mem_file_1",
                "store_id": "mem_project",
                "path": "/notes/overview.md",
                "content_sha256": "sha_old",
                "size_bytes": 512,
                "description": "项目总览"
              },
              {
                "id": "mem_file_2",
                "store_id": "mem_project",
                "path": "/tasks/todo.md",
                "content_sha256": "sha_todo",
                "size_bytes": 260,
                "description": "待办事项"
              }
            ]
          }
        },
        "codeBlocks": [
          {
            "title": "metadata 存在哪里",
            "code": "memories 表是扁平记录，不存目录行：\n\nid          store_id      path                 content_sha256  size_bytes  content\n----------  ------------  -------------------  --------------  ----------  ----------------\nmem_idx     mem_project   /MEMORY.md           sha_idx         180         有，但列表接口不返回\nmem_file_1  mem_project   /notes/overview.md   sha_old         512         有，但列表接口不返回\nmem_file_2  mem_project   /tasks/todo.md       sha_todo        260         有，但列表接口不返回\n\nGET /memories 只拿 metadata：path/hash/size/description...\nGET /memory/<path> 才拿 content。"
          },
          {
            "title": "FUSE 如何根据 path 合成目录树",
            "code": "接口返回的 path 列表：\n- /MEMORY.md\n- /notes/overview.md\n- /tasks/todo.md\n\nmemfs.snapshot.load() 会拆 path：\n/MEMORY.md          -> 根目录 / 下有文件 MEMORY.md\n/notes/overview.md  -> 根目录 / 下有目录 notes，/notes 下有文件 overview.md\n/tasks/todo.md      -> 根目录 / 下有目录 tasks，/tasks 下有文件 todo.md\n\n所以 agent 在容器里看到：\n/mnt/memory/mem_project/\n├── MEMORY.md\n├── notes/\n│   └── overview.md\n└── tasks/\n    └── todo.md\n\n数据库里没有 notes 这一行，也没有 tasks 这一行。\n它们是 FUSE 根据文件 path 的前缀临时算出来的目录节点。"
          },
          {
            "title": "源码：snapshot 进程内数据结构",
            "code": "// memory-fuse/internal/memfs/memfs.go:61-84\n// snapshot is a tree of dir/file nodes derived from a flat list of paths.\ntype snapshot struct {\n    mu        sync.RWMutex\n    files     map[string]*cpclient.MemoryLite // absolute path -> meta\n    children  map[string]map[string]bool      // dir path -> set of immediate child names\n    ephemeral map[string]bool                 // dir path -> exists (user-mkdir'd, no memory yet)\n}\n\nfunc newSnapshot() *snapshot {\n    return &snapshot{\n        files:     map[string]*cpclient.MemoryLite{},\n        children:  map[string]map[string]bool{\"/\": {}},\n        ephemeral: map[string]bool{},\n    }\n}\n\n对应示例：\nfiles = {\n  \"/MEMORY.md\": { Path: \"/MEMORY.md\", ContentSHA256: \"sha_idx\", SizeBytes: 180 },\n  \"/notes/overview.md\": { Path: \"/notes/overview.md\", ContentSHA256: \"sha_old\", SizeBytes: 512 },\n  \"/tasks/todo.md\": { Path: \"/tasks/todo.md\", ContentSHA256: \"sha_todo\", SizeBytes: 260 },\n}\n\nchildren = {\n  \"/\": { \"MEMORY.md\": true, \"notes\": true, \"tasks\": true },\n  \"/notes\": { \"overview.md\": true },\n  \"/tasks\": { \"todo.md\": true },\n}\n\nephemeral = {\n  \"/drafts\": true // 用户 mkdir 但还没有任何 memories.path 落库时，临时保存在进程内\n}\n\n解释：files 用来回答“这个 path 是不是文件、size/sha 是多少”；children 用来回答“这个目录下面有哪些直接子节点”。目录不是数据库行，是 children map 根据 path 前缀合成出来的。"
          },
          {
            "title": "源码：load() 构建 children，readdir 直接读 snapshot",
            "code": "// memory-fuse/internal/memfs/memfs.go:86-118\nfunc (s *snapshot) load(items []cpclient.MemoryLite) {\n    s.mu.Lock()\n    defer s.mu.Unlock()\n    s.files = map[string]*cpclient.MemoryLite{}\n    s.children = map[string]map[string]bool{\"/\": {}}\n    for i := range items {\n        m := items[i]\n        s.files[m.Path] = &m\n        parts := strings.Split(strings.TrimPrefix(m.Path, \"/\"), \"/\")\n        dir := \"/\"\n        for i, p := range parts {\n            if _, ok := s.children[dir]; !ok {\n                s.children[dir] = map[string]bool{}\n            }\n            s.children[dir][p] = true\n            if i == len(parts)-1 { break }\n            if dir == \"/\" { dir = \"/\" + p } else { dir = dir + \"/\" + p }\n            if _, ok := s.children[dir]; !ok {\n                s.children[dir] = map[string]bool{}\n            }\n        }\n    }\n}\n\n// memory-fuse/internal/memfs/memfs.go:178-188\nfunc (s *snapshot) listDir(dir string) ([]string, bool) {\n    s.mu.RLock()\n    defer s.mu.RUnlock()\n    c, ok := s.children[dir]\n    if !ok { return nil, false }\n    out := make([]string, 0, len(c))\n    for n := range c { out = append(out, n) }\n    return out, true\n}\n\n// memory-fuse/internal/memfs/memfs.go:746-760\nfunc readdirAt(root *Root, dir string) (fs.DirStream, syscall.Errno) {\n    names, ok := root.snap.listDir(dir)\n    if !ok { return nil, syscall.ENOENT }\n    entries := make([]fuse.DirEntry, 0, len(names))\n    for _, n := range names {\n        full := joinPath(dir, n)\n        mode := uint32(fuse.S_IFREG)\n        if root.snap.isDir(full) { mode = fuse.S_IFDIR }\n        entries = append(entries, fuse.DirEntry{Name: n, Mode: mode})\n    }\n    return fs.NewListDirStream(entries), 0\n}\n\n解释：agent 执行 ls 时，内核触发 FUSE Readdir；Readdir 走 readdirAt；readdirAt 只读 root.snap.children。这里没有 ListMemories，也没有请求 control-plane。"
          }
        ],
        "implementation": [
          "mountmgr.Mount(storeID) 先算出本地 mountpoint：/mnt/memory/{store_id}，例如 /mnt/memory/mem_project。这个路径是本地挂载位置，不是 /memories 接口参数。",
          "memfs.NewRoot() 创建 Root 后会先 refreshOnce()，调用 cpclient.ListMemories() 请求 /_cp/workspaces/:workspaceId/memory-stores/:storeId/memories。",
          "control-plane 从 memories 表按 store_id 查询 metadata 列表；列表接口不返回 content，是为了让 ls/readdir/getattr 轻量。",
          "memfs.snapshot 的真实内存结构是三张 map：files 保存文件 metadata，children 保存目录到直接子节点名集合，ephemeral 保存用户 mkdir 出来但还没有落库文件的空目录。",
          "以 /notes/overview.md 为例：files 里有 key=/notes/overview.md；children[\"/\"] 里有 notes；children[\"/notes\"] 里有 overview.md。数据库不需要保存 /notes 目录行。",
          "agent 执行 ls/readdir 时，Root.Readdir 或 dirNode.Readdir 调 readdirAt(root, dir)，readdirAt 再调 root.snap.listDir(dir)，只从 children map 返回目录项。",
          "snapshot 会在挂载时 refreshOnce() 拉一次，之后后台 refreshLoop 定时刷新；CP 推送 Invalidate 或写入成功后也会触发 refreshOnce()。",
          "这样数据库只存文件路径，FUSE 层负责让它看起来像目录树。"
        ],
        "db": [
          {
            "table": "memories",
            "op": "SELECT metadata",
            "values": [
              {
                "store_id": "mem_project",
                "path": "/MEMORY.md",
                "content_sha256": "sha_idx",
                "size_bytes": 180
              },
              {
                "store_id": "mem_project",
                "path": "/notes/overview.md",
                "content_sha256": "sha_old",
                "size_bytes": 512
              },
              {
                "store_id": "mem_project",
                "path": "/tasks/todo.md",
                "content_sha256": "sha_todo",
                "size_bytes": 260
              }
            ]
          }
        ],
        "note": "一句话：mountpoint 是本地路径；metadata 在 memories 表；sidecar 进程内 snapshot.files/snapshot.children 把扁平 path 合成目录树，ls 直接读内存 snapshot，不会每次请求 CP。"
      },
      {
        "title": "读 memory 文件",
        "actor": "Agent -> FUSE -> control-plane",
        "request": "cat /mnt/memory/mem_project/notes/overview.md",
        "url": "GET http://127.0.0.1:30080/_cp/workspaces/ws_codex_demo/memory-stores/mem_project/memory/notes/overview.md",
        "params": {
          "path": "/notes/overview.md",
          "store_id": "mem_project"
        },
        "paramNotes": [
          "path：由 FUSE 相对路径补成绝对 memory path。",
          "store_id：来自挂载点 /mnt/memory/mem_project。"
        ],
        "response": {
          "status": 200,
          "body": {
            "id": "mem_file_1",
            "path": "/notes/overview.md",
            "content": "---\nname: overview\n---\n项目说明...",
            "content_sha256": "sha_old"
          }
        },
        "implementation": [
          "fileNode.Read() 调 ensureLoaded()。",
          "ensureLoaded() 先查本地 cache；未命中再 GetMemory(path)。",
          "Open 返回 DIRECT_IO，降低内核缓存导致的陈旧内容风险。"
        ],
        "db": [
          {
            "table": "memories",
            "op": "SELECT full row",
            "values": {
              "store_id": "mem_project",
              "path": "/notes/overview.md",
              "content_sha256": "sha_old"
            }
          }
        ],
        "note": "列表接口不返回 content，读文件接口才返回 content。"
      },
      {
        "title": "读取新鲜度：read 是否每次请求 CP 检查变化",
        "actor": "Agent read -> memory-fuse sidecar",
        "request": "read /mnt/memory/mem_project/notes/overview.md",
        "url": "GET http://127.0.0.1:30080/_cp/workspaces/ws_codex_demo/memory-stores/mem_project/memory/notes/overview.md（仅 first load/cache miss 时触发；不是每次 read 都触发）",
        "params": {
          "path": "/notes/overview.md",
          "loaded": true,
          "snapshot_sha": "sha_other",
          "cache_sha": "sha_other"
        },
        "paramNotes": [
          "path：agent 读取的 FUSE 路径会被转换成 memory path。",
          "loaded：fileNode 是否已经有内存 buffer；如果是 true，Read() 直接返回 buffer，不会请求 CP。",
          "snapshot_sha：sidecar 最近一次 ListMemories 得到的元数据 sha。",
          "cache_sha：磁盘内容缓存旁边保存的 sha；只有 cache_sha 等于 snapshot_sha，cache 才可信。"
        ],
        "response": {
          "loaded_true": "直接返回 fileNode.buf",
          "loaded_false_cache_hit": "从磁盘 cache 返回 sha 匹配内容",
          "loaded_false_cache_miss": "GET control-plane 拉取 memories.content 并更新 baseSHA/cache"
        },
        "codeBlocks": [
          {
            "title": "源码 1：Read() 先看 fileNode 是否 loaded",
            "code": "// memory-fuse/internal/memfs/memfs.go:399-415\nfunc (f *fileNode) Read(ctx context.Context, _ fs.FileHandle, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {\n    if errno := f.ensureLoaded(ctx); errno != 0 {\n        return nil, errno\n    }\n    f.mu.Lock()\n    body := f.buf\n    ...\n    return fuse.ReadResultData(out), 0\n}\n\n// memory-fuse/internal/memfs/memfs.go:536-540\nif f.loaded {\n    f.mu.Unlock()\n    return 0\n}\n\n解释：已经 loaded 的 fileNode 不会每次 read 都问 CP。它直接读 sidecar 进程内的 buffer。"
          },
          {
            "title": "源码 2：snapshot 的刷新来源",
            "code": "// memory-fuse/internal/memfs/memfs.go:214-242\nfunc NewRoot(ctx context.Context, opt Options) *Root {\n    r := &Root{opt: opt, log: opt.Logger, snap: newSnapshot()}\n    r.refreshOnce(ctx)\n    go r.refreshLoop(ctx)\n    return r\n}\n\nfunc (r *Root) refreshOnce(ctx context.Context) {\n    items, err := r.opt.Backend.ListMemories(ctx)\n    if err != nil { return }\n    r.snap.load(items)\n}\n\nfunc (r *Root) refreshLoop(ctx context.Context) {\n    ticker := time.NewTicker(r.opt.RefreshInterval)\n    ...\n    r.refreshOnce(ctx)\n}\n\n解释：sidecar 启动时会拉一次 metadata 列表，之后靠定时 refresh 兜底。列表里主要是 path、size、content_sha256，不是正文。"
          },
          {
            "title": "源码 3：CP invalidate 推送只清 cache + 刷 snapshot",
            "code": "// memory-fuse/internal/mountmgr/mountmgr.go:158-170\nfunc (m *Manager) Invalidate(storeID string) error {\n    if m.opt.Cache != nil {\n        _ = m.opt.Cache.Invalidate(storeID)\n    }\n    mt, ok := m.mounts[storeID]\n    if !ok { return nil }\n    mt.root.RefreshNow(context.Background())\n    return nil\n}\n\n// memory-fuse/internal/cache/cache.go:57-78\n// Get returns cached content only if on-disk sha matches expectedSHA.\n\n解释：CP 写入后会广播 invalidate，sidecar 会丢磁盘 cache 并刷新 snapshot。但这段代码没有遍历所有已 loaded 的 fileNode 去强制清空内存 buffer。"
          }
        ],
        "implementation": [
          "第一层：FUSE 的 Read() 不会天然等价于数据库 SELECT。它先进入 ensureLoaded()，如果当前 fileNode.loaded=true，就直接返回 sidecar 内存里的 f.buf。",
          "第二层：如果 loaded=false，ensureLoaded() 才会用当前 snapshot 里的 content_sha256 判断磁盘 cache 是否可信；sha 对得上就读 cache。",
          "第三层：只有 loaded=false 且 cache miss，sidecar 才会调用 Backend.GetMemory，也就是 GET control-plane 读取 memories.content。",
          "第四层：snapshot 的新鲜度来自三类事件：挂载时 refreshOnce、CP 写/delete/move 后通过 gRPC Invalidate 触发 RefreshNow、以及 refreshLoop 定时兜底。",
          "第五层：因此当前实现不是“每次 read 都强一致检查数据库”。正常预期是 CP invalidate 及时送达，下一次未加载读取能看到新 sha；如果 invalidate 丢失，要等定时 refresh；如果同一个 fileNode 已经 loaded，它可能继续读到旧 buffer。",
          "第六层：如果业务要求每次读取必须线性一致，就需要在实现上增加 read 前 revalidate、invalidate 时清理 loaded fileNode，或提供显式 refresh/close-reopen 语义；当前代码没有做到这一点。"
        ],
        "db": [
          {
            "table": "memories",
            "op": "LIST metadata by ListMemories",
            "values": {
              "store_id": "mem_project",
              "path": "/notes/overview.md",
              "content_sha256": "sha_other",
              "content": "not returned in lite list"
            }
          },
          {
            "table": "memories",
            "op": "GET content only on load/cache miss",
            "values": {
              "path": "/notes/overview.md",
              "content_sha256": "sha_other",
              "content": "server current body"
            }
          }
        ],
        "note": "回答你的问题：不会每次 read 都请求 CP 检查变化；当前新鲜度依赖 invalidate + snapshot refresh + sha 校验缓存。它是接近实时/最终一致，不是每次 read 强一致。"
      },
      {
        "title": "修改 memory 文件：commit-on-close + CAS",
        "actor": "Agent -> FUSE Write/Flush -> control-plane",
        "request": "write then close /mnt/memory/mem_project/notes/overview.md",
        "url": "PUT http://127.0.0.1:30080/_cp/workspaces/ws_codex_demo/memory-stores/mem_project/memory/notes/overview.md",
        "params": {
          "content": "---\nname: overview\n---\nnew body",
          "if_match_sha256": "sha_old"
        },
        "paramNotes": [
          "content：agent 写入的完整文件内容。FUSE 最终会把整个 buffer 作为 content PUT 给 control-plane。",
          "if_match_sha256：打开或读取文件时看到的旧 sha，也就是 fileNode.baseSHA；CP 用它判断当前数据库内容有没有被别人改过。"
        ],
        "response": {
          "success": {
            "status": 200,
            "body": {
              "id": "mem_file_1",
              "path": "/notes/overview.md",
              "content_sha256": "sha_new",
              "size_bytes": 680
            }
          },
          "cas_failure": {
            "status": 412,
            "body": {
              "error": "sha256 precondition failed",
              "current_sha256": "sha_other"
            }
          }
        },
        "codeBlocks": [
          {
            "title": "源码 1：Write() 只改 sidecar 内存 buffer，不立刻写数据库",
            "code": "// memory-fuse/internal/memfs/memfs.go:418-432\nfunc (f *fileNode) Write(ctx context.Context, _ fs.FileHandle, data []byte, off int64) (uint32, syscall.Errno) {\n    if errno := f.ensureLoaded(ctx); errno != 0 {\n        return 0, errno\n    }\n    f.mu.Lock()\n    defer f.mu.Unlock()\n    end := int(off) + len(data)\n    if end > len(f.buf) {\n        grown := make([]byte, end)\n        copy(grown, f.buf)\n        f.buf = grown\n    }\n    copy(f.buf[off:], data)\n    f.dirty = true\n    return uint32(len(data)), 0\n}\n\n解释：agent 写文件时，数据先进 fileNode.buf，dirty=true。这里还没有 HTTP 请求，也没有更新 memories 表。"
          },
          {
            "title": "源码 2：close/Flush 或 fsync 才 commit；提交前校验 frontmatter",
            "code": "// memory-fuse/internal/memfs/memfs.go:456-495\nfunc (f *fileNode) Flush(ctx context.Context, _ fs.FileHandle) syscall.Errno {\n    return f.commit(ctx)\n}\n\nfunc (f *fileNode) Fsync(ctx context.Context, _ fs.FileHandle, _ uint32) syscall.Errno {\n    return f.commit(ctx)\n}\n\nfunc (f *fileNode) commit(ctx context.Context) syscall.Errno {\n    f.mu.Lock()\n    if !f.dirty {\n        f.mu.Unlock()\n        return 0\n    }\n    body := append([]byte(nil), f.buf...)\n    base := f.baseSHA\n    f.mu.Unlock()\n\n    path := nodePath(&f.Inode)\n    if err := validateMemoryContent(path, body); err != nil {\n        f.dirty = false\n        f.loaded = false\n        return syscall.EINVAL\n    }\n    m, err := f.root.opt.Backend.PutMemory(ctx, path, string(body), base)\n    ...\n}\n\n解释：commit-on-close 的意思是普通 Write 只积攒内容，close(2) 触发 Flush 后才真正提交。base 就是旧 sha，后面会作为 if_match_sha256。"
          },
          {
            "title": "源码 3：cpclient.PutMemory 发 PUT，并把 baseSHA 放进 if_match_sha256",
            "code": "// memory-fuse/internal/cpclient/client.go:169-184\nfunc (c *Client) PutMemory(ctx context.Context, path, content, ifMatchSHA256 string) (*Memory, error) {\n    body, _ := json.Marshal(map[string]any{\n        \"content\":          content,\n        \"if_match_sha256\":  ifMatchSHA256,\n    })\n    u, err := c.memoryURL(path)\n    ...\n    req, err := http.NewRequestWithContext(ctx, http.MethodPut, u, bytes.NewReader(body))\n    req.Header.Set(\"Content-Type\", \"application/json\")\n    ...\n}\n\n// memoryURL(path) -> /_cp/workspaces/<wsId>/memory-stores/<storeId>/memory/<path>\n\n解释：FUSE 不直接连数据库。它把 path/content/旧 sha 发给 CP internal API。"
          },
          {
            "title": "源码 4：control-plane 校验 attachment/read_only，然后 putMemory 做 CAS 事务",
            "code": "// control-plane/src/routes/internal.ts:165-189\ninternal.put('/workspaces/:wsId/memory-stores/:storeId/memory/*', async (c) => {\n  const att = await requireAttached(wsId, storeId)\n  if (!att) return c.json({ error: 'store not attached to workspace' }, 404)\n  if (att.access === 'read_only') return c.json({ error: 'store is read-only' }, 403)\n  ...\n  const m = await putMemory({\n    storeId, path, content: body.content,\n    ifMatchSha256: body.if_match_sha256,\n    actorKind: 'agent', actorId: wsId,\n  })\n  broadcastStoreInvalidate(storeId)\n  return c.json(m)\n})\n\n// control-plane/src/services/db/memory.ts:205-249\nBEGIN\nSELECT ... FROM memories WHERE store_id=$1 AND path=$2 FOR UPDATE\nif row exists:\n  if if_match_sha256 is empty -> conflict\n  if if_match_sha256 != row.content_sha256 -> PreconditionFailedError\n  UPDATE memories SET content=?, content_sha256=?, size_bytes=?, updated_at=NOW()\n  INSERT memory_versions(operation='update', actor_kind='agent', actor_id=workspace_id)\nCOMMIT\n\n解释：CAS 就是用旧 sha 做条件更新。别人先改了同一个文件，当前 sha 就不等于 if_match_sha256，CP 拒绝覆盖。"
          },
          {
            "title": "成功和失败后的 sidecar 行为",
            "code": "// memory-fuse/internal/memfs/memfs.go:495-527\nif err != nil {\n    if errors.As(err, &pre) {\n        f.loaded = false\n        f.dirty = false\n        f.root.refreshOnce(ctx)\n        return syscall.EIO\n    }\n    return syscall.EIO\n}\nf.baseSHA = m.ContentSHA256\nf.dirty = false\ncache.Put(storeID, path, m.ContentSHA256, body)\nf.root.refreshOnce(ctx)\n\n解释：成功后更新 baseSHA=新 sha，清掉 dirty，并刷新 metadata snapshot。CAS 失败时丢掉本地旧视图，刷新 snapshot，让下一次读拿到服务器当前版本。"
          },
          {
            "title": "源码 6：CAS 失败后的返回链路",
            "code": "// control-plane/src/services/db/memory.ts:213-219\nif (row) {\n  if (input.ifMatchSha256 === undefined || input.ifMatchSha256 === '') {\n    throw new PathConflictError()\n  }\n  if (input.ifMatchSha256 !== row.content_sha256) {\n    throw new PreconditionFailedError(row.content_sha256)\n  }\n  // sha 相等才会继续 UPDATE + INSERT memory_versions\n}\n\n// control-plane/src/routes/internal.ts:190-199\n} catch (e) {\n  if (e instanceof PathConflictError) return c.json({ error: 'path already exists' }, 409)\n  if (e instanceof PreconditionFailedError) {\n    return c.json({ error: 'sha256 precondition failed', current_sha256: e.currentSha }, 412)\n  }\n  throw e\n}\n\n// memory-fuse/internal/cpclient/client.go:271-289\nif resp.StatusCode == http.StatusPreconditionFailed {\n    var body struct { CurrentSHA256 string `json:\"current_sha256\"` }\n    _ = json.NewDecoder(resp.Body).Decode(&body)\n    return &ErrPrecondition{CurrentSHA256: body.CurrentSHA256}\n}\n\n// memory-fuse/internal/memfs/memfs.go:497-506\nif errors.As(err, &pre) {\n    f.root.log.Warn(\"memory flush sha mismatch\", \"path\", path, \"expected\", base, \"current\", pre.CurrentSHA256)\n    // Drop our local view; next read will fetch the new server copy.\n    f.mu.Lock()\n    f.loaded = false\n    f.dirty = false\n    f.mu.Unlock()\n    f.root.refreshOnce(ctx)\n    return syscall.EIO\n}\n\n解释：乐观锁失败不会自动合并，也不会自动重试。sidecar 会丢弃这次本地 dirty buffer，刷新目录快照，然后把 close/fsync 失败暴露给写文件的一方。下一次读文件时，ensureLoaded 会重新拿服务器当前版本。"
          },
          {
            "title": "源码 7：失败后的恢复是谁处理",
            "code": "// memory-fuse/internal/memfs/memfs.go:399-415\nfunc (f *fileNode) Read(ctx context.Context, _ fs.FileHandle, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {\n    if errno := f.ensureLoaded(ctx); errno != 0 {\n        return nil, errno\n    }\n    ...\n    return fuse.ReadResultData(out), 0\n}\n\n// memory-fuse/internal/memfs/memfs.go:536-584\nfunc (f *fileNode) ensureLoaded(ctx context.Context) syscall.Errno {\n    f.mu.Lock()\n    if f.loaded {\n        f.mu.Unlock()\n        return 0\n    }\n    f.mu.Unlock()\n\n    path := nodePath(&f.Inode)\n    if c := f.root.opt.Cache; c != nil {\n        if m, ok := f.root.snap.file(path); ok && m.ContentSHA256 != \"\" {\n            if content, hit := c.Get(f.root.opt.StoreID, path, m.ContentSHA256); hit {\n                f.loaded = true\n                f.buf = content\n                f.baseSHA = m.ContentSHA256\n                return 0\n            }\n        }\n    }\n\n    full, err := f.root.opt.Backend.GetMemory(ctx, path)\n    ...\n    f.loaded = true\n    f.buf = []byte(full.Content)\n    f.baseSHA = full.ContentSHA256\n    return 0\n}\n\n解释：CAS 失败后，sidecar 不会自己替 agent 做 merge。它只是把 loaded=false，让下一次 Read() 必须重新 ensureLoaded。真正决定“读最新版本、合并、再写一次”的，是最上层正在写文件的调用方：可能是 agent 的工具调用、编辑器、脚本，或者用户手动操作。"
          }
        ],
        "implementation": [
          "第一段：agent 写文件时，FUSE 的 Write() 只把字节写进 sidecar 进程内的 fileNode.buf，并标记 dirty=true；这一步不会访问数据库。",
          "第二段：应用 close 文件或 fsync 时，FUSE 调 Flush()/Fsync()，统一进入 commit()。commit 复制当前 buffer，拿到 baseSHA，然后先做 validateMemoryContent。",
          "第三段：校验通过后，sidecar 调 Backend.PutMemory(path, content, baseSHA)。baseSHA 会变成 HTTP body 里的 if_match_sha256。",
          "第四段：control-plane internal PUT 先确认这个 workspace 确实 attach 了该 store，且 access 不是 read_only，然后调用 putMemory()。",
          "第五段：putMemory() 在事务里 SELECT ... FOR UPDATE 锁住该 store/path 的当前行，比较 if_match_sha256 和当前 content_sha256；相等才 UPDATE memories，并 INSERT memory_versions。",
          "第六段：成功后 sidecar 更新自己的 baseSHA，刷新 metadata snapshot；失败时刷新 snapshot 并返回错误，避免覆盖别人刚写的新版本。",
          "第七段：如果 put 时发现 if_match_sha256=sha_old，但数据库当前 content_sha256 已经是 sha_other，control-plane 抛 PreconditionFailedError，不执行 UPDATE，也不插入 memory_versions。",
          "第八段：internal API 把这个错误返回成 HTTP 412，body 里带 current_sha256，告诉 sidecar 当前服务器版本的 sha。",
          "第九段：cpclient 把 412 转成 ErrPrecondition；FUSE commit 捕获后把 loaded=false、dirty=false，并 refreshOnce(ctx)。这表示本次写入内容被放弃，下一次读会加载服务器当前内容。",
          "第十段：后续恢复不是 control-plane、数据库或 memory-fuse 自动完成。memory-fuse 只负责把错误返回给写文件的一方，并把本地 fileNode 标记成 loaded=false，保证下一次 read 会重新加载当前服务器版本。",
          "第十一段：谁来处理取决于谁发起写入。如果是 agent 工具写文件，工具调用会收到 close/fsync 的 EIO，agent 应该重新 read 该 memory 文件、对比自己刚才想写的内容和服务器当前内容，再生成合并后的新内容。",
          "第十二段：如果是人用 vim/脚本/echo 写文件，操作系统会把 close/fsync 失败返回给这个进程；是否保留临时内容、重新打开文件、手动合并，是编辑器或用户自己的行为。当前 FUSE 实现不会保存一份冲突副本。",
          "第十三段：重新读的时候，Read() 调 ensureLoaded()。因为 CAS 失败时 loaded=false，ensureLoaded 会按当前 snapshot 的 sha 查 cache，cache 没命中就 GET control-plane，拿到 sha_other 对应的正文，并把 baseSHA 更新成 sha_other。下一次 PUT 才会用新的 if_match_sha256。"
        ],
        "db": [
          {
            "table": "memories",
            "op": "SELECT FOR UPDATE",
            "values": {
              "store_id": "mem_project",
              "path": "/notes/overview.md",
              "current_content_sha256": "sha_old"
            }
          },
          {
            "table": "memories",
            "op": "UPDATE",
            "values": {
              "id": "mem_file_1",
              "content_sha256": "sha_new",
              "size_bytes": 680,
              "updated_at": "now()"
            }
          },
          {
            "table": "memory_versions",
            "op": "INSERT",
            "values": {
              "store_id": "mem_project",
              "memory_id": "mem_file_1",
              "path": "/notes/overview.md",
              "operation": "update",
              "actor_kind": "agent",
              "actor_id": "ws_codex_demo",
              "content_sha256": "sha_new"
            }
          },
          {
            "table": "memories / memory_versions",
            "op": "NO UPDATE on CAS failure",
            "values": {
              "if_match_sha256": "sha_old",
              "current_content_sha256": "sha_other",
              "result": "ROLLBACK；memories 不变；memory_versions 不新增 update 版本"
            }
          }
        ],
        "note": "关键结论：CAS 失败时，本次写入不会落库；sidecar 只负责返回错误和刷新视图。真正的“重新读取、人工或 agent 合并、再次写入”由发起写入的上层调用方处理。"
      }
    ]
  },
  "afsSharedFiles": {
    "title": "共享文件 AFS：跨 workspace 挂载同一批文件",
    "intro": "AFS 解决的是“Agent A 把文件交给 Agent B，B 像读本地文件一样读取”的问题。它不是 workspace /workspace 持久化，而是 controller 颁发 capability、CP 记录 share 关系、sidecar 挂载共享存储的链路。",
    "quickLinks": [
      {
        "label": "数据库结构：afs_shares",
        "moduleId": "schema",
        "domain": "afs",
        "table": "afs_shares"
      },
      {
        "label": "数据库结构：afs_share_members",
        "moduleId": "schema",
        "domain": "afs",
        "table": "afs_share_members"
      },
      {
        "label": "Controller dirs",
        "moduleId": "schema",
        "domain": "afs",
        "table": "afs_controller_dirs"
      },
      {
        "label": "记忆挂载表",
        "moduleId": "schema",
        "domain": "memory",
        "table": "workspace_memory_attachments"
      },
      {
        "label": "记忆内容表",
        "moduleId": "schema",
        "domain": "memory",
        "table": "memories"
      }
    ],
    "steps": [
      {
        "title": "AFS 角色、抽象和表：先分清服务和数据模型",
        "actor": "human observer",
        "request": "read AFS architecture",
        "url": "file://D:/Code/koala/agent-platform/docs/wiki/afs/README.md + file://D:/Code/koala/afs",
        "params": {
          "afs_controller": "独立 Deployment / gRPC :9100 / 持有 controller.db(SQLite)",
          "afs_fuse_sidecar": "每个 workspace pod 一个 sidecar / gRPC :9101 / 负责 FUSE mount",
          "afs_storage": "共享存储后端 / local 模式是 RWX PVC 挂到 sidecar 的 /data/afs",
          "control_plane_runner": "业务编排者 / CP 直连 gRPC 或 BYOI runner 代为调用",
          "controller_filesystem": "controller SQLite 的 filesystems 行：name=shared, fs_type=local, config.base_path=/data/afs",
          "controller_dir": "controller SQLite 的 dirs 行：id + access_key + fs_name，表示一把可挂载的钥匙",
          "controller_session_stream": "sidecar 到 controller 的长连接，用 stream_id 标识这个 sidecar 连接",
          "controller_session": "某次 mount 成功后的会话，记录 session_id/dir_id/stream_id/mountpoint"
        },
        "paramNotes": [
          "afs_controller 是独立服务，不在 workspace pod 里；它负责发 dir/access_key、验 token、记录 session、撤销时推 ForceUnmount。",
          "afs_fuse_sidecar 是每个 workspace pod 内的服务；CP/runner 调它的 FuseService.Mount，它再找 controller ValidateToken，然后把 storage 挂成 /mnt/afs/<name>。",
          "afs_storage 不是进程，是文件字节所在的存储后端。local 模式下它就是 sidecar 容器里看到的 /data/afs，通常来自 RWX PVC。",
          "controller_filesystem 不是文件夹，也不是 share。它是 controller 里注册的“后端存储配置别名”：shared -> local -> /data/afs。",
          "controller_dir 不是人类看到的 share name。它是 capability：id + access_key + fs_name。sidecar 用 id/access_key 通过验证后，才知道应该用哪个 filesystem。",
          "stream_id 表示某个 afs-fuse sidecar 到 controller 的 SessionStream 长连接；同一个 sidecar 上多个 mount 会共用同一个 stream_id。",
          "session_id 表示一次具体挂载：某个 dir_id 挂在某个 mountpoint。controller revoke 时按 dir_id 找 sessions，再通过 stream_id 找到对应 sidecar 长连接推 ForceUnmount。"
        ],
        "response": {
          "mental_model": "AFS = CP 维护产品共享关系 + controller 维护钥匙/会话 + sidecar 把共享 storage 以 FUSE 形式挂进 workspace pod",
          "not_a_role": "afs_controller_filesystems / dirs / sessions 是 controller.db 里的表或抽象，不是独立服务"
        },
        "implementation": [
          "先分清服务：afs-controller 是独立 Deployment/gRPC 服务；afs-fuse 是每个 workspace pod 内的 sidecar；afs_storage 是共享存储后端/PVC，不是独立业务服务；control-plane/runner 是调用这些 gRPC 的编排者。",
          "再分清 controller.db 表：filesystems 是“存储后端配置注册表”，dirs 是“可挂载钥匙”，sessions 是“当前有哪些 sidecar 正挂着某个 dir”。这些表不是独立服务。",
          "afs_controller_filesystems 的作用：把一个名字 shared 绑定到 fs_type=local 和 config.base_path=/data/afs。它通常由 CP/runner 在创建 share 前调用 RegisterFs 初始化。",
          "CreateDir(fs_name=shared) 会先确认 filesystems 里存在 shared，然后创建 dirs 行：id/access_key/permission/fs_name。这个 dir 仍然不是物理目录名，而是一把 capability。",
          "sidecar Mount(id, access_key, mountpoint) 时，先找 controller ValidateToken。controller JOIN dirs + filesystems，返回 fs_type 和 fs_config。sidecar 才知道要创建 LocalStorage(/data/afs)。",
          "afs_storage 与 filesystem 的关联不是“表外键到某个服务”，而是 sidecar 根据 filesystems.config 创建 StorageBackend。local 模式下真实路径是 /data/afs/{dir_id}/相对文件路径。",
          "stream_id 是 sidecar 到 controller 的 SessionStream 长连接身份；session_id 是一次具体 mount 的登记。controller revoke 时按 dir_id 找 sessions，再通过 stream_id 找到连接推 ForceUnmount。"
        ],
        "db": [
          {
            "table": "afs_controller_filesystems",
            "op": "REGISTER FS",
            "values": {
              "name": "shared",
              "fs_type": "local",
              "config": {
                "base_path": "/data/afs"
              },
              "meaning": "后端存储配置别名，不是 share，不是目录"
            }
          },
          {
            "table": "afs_controller_dirs",
            "op": "CREATE DIR CAPABILITY",
            "values": {
              "id": "8f3a-c91",
              "access_key": "ak_live_xxx",
              "fs_name": "shared",
              "status": "active",
              "meaning": "指向 shared filesystem 的一把钥匙"
            }
          },
          {
            "table": "afs_controller_sessions",
            "op": "REGISTER MOUNT SESSION",
            "values": {
              "session_id": "sess_001",
              "dir_id": "8f3a-c91",
              "stream_id": "stream_fuse_ws_b",
              "mountpoint": "/mnt/afs/req",
              "meaning": "某个 sidecar 当前挂着这个 dir"
            }
          },
          {
            "table": "afs_shares",
            "op": "CP PRODUCT STATE",
            "values": {
              "id": "shr_req_001",
              "owner_workspace_id": "ws_a",
              "name": "req",
              "afs_dir_id": "8f3a-c91",
              "meaning": "CP 侧人类可读 share 与 controller dir 的映射"
            }
          }
        ],
        "note": "抓手：controller.filesystems 告诉 sidecar“数据根在哪”；controller.dirs 是“谁有钥匙”；controller.sessions 是“谁正在挂”；CP 的 afs_shares/afs_share_members 才是产品层的“谁共享给谁”。",
        "codeBlocks": [
          {
            "title": "源码 1：controller 是独立 gRPC 服务，启动时只打开 SQLite schema",
            "code": "// D:/Code/koala/afs/afs-controller/src/main.rs\nlet db = Arc::new(Database::open(&config.storage.db_path)?);\nlet service = ControllerServiceImpl::new(db);\nServer::builder()\n    .add_service(ControllerServiceServer::new(service))\n    .serve_with_shutdown(addr, shutdown_signal())\n    .await?;\n\n// afs-controller/src/db.rs\nCREATE TABLE IF NOT EXISTS filesystems (\n    name TEXT PRIMARY KEY,\n    fs_type TEXT NOT NULL,\n    config TEXT NOT NULL,\n    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP\n);\nCREATE TABLE IF NOT EXISTS dirs (\n    id TEXT PRIMARY KEY,\n    access_key TEXT NOT NULL,\n    permission TEXT NOT NULL DEFAULT 'READ_WRITE',\n    fs_name TEXT NOT NULL REFERENCES filesystems(name),\n    status TEXT NOT NULL DEFAULT 'active'\n);\nCREATE TABLE IF NOT EXISTS sessions (\n    session_id TEXT PRIMARY KEY,\n    dir_id TEXT NOT NULL REFERENCES dirs(id),\n    stream_id TEXT NOT NULL,\n    mountpoint TEXT NOT NULL\n);\n\n解释：controller 进程启动时创建三张 SQLite 表，但不会自动插入 shared。shared 是后续 RegisterFs 写进去的。"
          },
          {
            "title": "源码 2：filesystem 如何初始化：CP/runner 调 RegisterFs",
            "code": "// afs-controller/src/service.rs\nasync fn register_fs(&self, request: Request<RegisterFsRequest>) -> Result<Response<RegisterFsResponse>, Status> {\n    let req = request.into_inner();\n    let config_json = serde_json::to_string(&req.config)?;\n    self.db.register_fs(&req.name, &req.fs_type, &config_json)?;\n    Ok(Response::new(RegisterFsResponse { name: req.name }))\n}\n\n// afs-controller/src/db.rs\npub fn register_fs(&self, name: &str, fs_type: &str, config: &str) -> Result<()> {\n    conn.execute(\n      \"INSERT INTO filesystems (name, fs_type, config) VALUES (?1, ?2, ?3)\",\n      params![name, fs_type, config],\n    )?;\n}\n\n示例写入：\nfilesystems:\n  name    = \"shared\"\n  fs_type = \"local\"\n  config  = { \"base_path\": \"/data/afs\" }\n\n解释：afs_controller_filesystems 的概念是“后端存储配置注册表”。它告诉 sidecar：如果某个 dir.fs_name=shared，就用 local backend，并把 /data/afs 当根目录。"
          },
          {
            "title": "源码 3：dir 如何关联 filesystem，ValidateToken 如何返回 storage 配置",
            "code": "// afs-controller/src/db.rs\npub fn create_dir(&self, fs_name: &str) -> Result<DirRecord> {\n    if self.get_fs(fs_name)?.is_none() {\n        bail!(\"filesystem not found\")\n    }\n    let id = generate_hex_id();\n    let access_key = generate_hex_id();\n    INSERT INTO dirs (id, access_key, permission, fs_name)\n      VALUES (id, access_key, 'READ_WRITE', fs_name)\n}\n\n// afs-controller/src/db.rs\nSELECT d.id, d.access_key, d.permission, d.fs_name, d.status,\n       f.name, f.fs_type, f.config\nFROM dirs d\nJOIN filesystems f ON d.fs_name = f.name\nWHERE d.id = ?1 AND d.access_key = ?2 AND d.status = 'active'\n\n// afs-controller/src/service.rs\nValidateTokenResponse {\n  valid: true,\n  permission: dir.permission,\n  fs_type: fs.fs_type,\n  fs_config: fs.config,\n}\n\n解释：dirs.fs_name 是连接点。sidecar 只传 id/access_key；controller 通过 dirs 找到 fs_name，再 join filesystems，把 fs_type/local 和 base_path=/data/afs 返回给 sidecar。"
          },
          {
            "title": "源码 4：afs_storage 如何和 filesystem 关联：sidecar 用 fs_config 创建 backend",
            "code": "// afs-fuse/src/service.rs\nlet validation = self\n    .validate_with_controller(&req.id, &req.access_key, &req.controller_addr)\n    .await?;\nlet backend = Self::create_backend(&validation.fs_type, &validation.fs_config)?;\n\nfn create_backend(fs_type: &str, config: &HashMap<String, String>) -> Result<Arc<dyn StorageBackend>, Status> {\n    match fs_type {\n        \"local\" => {\n            let base_path = config.get(\"base_path\")?;\n            Ok(Arc::new(LocalStorage::new(PathBuf::from(base_path))))\n        }\n        \"nfs\" => { ... }\n    }\n}\n\n// afs-storage/src/local.rs\nfn dir_root(&self, id: &str) -> PathBuf {\n    resolve_dir_path(&self.base_path, id)\n}\nfn full_path(&self, id: &str, path: &Path) -> PathBuf {\n    self.dir_root(id).join(path)\n}\n\n例子：\n  filesystem.config.base_path = /data/afs\n  dir.id = 8f3a-c91\n  agent 读 /mnt/afs/req/spec.md\n  local storage 实际读 /data/afs/8f3a-c91/spec.md\n\n解释：afs_storage 不是通过数据库外键关联的服务。它通过 filesystems.config 里的 base_path 被 sidecar 创建成 LocalStorage。"
          },
          {
            "title": "源码 5：session 和 stream 是什么",
            "code": "// afs-fuse/src/service.rs\npub struct FuseServiceImpl {\n    controller_addr: String,\n    stream_id: String,\n    mounts: Arc<Mutex<HashMap<String, ActiveMount>>>,\n}\n\npub fn new(controller_addr: String) -> Self {\n    let stream_id = generate_stream_id();\n    Self { controller_addr, stream_id, mounts: ... }\n}\n\n// afs-fuse/src/main.rs\nsession_stream_loop(controller_addr, service)\n  -> 每 10 秒发送 Heartbeat { stream_id, mounts: [{ dir_id, mountpoint }] }\n  -> 同时读取 controller 推来的 SessionCommand::ForceUnmount\n\n// afs-fuse/src/service.rs\nregister_session_with_controller(dir_id, mountpoint) {\n  RegisterSessionRequest {\n    dir_id,\n    mountpoint,\n    stream_id: self.stream_id,\n  }\n}\n\n// afs-controller/src/service.rs\nregister_session(req) -> INSERT sessions(session_id, dir_id, stream_id, mountpoint)\n\n解释：\n  stream_id = sidecar 与 controller 的长连接 ID，用来找“往哪个连接推命令”。\n  session_id = 某次 mount 的登记 ID，用来记录“哪个 dir 挂到了哪个 mountpoint”。\n  一个 sidecar 通常一个 stream_id；这个 sidecar 上可以有多个 session。"
          }
        ]
      },
      {
        "title": "Pod 如何挂载 AFS：先看 manifest，再看宿主机和容器目录",
        "actor": "Kubernetes -> workspace Pod -> afs-fuse sidecar",
        "request": "Pod manifest volumes + sidecar FuseService.Mount",
        "url": "k8s://agent-pods/ws-a,ws-b,ws-c + gRPC afs-fuse:9101/FuseService.Mount",
        "params": {
          "example": "ws-a 和 ws-b 调度到 node-a；ws-c 调度到 node-b；三者都挂载 share=req",
          "share_name": "req",
          "dir_id": "8f3a-c91",
          "agent_visible_path": "/mnt/afs/req",
          "sidecar_storage_root": "/data/afs",
          "real_storage_path": "/data/afs/8f3a-c91"
        },
        "paramNotes": [
          "agent_visible_path：agent 容器里看到的路径。agent 只读写 /mnt/afs/req，不直接碰 /data/afs。",
          "sidecar_storage_root：sidecar 容器里挂载的共享存储根目录，来自 RWX PVC 或 NFS。",
          "real_storage_path：local backend 用 filesystems.config.base_path + dir_id 拼出的真实目录。",
          "同一个 share 的关键不是路径名相同，而是三个 sidecar 都拿到同一个 dir_id/access_key。"
        ],
        "response": {
          "result": "三个 agent 容器都看到 /mnt/afs/req；三个 sidecar 都把它映射到同一个 storage 目录 /data/afs/8f3a-c91。",
          "important": "A/B/C 各自有独立 FUSE mount，但背后的文件内容不是三份副本。"
        },
        "codeBlocks": [
          {
            "title": "Kubernetes manifest 关键片段：agent 看 /mnt/afs，sidecar 负责 FUSE 和 storage",
            "code": "apiVersion: v1\nkind: Pod\nmetadata:\n  name: ws-a-agent\nspec:\n  nodeName: node-a\n  containers:\n    - name: agent\n      image: agent-platform/codex-agent:latest\n      env:\n        - name: WORKSPACE_ID\n          value: ws_a\n      volumeMounts:\n        # agent 只需要看到 FUSE 挂载后的入口。\n        - name: afs-mount-root\n          mountPath: /mnt/afs\n          mountPropagation: HostToContainer\n\n    - name: afs-fuse\n      image: koala/afs-fuse:latest\n      securityContext:\n        privileged: true\n      env:\n        - name: CONTROLLER_ADDR\n          value: afs-controller:9100\n        - name: AFS_BOOTSTRAP_URL\n          value: http://control-plane/_cp/workspaces/ws_a/afs-mounts\n      volumeMounts:\n        # sidecar 在这里创建 /mnt/afs/req 这个 FUSE mountpoint。\n        - name: afs-mount-root\n          mountPath: /mnt/afs\n          mountPropagation: Bidirectional\n\n        # sidecar 的 local StorageBackend 真实读写这里。\n        - name: afs-storage\n          mountPath: /data/afs\n\n  volumes:\n    - name: afs-mount-root\n      hostPath:\n        path: /var/lib/agent-platform/afs-mounts/ws_a\n        type: DirectoryOrCreate\n\n    - name: afs-storage\n      persistentVolumeClaim:\n        claimName: afs-rwx-pvc"
          },
          {
            "title": "宿主机目录示例：A/B 在 node-a，C 在 node-b",
            "code": "前提：\n- ws-a、ws-b 在宿主机 node-a\n- ws-c 在宿主机 node-b\n- 三个 workspace 都被授权挂载同一个 share=req\n- share=req 对应同一个 controller dirs.id：8f3a-c91\n- afs-storage 是 RWX PVC/NFS，所以 node-a 和 node-b 都能挂到同一份后端存储\n\nnode-a 上的目录视角：\n/var/lib/agent-platform/afs-mounts/\n├── ws_a/                         # manifest: ws-a volumes.afs-mount-root.hostPath.path\n│   └── req/                      # sidecar A 创建的 FUSE mountpoint，不是文件正文目录\n└── ws_b/                         # manifest: ws-b volumes.afs-mount-root.hostPath.path\n    └── req/                      # sidecar B 创建的 FUSE mountpoint，不是文件正文目录\n\n/var/lib/kubelet/pods/<pod-a>/volumes/.../afs-rwx-pvc/  -> 同一个 PV\n/var/lib/kubelet/pods/<pod-b>/volumes/.../afs-rwx-pvc/  -> 同一个 PV\n└── 8f3a-c91/\n    └── spec.md                   # 文件正文实际在共享 storage 的 dir_id 目录下\n\nnode-b 上的目录视角：\n/var/lib/agent-platform/afs-mounts/\n└── ws_c/                         # manifest: ws-c volumes.afs-mount-root.hostPath.path\n    └── req/                      # sidecar C 创建的 FUSE mountpoint\n\n/var/lib/kubelet/pods/<pod-c>/volumes/.../afs-rwx-pvc/  -> 同一个 PV\n└── 8f3a-c91/\n    └── spec.md                   # 和 node-a 看到的是同一份后端内容\n\n人话理解：\n- /var/lib/agent-platform/afs-mounts/ws_*/req 是“入口”，每个 pod 一份入口。\n- /data/afs/8f3a-c91 是“内容目录”，由共享 PVC/NFS 提供。\n- A/B/C 的入口不同，但入口后面都转到同一个 dir_id 的内容目录。"
          },
          {
            "title": "容器内目录示例：agent 和 sidecar 看到的不是同一层职责",
            "code": "ws-a pod 内：\nagent 容器：\n/mnt/afs/\n└── req/\n    └── spec.md                   # agent 像本地文件一样读写这里\n\nsidecar 容器：\n/mnt/afs/\n└── req/                          # FUSE mountpoint，由 afs-fuse 进程接管\n/data/afs/\n└── 8f3a-c91/\n    └── spec.md                   # LocalStorage 真实读写这里\n\nws-b pod 内：\nagent 容器：\n/mnt/afs/req/spec.md              # 路径看起来一样\nsidecar 容器：\n/data/afs/8f3a-c91/spec.md        # dir_id 一样，所以读到 A 写入的内容\n\nws-c pod 内：\nagent 容器：\n/mnt/afs/req/spec.md              # 即使在 node-b，路径也一样\nsidecar 容器：\n/data/afs/8f3a-c91/spec.md        # 后端 PV/NFS 是同一份，所以内容也一样"
          }
        ],
        "implementation": [
          "Kubernetes 先把 afs-mount-root 挂进 agent 和 sidecar。sidecar 用 Bidirectional mountPropagation 创建 FUSE mount 后，agent 容器才能在同一个 volume 里看到 /mnt/afs/req。",
          "Kubernetes 还把 afs-storage PVC 只挂给 sidecar 的 /data/afs。agent 不需要知道真实 storage 根目录。",
          "CP/runner 调 sidecar 的 Mount(id=8f3a-c91, access_key, mountpoint=/mnt/afs/req)。sidecar 验证后把 /mnt/afs/req 挂成 FUSE。",
          "sidecar 的 LocalStorage(base_path=/data/afs) 会把 share 内相对路径 spec.md 翻译成 /data/afs/8f3a-c91/spec.md。",
          "A/B/C 是否在同一台宿主机不改变语义。真正决定共享的是同一个 dir_id 和同一个后端 storage。"
        ],
        "db": [
          {
            "table": "afs_controller_filesystems",
            "op": "READ storage config",
            "values": {
              "name": "shared",
              "fs_type": "local",
              "config": {
                "base_path": "/data/afs"
              }
            }
          },
          {
            "table": "afs_controller_dirs",
            "op": "same dir capability",
            "values": {
              "id": "8f3a-c91",
              "fs_name": "shared",
              "status": "active"
            }
          },
          {
            "table": "afs_controller_sessions",
            "op": "one session per mounted pod",
            "values": {
              "stream_id": "stream_fuse_ws_a/ws_b/ws_c",
              "mountpoint": "/mnt/afs/req"
            }
          }
        ],
        "note": "先记住两层路径：/mnt/afs/req 是 agent 入口；/data/afs/8f3a-c91 是 sidecar storage backend 的内容目录。"
      },
      {
        "title": "创建 share_folder：CP 拿钥匙并给 owner 挂载",
        "actor": "Agent A MCP -> control-plane -> afs-controller -> afs-fuse A",
        "request": "POST /:workspaceId/afs/shares { name: \"req\" }",
        "url": "POST http://control-plane/api/workspaces/ws_a/afs/shares",
        "params": {
          "workspace_id": "ws_a",
          "name": "req",
          "fs_name": "shared",
          "mountpoint": "/mnt/afs/req"
        },
        "paramNotes": [
          "workspace_id：创建者 workspace。",
          "name：CP 侧 share 名；controller 不接收这个字段。",
          "fs_name：controller filesystems.name，默认 shared。",
          "mountpoint：CP 把 name 拼成 /mnt/afs/{name} 后交给 sidecar。"
        ],
        "response": {
          "status": 201,
          "body": {
            "id": "shr_req_001",
            "name": "req",
            "afs_dir_id": "8f3a-c91",
            "mounted": true
          }
        },
        "codeBlocks": [
          {
            "title": "源码：controller CreateDir 只收 fs_name",
            "code": "// D:/Code/koala/afs/proto/afs.proto\nmessage CreateDirRequest { string fs_name = 1; }\nmessage CreateDirResponse { string id = 1; string access_key = 2; Permission permission = 3; }\n\n// afs-controller/src/service.rs\nasync fn create_dir(&self, request: Request<CreateDirRequest>) -> Result<Response<CreateDirResponse>, Status> {\n    let req = request.into_inner();\n    let record = self.db.create_dir(&req.fs_name)?;\n    Ok(Response::new(CreateDirResponse { id: record.id, access_key: record.access_key, permission: ... }))\n}\n\n解释：CreateDir 不创建名叫 req 的目录。它创建的是 controller 侧 capability 记录，返回 id/access_key。"
          }
        ],
        "implementation": [
          "CP 先查 afs_shares 是否已有 owner+name，命中则幂等返回。",
          "未命中时 CP 调 RegisterFs(shared, local, base_path=/data/afs)，再调 CreateDir(shared)。",
          "controller 返回 id/access_key 后，CP 写 afs_shares，并给 owner 写 afs_share_members(read_write)。",
          "最后 CP 调 owner workspace 的 FuseService.Mount，把 id/access_key/mountpoint 交给 sidecar。"
        ],
        "db": [
          {
            "table": "afs_controller_filesystems",
            "op": "INSERT if absent",
            "values": {
              "name": "shared",
              "fs_type": "local",
              "config": {
                "base_path": "/data/afs"
              }
            }
          },
          {
            "table": "afs_controller_dirs",
            "op": "INSERT",
            "values": {
              "id": "8f3a-c91",
              "fs_name": "shared",
              "status": "active"
            }
          },
          {
            "table": "afs_shares",
            "op": "INSERT",
            "values": {
              "id": "shr_req_001",
              "owner_workspace_id": "ws_a",
              "name": "req",
              "afs_dir_id": "8f3a-c91"
            }
          },
          {
            "table": "afs_share_members",
            "op": "INSERT owner",
            "values": {
              "share_id": "shr_req_001",
              "workspace_id": "ws_a",
              "permission": "read_write"
            }
          }
        ],
        "note": "创建 share 后 owner 自己也要 mount 一次，否则只有数据库记录，pod 里不会自动出现 /mnt/afs/req。"
      },
      {
        "title": "sidecar Mount：先 ValidateToken，再挂 FUSE",
        "actor": "control-plane/runner -> afs-fuse sidecar -> afs-controller",
        "request": "FuseService.Mount",
        "url": "gRPC tos-ws-a:9101/FuseService.Mount",
        "params": {
          "id": "8f3a-c91",
          "access_key": "ak_live_xxx",
          "mountpoint": "/mnt/afs/req",
          "controller_addr": "afs-controller:9100",
          "readonly": false
        },
        "paramNotes": [
          "id/access_key：controller 颁发的钥匙。",
          "mountpoint：目标 pod 内路径。",
          "controller_addr：只用于验 token 和注册 session，不是文件存储地址。",
          "readonly：请求侧降权开关；true 时内核层拒绝写。"
        ],
        "response": {
          "status": "OK",
          "body": {
            "mountpoint": "/mnt/afs/req"
          }
        },
        "codeBlocks": [
          {
            "title": "源码：Mount 的真实动作",
            "code": "// afs-fuse/src/service.rs\nlet validation = self.validate_with_controller(&req.id, &req.access_key, &req.controller_addr).await?;\nlet permission = if req.readonly || validation.permission == Permission::ReadOnly as i32 {\n    MountPermission::ReadOnly\n} else {\n    MountPermission::ReadWrite\n};\nlet backend = Self::create_backend(&validation.fs_type, &validation.fs_config)?;\nif !backend.dir_exists(&req.id).await? { backend.init_dir(&req.id).await?; }\ntokio::fs::create_dir_all(&req.mountpoint).await?;\nlet fs = AfsFilesystem::new(backend, req.id.clone(), permission, handle);\nlet session = fuser::spawn_mount2(fs, &req.mountpoint, &config)?;\nlet session_id = self.register_session_with_controller(&req.id, &req.mountpoint).await.unwrap_or_default();\n\n解释：sidecar 验证通过后才创建 backend 和 FUSE mount。local backend 的真实目录是 /data/afs/{dir_id}。"
          }
        ],
        "implementation": [
          "sidecar 收到 Mount 后先检查 mountpoint 是否已经挂过，重复挂会 already_exists。",
          "ValidateToken 查询 controller SQLite 的 dirs + filesystems，返回 permission/fs_type/fs_config。",
          "local 模式下 create_backend 得到 LocalStorage(base_path=/data/afs)。",
          "真实物理目录是 /data/afs/{dir_id}，pod 内暴露路径是 /mnt/afs/{name}。",
          "挂载成功后 sidecar RegisterSession，controller 记录 session_id/stream_id/mountpoint。"
        ],
        "db": [
          {
            "table": "afs_controller_dirs",
            "op": "SELECT validate token",
            "values": {
              "id": "8f3a-c91",
              "access_key": "ak_live_xxx",
              "status": "active"
            }
          },
          {
            "table": "afs_controller_sessions",
            "op": "INSERT",
            "values": {
              "dir_id": "8f3a-c91",
              "mountpoint": "/mnt/afs/req",
              "stream_id": "stream_fuse_a"
            }
          }
        ],
        "note": "AFS 文件读写不是每次经过 controller；controller 只负责控制权。真正 IO 走 sidecar 的 storage backend。"
      },
      {
        "title": "授权给 Agent B：同一把钥匙挂到另一个 workspace",
        "actor": "Agent A MCP -> control-plane -> afs-fuse B",
        "request": "POST /:workspaceId/afs/shares/:shareId/members",
        "url": "POST http://control-plane/api/workspaces/ws_a/afs/shares/shr_req_001/members",
        "params": {
          "target_workspace_id": "ws_b",
          "readonly": true,
          "dir_id": "8f3a-c91",
          "access_key": "ak_live_xxx",
          "mountpoint": "/mnt/afs/req"
        },
        "paramNotes": [
          "target_workspace_id：被授权的 workspace。",
          "readonly：true 表示 B 只能读。",
          "dir_id/access_key：来自 afs_shares，CP 把同一把钥匙交给 B 的 sidecar。",
          "mountpoint：仍然由 share name 拼出来。"
        ],
        "response": {
          "status": 200,
          "body": {
            "share_id": "shr_req_001",
            "workspace_id": "ws_b",
            "permission": "read_only",
            "mounted": true
          }
        },
        "implementation": [
          "CP 校验目标 workspace 存在、同用户、同环境。跨用户或跨环境不能挂。",
          "如果成员已存在，CP 通常先 unmount 再 mount，用于刷新 readonly/read_write 权限。",
          "Mount 成功后写 afs_share_members。B 的 pod 里出现 /mnt/afs/req。"
        ],
        "db": [
          {
            "table": "afs_share_members",
            "op": "INSERT/UPSERT target",
            "values": {
              "share_id": "shr_req_001",
              "workspace_id": "ws_b",
              "permission": "read_only"
            }
          },
          {
            "table": "afs_controller_sessions",
            "op": "INSERT B mount session",
            "values": {
              "dir_id": "8f3a-c91",
              "mountpoint": "/mnt/afs/req",
              "stream_id": "stream_fuse_b"
            }
          }
        ],
        "note": "B 看到的名字可以和 A 一样是 /mnt/afs/req，但真正把两边连起来的是相同的 dir_id。"
      },
      {
        "title": "读写共享文件：A 写入后，B/C 如何读到",
        "actor": "Agent A/B/C -> Linux VFS/FUSE -> afs-fuse -> StorageBackend",
        "request": "write/read /mnt/afs/req/spec.md",
        "url": "file:///mnt/afs/req/spec.md -> local:///data/afs/8f3a-c91/spec.md",
        "params": {
          "writer": "ws-a agent: echo hello > /mnt/afs/req/spec.md",
          "readers": "ws-b/ws-c agent: cat /mnt/afs/req/spec.md",
          "dir_id": "8f3a-c91",
          "relative_path": "spec.md",
          "storage_path": "/data/afs/8f3a-c91/spec.md",
          "permission": "ws-a read_write；ws-b/ws-c 如果 read_only，则只能读不能写"
        },
        "paramNotes": [
          "writer：写操作由发起写入的 agent 进程触发，但真正落盘的是该 pod 内的 afs-fuse sidecar。",
          "readers：其他 pod 不接收 controller 推送；它们下次 read/readdir 时，自己的 sidecar 再去同一个 storage_path 读取。",
          "relative_path：FUSE 根据 inode 找到 share 内相对路径，例如 spec.md。",
          "storage_path：LocalStorage 用 base_path + dir_id + relative_path 拼出来。",
          "permission：只读挂载会在 FUSE write/create 阶段直接返回 EACCES，不会写 storage。"
        ],
        "response": {
          "write_ws_a": "OK，sidecar A 写入 /data/afs/8f3a-c91/spec.md",
          "read_ws_b": "OK，sidecar B 从同一个 /data/afs/8f3a-c91/spec.md 读取最新文件内容",
          "read_ws_c": "OK，即使 C 在 node-b，只要 /data/afs 来自同一个 RWX PVC/NFS，也读取同一份内容",
          "write_readonly_member": "EACCES，FUSE 层拒绝写入"
        },
        "codeBlocks": [
          {
            "title": "源码：FUSE read 每次调用 backend.read_file",
            "code": "// D:/Code/koala/afs/afs-fuse/src/filesystem.rs:282\nfn read(&self, _req: &Request, ino: INodeNo, _fh: FileHandle, offset: u64, size: u32, ..., reply: ReplyData) {\n    let inodes = self.inodes.lock().unwrap();\n    let path = match inodes.get_path(ino) { Some(p) => p.clone(), None => ENOENT };\n    drop(inodes);\n\n    match self.handle.block_on(self.backend.read_file(&self.dir_id, &path)) {\n        Ok(data) => reply.data(&data[offset..end]),\n        Err(_) => reply.error(Errno::EIO),\n    }\n}\n\n解释：B/C 读 /mnt/afs/req/spec.md 时，不是问 controller，也不是读 CP 数据库。\n它们各自的 sidecar 根据 inode 找到 spec.md，然后调用 backend.read_file(dir_id=8f3a-c91, path=spec.md)。"
          },
          {
            "title": "源码：FUSE write 由当前 pod 的 sidecar 写入 backend",
            "code": "// D:/Code/koala/afs/afs-fuse/src/filesystem.rs:322\nfn write(&self, _req: &Request, ino: INodeNo, _fh: FileHandle, offset: u64, data: &[u8], ..., reply: ReplyWrite) {\n    if self.is_readonly() {\n        reply.error(Errno::EACCES);\n        return;\n    }\n\n    let path = inodes.get_path(ino)?;\n    let existing = self.handle\n        .block_on(self.backend.read_file(&self.dir_id, &path))\n        .unwrap_or_default();\n\n    let mut new_data = existing;\n    new_data[offset..offset + data.len()].copy_from_slice(data);\n\n    match self.handle.block_on(self.backend.write_file(&self.dir_id, &path, &new_data)) {\n        Ok(()) => reply.written(data.len() as u32),\n        Err(_) => reply.error(Errno::EIO),\n    }\n}\n\n解释：谁负责写？不是 controller，也不是 CP。\n写入由“发起写操作所在 pod 的 afs-fuse sidecar”负责。A 写文件，就是 sidecar A 写 storage。"
          },
          {
            "title": "源码：create 和 LocalStorage 的真实落点",
            "code": "// D:/Code/koala/afs/afs-fuse/src/filesystem.rs:480\nfn create(&self, _req: &Request, parent: INodeNo, name: &OsStr, ..., reply: ReplyCreate) {\n    if self.is_readonly() { EACCES }\n    let parent_path = inodes.get_path(parent)?;\n    let child_path = parent_path.join(name);\n    self.handle.block_on(self.backend.write_file(&self.dir_id, &child_path, &[]))?;\n}\n\n// D:/Code/koala/afs/afs-storage/src/local.rs:18\nfn dir_root(&self, id: &str) -> PathBuf {\n    resolve_dir_path(&self.base_path, id)\n}\nfn full_path(&self, id: &str, path: &Path) -> PathBuf {\n    self.dir_root(id).join(path)\n}\nasync fn read_file(&self, id: &str, path: &Path) -> Result<Vec<u8>> {\n    tokio::fs::read(self.full_path(id, path)).await\n}\nasync fn write_file(&self, id: &str, path: &Path, data: &[u8]) -> Result<()> {\n    tokio::fs::write(self.full_path(id, path), data).await\n}\n\n路径翻译：\n/mnt/afs/req/spec.md\n  -> FUSE mountpoint /mnt/afs/req\n  -> relative_path spec.md\n  -> dir_id 8f3a-c91\n  -> /data/afs/8f3a-c91/spec.md"
          },
          {
            "title": "A/B/C 读写观察脚本：从人类视角看变化",
            "code": "# ws-a agent 容器内\n$ echo \"hello from A\" > /mnt/afs/req/spec.md\n\n# ws-a sidecar 容器内，对应真实 storage\n$ ls /data/afs/8f3a-c91\nspec.md\n$ cat /data/afs/8f3a-c91/spec.md\nhello from A\n\n# ws-b agent 容器内，B 在 node-a\n$ cat /mnt/afs/req/spec.md\nhello from A\n\n# ws-c agent 容器内，C 在 node-b\n$ cat /mnt/afs/req/spec.md\nhello from A\n\n# 如果 ws-b 是 read_only\n$ echo \"edit from B\" >> /mnt/afs/req/spec.md\nPermission denied\n\n结论：\n- A 写入后，内容进入共享 storage 的 /data/afs/8f3a-c91/spec.md。\n- B/C 感知变化的方式是“下一次读/列目录时读同一个 storage”，不是 controller 发文件变更事件。\n- 同节点或跨节点都一样，前提是 /data/afs 背后是同一个 RWX PVC/NFS。"
          }
        ],
        "implementation": [
          "写入路径：Agent A 调用普通文件写入 -> Linux VFS 判断 /mnt/afs/req 是 FUSE -> 请求进入 sidecar A 的 AfsFilesystem.write -> sidecar A 调 backend.write_file(dir_id, relative_path, bytes) -> LocalStorage 写 /data/afs/8f3a-c91/spec.md。",
          "读取路径：Agent B/C 调用普通文件读取 -> 请求进入各自 pod 的 sidecar B/C -> backend.read_file(dir_id=8f3a-c91, path=spec.md) -> 从同一个共享 storage 目录读取。",
          "变化感知：AFS 这里没有通过 controller 给其他 pod 推“文件已变化”事件。其他 pod 是在下一次 read/readdir/getattr 时重新走自己的 FUSE/backend 调用，因此看到共享 storage 当前内容。",
          "数据库变化：普通 read/write/create 文件时，afs_shares、afs_share_members、controller dirs 通常不变。controller 只管 capability、session、revoke，不管每次文件 IO。",
          "一致性边界：如果两个 read_write pod 同时写同一个文件，当前解释链路里没有展示分布式锁或 CAS；最后内容取决于底层文件系统和写入时序。"
        ],
        "db": [
          {
            "table": "afs_shares",
            "op": "UNCHANGED during IO",
            "values": {
              "id": "shr_req_001",
              "afs_dir_id": "8f3a-c91",
              "name": "req"
            }
          },
          {
            "table": "afs_controller_dirs",
            "op": "UNCHANGED during IO",
            "values": {
              "id": "8f3a-c91",
              "fs_name": "shared",
              "status": "active"
            }
          },
          {
            "table": "afs_controller_filesystems",
            "op": "READ at mount time, not each IO",
            "values": {
              "name": "shared",
              "config": {
                "base_path": "/data/afs"
              }
            }
          }
        ],
        "note": "一句话：谁写文件，谁所在 pod 的 sidecar 负责写入；别人怎么知道，别人下一次通过自己的 FUSE mount 读同一个 storage 目录。"
      },
      {
        "title": "AFS vs 记忆系统：实现模型和权限边界对比",
        "summary": "AFS 和记忆系统都能用 FUSE 给 agent 展示成目录树，所以“看起来像 /mnt/.../notes/overview.md”不是本质差别。本质差别是权威数据模型：AFS 的权威内容是 storage 里的文件；Memory 的权威内容是 control-plane 数据库里的业务记录。这个差异决定了版本、索引、审计、回滚、权限校验和长期知识管理的实现成本。",
        "request": "对比观察：同样是 agent 读写 /mnt 下的文件，AFS 和 Memory 分别由谁存数据、谁校验权限、谁阻止未授权访问？",
        "params": [
          {
            "name": "AFS agent 路径",
            "value": "/mnt/afs/req/spec.md",
            "desc": "agent 容器内看到的共享文件路径。它最终落到 AFS storage 后端的某个 dir_id 目录下。"
          },
          {
            "name": "AFS storage 路径",
            "value": "/data/afs/8f3a-c91/spec.md",
            "desc": "afs-fuse sidecar 通过 controller 返回的 fs_config 找到后端目录。这个路径是示例，真实路径由 afs_controller_filesystems.config 决定。"
          },
          {
            "name": "Memory agent 路径",
            "value": "/mnt/memory/mem_project/notes/overview.md",
            "desc": "agent 容器内看到的记忆文件路径。目录和文件列表由 sidecar 的内存 snapshot 合成。"
          },
          {
            "name": "Memory 权威数据",
            "value": "memories(store_id=mem_project, path=/notes/overview.md)",
            "desc": "记忆内容不是落到宿主机共享目录，而是保存到 control-plane 的 memories/memory_versions 表。"
          },
          {
            "name": "未授权 AFS",
            "value": "没有 id/access_key 或 key 已撤销 -> Mount 失败/被 ForceUnmount",
            "desc": "AFS 用 capability 风格授权：拿不到 controller 发出的钥匙，就不能拿到 fs_config，也不能挂载真实 storage。"
          },
          {
            "name": "未授权 Memory",
            "value": "workspace_memory_attachments 不存在 -> 404；read_only 写入 -> 403",
            "desc": "Memory 每个 CP IO 都按 workspace_id + store_id 检查 attachment，服务端拒绝越权读写。"
          }
        ],
        "response": "可以理解成：两者在目录展示上可以做得很像，但不是“目录像不像”的差别，而是“谁是权威存储”的差别。AFS 的权威存储是文件系统；Memory 的权威存储是数据库。数据库让版本、索引、审计、回滚、权限和 prompt 注入成为一等业务能力。",
        "implementation": [
          "AFS 的数据模型：control-plane 的 afs_shares/afs_share_members 决定哪个 workspace 能拿到共享目录；afs-controller 的 dirs 记录 dir_id/access_key/permission/fs_name；filesystems 记录 storage 后端配置；sessions 只记录当前在线挂载。文件内容本身不在 controller 数据库里。",
          "Memory 的数据模型：memory_stores 是记忆库；workspace_memory_attachments 决定某个 workspace 是否挂载某个 store 以及 read_only/read_write；memories 存当前文件内容；memory_versions 存历史版本。文件内容本身就在 CP 数据库里。",
          "AFS 读写路径：agent 写 /mnt/afs/req/spec.md -> Linux kernel FUSE -> afs-fuse sidecar -> backend.write_file(dir_id=8f3a-c91, spec.md) -> storage。controller 不参与每次文件 read/write，只参与创建、授权、ValidateToken、session、revoke。",
          "Memory 读写路径：agent 读 /mnt/memory/mem_project/notes/overview.md -> memory-fuse snapshot/cache -> 必要时 HTTP GET CP；写入时先写入 sidecar 文件 buffer，close/fsync 时 PUT CP，并带 if_match_sha256 做 CAS。CP 成功后更新 memories 和 memory_versions。",
          "AFS 如何阻止无权限访问：CP 不把 id/access_key 发给非成员；sidecar Mount 必须拿 id/access_key 去 controller ValidateToken；controller 校验 dirs.status=active 且 access_key 匹配；只读权限会传给 FUSE，write/create 直接返回 EACCES；撤销时 controller 根据 sessions 推 ForceUnmount。",
          "Memory 如何阻止无权限访问：sidecar 启动只能拉自己 workspace 的 memory_attachments；每个 /_cp/workspaces/:wsId/memory-stores/:storeId/... 接口先 requireAttached；没有 attachment 返回 404；read_only 的 PUT/DELETE/MOVE 返回 403。注意当前 memory-fuse 的 readOnly 主要保存在 mount 管理状态里，真正不可绕过的服务端权限检查在 CP 路由。",
          "新鲜度差异：AFS 多个 pod 共享同一个 storage 后端，其他 pod 下一次 read/readdir 走 storage 就能看到变化，controller 不广播文件变化；Memory 会在写入后广播 invalidate，sidecar 清 cache/刷新 snapshot，读取内容时用 snapshot sha 校验缓存，不匹配才重新 GET CP。",
          "并发差异：AFS 依赖底层文件系统语义，当前这条路径没有看到针对文件内容的 sha CAS；Memory 每个文件写入带 if_match_sha256，别人先改了会返回 412/Precondition，sidecar 丢弃本地脏 buffer 并刷新，调用方需要重新读最新内容后合并。",
          "目录表现形式不是核心差别：AFS 完全可以把目录组织成和 Memory 类似的 /mnt/afs/{share}/notes/overview.md；Memory 也可以把数据库记录通过 FUSE 展示成 /mnt/memory/{store}/notes/overview.md。FUSE 是展示层，目录长什么样可以设计。",
          "真正的差别是权威内容存在哪里：AFS 的权威内容是文件系统里的文件；Memory 的权威内容是数据库里的 memories 行和 memory_versions 历史行。",
          "数据库不是为了“看起来像文件”，而是为了记忆业务：可以直接记录 version、actor_kind、actor_id、operation、content_sha256，可以按 store_id/path/content 建索引，也可以按 workspace_memory_attachments 做权限过滤。",
          "文件系统不是不能做版本控制、索引、审计、回滚，而是需要额外补业务层：文件变更监听、元数据表、版本快照、冲突检测、审计记录、索引任务、权限校验、缓存失效。补到最后就接近“文件系统 + 数据库业务层”。",
          "Memory 一开始选择数据库作为权威存储，是因为记忆更像长期知识资产：需要持久保存、跨 workspace/agent 生命周期复用、可检索、可审计、可回滚，并能在 agent 启动时组织成 prompt/index。",
          "AFS 不等于只能临时使用，它也可以持久保存共享项目文件、用户上传文件或协作目录。但在这个系统语义里，AFS 更偏运行期共享文件空间；Memory 更偏长期结构化知识库。",
          "一句话：AFS 关注“多个 agent 共享一批文件”；Memory 关注“把长期知识作为可版本化、可审计、可检索、可挂载的数据资产”。"
        ],
        "relatedTables": [
          "afs_shares",
          "afs_share_members",
          "afs_controller_dirs",
          "afs_controller_filesystems",
          "afs_controller_sessions",
          "memory_stores",
          "workspace_memory_attachments",
          "memories",
          "memory_versions"
        ],
        "codeBlocks": [
          {
            "title": "一句话对比：AFS 是共享 storage，Memory 是数据库虚拟文件系统",
            "language": "text",
            "code": "维度                    AFS                                   记忆系统\n权威内容存储            afs_storage / local / NFS / PVC          control-plane 数据库 memories\n控制面数据库            只存 share/member/key/session            存 store/attachment/content/version\n每次读写是否过 CP        否，读写走 sidecar -> storage             是，读写走 sidecar -> CP HTTP\n目录从哪里来            storage 真实目录                         sidecar snapshot 根据 memories.path 合成\n共享方式                多个 workspace 拿同一个 dir_id/key        多个 workspace attach 同一个 store_id\n只读控制                controller permission + FUSE EACCES       CP route: read_only 写入返回 403\n未授权读                拿不到 key，ValidateToken 失败             requireAttached 失败，返回 404\n撤销                    controller ForceUnmount                   删除 attachment 后后续 CP IO 失败"
          },
          {
            "title": "为什么不直接用 AFS 实现 Memory",
            "language": "text",
            "code": "可以这样理解：\n\n1. 目录视图可以一样\n   AFS:    /mnt/afs/project/notes/overview.md\n   Memory: /mnt/memory/mem_project/notes/overview.md\n   这只是 FUSE 展示层，不是本质差别。\n\n2. 权威数据模型不同\n   AFS 权威内容：storage 后端里的真实文件。\n   Memory 权威内容：control-plane 数据库里的 memories / memory_versions。\n\n3. 数据库更贴近“记忆”的业务需求\n   - 版本历史：memory_versions\n   - 乐观锁：if_match_sha256\n   - 审计：actor_kind / actor_id / operation\n   - 索引：按 store_id / path / metadata / content 建索引\n   - 权限：workspace_memory_attachments 每次 CP IO 校验\n   - 启动注入：可以把 store 摘要、instructions、index 写入 agent prompt\n\n4. 文件系统不是不能做这些\n   但 AFS 如果要补齐 Memory 能力，需要额外做：\n   文件变更监听、内容入库、元数据表、版本快照、冲突检测、审计日志、索引任务、权限过滤、失效通知。\n   这会把 AFS 推向“文件系统 + 一整套数据库业务层”。\n\n5. 生命周期语义也不同\n   AFS 可以持久，但更像运行期共享文件空间。\n   Memory 默认是长期知识资产，要跨 workspace/agent 生命周期复用。"
          },
          {
            "title": "AFS：Mount 前必须通过 controller ValidateToken",
            "language": "rust",
            "code": "// afs-controller/src/db.rs\n// validate_token(id, access_key) 只接受 active dir + 正确 access_key，\n// 成功后才返回 fs_type/fs_config，sidecar 才知道真实 storage 在哪。\nSELECT d.id, d.permission, d.fs_name, d.status, f.fs_type, f.config\nFROM dirs d JOIN filesystems f ON d.fs_name = f.name\nWHERE d.id = ? AND d.access_key = ? AND d.status = 'active';\n\n// afs-fuse/src/service.rs\nlet validation = controller.validate_token(...).await?;\nlet permission = if req.readonly || validation.permission == ReadOnly {\n    MountPermission::ReadOnly\n} else {\n    MountPermission::ReadWrite\n};"
          },
          {
            "title": "AFS：只读挂载在 FUSE 写路径直接拒绝",
            "language": "rust",
            "code": "// afs-fuse/src/filesystem.rs\nfn write(...) {\n    if self.is_readonly() {\n        reply.error(libc::EACCES);\n        return;\n    }\n    self.backend.write_file(...);\n}\n\nfn create(...) {\n    if self.is_readonly() {\n        reply.error(libc::EACCES);\n        return;\n    }\n    self.backend.create_file(...);\n}"
          },
          {
            "title": "Memory：每个 CP IO 都先检查 workspace attachment",
            "language": "ts",
            "code": "// control-plane/src/routes/internal.ts\nasync function requireAttached(workspaceId: string, storeId: string) {\n  const a = await getAttachment(workspaceId, storeId)\n  if (!a) return null\n  return a\n}\n\n// GET /_cp/workspaces/:wsId/memory-stores/:storeId/memory/*\nconst att = await requireAttached(wsId, storeId)\nif (!att) return c.json({ error: 'not attached' }, 404)\n\n// PUT /_cp/workspaces/:wsId/memory-stores/:storeId/memory/*\nif (att.access === 'read_only') return c.json({ error: 'read only' }, 403)"
          },
          {
            "title": "两个系统的真实读写链路示例",
            "language": "text",
            "code": "AFS 写入：\nagent(A) echo hi > /mnt/afs/req/spec.md\n  -> kernel FUSE\n  -> afs-fuse sidecar\n  -> LocalStorage.write_file(dir_id=8f3a-c91, path=/spec.md)\n  -> /data/afs/8f3a-c91/spec.md\nagent(B/C) 下次 cat /mnt/afs/req/spec.md 时从同一个 storage 读取。\n\nMemory 写入：\nagent(A) echo frontmatter... > /mnt/memory/mem_project/notes/overview.md\n  -> kernel FUSE\n  -> memory-fuse file buffer\n  -> close/fsync 触发 PUT /_cp/workspaces/ws-a/memory-stores/mem_project/memory/notes/overview.md\n  -> CP 检查 workspace_memory_attachments\n  -> 更新 memories 当前行 + 插入 memory_versions\nagent(B/C) 收到 invalidate 或刷新 snapshot 后，再读时按最新 sha 从 CP 获取内容。"
          }
        ]
      },
      {
        "title": "sidecar 与 controller 断开或 Pod 重启：stream/session 如何变化",
        "actor": "afs-fuse sidecar <-> afs-controller + control-plane bootstrap",
        "request": "SessionStream heartbeat reconnect / boot pull remount",
        "url": "gRPC afs-controller:9100/ControllerService.SessionStream + GET http://control-plane/_cp/workspaces/:id/afs-mounts",
        "params": {
          "normal_stream_id": "stream_fuse_ws_a_1",
          "normal_session_id": "sess_old_001",
          "mount": "/mnt/afs/req -> dir_id 8f3a-c91",
          "disconnect_case": "sidecar 进程还活着，只是到 controller 的 gRPC 连接断了",
          "pod_restart_case": "sidecar 进程重启，生成新的 stream_id，mounts 内存表为空",
          "canonical_source": "CP 的 afs_shares + afs_share_members 是恢复挂载的权威来源"
        },
        "paramNotes": [
          "stream_id：sidecar 进程启动时生成，表示这个 sidecar 到 controller 的长连接身份。",
          "session_id：一次具体 mount 的 controller 记录。连接断开或重启后，旧 session_id 通常会被删除，新挂载会产生新 session_id。",
          "disconnect_case：如果只是 gRPC 连接断开，sidecar 进程没死，内存里的 mounts 还在；重连后 heartbeat 会把当前 mounts 重新报给 controller。",
          "pod_restart_case：如果 Pod/sidecar 进程重启，旧 mounts 内存没了；sidecar 需要通过 AFS_BOOTSTRAP_URL 从 CP 拉“这个 workspace 应该挂哪些 share”，再重新 Mount。",
          "canonical_source：controller.sessions 只是“当前在线挂载索引”，不是恢复依据；恢复依据是 CP 保存的 share/member/access_key。"
        ],
        "response": {
          "normal": "controller.streams[stream_id] 有一条内存 sender；controller.sessions 有 session_id/dir_id/stream_id/mountpoint。",
          "link_disconnect": "controller 发现 stream dropped 后删除 streams[stream_id]，并 delete_sessions_by_stream(stream_id)。sidecar 重连后用同一个 stream_id heartbeat，controller 根据 mounts 重新 insert sessions。",
          "pod_restart": "旧 stream 断开，旧 sessions 被清理；新 sidecar 生成新 stream_id，boot_pull 拉 CP 挂载列表，重新 Mount，RegisterSession 写入新 sessions。",
          "cannot_do": "controller 不能用旧 session 找回旧 pod；旧 stream 没了就没有可推命令的连接。它只能等待 sidecar 重连/重启后重新登记。"
        },
        "codeBlocks": [
          {
            "title": "源码：sidecar 启动生成 stream_id，并维护当前 mounts 内存表",
            "code": "// D:/Code/koala/afs/afs-fuse/src/service.rs:26\npub struct FuseServiceImpl {\n    controller_addr: String,\n    stream_id: String,\n    mounts: Arc<Mutex<HashMap<String, ActiveMount>>>,\n}\n\npub fn new(controller_addr: String) -> Self {\n    let stream_id = generate_stream_id();\n    Self {\n        controller_addr,\n        stream_id,\n        mounts: Arc::new(Mutex::new(HashMap::new())),\n    }\n}\n\npub fn mount_statuses(&self) -> Vec<(String, String)> {\n    let mounts = self.mounts.lock().unwrap();\n    mounts.values().map(|m| (m.id.clone(), m.mountpoint.clone())).collect()\n}\n\n解释：\n- stream_id 属于 sidecar 进程生命周期。\n- mounts 是 sidecar 进程内存里的“我当前挂了哪些 dir/mountpoint”。\n- 只断网不重启时，mounts 还在；Pod 重启时，mounts 清空，stream_id 也变。"
          },
          {
            "title": "源码：sidecar SessionStream 会自动重连，并每 10 秒发送 heartbeat",
            "code": "// D:/Code/koala/afs/afs-fuse/src/main.rs:79\nasync fn session_stream_loop(controller_addr: String, service: Arc<FuseServiceImpl>) {\n    let mut backoff = Duration::from_secs(1);\n    loop {\n        match connect_and_stream(controller_addr.clone(), service.clone()).await {\n            Ok(_) => backoff = Duration::from_secs(1),\n            Err(_) => {\n                tokio::time::sleep(backoff).await;\n                backoff = std::cmp::min(backoff * 2, Duration::from_secs(30));\n            }\n        }\n    }\n}\n\n// D:/Code/koala/afs/afs-fuse/src/main.rs:124\nlet initial_mounts = service.mount_statuses();\ntx.send(Heartbeat { stream_id: stream_id.clone(), mounts: initial_mounts }).await?;\n\n// D:/Code/koala/afs/afs-fuse/src/main.rs:136\nloop {\n    tokio::time::sleep(Duration::from_secs(10)).await;\n    let mounts = heartbeat_service.mount_statuses();\n    tx.send(Heartbeat { stream_id: heartbeat_stream_id.clone(), mounts }).await?;\n}\n\n解释：\n- 网络断开后，sidecar 会退避重连。\n- 如果 sidecar 进程没死，重连时还是同一个 stream_id，并把内存 mounts 重新报上去。"
          },
          {
            "title": "源码：controller 用 heartbeat 对 sessions 做 reconciliation",
            "code": "// D:/Code/koala/afs/afs-controller/src/service.rs:344\nasync fn session_stream(&self, request: Request<Streaming<Heartbeat>>) -> Result<Response<Self::SessionStreamStream>, Status> {\n    let mut inbound = request.into_inner();\n    let (tx, rx) = mpsc::channel(32);\n\n    tokio::spawn(async move {\n        let mut stream_id: Option<String> = None;\n        while let Ok(Some(heartbeat)) = inbound.message().await {\n            let sid = heartbeat.stream_id.clone();\n            if stream_id.is_none() {\n                stream_id = Some(sid.clone());\n                streams.lock().await.insert(sid.clone(), tx.clone());\n            }\n\n            let mounts = heartbeat.mounts.iter()\n                .map(|m| (m.dir_id.as_str(), m.mountpoint.as_str()))\n                .collect::<Vec<_>>();\n            db.reconcile_sessions_from_heartbeat(&sid, &mounts)?;\n        }\n\n        if let Some(sid) = stream_id {\n            streams.lock().await.remove(&sid);\n            db.delete_sessions_by_stream(&sid)?;\n        }\n    });\n}\n\n// D:/Code/koala/afs/afs-controller/src/db.rs:363\nreconcile_sessions_from_heartbeat(stream_id, mounts):\n  - heartbeat 里有、DB 没有：INSERT sessions\n  - DB 里有、heartbeat 没有：DELETE sessions\n\n解释：\ncontroller.sessions 会跟 sidecar heartbeat 报上来的 mounts 对齐。它不是永久事实表，而是在线挂载索引。"
          },
          {
            "title": "状态变化示例：只断连接 vs Pod 重启",
            "code": "场景 1：只断开 sidecar <-> controller 连接，sidecar 进程还活着\n\nT0 正常：\ncontroller.streams:\n  stream_fuse_ws_a_1 -> sender_to_sidecar_A\ncontroller.sessions:\n  sess_old_001 | dir_id=8f3a-c91 | stream_id=stream_fuse_ws_a_1 | mountpoint=/mnt/afs/req\nsidecar A memory:\n  stream_id = stream_fuse_ws_a_1\n  mounts = { /mnt/afs/req -> dir_id=8f3a-c91 }\n\nT1 gRPC 断开：\ncontroller.streams:\n  删除 stream_fuse_ws_a_1\ncontroller.sessions:\n  delete where stream_id = stream_fuse_ws_a_1\nsidecar A memory:\n  mounts 还在，FUSE 还在，agent 仍可能继续通过 /mnt/afs/req 读写 storage\n\nT2 重连成功：\nsidecar A 发送 heartbeat:\n  stream_id = stream_fuse_ws_a_1\n  mounts = [{ dir_id=8f3a-c91, mountpoint=/mnt/afs/req }]\ncontroller.sessions:\n  sess_new_101 | dir_id=8f3a-c91 | stream_id=stream_fuse_ws_a_1 | mountpoint=/mnt/afs/req\n\n场景 2：Pod/sidecar 重启\n\nT0 旧 pod 退出：\ncontroller 删除旧 stream 和旧 sessions。\n宿主机旧 FUSE mountpoint 会随进程退出/清理失效。\n\nT1 新 pod 启动：\nsidecar A' memory:\n  stream_id = stream_fuse_ws_a_2   # 新生成\n  mounts = {}                      # 进程内存为空\n\nT2 boot_pull：\nGET /_cp/workspaces/ws_a/afs-mounts\n返回 CP 权威挂载列表：[{ id=8f3a-c91, access_key=ak_live_xxx, mountpoint=/mnt/afs/req }]\n\nT3 重新 Mount：\nsidecar A' ValidateToken -> create_backend(/data/afs) -> spawn FUSE -> RegisterSession\ncontroller.sessions:\n  sess_new_202 | dir_id=8f3a-c91 | stream_id=stream_fuse_ws_a_2 | mountpoint=/mnt/afs/req\n\n关键：controller 没有“找回旧 pod”。旧 pod 没了就是没了；新 pod 靠 CP 的 share/member 数据重新挂载。"
          },
          {
            "title": "源码：Pod 重启后靠 AFS_BOOTSTRAP_URL 拉 CP 权威挂载列表",
            "code": "// D:/Code/koala/afs/afs-fuse/src/main.rs:42\nif let Ok(url) = std::env::var(\"AFS_BOOTSTRAP_URL\") {\n    if !url.is_empty() {\n        tokio::spawn(async move {\n            if let Err(e) = boot_pull(&url, &boot_service).await {\n                warn!(\"boot pull failed (url={}): {}\", url, e);\n            }\n        });\n    }\n}\n\n// D:/Code/koala/afs/afs-fuse/src/main.rs:197\nasync fn boot_pull(url: &str, service: &Arc<FuseServiceImpl>) -> Result<()> {\n    let resp = client.get(url).send().await?.error_for_status()?;\n    let body: BootPullResponse = resp.json().await?;\n    for mount in body.mounts {\n        service.mount(MountRequest {\n            id: mount.id,\n            access_key: mount.access_key,\n            mountpoint: mount.mountpoint,\n            readonly: mount.readonly,\n            controller_addr: service.controller_addr(),\n        }).await;\n    }\n}\n\n解释：\nPod 重启后，新 sidecar 不知道旧 sessions。它只知道 AFS_BOOTSTRAP_URL。\nCP 用 afs_share_members JOIN afs_shares 算出“这个 workspace 应该挂哪些 share”，sidecar 再重新 Mount。"
          }
        ],
        "implementation": [
          "正常状态：controller 内存 streams 保存 stream_id -> sender，用来给某个在线 sidecar 推 ForceUnmount；controller.sessions 保存这个 stream 下有哪些 mount。",
          "连接断开：controller 的 session_stream 循环结束后，会删除 streams[stream_id]，并 delete_sessions_by_stream(stream_id)。所以旧 session 不是恢复依据，会被清掉。",
          "只断网不重启：sidecar 进程内 mounts 还在，session_stream_loop 会重连。重连后发 heartbeat，controller 用 heartbeat 里的 mounts 重新插入 sessions。session_id 可能变，stream_id 通常不变。",
          "Pod 重启：sidecar 进程重新创建，stream_id 变，mounts 为空。它先用 AFS_BOOTSTRAP_URL 拉 CP 的挂载清单，再逐个 Mount/RegisterSession。",
          "controller 如何“找回旧 session 对应 pod”：严格说找不回。旧 stream 已断就没有可通信对象。系统设计是清理旧 session，等待 sidecar 重新声明当前 mounts，或者新 pod 通过 bootstrap 重新挂载。",
          "如果 revoke 发生在 stream 不在线时，controller 会找不到 sender 或发送失败，但仍会删除 session 记录；CP 还会依赖成员数据和后续 bootstrap/挂载校验避免已撤销的 share 被重新挂载。"
        ],
        "db": [
          {
            "table": "afs_controller_sessions",
            "op": "DELETE on stream dropped",
            "values": {
              "stream_id": "stream_fuse_ws_a_1",
              "effect": "旧 stream 下的 session 全部删除"
            }
          },
          {
            "table": "afs_controller_sessions",
            "op": "INSERT on reconnect heartbeat",
            "values": {
              "stream_id": "stream_fuse_ws_a_1",
              "dir_id": "8f3a-c91",
              "mountpoint": "/mnt/afs/req",
              "session_id": "new generated"
            }
          },
          {
            "table": "afs_share_members",
            "op": "SELECT on pod restart bootstrap",
            "values": {
              "workspace_id": "ws_a",
              "share_id": "shr_req_001",
              "permission": "read_write"
            }
          },
          {
            "table": "afs_shares",
            "op": "JOIN on pod restart bootstrap",
            "values": {
              "id": "shr_req_001",
              "afs_dir_id": "8f3a-c91",
              "access_key": "ak_live_xxx",
              "name": "req"
            }
          }
        ],
        "note": "把 sessions 理解成“在线挂载索引”，不要理解成“pod 永久身份”。pod 永久恢复靠 CP 的 share/member 数据，不靠 controller.sessions。"
      },
      {
        "title": "Pod 重启自愈：sidecar 拉挂载列表重新 Mount",
        "actor": "afs-fuse sidecar boot -> control-plane internal API",
        "request": "GET /_cp/workspaces/:id/afs-mounts",
        "url": "GET http://control-plane/_cp/workspaces/ws_b/afs-mounts",
        "params": {
          "workspace_id": "ws_b",
          "bootstrap_url": "AFS_BOOTSTRAP_URL"
        },
        "paramNotes": [
          "workspace_id：当前重启的 workspace。",
          "bootstrap_url：注入到 sidecar 的环境变量，告诉它启动时去哪拉应该挂哪些 share。"
        ],
        "response": {
          "status": 200,
          "body": {
            "mounts": [
              {
                "id": "8f3a-c91",
                "access_key": "ak_live_xxx",
                "mountpoint": "/mnt/afs/req",
                "readonly": true
              }
            ]
          }
        },
        "implementation": [
          "这一节是上一节 pod_restart_case 的具体 HTTP 拉取：sidecar 启动后读取 AFS_BOOTSTRAP_URL。",
          "CP 从 afs_share_members JOIN afs_shares 查出该 workspace 应有挂载，返回 id/access_key/mountpoint/readonly。",
          "sidecar 对返回的每一项执行 Mount。Mount 成功后会 RegisterSession，controller.sessions 写入新的 stream_id/session_id。",
          "这就是 CP 必须保存 access_key 的原因：pod 重启后 controller.sessions 已经不是恢复来源，sidecar 需要重新拿钥匙挂载。"
        ],
        "db": [
          {
            "table": "afs_share_members",
            "op": "SELECT for workspace",
            "values": {
              "workspace_id": "ws_b"
            }
          },
          {
            "table": "afs_shares",
            "op": "JOIN",
            "values": {
              "afs_dir_id": "8f3a-c91",
              "access_key": "ak_live_xxx",
              "name": "req"
            }
          }
        ],
        "note": "自愈恢复的是挂载状态；旧 session 不会复活，新 pod 会产生新的 stream_id 和 session_id。"
      },
      {
        "title": "撤销 unshare_from_all：controller 作废并强制卸载",
        "actor": "Agent A MCP -> control-plane -> afs-controller -> afs-fuse sessions",
        "request": "DELETE /:workspaceId/afs/shares/:shareId",
        "url": "DELETE http://control-plane/api/workspaces/ws_a/afs/shares/shr_req_001",
        "params": {
          "share_id": "shr_req_001",
          "dir_id": "8f3a-c91",
          "access_key": "ak_live_xxx"
        },
        "paramNotes": [
          "share_id：CP 侧要删除的共享记录。",
          "dir_id/access_key：CP 调 RevokeDir 时交给 controller，用来证明有权作废这把钥匙。"
        ],
        "response": {
          "status": 200,
          "body": {
            "sessions_revoked": 2,
            "errors": 0
          }
        },
        "codeBlocks": [
          {
            "title": "源码：RevokeDir 通过 SessionStream 推 ForceUnmount",
            "code": "// afs-controller/src/service.rs\nasync fn revoke_dir(&self, request: Request<RevokeDirRequest>) -> Result<Response<RevokeDirResponse>, Status> {\n    match self.db.validate_token(&req.id, &req.access_key) { Ok(Some(_)) => {}, _ => permission_denied }\n    let (sessions_revoked, errors) = self.revoke_sessions_for_dir(&req.id).await;\n    Ok(Response::new(RevokeDirResponse { sessions_revoked, errors }))\n}\n\nasync fn revoke_sessions_for_dir(&self, dir_id: &str) -> (i32, i32) {\n    let sessions = self.db.get_sessions_for_dir(dir_id)?;\n    for session in sessions {\n        tx.send(Ok(SessionCommand { force_unmount: ForceUnmount { mountpoint: session.mountpoint } })).await;\n        self.db.deregister_session(&session.session_id);\n    }\n}\n\n// afs-fuse/src/service.rs\npub fn force_unmount(&self, mountpoint: &str) -> bool {\n    mounts.remove(mountpoint);\n    fusermount_u(mountpoint);\n    remove_mountpoint_dir(mountpoint);\n    true\n}\n\n解释：revoke 是全局作废。controller 根据 sessions 找到所有正在挂这个 dir 的 sidecar，反推 ForceUnmount。"
          }
        ],
        "implementation": [
          "CP 先调 RevokeDir，让 controller 验 access_key 并推 ForceUnmount。",
          "controller 删除 sessions 记录；sidecar 收到命令后本地 fusermount -u 并移除 mountpoint 目录。",
          "CP 再对成员做 best-effort unmount，处理 controller 推送失败或 sidecar 离线的情况。",
          "最后 CP 删除 afs_share_members/afs_shares。"
        ],
        "db": [
          {
            "table": "afs_controller_sessions",
            "op": "DELETE per revoked session",
            "values": {
              "dir_id": "8f3a-c91"
            }
          },
          {
            "table": "afs_controller_dirs",
            "op": "status no longer active/delete path",
            "values": {
              "id": "8f3a-c91"
            }
          },
          {
            "table": "afs_share_members",
            "op": "DELETE",
            "values": {
              "share_id": "shr_req_001"
            }
          },
          {
            "table": "afs_shares",
            "op": "DELETE",
            "values": {
              "id": "shr_req_001"
            }
          }
        ],
        "note": "revokeDir 是全局撤销；unmount 只是某个 pod 本地下挂。两者不要混。"
      }
    ]
  },
  "usageParsing": {
    "title": "Usage 解析：Claude/Codex/Goose transcript 到用量账本",
    "intro": "agent 暴露 POST /usage，control-plane 带 workspace_usage_cursor 过去；agent 扫描本地 transcript 文件并返回标准化 UsageRecord，CP 写 workspace_usage_events。增量靠文件 fingerprint + dedup_key，不靠行 offset。",
    "quickLinks": [
      {
        "label": "数据库结构：workspace_usage_events",
        "moduleId": "schema",
        "domain": "runtime",
        "table": "workspace_usage_events"
      },
      {
        "label": "数据库结构：workspace_usage_cursor",
        "moduleId": "schema",
        "domain": "runtime",
        "table": "workspace_usage_cursor"
      }
    ],
    "steps": [
      {
        "title": "agent 挂载 /usage endpoint",
        "actor": "Claude/Codex/Goose agent server",
        "request": "registerUsageRoutes(app, '/usage')",
        "url": "http://127.0.0.1:<agent-port>/usage",
        "params": {
          "homeDir": "$HOME or $WORKSPACE_DIR/.home",
          "fallbackModel": "loadRuntimeConfig()?.model"
        },
        "paramNotes": [
          "homeDir：usage parser 扫描 transcript 的根目录。",
          "fallbackModel：transcript 没写 model 时兜底。",
          "Claude/Codex/Goose server 都挂这个 endpoint。"
        ],
        "response": {
          "internal": true,
          "route": "POST /usage"
        },
        "implementation": [
          "registerUsageRoutes 定义 POST /usage。",
          "agent 自身不持久化 cursor；状态在 control-plane 数据库。",
          "Goose 还会额外把 ACP usage 写成 $HOME/.acp-usage/*.jsonl。"
        ],
        "db": [],
        "note": "CP 可以在 session.ended 或定时 sweep 时主动 pull 用量。"
      },
      {
        "title": "control-plane 带 cursor 拉取 usage",
        "actor": "control-plane -> agent",
        "request": "POST /usage",
        "url": "http://agent-service/usage",
        "params": {
          "cursors": {},
          "maxFiles": 50
        },
        "paramNotes": [
          "cursors：来自 workspace_usage_cursor.cursor；第一次为空对象。",
          "maxFiles：单批最多解析多少个变化文件。",
          "workspace_id/user_id 不传给 agent；CP 写库时补上。"
        ],
        "response": {
          "status": 200,
          "body": {
            "records": [],
            "cursors": {},
            "hasMore": false
          }
        },
        "implementation": [
          "pullWorkspaceUsage 先 getUsageCursor(workspaceId)，再 postToAgent('/usage')。",
          "收到 records 后先 insertUsageRecords，再 setUsageCursor。",
          "hasMore=true 时继续下一批，最多 40 批。"
        ],
        "db": [
          {
            "table": "workspace_usage_cursor",
            "op": "SELECT",
            "values": {
              "workspace_id": "ws_codex_demo",
              "cursor": {}
            }
          }
        ],
        "note": "cursor 是 CP 给 agent 的输入，也是 agent 返回给 CP 的新进度。"
      },
      {
        "title": "Cursor 用文件 fingerprint 判断增量",
        "actor": "internal/agent-usage sweepUsage",
        "request": "sweepUsage({ homeDir, cursors, maxFiles })",
        "url": "file://$HOME/.claude|.codex|.acp-usage",
        "params": {
          "oldCursor": {
            "/home/oai/.codex/sessions/2026/05/30/rollout-a.jsonl": {
              "dev": 2049,
              "ino": 81122,
              "size": 4096,
              "mtimeMs": 1780123456789
            }
          }
        },
        "paramNotes": [
          "文件没变：跳过并保留 cursor。",
          "文件变了：整文件重读并返回新 fingerprint。",
          "达到 maxFiles：未处理 changed 文件不写 cursor，hasMore=true。",
          "Claude 新鲜尾部 assistant 会暂缓，并删除该文件 cursor，避免漏掉最终 rewrite。"
        ],
        "response": {
          "records": "UsageRecord[]",
          "cursors": "new SweepCursors",
          "hasMore": false
        },
        "implementation": [
          "fingerprint 使用 dev/ino/size/mtimeMs。",
          "cursor 不保存行号，所以追加一行也会整文件重读。",
          "重复记录靠 workspace_usage_events.dedup_key 去重。"
        ],
        "db": [],
        "note": "这是理解 Cursors 的关键：它是文件变化检测，不是已读到第几行。"
      },
      {
        "title": "Claude/Codex/Goose 记录格式",
        "actor": "agent-usage parsers",
        "request": "parse transcripts",
        "url": "file://$HOME/.claude/projects/<slug>/<session>.jsonl | file://$HOME/.codex/sessions/YYYY/MM/DD/rollout-*.jsonl | file://$HOME/.acp-usage/<sessionId>.jsonl",
        "params": {
          "claude": {
            "dedupKey": "claude:<message.id>",
            "fields": [
              "message.usage.input_tokens",
              "cache_creation",
              "web_search_requests"
            ]
          },
          "codex": {
            "cumulative": "payload.info.total_token_usage",
            "dedupKey": "codex:<session>:<cur.total>"
          },
          "goose": {
            "accumulated": "accumulated_input_tokens/output_tokens",
            "dedupKey": "goose:<session>:<accTotal>"
          }
        },
        "paramNotes": [
          "Claude：只取 assistant + message.id + usage；同 message.id 取最后一次。",
          "Codex：token_count 是累计值，parser 计算相邻累计 delta；cached_input_tokens 变成 cache_read_tokens。",
          "Goose：不解析 SQLite，而是解析 .acp-usage JSONL；cache/reasoning 不可得，fieldsIncomplete=true。"
        ],
        "response": {
          "normalized": {
            "source": "codex",
            "inputTokens": 300,
            "cacheReadTokens": 500,
            "outputTokens": 120,
            "reasoningTokens": 40,
            "dedupKey": "codex:sess:5780"
          }
        },
        "implementation": [
          "internal/agent-usage/src/index.ts 分别实现 parseClaudeTranscript、parseCodexRollout、parseGooseUsage。",
          "三种来源最后都归一成 UsageRecord。"
        ],
        "db": [
          {
            "table": "workspace_usage_events",
            "op": "INSERT later",
            "values": {
              "source": "codex",
              "dedup_key": "codex:sess:5780"
            }
          }
        ],
        "note": "Codex 最容易误解：不能累加 last_token_usage，要从 cumulative total 算 delta。"
      },
      {
        "title": "写入 ledger 并保存新 cursor",
        "actor": "control-plane usage pull",
        "request": "insertUsageRecords + setUsageCursor",
        "url": "postgres://workspace_usage_events,workspace_usage_cursor",
        "params": {
          "workspaceId": "ws_codex_demo",
          "userId": "u_alice",
          "records": [
            {
              "dedupKey": "codex:sess:5780"
            }
          ],
          "cursors": {
            "/home/oai/.codex/sessions/2026/05/30/rollout-abc.jsonl": {
              "size": 4600,
              "mtimeMs": 1780123550000
            }
          }
        },
        "paramNotes": [
          "workspaceId/userId：CP 写库时补上。",
          "records：agent 标准化后的 usage。",
          "cursors：agent 返回的新文件 fingerprint map。"
        ],
        "response": {
          "inserted": 1,
          "cursorSaved": true
        },
        "implementation": [
          "insertUsageRecords 用 jsonb_to_recordset 批量写。",
          "ON CONFLICT (dedup_key) DO NOTHING，整文件重读不会重复入账。",
          "setUsageCursor 对 workspace_usage_cursor 做 upsert。"
        ],
        "db": [
          {
            "table": "workspace_usage_events",
            "op": "INSERT ON CONFLICT DO NOTHING",
            "values": {
              "workspace_id": "ws_codex_demo",
              "user_id": "u_alice",
              "source": "codex",
              "dedup_key": "codex:sess:5780"
            }
          },
          {
            "table": "workspace_usage_cursor",
            "op": "UPSERT",
            "values": {
              "workspace_id": "ws_codex_demo",
              "cursor": {
                "rollout-abc.jsonl": {
                  "size": 4600
                }
              }
            }
          }
        ],
        "note": "顺序是先写 ledger，再推进 cursor；失败最多导致下次重拉。"
      },
      {
        "title": "示例：Cursor 如何算增量",
        "actor": "human observer",
        "request": "compare old cursor and file stat",
        "url": "file://$HOME/.codex/sessions/2026/05/30/rollout-abc.jsonl",
        "params": {
          "firstPull": {
            "cursor": {},
            "parsed": [
              "codex:sess:1000",
              "codex:sess:1800"
            ]
          },
          "secondPull": {
            "sameFingerprint": true,
            "parsed": []
          },
          "afterAppend": {
            "fileChanged": true,
            "rereadWholeFile": true,
            "parsed": [
              "codex:sess:1000",
              "codex:sess:1800",
              "codex:sess:2300"
            ],
            "inserted": [
              "codex:sess:2300"
            ]
          }
        },
        "paramNotes": [
          "第一次 cursor 空，解析文件并保存 fingerprint。",
          "第二次 fingerprint 一样，直接跳过。",
          "第三次 size/mtimeMs 变了，整文件重读。",
          "旧 dedup_key 被 ON CONFLICT 吃掉，只新增最后一条。"
        ],
        "response": {
          "finalEvents": [
            "codex:sess:1000",
            "codex:sess:1800",
            "codex:sess:2300"
          ],
          "finalCursor": {
            "size": 4600
          }
        },
        "implementation": [
          "人类排查时先看 cursor 里的 fingerprint，再看文件 stat，再查 dedup_key 是否已经存在。"
        ],
        "db": [
          {
            "table": "workspace_usage_cursor",
            "op": "UPDATE",
            "values": {
              "cursor": "new file fingerprint"
            }
          },
          {
            "table": "workspace_usage_events",
            "op": "INSERT only new dedup_key",
            "values": {
              "dedup_key": "codex:sess:2300"
            }
          }
        ],
        "note": "Cursor 负责发现变化，dedup_key 负责只计一次。"
      }
    ]
  },
  "agentStartup": {
    "title": "Agent 启动：从 WORKSPACE_ID 到本地文件",
    "intro": "观察重点是 workspace_config 不会直接交给 Codex 使用，control-plane 会先联表解析成 agent-facing config。另一个重点是 token 边界：agent 启动拉 /_cp/config/skills/credentials 这条链路当前不带 Bearer；chat dispatch 下发的是 session_token，用于 MCP/session 归属；service_token 主要用于 API/服务间调用，不是启动 config 的认证凭据。",
    "steps": [
      {
        "title": "agent 请求 config",
        "actor": "Codex agent -> control-plane",
        "request": "GET /_cp/workspaces/ws_codex_demo/config",
        "url": "http://127.0.0.1:30080/_cp/workspaces/ws_codex_demo/config",
        "params": {
          "workspace_id": "ws_codex_demo"
        },
        "paramNotes": [
          "workspace_id：来自 agent 启动环境变量或请求路径。CP 用它读取 workspace_config 并合并模板/provider/prompt。"
        ],
        "response": {
          "status": 200,
          "body": {
            "workspace_id": "ws_codex_demo",
            "model": "gpt-5",
            "base_url": "https://llm.example.com/v1",
            "prompt_content": "You are Codex...",
            "mcp_config": "merged mcp config"
          }
        },
        "db": [
          {
            "table": "workspaces",
            "op": "SELECT",
            "values": {
              "id": "ws_codex_demo",
              "user_id": "u_alice",
              "status": "running"
            }
          },
          {
            "table": "workspace_config",
            "op": "SELECT",
            "values": {
              "workspace_id": "ws_codex_demo",
              "template_id": "tpl_research",
              "template_version": 3,
              "provider_id": null,
              "prompt_id": "prompt_codex",
              "model": "gpt-5"
            }
          },
          {
            "table": "templates",
            "op": "LEFT JOIN",
            "values": {
              "id": "tpl_research",
              "latest_version": 4
            }
          },
          {
            "table": "template_versions",
            "op": "LEFT JOIN",
            "values": {
              "template_id": "tpl_research",
              "version": 3,
              "provider_id": "provider_company",
              "model": "gpt-5-mini",
              "prompt_id": "prompt_research",
              "prompt_version": 2
            }
          },
          {
            "table": "prompt_versions",
            "op": "LEFT JOIN",
            "values": {
              "prompt_id": "prompt_research",
              "version": 2,
              "content": "Research carefully..."
            }
          },
          {
            "table": "template_version_skills",
            "op": "SELECT",
            "values": {
              "template_version_id": "tpl_research_v3",
              "skill_id": "skill_media"
            }
          },
          {
            "table": "model_providers",
            "op": "LEFT JOIN",
            "values": {
              "id": "provider_company",
              "base_url": "https://llm.example.com/v1"
            }
          },
          {
            "table": "prompts",
            "op": "LEFT JOIN",
            "values": {
              "id": "prompt_codex",
              "content": "You are Codex..."
            }
          }
        ],
        "implementation": [
          "参数加载优先级可以按三层看：第一层是 workspace_config 的覆盖值；第二层是 template_versions 的默认值；第三层是被 id 引用的 model_providers/prompts 详情。",
          "普通字段规则：agent_type/model/small_model/system_prompt 使用 workspace_config 非空值，否则回退 template_versions；mcp_config/agent_settings 只有 workspace_config 不是 {} 时才覆盖模板。",
          "provider 规则：workspace_config.provider_id 优先；为空时用 template_versions.provider_id；最后 JOIN model_providers 得到 provider_type/base_url/api_key。",
          "prompt 规则：workspace_config.prompt_id 优先；如果 workspace_config.system_prompt 非空，说明用户写了内联 prompt，不再使用模板 prompt；否则用 template_versions.prompt_id + prompt_version 去读 prompt_versions，prompt_version 为空时才退回 prompts.content 当前内容。",
          "template 规则：workspace_config.template_id + template_version 锁定某个模板版本，不会自动跟随 templates.latest_version；prompt 若保存 prompt_version 则内容稳定，skills 只通过 template_version_skills 固定 skill_id，下载包仍跟随 skills.active_version_id。"
        ],
        "note": "返回给 agent 的 payload 已经包含 prompt_content、base_url、api_key、mcp_config 等运行视图。"
      },
      {
        "title": "认证与 token 边界：service_token 不等于 session_token",
        "actor": "UI/API -> control-plane -> agent Pod",
        "request": "POST /api/workspaces/:id/chat then POST agent /chat",
        "url": "CP API: POST http://127.0.0.1:30080/api/workspaces/ws_codex_demo/chat；CP -> Agent: POST http://agent-ws-codex-demo:3000/chat；Agent startup: GET http://127.0.0.1:30080/_cp/workspaces/ws_codex_demo/config",
        "params": {
          "user_auth": "cookie JWT / Authorization: Bearer tos_xxx / Authorization: Bearer OAuth JWT",
          "workspace_id": "ws_codex_demo",
          "session_id": "sess_123 or omitted",
          "session_token": "generated uuid, e.g. 6f6c7c8a-...",
          "service_token": "tos_xxx, only for /api or service-to-service calls"
        },
        "paramNotes": [
          "user_auth：用户或外部调用方访问 CP /api 时使用。CP 全局 auth middleware 会按 Service Token -> OAuth JWT -> cookie JWT 的顺序识别。",
          "workspace_id：agent Pod 环境变量和 /_cp 路径参数都使用它。/_cp 内部路由依赖这个 id 读 workspace 数据。",
          "session_id：恢复已有会话时由 CP 传给 agent；新会话时省略。",
          "session_token：CP 在 chat dispatch 时写入 session_tokens 表，并放进 agent /chat body。agent 后续 MCP HTTP 请求把它放到 X-Session-Token。",
          "service_token：tos_ 前缀的机器/API token，存 service_tokens 表。它不是 agent 启动拉 config/skills/credentials 的凭据。"
        ],
        "response": {
          "cp_to_agent_chat_body": {
            "message": "用户消息",
            "session_id": "sess_123 when resume; omitted for new session",
            "source": "web",
            "session_token": "6f6c7c8a-..."
          },
          "startup_internal_fetch": {
            "method": "GET",
            "url": "/_cp/workspaces/ws_codex_demo/config",
            "authorization_header": null
          },
          "mcp_request_from_agent": {
            "url": "/mcp or /_cp/mcp/:userId/:origin/*",
            "headers": {
              "X-Workspace-ID": "ws_codex_demo",
              "X-Agent-ID": "ws_codex_demo",
              "X-Session-Token": "6f6c7c8a-..."
            }
          }
        },
        "implementation": [
          "CP 对普通 /api 请求做认证：control-plane/src/index.ts 全局中间件先跳过 public routes；非 public 的 /api 请求如果带 Authorization: Bearer，会先按 service token 查 service_tokens.token_hash，命中后把 currentUser 设置成 token.created_by；没命中再按 CP OAuth JWT 校验；最后再尝试 cookie JWT。",
          "但 /_cp/* 是显式跳过全局认证的内部路由。index.ts 里 path.startsWith(\"/_cp/\") 直接 next()。所以 agent 启动时 GET /_cp/workspaces/:id/config、/skills、/credentials 没有 Bearer token 校验。",
          "这意味着启动 config 这条链路的保护边界主要是集群网络隔离、服务只暴露给 Pod 内/集群内、以及 workspace_id 只由平台在 Pod 环境变量里下发。它不是 OAuth/service_token 级别的每请求认证。",
          "service_token 的下发：用户登录或创建 token 时，CP 在 service_tokens 表保存 hash；特殊的 platform token 由 ensurePlatformToken(userId) 懒创建，is_platform=true，并额外保存明文 token，供 Browser Service、Sandbox、Channel Gateway、scheduler 等内部服务代表用户调用 CP 或下游服务。",
          "agent 启动阶段没有看到 CP 把 service_token 下发给 agent 去拉 /_cp/config。Codex agent 的 loadConfig/loadCredentials/loadSkills 直接 fetch CP_URL + /_cp/...，没有 Authorization header。",
          "chat 阶段下发的是 session_token：executeChat.ts 在 POST agent /chat 之前调用 mintToken 或 ensureTokenForSession，把 uuid 写入 session_tokens，并在 buildAgentChatBody() 中作为 session_token 放进 JSON body。",
          "agent 收到 session_token 后，在 loadAcpMcpServers(sessionToken) 中给 HTTP MCP server headers 注入 X-Session-Token。stdio MCP 不经过 CP proxy，不需要这个 token。",
          "CP 的 /mcp 会 resolveToken(X-Session-Token)，确认 token.workspace_id 与当前 workspace 匹配后得到 session_id/task_id。/_cp/mcp proxy 会 resolveTokenForUser，把内部 X-Session-Token 删除，转换成给上游看的 X-Workspace-Id / X-Session-Id，避免把原始 token 泄露给第三方 MCP。",
          "所以要分清：service_token 认证“谁在调 CP /api”；session_token 标记“这次 agent/MCP 调用属于哪个 chat session”；/_cp 启动接口当前靠内部网络和 workspace_id，而不是 Bearer token。"
        ],
        "db": [
          {
            "table": "service_tokens",
            "op": "SELECT by hash for /api Bearer auth",
            "values": {
              "token_hash": "sha256(tos_xxx)",
              "created_by": "u_alice",
              "is_platform": false,
              "revoked_at": null
            }
          },
          {
            "table": "service_tokens",
            "op": "ENSURE platform token for service-to-service",
            "values": {
              "name": "__platform__",
              "token": "tos_plaintext_saved_only_for_platform_token",
              "token_hash": "sha256(token)",
              "created_by": "u_alice",
              "is_platform": true
            }
          },
          {
            "table": "session_tokens",
            "op": "INSERT before CP -> agent /chat",
            "values": {
              "token": "gen_random_uuid()",
              "workspace_id": "ws_codex_demo",
              "session_id": null,
              "resolved_at": null
            }
          },
          {
            "table": "session_tokens",
            "op": "RESOLVE on MCP request",
            "values": {
              "header": "X-Session-Token: gen_random_uuid()",
              "workspace_check": "record.workspace_id === X-Workspace-ID"
            }
          }
        ],
        "codeBlocks": [
          {
            "title": "CP 全局认证：/_cp/* 被跳过，/api Bearer 才走 service_token/JWT",
            "language": "ts",
            "code": "// control-plane/src/index.ts:147-179\napp.use('/*', async (c, next) => {\n  const path = c.req.path\n  if (path.startsWith('/_cp/')) {\n    return next()\n  }\n\n  const isApiRequest = path.startsWith('/api/')\n  const authHeader = c.req.header('Authorization')\n  if (isApiRequest && authHeader?.startsWith('Bearer ')) {\n    const bearerToken = authHeader.slice(7)\n    const tokenRecord = await getServiceTokenByHash(hashToken(bearerToken))\n    if (tokenRecord) {\n      c.set('user', { sub: tokenRecord.created_by || 'service', role: 'user' })\n      return next()\n    }\n    const jwtPayload = await verifyToken(bearerToken)\n    if (jwtPayload) { c.set('user', jwtPayload); return next() }\n  }\n})"
          },
          {
            "title": "service_tokens：普通 token 只存 hash，platform token 额外存明文",
            "language": "ts",
            "code": "// services/db/shares.ts\nexport async function createServiceToken(name, tokenHash, createdBy) {\n  INSERT INTO service_tokens (id, name, token_hash, created_by)\n}\n\nexport async function ensurePlatformToken(userId) {\n  SELECT token FROM service_tokens\n   WHERE created_by = $1 AND is_platform = true AND revoked_at IS NULL\n\n  INSERT INTO service_tokens (id, name, token_hash, token, created_by, is_platform)\n  VALUES (..., '__platform__', hashToken(token), token, userId, true)\n}"
          },
          {
            "title": "session_token：CP 下发给 agent chat body，再由 agent 注入 MCP headers",
            "language": "ts",
            "code": "// executeChat.ts\nconst sessionToken = sessionId\n  ? await ensureTokenForSession(workspaceId, sessionId)\n  : await mintToken({ workspaceId })\n\nawait fetch(agentAddress + '/chat', {\n  method: 'POST',\n  body: JSON.stringify({ message, session_id, source, session_token: sessionToken })\n})\n\n// agents/codex/src/config.ts\nexport function loadAcpMcpServers(sessionToken?: string) {\n  if (sessionToken) headers.set('X-Session-Token', sessionToken)\n}"
          },
          {
            "title": "MCP 侧解析 session_token，但不把原始 token 泄露给上游",
            "language": "ts",
            "code": "// mcp/server.ts\nconst tokenHeader = request.headers.get('x-session-token')\nconst record = tokenHeader ? await resolveToken(tokenHeader) : null\nif (record && record.workspaceId === workspaceId) sessionId = record.sessionId\n\n// routes/mcp-proxy.ts\nconst sessionToken = upstreamHeaders.get('x-session-token')\nupstreamHeaders.delete('x-session-token')\nconst record = await resolveTokenForUser(sessionToken, userId)\nif (record?.sessionId) upstreamHeaders.set('X-Session-Id', record.sessionId)"
          }
        ],
        "note": "结论：启动拉配置不是 service_token 认证链路；service_token 服务 API/服务间调用，session_token 服务 chat/MCP 会话归属。"
      },
      {
        "title": "agent 写本地文件",
        "actor": "Codex agent",
        "request": "local filesystem writes",
        "params": {
          "config_payload": "GET /_cp/workspaces/:id/config response",
          "target_home": "~/.codex",
          "workspace_dir": "/workspace"
        },
        "paramNotes": [
          "config_payload：上一步 CP 返回的 agent-facing config。",
          "target_home：agent 容器内 Codex 配置目录。",
          "workspace_dir：AGENTS.md/runtime.json 等工作区文件写入位置。"
        ],
        "response": "无 HTTP 返回。本地副作用是写入 AGENTS.md、runtime.json、config.toml、auth.json、mcp.json。AGENTS.md 内容选择规则：prompt_content 非空字符串优先；否则使用 system_prompt；否则写空字符串。",
        "implementation": [
          "处理位置：agent 进程本地 hook/config writer。",
          "参数来源：来自 CP config response 和 agent 容器环境变量。",
          "prompt 处理：write_prompt_file() 调 prompt_from_workspace_config(config)。如果 prompt_content 是非空字符串，写 prompt_content；否则写 system_prompt；否则写空字符串。",
          "实现结果：把远端配置 materialize 成 Codex 可读取的本地配置文件。"
        ],
        "db": [],
        "note": "写 /workspace/AGENTS.md、runtime.json、~/.codex/config.toml、auth.json、mcp.json。stdio MCP 进 config.toml，HTTP MCP 等 session 阶段。"
      },
      {
        "title": "agent 拉 Skills",
        "actor": "Codex agent -> control-plane",
        "request": "GET /_cp/workspaces/ws_codex_demo/skills",
        "url": "http://127.0.0.1:30080/_cp/workspaces/ws_codex_demo/skills",
        "params": {
          "workspace_id": "ws_codex_demo"
        },
        "paramNotes": [
          "workspace_id：决定读取哪些 workspace_skills。",
          "skill_id：workspace_skills 只存稳定 skill 身份。",
          "active_version_id：skills 当前版本指针，决定下载哪个 skill_versions.package。"
        ],
        "response": {
          "status": 200,
          "body": {
            "skills": [
              {
                "id": "skill_media",
                "name": "media-helper",
                "active_version_id": "ver_media_2",
                "package_url": "/_cp/skills/skill_media/package"
              }
            ]
          }
        },
        "implementation": [
          "处理位置：agent 调 CP 的 workspace skills endpoint，再下载 package。",
          "参数来源：workspace_id 来自 agent 当前工作区。",
          "本地处理：解压 skill package，写入或 symlink 到 ~/.codex/skills；遇到 .editing 跳过覆盖。"
        ],
        "db": [
          {
            "table": "workspace_skills",
            "op": "SELECT",
            "values": {
              "workspace_id": "ws_codex_demo",
              "skill_id": "skill_media"
            }
          },
          {
            "table": "skills",
            "op": "JOIN",
            "values": {
              "id": "skill_media",
              "active_version_id": "ver_media_2"
            }
          },
          {
            "table": "skill_versions",
            "op": "package fetch",
            "values": {
              "id": "ver_media_2",
              "content_hash": "a1b2c3"
            }
          }
        ],
        "note": "agent 下载 active version package，解压后 symlink 到 ~/.codex/skills/{name}。编辑态 .editing 会跳过覆盖。"
      },
      {
        "title": "agent 拉 Credentials",
        "actor": "Codex agent -> control-plane",
        "request": "GET /_cp/workspaces/ws_codex_demo/credentials",
        "url": "http://127.0.0.1:30080/_cp/workspaces/ws_codex_demo/credentials",
        "params": {
          "workspace_id": "ws_codex_demo",
          "user_id": "u_alice"
        },
        "paramNotes": [
          "workspace_id：用于判断 selected scope 凭据是否授权给当前 workspace。",
          "user_id：来自 workspace owner，用于查询 user_credentials。",
          "credential_name：凭据名称，例如 GITHUB_TOKEN。"
        ],
        "response": {
          "status": 200,
          "body": {
            "credentials": [
              {
                "name": "GITHUB_TOKEN",
                "inject": "env",
                "value": "***plaintext-for-agent***"
              }
            ]
          }
        },
        "implementation": [
          "处理位置：agent 调 CP credentials endpoint。",
          "参数来源：workspace_id 来自 agent；user_id 由 CP 根据 workspace 查出。",
          "后端处理：CP 解密凭据，并按 scope/all 或 selected workspace 过滤。",
          "本地处理：agent 按 inject=env/file 写入环境变量或文件。"
        ],
        "db": [
          {
            "table": "user_credentials",
            "op": "SELECT + decrypt",
            "values": {
              "user_id": "u_alice",
              "name": "GITHUB_TOKEN",
              "scope": "selected"
            }
          },
          {
            "table": "user_credential_workspaces",
            "op": "EXISTS",
            "values": {
              "user_id": "u_alice",
              "credential_name": "GITHUB_TOKEN",
              "workspace_id": "ws_codex_demo"
            }
          }
        ],
        "note": "control-plane 返回明文给 agent，agent 按 inject=env/file 写入环境变量或文件。"
      },
      {
        "title": "每种 Agent 启动后的容器目录结构",
        "actor": "Codex / Claude Code / Goose agent 容器",
        "request": "container filesystem after startup",
        "method": "LOCAL",
        "url": "file:///workspace",
        "params": {
          "WORKSPACE_DIR": "/workspace",
          "HOME": "/workspace/.home",
          "Codex": "/workspace/AGENTS.md + /workspace/.home/.codex",
          "Claude Code": "/workspace/CLAUDE.md + /workspace/.claude + /workspace/.home/.claude",
          "Goose": "/workspace/AGENTS.md + /workspace/.home/.config/goose"
        },
        "paramNotes": [
          "WORKSPACE_DIR：三种 agent Dockerfile 都设置为 /workspace，agent 进程的工作目录也是 /workspace。",
          "HOME：三种 agent 都把 node 用户 home 改成 /workspace/.home，所以 ~/.codex、~/.config/goose、~/.ssh 等都会落在 /workspace/.home 下。",
          "Codex：平台 prompt 写入 /workspace/.home/.codex/AGENTS.md，业务 prompt 写入 /workspace/AGENTS.md，Codex 配置写入 /workspace/.home/.codex。",
          "Claude Code：平台 prompt 写入 /workspace/.home/.claude/CLAUDE.md，业务 prompt 写入 /workspace/CLAUDE.md；Claude 项目级配置在 /workspace/.claude，MCP 文件在 /workspace/.mcp.json。",
          "Goose：平台 prompt 写入 /workspace/.home/.config/goose/AGENTS.md，业务 prompt 写入 /workspace/AGENTS.md，Goose 配置写入 /workspace/.home/.config/goose。",
          "memory 和 AFS 目录不是 agent config.ts 直接创建的；只有 Pod manifest 挂载了对应 sidecar/FUSE volume 后，agent 容器内才会看到 /mnt/memory 或 /mnt/afs。"
        ],
        "response": {
          "status": "started",
          "filesystem_view": {
            "common": [
              "/workspace",
              "/workspace/.home",
              "/workspace/runtime.json",
              "/workspace/.skills-draft",
              "/tmp"
            ],
            "codex": [
              "/workspace/AGENTS.md",
              "/workspace/.home/.codex/AGENTS.md",
              "/workspace/.home/.codex/config.toml",
              "/workspace/.home/.codex/mcp.json",
              "/workspace/.home/.codex/skills"
            ],
            "claude_code": [
              "/workspace/CLAUDE.md",
              "/workspace/.mcp.json",
              "/workspace/.claude/settings.json",
              "/workspace/.claude/skills",
              "/workspace/.home/.claude/CLAUDE.md"
            ],
            "goose": [
              "/workspace/AGENTS.md",
              "/workspace/.home/.config/goose/AGENTS.md",
              "/workspace/.home/.config/goose/config.yaml",
              "/workspace/.home/.config/goose/mcp.json",
              "/workspace/.home/.agents/skills",
              "/workspace/.home/.acp-usage"
            ]
          }
        },
        "implementation": [
          "处理位置：各 agent 的 Dockerfile 先统一 WORKSPACE_DIR=/workspace、HOME=/workspace/.home、WORKDIR=/workspace；启动时再由 src/config.ts 根据 /_cp/config、/_cp/skills、/_cp/credentials 写入本地文件。",
          "平台 prompt：internal/platform-prompt 的 writePlatformPrompt() 使用 process.env.HOME + homeSubdir + filename，所以三种 agent 的平台提示词都落在 /workspace/.home 下。",
          "业务 prompt：Codex 和 Goose 写 /workspace/AGENTS.md；Claude Code 写 /workspace/CLAUDE.md。内容来自 config.prompt_content || config.system_prompt。",
          "Skills：发布版本一般下载到 /tmp 后解压，再 symlink 到 agent 的 skillsDir；编辑态草稿放在 /workspace/.skills-draft，避免被下一次 loadSkills 覆盖。",
          "Credentials：env 型凭据写入 /workspace/.home/.agent-credentials.env，并由 /workspace/.home/.bashrc、/workspace/.home/.profile source；file 型凭据如果路径以 ~ 开头，会展开到 /workspace/.home。",
          "Usage：Goose ACP usage 写 /workspace/.home/.acp-usage/*.jsonl；Codex/Claude usage 读取 HOME 下各自工具的运行记录，具体格式看 Usage 解析模块。",
          "挂载边界：/workspace 是 agent 主要可见工作区；/tmp 是运行期临时位置；/mnt/memory 和 /mnt/afs 是 sidecar 挂载入口，不是 agent 自己写 config 时生成的目录。"
        ],
        "codeBlocks": [
          {
            "title": "三种镜像共同的基础目录",
            "language": "text",
            "code": "/workspace\n├── .home/                         # HOME=/workspace/.home，node 用户 home\n│   ├── .agent-credentials.env      # env 型凭据，存在凭据时生成\n│   ├── .bashrc                     # source .agent-credentials.env\n│   ├── .profile                    # source .agent-credentials.env\n│   └── .ssh/\n│       └── config                  # 凭据文件/SSH 配置需要时生成\n├── .skills-draft/                  # skill 编辑态草稿，持久在 workspace 内\n├── runtime.json                    # agent 运行时信息\n└── <业务 prompt 文件>              # Codex/Goose: AGENTS.md；Claude: CLAUDE.md\n\n/tmp\n└── <downloaded-skill-packages>/     # 发布版 skill package 的临时下载/解压位置，可重建\n\n/mnt\n├── memory/<store_id>/              # 启用 memory sidecar 后才存在\n└── afs/<share_or_mount>/            # 启用 AFS sidecar 后才存在"
          },
          {
            "title": "Codex 启动后常见目录结构",
            "language": "text",
            "code": "/workspace\n├── AGENTS.md                       # workspace 业务 prompt：prompt_content || system_prompt\n├── runtime.json\n├── .skills-draft/\n│   └── <skill_name>/               # 编辑中的 skill 草稿\n└── .home/\n    ├── .agent-credentials.env\n    ├── .ssh/\n    │   └── config\n    └── .codex/\n        ├── AGENTS.md               # 平台 prompt，由 internal/platform-prompt 渲染\n        ├── config.toml             # Codex provider/model/sandbox 等配置\n        ├── mcp.json                # MCP server 配置\n        ├── auth.json               # 使用 _apiKey 时生成\n        └── skills/\n            └── <skill_name> -> /tmp/<package> 或草稿目录 symlink"
          },
          {
            "title": "Claude Code 启动后常见目录结构",
            "language": "text",
            "code": "/workspace\n├── CLAUDE.md                       # workspace 业务 prompt：prompt_content || system_prompt\n├── .mcp.json                       # Claude Code 项目级 MCP 配置\n├── runtime.json\n├── .skills-draft/\n│   └── <skill_name>/               # 编辑中的 skill 草稿\n├── .claude/\n│   ├── settings.json               # Claude Code 项目级设置\n│   └── skills/\n│       └── <skill_name> -> /tmp/<package> 或草稿目录 symlink\n└── .home/\n    ├── .agent-credentials.env\n    ├── .ssh/\n    │   └── config\n    └── .claude/\n        └── CLAUDE.md               # 平台 prompt，由 internal/platform-prompt 渲染"
          },
          {
            "title": "Goose 启动后常见目录结构",
            "language": "text",
            "code": "/workspace\n├── AGENTS.md                       # workspace 业务 prompt：prompt_content || system_prompt\n├── runtime.json\n├── .skills-draft/\n│   └── <skill_name>/               # 编辑中的 skill 草稿\n└── .home/\n    ├── .agent-credentials.env\n    ├── .ssh/\n    │   └── config\n    ├── .agents/\n    │   └── skills/\n    │       └── <skill_name> -> /tmp/<package> 或草稿目录 symlink\n    ├── .acp-usage/\n    │   └── <session_id>.jsonl       # Goose ACP usage 增量记录\n    └── .config/\n        └── goose/\n            ├── AGENTS.md           # 平台 prompt，由 internal/platform-prompt 渲染\n            ├── config.yaml         # Goose provider/model/extensions 配置\n            └── mcp.json            # MCP server 配置"
          },
          {
            "title": "启动完成后怎么观察",
            "language": "bash",
            "code": "pwd\n# /workspace\n\necho \"$HOME\"\n# /workspace/.home\n\nfind /workspace -maxdepth 3 -type f | sort\nfind /workspace/.home -maxdepth 4 -type f | sort\nls -la /mnt/memory 2>/dev/null || true\nls -la /mnt/afs 2>/dev/null || true"
          }
        ],
        "relatedTables": [
          "workspaces",
          "workspace_configs",
          "prompts",
          "prompt_versions",
          "skills",
          "skill_versions",
          "workspace_skills",
          "user_credentials",
          "user_credential_workspaces"
        ],
        "db": [],
        "note": "这个章节是容器内部视角：它不新增数据库关系，而是把前面 config、skills、credentials、memory、AFS 加载后的最终落盘位置串起来。目录是否存在取决于对应能力是否启用。"
      }
    ],
    "quickLinks": [
      {
        "label": "Service Token 表",
        "moduleId": "schema",
        "domain": "runtime",
        "table": "service_tokens"
      },
      {
        "label": "Session Token 表",
        "moduleId": "schema",
        "domain": "runtime",
        "table": "session_tokens"
      }
    ]
  },
  "agentQuestions": {
    "title": "Question 流程：Agent 提问到用户回答",
    "intro": "这里说的 Question 指 Claude Code 的 AskUserQuestion：agent 在同一轮 chat turn 中暂停，把问题交给人；用户回答后，原来的 SDK 调用继续执行。它和 Builder approval 不一样：不是一条持久化审批任务，而是 agent 进程内一个等待中的 tool permission promise。注意：这条完整 Question 流程当前主要属于 Claude Code adapter；Codex/Goose 也可以向用户提问，但方式是普通 assistant 文本提问，当前 turn 结束，用户下一条消息再通过 /chat 继续；它们不支持 /pending-question + /respond 这种 AskUserQuestion 挂起补答。",
    "quickLinks": [
      {
        "label": "数据库结构：sessions",
        "moduleId": "schema",
        "domain": "runtime",
        "table": "sessions"
      },
      {
        "label": "Agent 启动",
        "moduleId": "agentStartup"
      },
      {
        "label": "Config 加载",
        "moduleId": "configLoad"
      },
      {
        "label": "Workspace 平台 tools",
        "moduleId": "workspace"
      }
    ],
    "steps": [
      {
        "title": "总览：一次 Question 是同一轮 turn 的暂停和继续",
        "actor": "Browser / control-plane / agent / Claude SDK",
        "request": "SSE lifecycle: session.started -> question.requested -> respond -> session.ended",
        "url": "POST /api/workspaces/ws_claude_demo/chat, then POST /_proxy/agent/ws_claude_demo/sessions/sess_9x7/respond",
        "params": {
          "workspace_id": "ws_claude_demo",
          "session_id": "sess_9x7",
          "request_id": "req_8f3c",
          "answers": {
            "env": "prod",
            "region": "ap-southeast-1"
          }
        },
        "paramNotes": [
          "workspace_id：用户当前打开的 workspace，CP 用它做 ACL 和定位 agent pod。",
          "session_id：agent 返回的会话 ID，后续 pending-question、respond、reconnect 都靠它回到同一个会话。",
          "request_id：agent 进程为这次 AskUserQuestion 生成的 UUID，用来找到 pendingResponses 里的 resolver。",
          "answers：用户在前端表单里填写的回答，字段名来自 agent 发出的 questions。"
        ],
        "response": {
          "sse": [
            {
              "type": "session.started",
              "session_id": "sess_9x7"
            },
            {
              "type": "question.requested",
              "session_id": "sess_9x7",
              "request_id": "req_8f3c",
              "questions": [
                {
                  "id": "env",
                  "label": "部署到哪个环境？"
                }
              ]
            },
            {
              "type": "session.ended",
              "session_id": "sess_9x7",
              "reason": "complete"
            }
          ],
          "respond": {
            "status": 200,
            "body": {
              "success": true
            }
          }
        },
        "implementation": [
          "核心抽象：Question 不是新开一轮 chat，而是当前 turn 中 Claude SDK 的 AskUserQuestion tool 在 canUseTool 回调里被拦住。",
          "agent 暂停点：agent.ts 保存 requestId -> resolver，这个 resolver 不被调用，SDK 就不会继续。",
          "control-plane 作用：CP 不回答问题；它负责鉴权、转发、把 session 状态切到 human，并把 question.requested 广播给 UI。",
          "用户回答后：UI 把 requestId 和 answers 发给 CP proxy，CP 转发到 agent 的 /sessions/:id/respond，agent 调 resolver，SDK 继续跑。"
        ],
        "codeBlocks": [
          {
            "title": "泳道图：从提问到回答",
            "code": "sequenceDiagram\n  participant User as 用户\n  participant UI as 浏览器 UI\n  participant CP as control-plane\n  participant SSE as CP SSE interceptor\n  participant Agent as claude-code agent\n  participant SDK as Claude SDK\n  participant DB as 数据库\n  User->>UI: 输入需求并发送\n  UI->>CP: POST /api/workspaces/:id/chat\n  CP->>Agent: POST /chat\n  Agent->>SDK: query(...canUseTool)\n  SDK->>Agent: AskUserQuestion\n  Agent->>Agent: 保存 pendingResponses/requestId\n  Agent-->>SSE: question.requested SSE\n  SSE->>DB: UPDATE sessions.chat_status=human\n  SSE-->>UI: 广播 question.requested\n  User->>UI: 填写答案\n  UI->>CP: POST /_proxy/agent/:wid/sessions/:sid/respond\n  CP->>Agent: POST /sessions/:sid/respond\n  Agent->>Agent: respondToQuestion 调 resolver\n  Agent-->>SDK: updatedInput 带 answers\n  SDK-->>Agent: 继续生成后续事件\n  Agent-->>SSE: item.delta/session.ended\n  SSE->>DB: UPDATE sessions.chat_status=idle\n  SSE-->>UI: 广播完成事件"
          }
        ],
        "db": [
          {
            "table": "sessions",
            "op": "UPDATE",
            "values": {
              "id": "sess_9x7",
              "chat_status": "human -> agent -> idle"
            }
          }
        ],
        "note": "先记住一句话：CP 维护“会话该谁处理”的状态，agent 维护“这个问题等哪个 answer”的内存状态。"
      },
      {
        "title": "用户发送第一条消息：CP 建立受控的 SSE turn",
        "actor": "Browser -> control-plane -> agent",
        "request": "POST /api/workspaces/:id/chat",
        "url": "POST http://127.0.0.1:30080/api/workspaces/ws_claude_demo/chat",
        "params": {
          "message": "帮我部署这个服务，缺参数就问我",
          "session_id": null,
          "images": [],
          "mode": "stream"
        },
        "paramNotes": [
          "message：用户在前端输入框提交的自然语言。前端阻止默认表单刷新，调用 chat API，并开始读取 SSE。",
          "session_id：第一次会话为空；继续旧会话时前端会带已有 session id，agent 用它 resume。",
          "images：可选附件，Claude adapter 会写到 workspace 附件目录并把说明拼进输入。",
          "mode：示例值，重点是 Accept/SSE；CP 返回 text/event-stream 后，UI 按事件逐条更新界面。"
        ],
        "response": {
          "status": 200,
          "headers": {
            "Content-Type": "text/event-stream"
          },
          "firstEvents": [
            "data: {\"type\":\"session.started\",\"session_id\":\"sess_9x7\"}",
            "data: {\"type\":\"item.delta\",\"delta\":\"我需要确认部署环境...\"}"
          ]
        },
        "implementation": [
          "前端处理：用户点击发送或按 Enter 后，组件把输入框里的字符串组成 body，请求 /api/workspaces/:id/chat，并监听返回的 ReadableStream/EventSource。",
          "CP chat route：control-plane/src/routes/workspaces/chat.ts 校验 workspace 存在、当前用户 canManage，然后调用 dispatchChatTurn。",
          "CP 转发：dispatch/executeChat 找到 workspace 对应 agent 地址，把请求转成 agent 本地 /chat，并用 createInterceptedSSEResponse 包住 agent 返回的 SSE。",
          "为什么要包 SSE：CP 要在不改变 agent 事件格式的前提下，补数据库状态、消息持久化、广播连接管理和结束清理。"
        ],
        "codeBlocks": [
          {
            "title": "源码锚点",
            "code": "control-plane/src/routes/workspaces/chat.ts:120-136\ncontrol-plane/src/lib/sse.ts:324-390\nagents/claude-code/src/server.ts:116-150"
          }
        ],
        "db": [
          {
            "table": "sessions",
            "op": "INSERT/UPDATE",
            "values": {
              "workspace_id": "ws_claude_demo",
              "chat_status": "agent"
            }
          },
          {
            "table": "messages",
            "op": "INSERT",
            "values": {
              "role": "user",
              "content": "帮我部署这个服务，缺参数就问我"
            }
          }
        ],
        "note": "这一阶段用户还没有看到 Question 表单；页面只是在正常接收 agent 的 SSE 输出。"
      },
      {
        "title": "canUseTool 是谁调用的：Claude SDK 的工具调用前拦截器",
        "actor": "Claude SDK -> claude-code adapter",
        "request": "query({ canUseTool }) internal callback",
        "url": "LOCAL @anthropic-ai/claude-agent-sdk query(...) calls canUseTool before tool execution",
        "params": {
          "queryOption": "canUseTool: async (toolName, input, { signal }) => PermissionResult | Promise<PermissionResult>",
          "calledWhen": "模型准备使用某个 tool 时，由 Claude SDK 调用，不是 HTTP 接口，也不是 control-plane 主动调用。",
          "normalReturn": {
            "behavior": "allow",
            "updatedInput": "input"
          },
          "questionReturn": "Promise pending，直到用户 POST /respond 后 resolve"
        },
        "paramNotes": [
          "queryOption：claude-code/src/agent.ts 调 query(...) 时传进去的回调函数。它是 adapter 给 Claude SDK 的“工具使用策略”。",
          "calledWhen：模型在一轮生成中决定调用工具，SDK 执行工具前会问 adapter：这个工具能不能用、输入要不要改。",
          "normalReturn：对于非 AskUserQuestion 的工具，当前代码直接 allow，相当于不拦截。",
          "questionReturn：对于 AskUserQuestion，adapter 不立即返回 allow，而是返回一个未完成的 Promise，让 SDK 当前 turn 暂停。"
        ],
        "response": {
          "nonQuestionTool": {
            "behavior": "allow",
            "updatedInput": "original input"
          },
          "askUserQuestion": "Promise waits; later resolved with { behavior: \"allow\", updatedInput: { ...originalInput, answers } }"
        },
        "implementation": [
          "从调用方向看：不是 CP 调 canUseTool，也不是前端调 canUseTool；是 Claude SDK 在 query() 内部执行 tool 前调用它。",
          "claude-code adapter 把 canUseTool 当成“工具调用闸口”：看到 toolName 不是 AskUserQuestion，就直接放行。",
          "看到 toolName 是 AskUserQuestion，就生成 requestId，把 questions 发成 question.requested SSE，同时把 Promise resolver 存到 pendingResponses。",
          "因为 canUseTool 返回的是 Promise，所以 SDK 会自然等待；用户回答后 respondToQuestion 调 resolver，SDK 才拿到 updatedInput 继续执行。",
          "这也是为什么 Question 能在同一轮 turn 里继续：它不是新建一轮 chat，而是把原本悬挂的 canUseTool 返回值补上。"
        ],
        "codeBlocks": [
          {
            "title": "源码：canUseTool 挂在 Claude SDK query options 上",
            "code": "// D:/Code/koala/agent-platform/agents/claude-code/src/agent.ts:253\nconst messageStream = query({\n  prompt,\n  options: {\n    cwd: WORKSPACE_DIR,\n    includePartialMessages: true,\n    canUseTool: async (toolName, input, { signal }) => {\n      if (toolName !== \"AskUserQuestion\") {\n        return { behavior: \"allow\", updatedInput: input }\n      }\n\n      const requestId = crypto.randomUUID()\n      const questions = input.questions\n      callbacks.onAskUser?.({ requestId, questions })\n\n      return new Promise((resolve, reject) => {\n        pendingResponses.set(requestId, { resolve, originalInput: input })\n        signal.addEventListener(\"abort\", () => {\n          pendingResponses.delete(requestId)\n          reject(signal.reason ?? new Error(\"aborted\"))\n        })\n      })\n    }\n  }\n})"
          },
          {
            "title": "人类理解版调用栈",
            "code": "用户消息 -> agent.ts query(...) -> Claude SDK 开始生成\n  -> 模型决定调用工具\n    -> SDK 调用 canUseTool(toolName, input, signal)\n      -> 普通工具：立即返回 allow\n      -> AskUserQuestion：返回 pending Promise\n        -> agent 发 question.requested SSE 给 CP/UI\n        -> 用户回答 POST /respond\n        -> respondToQuestion 调 resolve\n    -> SDK 拿到 updatedInput 后继续这一轮 turn"
          }
        ],
        "db": [],
        "relatedTables": [
          "sessions"
        ],
        "note": "把 canUseTool 理解成“Claude SDK 工具执行前问 adapter 要不要放行”的钩子。Question 只是这个钩子的一种特殊返回方式：先不回答 SDK，等人回答后再 resolve。"
      },
      {
        "title": "agent 触发 AskUserQuestion：问题先存在 agent 内存里",
        "actor": "Claude SDK -> claude-code agent",
        "request": "SDK canUseTool(\"AskUserQuestion\", input)",
        "url": "LOCAL agents/claude-code/src/agent.ts canUseTool callback",
        "params": {
          "toolName": "AskUserQuestion",
          "input": {
            "questions": [
              {
                "id": "env",
                "label": "部署到哪个环境？",
                "type": "text"
              }
            ]
          },
          "resultSessionId": "sess_9x7"
        },
        "paramNotes": [
          "toolName：Claude SDK 请求使用的工具名。当前 adapter 只特殊处理 AskUserQuestion，其他工具直接 allow。",
          "input.questions：模型提出的问题数组，后续 UI 表单字段来自这里。",
          "resultSessionId：SDK 确认出来的底层会话 ID；有它以后，agent 才能把 pending question 按 session 保存，方便刷新后查回。"
        ],
        "response": {
          "emittedToSSE": {
            "type": "question.requested",
            "session_id": "sess_9x7",
            "request_id": "req_8f3c",
            "questions": [
              {
                "id": "env",
                "label": "部署到哪个环境？"
              }
            ]
          },
          "sdkReturn": "Promise pending until /respond resolves it"
        },
        "implementation": [
          "agent.ts 生成 requestId = crypto.randomUUID()。",
          "pendingResponses.set(requestId, { resolve, originalInput })：这是“等待用户回答”的真正暂停点。",
          "pendingQuestionsBySession.set(sessionId, { requestId, questions })：这是给 UI 刷新恢复用的查询索引。",
          "universal-events.ts 的 questionRequested() 把 requestId/questions 转成统一 SSE 事件 question.requested。",
          "如果 abort signal 触发，agent 会删除 pendingResponses 和 pendingQuestionsBySession，避免悬挂等待。"
        ],
        "codeBlocks": [
          {
            "title": "agent 进程内数据结构",
            "code": "pendingResponses: Map<requestId, {\n  resolve: (result) => void,\n  originalInput: { questions, ... }\n}>\n\npendingQuestionsBySession: Map<sessionId, {\n  requestId,\n  questions\n}>\n\n示例：\npendingResponses[\"req_8f3c\"] = {\n  resolve: <Promise resolver>,\n  originalInput: { questions: [{ id: \"env\", label: \"部署到哪个环境？\" }] }\n}\npendingQuestionsBySession[\"sess_9x7\"] = {\n  requestId: \"req_8f3c\",\n  questions: [{ id: \"env\", label: \"部署到哪个环境？\" }]\n}"
          },
          {
            "title": "源码锚点",
            "code": "agents/claude-code/src/agent.ts:72-88\nagents/claude-code/src/agent.ts:263-291\nagents/claude-code/src/universal-events.ts:70-77"
          }
        ],
        "db": [],
        "relatedTables": [
          "sessions"
        ],
        "note": "这一步没有把 pending question 作为权威数据写进数据库。agent 如果在等待回答期间重启，这个内存 pending 状态就不是天然可恢复的。"
      },
      {
        "title": "CP 处理 question.requested：先改状态，再让 UI 看见问题",
        "actor": "agent SSE -> control-plane SSE interceptor -> Browser",
        "request": "SSE event question.requested",
        "url": "EVENT data: {\"type\":\"question.requested\",...}",
        "params": {
          "type": "question.requested",
          "session_id": "sess_9x7",
          "request_id": "req_8f3c",
          "questions": [
            {
              "id": "env",
              "label": "部署到哪个环境？"
            }
          ]
        },
        "paramNotes": [
          "type：CP 用它判断这是“交回人处理”的事件。",
          "session_id：CP 用它更新 sessions.chat_status。",
          "request_id：CP 不消费它，只原样广播给 UI；后续 UI 回答时必须带回 agent。",
          "questions：CP 不理解业务含义，只透传给前端渲染表单。"
        ],
        "response": {
          "browserSse": "data: {\"type\":\"question.requested\",\"session_id\":\"sess_9x7\",\"request_id\":\"req_8f3c\",\"questions\":[...]}",
          "visibleEffect": "UI 停止显示 agent 正在输出，切成等待用户回答的表单状态。"
        },
        "implementation": [
          "sse.ts 的 persist plugin 收到 question.requested 后，queue.run(() => transitionSessionStatus(sessionId, \"human\"))。",
          "sse.ts 的 broadcast plugin 对 question.requested 不立刻 emit，而是 enqueueDeferredEmit(rawData)。",
          "因为 persist 和 broadcast 使用同一个串行 queue，所以浏览器收到 question.requested 时，数据库里 session 已经是 human。",
          "这个顺序很重要：否则 UI 可能先看到问题，再查 session 列表却还是 agent，状态会闪烁或误判。"
        ],
        "codeBlocks": [
          {
            "title": "源码锚点",
            "code": "control-plane/src/lib/sse.ts:1117-1123\ncontrol-plane/src/lib/sse.ts:1213-1223"
          },
          {
            "title": "状态 SQL 的人类等价写法",
            "code": "UPDATE sessions\nSET chat_status = 'human', updated_at = now()\nWHERE id = 'sess_9x7';"
          }
        ],
        "db": [
          {
            "table": "sessions",
            "op": "UPDATE",
            "values": {
              "id": "sess_9x7",
              "chat_status": "human"
            }
          }
        ],
        "note": "CP 在这里像交通警察：它告诉整个系统“这轮现在轮到人处理”，但它不替用户回答，也不把问题变成审批任务。"
      },
      {
        "title": "UI 收到 question.requested 后如何渲染",
        "actor": "agent SSE -> web SSE parser -> session store -> AskUserQuestionPanel",
        "request": "SSE event question.requested",
        "url": "EVENT data: {\"type\":\"question.requested\",\"request_id\":\"req_8f3c\",\"questions\":[...]}",
        "params": {
          "request_id": "req_8f3c",
          "questions": [
            {
              "question": "部署到哪个环境？",
              "header": "部署环境",
              "options": [
                {
                  "label": "prod",
                  "description": "生产环境"
                },
                {
                  "label": "staging",
                  "description": "预发环境"
                }
              ]
            }
          ]
        },
        "paramNotes": [
          "web/src/lib/api/sse.ts 只负责把 question.requested 事件里的 request_id/questions 转成 AskUserRequest。",
          "agent-session-store.ts 的 onQuestionRequested 把 AskUserRequest 写入 pendingQuestion。",
          "WorkspaceChatPanel 订阅 pendingQuestion；只要 pendingQuestion 不为空，就在消息区下方渲染 AskUserQuestionPanel。",
          "AskUserQuestionPanel 是普通 React 表单：每个 question 渲染标题、问题文本、选项按钮和自定义输入框；确认按钮 disabled 状态由 allAnswered 控制。",
          "点击确认时，panel 把本地 selections/customInputs 组装成 answers，然后调用 actions.respondToQuestion。"
        ],
        "response": {
          "storeState": {
            "pendingQuestion": {
              "requestId": "req_8f3c",
              "questions": "原样来自 SSE"
            }
          },
          "visibleEffect": "页面出现一个等待用户回答的表单，而不是把问题当普通 markdown 消息渲染。"
        },
        "implementation": [
          "sse.ts: question.requested -> handlers.onQuestionRequested({ requestId, questions })。",
          "agent-session-store.ts: onQuestionRequested -> setState({ pendingQuestion: request })。",
          "WorkspaceChatPanel.tsx: pendingQuestion && <AskUserQuestionPanel request={pendingQuestion} onRespond={actions.respondToQuestion} />。",
          "AskUserQuestionPanel.tsx: buildAnswers 用问题文本作为 key，把选中的 option label 或自定义输入作为 value。",
          "respondToQuestion 先把 pendingQuestion 清空，再 POST /_proxy/agent/:workspaceId/sessions/:sessionId/respond；它不会新开一轮 /chat。"
        ],
        "codeBlocks": [
          {
            "title": "源码锚点",
            "code": "web/src/lib/api/sse.ts:101-107\nweb/src/stores/agent-session-store.ts:615-616\nweb/src/components/workspace/WorkspaceChatPanel.tsx:930-931\nweb/src/components/workspace/AskUserQuestionPanel.tsx:6-83\nweb/src/stores/agent-session-store.ts:824-835"
          },
          {
            "title": "UI 状态流",
            "code": "question.requested SSE\n  -> onQuestionRequested({ requestId, questions })\n  -> store.pendingQuestion = request\n  -> WorkspaceChatPanel render AskUserQuestionPanel\n  -> 用户填写并确认\n  -> respondToQuestion(requestId, answers)\n  -> POST /sessions/:sessionId/respond"
          }
        ],
        "db": [],
        "relatedTables": [
          "sessions"
        ],
        "note": "这里的渲染入口是 pendingQuestion 状态，不是 markdown parser，也不是消息列表里的普通 assistant message。"
      },
      {
        "title": "用户不回答又发送新输入：正常 UI 是排队，不是新开 turn",
        "actor": "Browser UI / control-plane / claude-code agent",
        "request": "User submits composer while AskUserQuestion is pending",
        "url": "正常 UI：不会立刻 POST /api/workspaces/:id/chat；接口打穿：可能强行 POST /chat",
        "params": {
          "currentState": {
            "pendingQuestion": "req_8f3c",
            "isLoading": true,
            "sessionChatStatus": "human",
            "agentPending": "pendingResponses has req_8f3c"
          },
          "newInput": "那先改成 staging 吧"
        },
        "paramNotes": [
          "Claude Code 提问时，原 /chat SSE 流没有结束；agent 仍卡在 AskUserQuestion 的 canUseTool Promise。",
          "正常页面里 isLoading 仍然为 true；用户在 composer 里发送新输入时，WorkspaceChatPanel 和 store 都会走 updatePendingMessage。",
          "updatePendingMessage 只是把新输入保存成 pendingMessage，不会马上调用 createAgentChat，也不会 POST 新的 /chat。",
          "CP 的 drainPendingMessage 只在当前 turn session.ended 后尝试执行；如果用户一直不回答 AskUserQuestion，当前 turn 不结束，pendingMessage 也不会被 drain。",
          "agent 侧旧 requestId 仍留在 pendingResponses/pendingQuestionsBySession；只有 respond、abort、中断或进程重启等路径会清掉。"
        ],
        "response": {
          "normalUiResult": "新输入变成 pendingMessage，等待当前 Question 被回答并结束后再进入下一轮。",
          "agentResult": "Claude SDK 继续等待 AskUserQuestion 的 resolver；不会因为用户在输入框又写了一句就自动跳过问题。",
          "forcedApiResult": "如果绕过 UI 强行发 POST /chat，CP 会按新 turn 处理并把 session 切回 agent；当前代码没有显式取消旧 pendingResponses。"
        },
        "implementation": [
          "WorkspaceChatPanel.handleSubmit: if (isLoading) actions.updatePendingMessage(...); return。",
          "agent-session-store.sendMessage 也有同样保护：如果 isLoading，就 updatePendingMessage 后返回。",
          "control-plane/src/lib/sse.ts 收到 question.requested 后把 session 切到 human，同时 activeStream.turnActive=false，表示当前 turn 交回人。",
          "control-plane/src/lib/sse.ts 只在 session.ended 的路径里触发 drainPendingMessage；Question 未回答时没有 session.ended，所以排队消息不会执行。",
          "如果通过接口强行发新 /chat，executeChat 会 transitionSessionStatus(sessionId, 'agent') 并把用户新消息持久化，然后转发 agent /chat。",
          "Claude agent 的 /chat 会启动/恢复一次新的 SDK query；但 agent.ts 没有在新 /chat 开始时删除旧 requestId，所以旧 AskUserQuestion 仍可能悬挂。"
        ],
        "codeBlocks": [
          {
            "title": "源码锚点",
            "code": "web/src/components/workspace/WorkspaceChatPanel.tsx:488-495\nweb/src/stores/agent-session-store.ts:738-741\ncontrol-plane/src/lib/sse.ts:1117-1123\ncontrol-plane/src/lib/sse.ts:587-589\ncontrol-plane/src/services/chat/executeChat.ts:184-188\nagents/claude-code/src/agent.ts:72-88\nagents/claude-code/src/agent.ts:284-291"
          },
          {
            "title": "正常 UI 的等价流程",
            "code": "AskUserQuestion pending\n  -> UI isLoading=true\n  -> 用户发送新输入\n  -> updatePendingMessage(newInput)\n  -> 不 POST /chat\n  -> 等用户回答原问题\n  -> 原 turn 继续并 session.ended\n  -> CP 才可能 drain pendingMessage"
          },
          {
            "title": "强行接口打穿的边界",
            "code": "POST /chat(session_id=sess_9x7, message=新问题)\n  -> CP transitionSessionStatus(sess_9x7, 'agent')\n  -> CP 转发 agent /chat\n  -> agent 开始新的 chat/resume\n  -> 旧 pendingResponses[req_8f3c] 没有被这条路径自动删除"
          }
        ],
        "db": [
          {
            "table": "sessions",
            "op": "UPDATE",
            "values": {
              "normal_question": "chat_status = human",
              "forced_new_chat": "chat_status = agent"
            }
          }
        ],
        "relatedTables": [
          "sessions"
        ],
        "note": "所以“跳过不答再发消息”在正常 UI 中不是跳过，而是排队；真正跳过只能靠中断/取消或接口层绕过，而当前 agent 代码没有把绕过路径做成一个干净的 pending question 取消流程。"
      },
      {
        "title": "页面刷新后的恢复：UI 向同一个 agent 查询 pending question",
        "actor": "Browser -> CP proxy -> claude-code agent",
        "request": "GET /_proxy/agent/:workspaceId/sessions/:sessionId/pending-question",
        "url": "GET http://127.0.0.1:30080/_proxy/agent/ws_claude_demo/sessions/sess_9x7/pending-question",
        "params": {
          "workspaceId": "ws_claude_demo",
          "sessionId": "sess_9x7"
        },
        "paramNotes": [
          "workspaceId：来自当前页面路由或 workspace 上下文，CP 用它检查当前用户是否能访问这个 workspace。",
          "sessionId：来自 URL、session 列表或 SSE 事件，CP 用它 resolveAgentAddress(workspace.id, { sessionId })，尽量路由回原来的 agent replica。"
        ],
        "response": {
          "status": 200,
          "body": {
            "requestId": "req_8f3c",
            "questions": [
              {
                "id": "env",
                "label": "部署到哪个环境？"
              }
            ]
          },
          "emptyCase": null
        },
        "implementation": [
          "CP proxy route 先校验 workspace 存在、用户是 owner 或 system workspace 的 admin，并要求 workspace.status === running。",
          "CP 从路径 /sessions/:sid/... 里解析 sessionId，然后 resolveAgentAddress 按 sessionId 找到应该转发到哪个 agent。",
          "agent server.ts 的 GET /sessions/:id/pending-question 调 getPendingQuestion(sessionId)。",
          "getPendingQuestion 只是读 pendingQuestionsBySession 这个 Map：如果进程还活着就返回问题；如果 agent 重启或 pending 已清理，就返回 null。"
        ],
        "codeBlocks": [
          {
            "title": "源码锚点",
            "code": "control-plane/src/routes/proxy.ts:36-80\nagents/claude-code/src/server.ts:268-273\nagents/claude-code/src/agent.ts:117-121"
          }
        ],
        "db": [],
        "relatedTables": [
          "sessions"
        ],
        "note": "这个恢复是“同一个运行中 agent 进程的内存恢复”，不是“从数据库恢复 pending question”。这点和持久化审批流不同。"
      },
      {
        "title": "用户提交答案：CP 只鉴权和转发，agent 唤醒 SDK",
        "actor": "Browser -> CP proxy -> claude-code agent -> Claude SDK",
        "request": "POST /_proxy/agent/:workspaceId/sessions/:sessionId/respond",
        "url": "POST http://127.0.0.1:30080/_proxy/agent/ws_claude_demo/sessions/sess_9x7/respond",
        "params": {
          "requestId": "req_8f3c",
          "answers": {
            "env": "prod",
            "region": "ap-southeast-1"
          }
        },
        "paramNotes": [
          "requestId：来自 question.requested 事件或 pending-question 返回值。agent 用它 pendingResponses.get(requestId)。",
          "answers：来自用户在前端表单中填写/选择的值。字段名要能被 AskUserQuestion 的 questions 对应上。"
        ],
        "response": {
          "status": 200,
          "body": {
            "success": true
          },
          "failureBody": {
            "success": false,
            "meaning": "agent 进程里没有这个 requestId，通常表示已回答、已 abort、agent 重启或路由错了"
          }
        },
        "implementation": [
          "前端处理：用户点击提交后，前端把表单 state 组织成 answers，并把原 question.requested 里的 requestId 一起 POST 给 CP proxy。",
          "CP proxy：不理解 answers 的业务含义，只做 ACL、workspace running 检查、按 sessionId 转发。",
          "agent server.ts：POST /sessions/:id/respond 校验 requestId 必填，然后调用 respondToQuestion(requestId, answers)。",
          "respondToQuestion：找到 pendingResponses 里的 resolver，resolve({ behavior: \"allow\", updatedInput: { ...originalInput, answers } })。",
          "为什么要合并 originalInput：Claude SDK 会重新执行内置 AskUserQuestion 工具，代码里会访问 questions.map，所以不能只传 answers，必须保留原来的 questions。",
          "清理：resolve 后删除 pendingResponses，并从 pendingQuestionsBySession 删除对应 session 的问题。"
        ],
        "codeBlocks": [
          {
            "title": "源码锚点",
            "code": "control-plane/src/routes/proxy.ts:58-80\nagents/claude-code/src/server.ts:275-287\nagents/claude-code/src/agent.ts:101-115"
          },
          {
            "title": "answer 唤醒 pending promise 的等价过程",
            "code": "const pending = pendingResponses.get(\"req_8f3c\")\nif (!pending) return { success: false }\n\npending.resolve({\n  behavior: \"allow\",\n  updatedInput: {\n    ...pending.originalInput,\n    answers: { env: \"prod\", region: \"ap-southeast-1\" }\n  }\n})\n\npendingResponses.delete(\"req_8f3c\")\npendingQuestionsBySession.delete(\"sess_9x7\")"
          }
        ],
        "db": [],
        "relatedTables": [
          "sessions"
        ],
        "note": "用户回答本身不是 CP 写库后再由 agent 轮询读取；它是一次直接回到 agent 进程的 respond 调用。"
      },
      {
        "title": "继续输出和 reconnect：状态从 human 回到 agent，再到 idle",
        "actor": "Browser / CP proxy / CP SSE interceptor / agent",
        "request": "POST /_proxy/agent/:workspaceId/sessions/:sessionId/reconnect",
        "url": "POST http://127.0.0.1:30080/_proxy/agent/ws_claude_demo/sessions/sess_9x7/reconnect",
        "params": {
          "workspaceId": "ws_claude_demo",
          "sessionId": "sess_9x7"
        },
        "paramNotes": [
          "workspaceId：同样用于 ACL 和 workspace running 检查。",
          "sessionId：要 reconnect 的会话。CP 会在处理 agent 返回的 SSE 前把这个 session 切回 agent。"
        ],
        "response": {
          "status": 200,
          "headers": {
            "Content-Type": "text/event-stream"
          },
          "events": [
            {
              "type": "item.delta",
              "delta": "我会按 prod/ap-southeast-1 继续部署..."
            },
            {
              "type": "session.ended",
              "session_id": "sess_9x7",
              "reason": "complete"
            }
          ]
        },
        "implementation": [
          "如果原 SSE 连接还在，answer resolve 后 agent 会继续往同一个流里吐事件。",
          "如果 UI 需要重新挂上流，CP proxy 的 /sessions/:sid/reconnect 会把 agent 的 SSE 再交给 createInterceptedSSEResponse。",
          "proxy.ts 对 reconnect SSE 有一个额外动作：先 transitionSessionStatus(sessionId, \"agent\")，表示这轮又交回 agent 执行。",
          "之后 sse.ts 的普通 item.delta/item.completed 会让 activeStream.turnActive = true；session.ended 或流结束后状态回到 idle。"
        ],
        "codeBlocks": [
          {
            "title": "源码锚点",
            "code": "control-plane/src/routes/proxy.ts:88-99\ncontrol-plane/src/lib/sse.ts:1213-1228\ncontrol-plane/src/lib/sse.ts:1130-1144\nagents/claude-code/src/server.ts:289-299"
          }
        ],
        "db": [
          {
            "table": "sessions",
            "op": "UPDATE",
            "values": {
              "id": "sess_9x7",
              "chat_status": "agent"
            }
          },
          {
            "table": "sessions",
            "op": "UPDATE",
            "values": {
              "id": "sess_9x7",
              "chat_status": "idle"
            }
          }
        ],
        "note": "从人的视角看，Question 就是页面从“agent 正在跑”切到“等我填答案”，提交后又切回“agent 继续跑”。数据库里最明显的观察点就是 sessions.chat_status 的变化。"
      },
      {
        "title": "为什么 Codex 没有 Question 流程：它走 ACP prompt，不支持 AskUserQuestion 补答",
        "actor": "Browser -> CP -> codex ACP adapter -> codex-acp",
        "request": "POST /api/workspaces/:id/chat -> agent /chat -> AcpBridge.prompt()",
        "url": "POST http://127.0.0.1:30080/api/workspaces/ws_codex_demo/chat",
        "params": {
          "message": "请帮我修改配置文件",
          "session_id": "optional existing codex session id",
          "capabilities": {
            "questions": false,
            "permissions": true,
            "reconnect": true
          },
          "unsupportedQuestionEndpoints": {
            "GET /sessions/:id/pending-question": null,
            "POST /sessions/:id/respond": {
              "status": 501,
              "body": {
                "success": false,
                "error": "Not supported"
              }
            }
          }
        },
        "paramNotes": [
          "message：用户输入仍然走同一个 CP chat API，然后 CP 转发到 Codex agent 的 /chat。",
          "session_id：Codex adapter 会复用已有 AcpBridge，或者 loadSession/createSession。",
          "capabilities.questions：agents/codex/src/server.ts 明确是 false，意思不是 Codex 不能问用户，而是不提供 Claude AskUserQuestion 那种挂起式补答能力。",
          "Goose 同样是 questions=false；它也可以通过普通 assistant 文本问用户，等用户下一轮 /chat 回答。",
          "capabilities.permissions：Codex 仍然可能有 ACP permission request，但当前 acp-server.ts 默认选择第一个 optionId 放行，这和“向用户提问题拿答案”不是同一件事。",
          "unsupportedQuestionEndpoints：ACP server 为了接口形状兼容保留了 pending-question/respond，但它们是 stub，不会产生 Claude 那种 pending Map。",
          "普通文本提问：如果 Codex/Goose 输出“请确认 prod 还是 staging”，这就是一条普通 assistant message；随后 session.ended，DB 状态回到 idle，用户下一条消息再进入 POST /chat。"
        ],
        "response": {
          "info": {
            "agent_type": "codex",
            "capabilities": {
              "questions": false,
              "permissions": true,
              "reconnect": true,
              "streaming_deltas": true
            }
          },
          "chatSse": [
            {
              "type": "session.started",
              "session_id": "codex_sess_123"
            },
            {
              "type": "item.delta",
              "delta": "正在修改配置..."
            },
            {
              "type": "session.ended",
              "reason": "completed"
            }
          ]
        },
        "implementation": [
          "Codex server.ts 调 createAcpAgentApp({ agentType: \"codex\", capabilities: { questions: false, permissions: true } })；Goose server.ts 也是 questions=false。",
          "用户提问时，internal/acp-adapter/acp-server.ts 的 POST /chat 读取 message/session_id/session_token。",
          "如果 session_id 已有，adapter 先尝试复用 sessionBridges；没有活 bridge 就 new AcpBridge 并 loadSession。",
          "如果是新会话，adapter new AcpBridge，调用 createSession({ mcpServers })，再发 session.started。",
          "真正把用户问题交给 Codex 的位置是 AcpBridge.prompt(sessionId, message, images)：它把 message 变成 ACP contentBlocks，然后调用 this.connection.prompt({ sessionId, prompt })。",
          "Codex 的中间事件通过 ACP session update 通知回来，AcpEventTranslator.translateUpdate(update) 转成统一 SSE item.started/item.delta/item.completed。",
          "如果 Codex 需要权限选择，ACP 会触发 onPermissionRequest；当前实现直接选择 req.options[0] 或 allow-once，并不会像 Claude AskUserQuestion 一样把问题发给用户等待答案。",
          "所以要分清两个层次：Codex/Goose 支持对话式向用户提问；但不支持结构化 AskUserQuestion，也不会把当前 tool call promise 挂起等待 /respond。",
          "你说的方案对应 Codex/Goose：assistant 用文本问问题 -> 当前 turn 正常 session.ended -> sessions.chat_status 回 idle -> 用户下一条输入作为新一轮 /chat -> 模型根据上下文继续。"
        ],
        "codeBlocks": [
          {
            "title": "源码：Codex 声明不支持 questions",
            "code": "// D:/Code/koala/agent-platform/agents/codex/src/server.ts:20\nconst { app } = createAcpAgentApp({\n  agentType: \"codex\",\n  capabilities: {\n    system_prompt: true,\n    mcp: true,\n    skills: false,\n    questions: false,\n    reconnect: true,\n    permissions: true,\n    streaming_deltas: true,\n  },\n})"
          },
          {
            "title": "源码：ACP chat 把用户 message 送给 bridge.prompt",
            "code": "// D:/Code/koala/agent-platform/internal/acp-adapter/acp-server.ts:271\napp.post(\"/chat\", async (c) => {\n  const body = await c.req.json()\n  const { message, session_id: sessionId, images, session_token: sessionToken } = body\n\n  return streamSSE(c, async (stream) => {\n    // 复用 / load / create ACP session\n    const sessionHandler = {\n      onUpdate(update) {\n        for (const evt of translator.translateUpdate(update)) {\n          sink.write(\"message\", JSON.stringify(evt))\n        }\n      },\n      async onPermissionRequest(req) {\n        const optionId = req.options?.[0]?.optionId ?? \"allow-once\"\n        return { outcome: { outcome: \"selected\", optionId } }\n      },\n    }\n\n    result = await bridge.prompt(currentSessionId, message, images)\n    await sink.write(\"message\", JSON.stringify(translator.sessionEnded(reason, stats)))\n  })\n})"
          },
          {
            "title": "源码：Codex 的 pending-question/respond 是 stub",
            "code": "// D:/Code/koala/agent-platform/internal/acp-adapter/acp-server.ts:600\napp.get(\"/sessions/:id/pending-question\", (c) => {\n  return c.json(null)\n})\n\napp.post(\"/sessions/:id/respond\", async (c) => {\n  return c.json({ success: false, error: \"Not supported\" }, 501)\n})"
          },
          {
            "title": "普通文本提问：Codex/Goose 的用户补充信息方式",
            "code": "例子：用户说“帮我部署”。Codex/Goose 如果缺少环境信息，会像普通 assistant 一样输出“请确认 prod 还是 staging？”。这条输出走 item.delta/item.completed，随后当前 turn 正常 session.ended，CP 把 sessions.chat_status 置回 idle。用户再输入“prod”时，UI 发起新的 POST /chat，并带同一个 session_id；Codex/Goose 根据会话上下文理解这是上一轮问题的答案，然后继续执行。这里没有 requestId，没有 pendingResponses，也没有 /respond。"
          },
          {
            "title": "和 Claude AskUserQuestion 的关键差异",
            "code": "Claude AskUserQuestion：同一轮 turn 内暂停工具调用，question.requested 后 session 状态为 human；用户答案通过 /respond 回到 agent 进程，resolve 原 canUseTool Promise，SDK 继续原 turn。\n\nCodex/Goose 普通文本提问：当前 turn 正常结束，session 状态回到 idle；用户答案是下一轮 /chat 的 message。它能完成“向用户要补充信息”这个交互效果，但不是结构化 pending question，也不能通过 /pending-question + /respond 补回原 tool call。"
          }
        ],
        "db": [
          {
            "table": "sessions",
            "op": "INSERT/UPDATE",
            "values": {
              "workspace_id": "ws_codex_demo",
              "chat_status": "agent -> idle"
            }
          }
        ],
        "note": "所以“Codex/Goose 如何向用户提问”的答案是：它们用普通 assistant 文本提问，并在下一轮 /chat 接收用户答案；不支持的是 Claude Code 那种同一 turn 内暂停 AskUserQuestion、通过 /respond 把答案补回原工具调用。"
      },
      {
        "title": "和 Builder approval 的边界：一个是内存等待，一个是持久化审批",
        "actor": "源码阅读对比",
        "request": "Concept compare",
        "url": "LOCAL compare AskUserQuestion vs Builder approval",
        "params": {
          "AskUserQuestion": "当前 chat turn 的临时问题",
          "BuilderApproval": "平台能力/构建模式里的持久化人工确认或任务请求"
        },
        "paramNotes": [
          "AskUserQuestion：requestId 由 agent 进程生成，pendingResponses 是 Map，answer 直接 resolve 当前 SDK promise。",
          "BuilderApproval：通常会有平台级 tool/controller 参与，适合做持久化任务状态、审计和跨进程恢复。"
        ],
        "response": {
          "takeaway": "不要用 agent_requests 去解释 Claude Code AskUserQuestion 的主流程；它们解决的问题不同。"
        },
        "implementation": [
          "Question 适合问“这一步缺一个参数，我现在要你补充”。它依赖当前 agent turn 还在。",
          "Approval 适合问“这个平台动作是否允许执行”。它更像业务任务，应该能被数据库追踪、审计、重试或恢复。",
          "所以源码里 Question 的核心文件是 agents/claude-code/src/agent.ts、server.ts、universal-events.ts，加上 CP 的 sse.ts/proxy.ts；不是数据库表驱动的 approval worker。"
        ],
        "db": [],
        "relatedTables": [
          "sessions"
        ],
        "note": "如果你调试时看到页面在等用户回答，先看 SSE 的 question.requested、sessions.chat_status、agent 进程 pending Map，而不是先找审批表。"
      }
    ]
  },
  "serviceTokenFlow": {
    "title": "Service Token：服务间认证、Session Token 与平台 MCP",
    "intro": "这一章先把三个容易混的东西拆开，并先用一张总时序图看完整生命周期：service_token 是 Authorization: Bearer tos_xxx，用于 /api 和 /_proxy 的服务间/API 调用；platform service token 是登录后为内部服务懒创建的代用户 token；session_token 是 CP 在 chat turn 下发给 agent 的会话归属 token，用于平台 MCP 和用户 HTTP MCP 的请求归属。",
    "quickLinks": [
      {
        "label": "数据库结构：service_tokens",
        "moduleId": "schema",
        "domain": "runtime",
        "table": "service_tokens"
      },
      {
        "label": "数据库结构：session_tokens",
        "moduleId": "schema",
        "domain": "runtime",
        "table": "session_tokens"
      },
      {
        "label": "MCP OAuth",
        "moduleId": "mcpOauth"
      },
      {
        "label": "Agent 启动",
        "moduleId": "agentStartup"
      }
    ],
    "steps": [
      {
        "title": "总览时序图：三种 token 从创建到使用",
        "actor": "User / CP / internal services / agent / MCP",
        "request": "Lifecycle overview",
        "url": "LOCAL service_token + platform_token + session_token lifecycle",
        "params": {
          "service_token": "用户手动创建 -> service_tokens 只存 hash -> API/服务用 Authorization: Bearer 调 CP",
          "platform_service_token": "用户登录 -> ensurePlatformToken 懒创建 -> service_tokens 同时存 hash 和明文 -> 内部服务代表用户调用",
          "session_token": "用户发起 chat -> CP mint/ensure session_tokens -> 放进 agent /chat body -> agent 注入 X-Session-Token 调 MCP"
        },
        "paramNotes": [
          "service_token：创建动作由用户/API 显式发起；使用时出现在 Authorization header；CP 用 hash 查 service_tokens。",
          "platform_service_token：创建动作由登录成功后后台触发；明文保存在 service_tokens.token，供 CP 交给 Browser/Sandbox/Channel Gateway 等内部服务。",
          "session_token：创建动作由 CP chat dispatch 触发；使用时出现在 X-Session-Token header；CP /mcp 或 MCP proxy 用它查 session_tokens。"
        ],
        "response": {
          "service_token_use": "CP currentUser = token.created_by / svc:<token-name>",
          "platform_service_token_use": "内部服务拿 Bearer tos_xxx 代表用户访问 CP /api 或相关服务",
          "session_token_use": "CP 恢复 workspaceId/sessionId/taskId，上下文传给平台 MCP tools"
        },
        "implementation": [
          "看创建点：service_token 来自 /api/service-tokens；platform service token 来自登录后的 ensurePlatformToken；session_token 来自 executeChat.ts 发 agent /chat 前。",
          "看存储点：service_token 和 platform token 都在 service_tokens；session_token 在 session_tokens。",
          "看传递方式：service_token/platform token 用 Authorization: Bearer；session_token 用 agent /chat body 和后续 X-Session-Token header。",
          "看消费方：service_token 被 CP 全局 auth middleware 消费；session_token 被 CP /mcp 和 MCP proxy 消费；platform token 被内部服务当 Bearer token 使用。",
          "看权限含义：service_token 认证“谁在调用 CP”；session_token 标记“这次 MCP 调用属于哪个会话”；platform token 是“内部服务代表哪个用户”。"
        ],
        "codeBlocks": [
          {
            "title": "总泳道图：创建、传递、使用",
            "code": "sequenceDiagram\n  participant User as 用户/浏览器\n  participant CP as control-plane\n  participant DB as 数据库\n  participant Internal as 内部服务\n  participant Agent as agent Pod\n  participant MCP as CP /mcp 或 MCP proxy\n  User->>CP: POST /api/service-tokens\n  CP->>DB: INSERT service_tokens(token_hash, created_by)\n  CP-->>User: 返回 tos_xxx 明文一次\n  User->>CP: Authorization: Bearer tos_xxx 调 /api 或 /_proxy\n  CP->>DB: SELECT service_tokens by sha256(tos_xxx)\n  DB-->>CP: created_by=u_alice\n  CP->>CP: currentUser = svc token 的 created_by\n  User->>CP: 登录成功 /api/auth/login\n  CP->>DB: ensurePlatformToken(user_id)\n  DB-->>CP: platform token 明文或新建 __platform__\n  CP-->>Internal: 需要时把 platform token 交给 Browser/Sandbox/Channel Gateway\n  Internal->>CP: Authorization: Bearer platform_tos_xxx\n  CP->>DB: SELECT service_tokens by hash\n  CP->>CP: 代表 created_by 用户处理请求\n  User->>CP: POST /api/workspaces/:id/chat\n  CP->>DB: INSERT/SELECT session_tokens(token, workspace_id, session_id)\n  CP->>Agent: POST /chat body 包含 session_token\n  Agent->>MCP: POST /mcp headers 带 X-Workspace-ID + X-Session-Token\n  MCP->>DB: resolveToken(X-Session-Token) 查 session_tokens\n  DB-->>MCP: workspace_id/session_id\n  MCP->>MCP: 注册并执行平台 tools，带 workspace/session/task 上下文\n  MCP-->>Agent: tool result\n  Agent-->>CP: SSE item/tool result\n  CP-->>User: 展示 agent 输出"
          },
          {
            "title": "同一张图的极简文字版",
            "code": "service_token:\n  create: POST /api/service-tokens\n  store: service_tokens.token_hash\n  carry: Authorization: Bearer tos_xxx\n  verify: index.ts -> getServiceTokenByHash(hashToken(token))\n\nplatform service token:\n  create: login success -> ensurePlatformToken(userId)\n  store: service_tokens.token_hash + service_tokens.token plaintext\n  carry: Authorization: Bearer tos_xxx by internal service\n  verify: same service token middleware\n\nsession_token:\n  create: executeChat.ts -> mintToken/ensureTokenForSession\n  store: session_tokens.token/workspace_id/session_id\n  carry: agent /chat body -> X-Session-Token on MCP request\n  verify: /mcp or MCP proxy -> resolveToken/resolveTokenForUser"
          }
        ],
        "db": [
          {
            "table": "service_tokens",
            "op": "INSERT/SELECT",
            "values": {
              "normal": "token_hash only",
              "platform": "token_hash + plaintext token"
            }
          },
          {
            "table": "session_tokens",
            "op": "INSERT/SELECT/RESOLVE",
            "values": {
              "token": "uuid",
              "workspace_id": "ws_codex_demo",
              "session_id": "sess_123"
            }
          }
        ],
        "note": "先看这张图再读后面的分步：service_tokens 负责 Bearer 认证，session_tokens 负责 MCP 会话上下文。"
      },
      {
        "title": "先分清三种 token：谁认证、谁归属、谁代表用户",
        "actor": "人类视角 / 系统边界",
        "request": "Concept map",
        "url": "LOCAL token boundary map",
        "params": {
          "service_token": "tos_xxx; Authorization: Bearer; service_tokens 表; 认证调用方是谁",
          "platform_service_token": "is_platform=true; service_tokens.token 保存明文; 内部服务代表某个用户调用 CP/下游",
          "session_token": "uuid; session_tokens 表; X-Session-Token; 说明这次 MCP 调用属于哪个 chat session",
          "cookie_or_oauth_jwt": "浏览器登录态或 CP OAuth JWT; 认证普通用户"
        },
        "paramNotes": [
          "service_token：外部 API 客户端或内部服务访问 CP /api、/_proxy 时使用。CP hash 后查 service_tokens。",
          "platform_service_token：特殊 service token。普通 token 只保存 hash；platform token 因为要交给 Browser/Sandbox/Channel Gateway 等服务使用，所以额外保存明文 token。",
          "session_token：不是认证用户身份，而是把 agent 后续 MCP 调用归属到某个 workspace/session/task。它走 X-Session-Token。",
          "cookie_or_oauth_jwt：用户登录 CP 或第三方 OAuth client 调 CP API 时使用。"
        ],
        "response": {
          "service_token_auth_result": {
            "user": {
              "sub": "token.created_by",
              "username": "svc:token-name",
              "role": "user"
            }
          },
          "session_token_resolve_result": {
            "workspaceId": "ws_codex_demo",
            "sessionId": "sess_123",
            "taskId": "optional"
          }
        },
        "implementation": [
          "最容易误解的是：service_token 和 session_token 都是 token，但解决的问题完全不同。",
          "service_token 回答“这个 HTTP 请求是谁发起的，能不能进入 CP /api 或 /_proxy”。",
          "session_token 回答“agent 调平台 MCP tool 时，这个工具调用属于哪个 chat session/task”。",
          "agent 启动拉 /_cp/workspaces/:id/config 没有看到 service_token；那条链路主要依赖集群内网和 workspace_id。"
        ],
        "db": [
          {
            "table": "service_tokens",
            "op": "AUTH LOOKUP",
            "values": {
              "token_hash": "sha256(tos_xxx)",
              "created_by": "u_alice",
              "revoked_at": null
            }
          },
          {
            "table": "session_tokens",
            "op": "SESSION LOOKUP",
            "values": {
              "token": "uuid-session-token",
              "workspace_id": "ws_codex_demo",
              "session_id": "sess_123"
            }
          }
        ],
        "note": "一句话：service_token 是“调用者认证”，session_token 是“会话归属”。"
      },
      {
        "title": "普通 service token 创建：明文只返回一次，数据库只存 hash",
        "actor": "Browser/UI or API client -> control-plane",
        "request": "POST /api/service-tokens",
        "url": "POST http://127.0.0.1:30080/api/service-tokens",
        "params": {
          "name": "ci-bot"
        },
        "paramNotes": [
          "name：用户给 token 起的显示名，用于列表和审计。",
          "Authorization/cookie：创建 token 本身需要当前用户已登录或已通过其他方式认证。",
          "plaintext token：CP 生成 tos_xxx 后只在创建响应里返回一次。"
        ],
        "response": {
          "status": 201,
          "body": {
            "id": "tok_123",
            "name": "ci-bot",
            "token": "tos_plaintext_once",
            "created_at": "2026-07-28T10:00:00Z"
          }
        },
        "implementation": [
          "routes/service-tokens.ts 读取 currentUser 和 body.name。",
          "generateToken() 生成明文 token，hashToken(token) 计算 SHA-256。",
          "createServiceToken(name, hash, currentUser.sub) 写 service_tokens。",
          "响应返回明文 token；后续 GET /api/service-tokens 不会再返回 token_hash 或明文。",
          "DELETE /api/service-tokens/:id 只允许撤销自己的普通 token；is_platform=true 的 token 不能走普通 revoke。"
        ],
        "codeBlocks": [
          {
            "title": "源码：普通 service token 创建",
            "code": "control-plane/src/routes/service-tokens.ts:8-21\ncontrol-plane/src/services/db/shares.ts:84-99\ncontrol-plane/src/lib/service-token.ts:8-10\n\nPOST /api/service-tokens\n  token = generateToken()\n  token_hash = sha256(token)\n  INSERT service_tokens(name, token_hash, created_by)\n  return plaintext token once"
          }
        ],
        "db": [
          {
            "table": "service_tokens",
            "op": "INSERT",
            "values": {
              "id": "tok_123",
              "name": "ci-bot",
              "token_hash": "sha256(tos_plaintext_once)",
              "token": null,
              "created_by": "u_alice",
              "is_platform": false
            }
          }
        ],
        "note": "普通 service token 泄露后无法从数据库反推出明文；只能 revoke 后重新创建。"
      },
      {
        "title": "platform service token：登录时懒创建，内部服务可代表用户",
        "actor": "Login route -> service_tokens",
        "request": "ensurePlatformToken(userId)",
        "url": "LOCAL control-plane/src/routes/auth.ts login success",
        "params": {
          "user_id": "u_alice",
          "token_name": "__platform__",
          "is_platform": true
        },
        "paramNotes": [
          "user_id：当前登录成功的用户。",
          "token_name=__platform__：平台内部约定的 token 名。",
          "is_platform=true：表示这是平台服务 token，和用户手动创建的普通 token 不同。"
        ],
        "response": {
          "status": "async best effort",
          "result": "existing plaintext token or newly generated token"
        },
        "implementation": [
          "auth.ts 和 wecom-auth.ts 登录成功后都会调用 ensurePlatformToken(dbUser.id)。",
          "ensurePlatformToken 先查 created_by=userId 且 is_platform=true 且未 revoked 的 token。",
          "如果不存在，生成 token，同时写 token_hash 和 token 明文。",
          "为什么 platform token 要存明文：CP 后续需要把它交给 Browser Service、Sandbox、Channel Gateway、scheduler 等内部服务，让这些服务带 Bearer 调 CP 或相关服务。",
          "这不是 agent 启动拉 /_cp/config 的凭据；agent 启动代码里没有 Authorization: Bearer service token。"
        ],
        "codeBlocks": [
          {
            "title": "源码：登录成功后自动 provision platform token",
            "code": "control-plane/src/routes/auth.ts:121-124\ncontrol-plane/src/routes/wecom-auth.ts:91-95\ncontrol-plane/src/services/db/shares.ts:129-150\n\nensurePlatformToken(userId):\n  SELECT token FROM service_tokens\n   WHERE created_by=$1 AND is_platform=true AND revoked_at IS NULL\n  if missing:\n    INSERT service_tokens(name=\"__platform__\", token_hash=sha256(token), token=plaintext, is_platform=true)"
          }
        ],
        "db": [
          {
            "table": "service_tokens",
            "op": "UPSERT/SELECT",
            "values": {
              "name": "__platform__",
              "token": "tos_plaintext_saved_for_platform",
              "token_hash": "sha256(token)",
              "created_by": "u_alice",
              "is_platform": true
            }
          }
        ],
        "note": "platform token 的风险边界更高，因为数据库里有明文 token；它是为了服务间代用户调用。"
      },
      {
        "title": "platform token 传给内部服务：Browser / Sandbox / Channel Gateway",
        "actor": "CP MCP tools / Channel Gateway / Browser Service / Sandbox Service",
        "request": "先确定“这次要代表哪个用户”，再读取这个用户的 platform token，并把它放到 Authorization Bearer 里调用内部服务",
        "url": "Browser: POST http://nap-browser:3005/api/browsers；Sandbox: POST http://nap-sandbox:3006/api/sandboxes；Channel Gateway: POST http://control-plane:3000/api/workspaces/:workspaceId/jobs",
        "params": {
          "example_story": "Alice 打开 ws_codex_demo 这个 workspace，让 agent 创建一个浏览器或 sandbox。Slack 里也有一条 route 指向 Alice 的 workspace。",
          "user_id_u_alice": "数据库里的用户主键。它回答的问题是：这次内部服务调用应该算 Alice 的权限，还是 Bob 的权限。",
          "workspace_id": "ws_codex_demo。agent 只传 workspace_id，不传 user_id，也不传 platform token。",
          "workspace_owner_lookup": {
            "sql": "SELECT user_id FROM workspaces WHERE id = 'ws_codex_demo'",
            "result": "user_id = 'u_alice'",
            "purpose": "CP 用这个结果确认 workspace 属于 Alice，所以后续必须拿 Alice 的 platform token。"
          },
          "platform_token_lookup": {
            "sql": "SELECT token FROM service_tokens WHERE created_by = 'u_alice' AND is_platform = true AND revoked_at IS NULL",
            "result": "tos_platform_plaintext",
            "purpose": "拿到能代表 Alice 调用内部服务/CP API 的 Bearer token。"
          },
          "browser_or_sandbox_call": {
            "header": "Authorization: Bearer tos_platform_plaintext",
            "meaning": "Browser/Sandbox 收到后知道：这不是匿名请求，而是平台代表 Alice 发起的请求。"
          },
          "channel_gateway_call": {
            "header": "Authorization: Bearer tos_platform_plaintext",
            "meaning": "Channel Gateway 创建 job 时，CP 会把请求识别为 Alice，从而允许访问 Alice 的 workspace。"
          }
        },
        "paramNotes": [
          "你在泳道图里看到的 user_id='u_alice'，不是接口响应给 agent 的业务结果。它只是 CP 从 workspaces 表查出来的 owner id，用来决定“下一步查谁的 platform token”。",
          "为什么要先查 workspace owner：agent 请求里只有 workspace_id。CP 不能让 agent 自己声明 user_id，否则 agent 可以伪造 user_id 去拿别人的权限。",
          "为什么 platform token 要按 user_id 查：platform token 的含义就是“内部服务代表这个用户调用”。所以 Alice 的 workspace 必须用 Alice 的 token，不能用 connector owner、agent 或系统默认身份。",
          "为什么 Browser/Sandbox 还要校验 Bearer：CP 把 token 交给内部服务后，内部服务不能盲信这个字符串。Browser Service 会拿 Bearer 调 CP /api/auth/me，让 CP 用 service_tokens 表确认它确实有效。",
          "Channel Gateway 的情况不同：它不是 agent tool 的下游服务。它监听外部消息，然后自己查 route/connector owner 的 platform token，再用这个 token 调 CP 创建 job。"
        ],
        "response": {
          "browser_service": {
            "status": 200,
            "body": {
              "id": "browser_123",
              "status": "running",
              "endpoints": {
                "cdp": "ws://...",
                "live_view": "https://..."
              }
            }
          },
          "sandbox_service": {
            "status": 200,
            "body": {
              "id": "sandbox_123",
              "status": "running",
              "metadata": {
                "workspace_id": "ws_codex_demo"
              }
            }
          },
          "channel_gateway_to_cp": {
            "status": 200,
            "body": {
              "id": "job_123"
            }
          }
        },
        "implementation": [
          "用人话讲 Browser/Sandbox：agent 说“我要创建浏览器”。CP 接到 tool call 后先看 workspace_id=ws_codex_demo 属于谁，查到 owner 是 u_alice；然后查 Alice 的 __platform__ token；最后 CP 带着这个 token 调 nap-browser 或 nap-sandbox。",
          "所以 user_id=u_alice 这一步的作用只有一个：确定权限归属。它不是创建浏览器的参数，也不是 agent 需要理解的配置值。",
          "Browser Service 收到 Authorization: Bearer tos_platform_plaintext 后，会反向请求 CP /api/auth/me。CP hash token 后查 service_tokens，查到 created_by=u_alice，于是 Browser Service 知道这次请求代表 Alice。",
          "Sandbox Service 的转发方式和 Browser 一样：CP service client 在请求头放 Authorization: Bearer ${token}，Sandbox 侧据此恢复平台用户身份。",
          "用人话讲 Channel Gateway：Slack/Webhook/WeCom 来了一条消息，它要帮用户创建一个 job。它先看这条 route 指向哪个 workspace、route owner 是谁，然后查这个 owner 的 platform token，用 NapClient 带 Bearer 调 POST /api/workspaces/:workspaceId/jobs。",
          "Slack/WeCom 还有一个特殊处理：connector owner 和 route owner 可能不是同一个人。如果 route 指向 Alice 的 workspace，就必须换成 Alice 的 platform token，否则 CP 会按 connector owner 校验 workspace，可能直接 404 Workspace not found。"
        ],
        "codeBlocks": [
          {
            "title": "先别看图：这条链路到底在解决什么问题",
            "code": "场景：Alice 的 workspace 里，agent 要创建一个 Browser。\n\n1. agent 不能直接拿 Alice 的 platform token。\n   否则 token 会进入 agent 容器，权限边界太大。\n\n2. agent 只告诉 CP：我要在 ws_codex_demo 里调用 create_browser。\n\n3. CP 自己查数据库：ws_codex_demo 属于谁？\n   查到 workspaces.user_id = u_alice。\n\n4. CP 再查 Alice 的 platform token：\n   service_tokens.created_by = u_alice AND is_platform = true。\n\n5. CP 用这个 token 调 Browser Service：\n   Authorization: Bearer tos_platform_plaintext。\n\n6. Browser Service 再向 CP 校验这个 token。\n   校验通过后，它才知道“这次创建 browser 是代表 Alice”。\n\n所以图里的 user_id='u_alice' 不是业务返回值，\n它是权限归属判断：这次内部服务调用应该代表 Alice。"
          },
          {
            "title": "泳道图：Agent 调 Browser/Sandbox tool 时，platform token 怎么传给内部服务",
            "code": "sequenceDiagram\n  participant User as 用户 Alice\n  participant CP as control-plane /mcp\n  participant Agent as agent Pod\n  participant DB as PostgreSQL\n  participant Tool as CP Browser/Sandbox tool\n  participant Browser as Browser Service\n  participant Sandbox as Sandbox Service\n\n  User->>CP: POST /api/workspaces/ws_codex_demo/chat\n用户让 agent 创建 browser/sandbox\n  CP->>Agent: POST /chat\nbody 里有 workspace_id + session_token\n  Note over Agent: agent 只知道 workspace_id 和 session_token\n它不知道 Alice 的 platform token\n  Agent->>CP: POST /mcp\nX-Workspace-ID: ws_codex_demo\ntool=create_browser/create_sandbox\n  CP->>Tool: 执行平台 tool(workspaceId=ws_codex_demo)\n  Tool->>DB: 查 workspace owner\nSELECT user_id FROM workspaces WHERE id='ws_codex_demo'\n  DB-->>Tool: user_id='u_alice'\n  Note over Tool,DB: 这一步只是在确认“这个 workspace 属于 Alice”\n后面要代表 Alice 调内部服务\n  Tool->>DB: 查 Alice 的 platform token\nSELECT token FROM service_tokens\nWHERE created_by='u_alice' AND is_platform=true\n  DB-->>Tool: tos_platform_plaintext\n  Note over Tool: 现在 CP 拿到了“代表 Alice”的 Bearer token\n  alt create_browser\n    Tool->>Browser: POST /api/browsers\nAuthorization: Bearer tos_platform_plaintext\nbody={timeout_seconds, metadata}\n    Browser->>CP: GET /api/auth/me\nAuthorization: Bearer tos_platform_plaintext\n    CP->>DB: 用 sha256(token) 查 service_tokens\n    DB-->>CP: created_by='u_alice', name='__platform__'\n    CP-->>Browser: 200 当前用户是 Alice 的 platform token\n    Browser-->>Tool: 200 {id:'browser_123', status:'running'}\n  else create_sandbox\n    Tool->>Sandbox: POST /api/sandboxes\nAuthorization: Bearer tos_platform_plaintext\nbody={image, timeoutSeconds, metadata}\n    Sandbox->>CP: 校验 Bearer token，恢复用户身份\n    CP->>DB: 用 sha256(token) 查 service_tokens\n    DB-->>CP: created_by='u_alice', name='__platform__'\n    Sandbox-->>Tool: 200 {id:'sandbox_123', status:'running'}\n  end\n  Tool-->>CP: tool result\n  CP-->>Agent: MCP tool result\n  Agent-->>CP: SSE/tool output\n  CP-->>User: 页面展示结果"
          },
          {
            "title": "泳道图：Channel Gateway 代表用户创建 job 时，platform token 从哪里来",
            "code": "sequenceDiagram\n  participant Ext as Slack/Webhook/WeCom\n  participant CG as Channel Gateway\n  participant DB as PostgreSQL\n  participant Client as NapClient\n  participant CP as control-plane /api\n  participant Agent as agent Pod\n\n  Ext->>CG: 外部消息进来\n例如 Slack 频道里 @agent\n  CG->>DB: 查这条消息命中的 connector 和 route\n  DB-->>CG: connector.user_id='u_connector'\nroute.user_id='u_alice'\nworkspace_id='ws_codex_demo'\n  Note over CG,DB: route.user_id 表示“这个 route 最终要操作谁的 workspace”\n如果 workspace 是 Alice 的，就要代表 Alice 调 CP\n  alt route.user_id == connector.user_id\n    CG->>DB: getPlatformToken(connector.user_id)\n  else route 指向另一个用户的 workspace\n    CG->>DB: getPlatformToken(route.user_id)\n这里会查 Alice 的 token\n  end\n  DB-->>CG: tos_platform_plaintext\n  CG->>Client: new NapClient({serviceToken: tos_platform_plaintext})\n  Client->>CP: POST /api/workspaces/ws_codex_demo/jobs\nAuthorization: Bearer tos_platform_plaintext\nbody={prompt, trigger}\n  CP->>DB: 用 sha256(token) 查 service_tokens\n  DB-->>CP: created_by='u_alice', name='__platform__'\n  Note over CP: CP 把这个请求当成 Alice 发起\n所以可以访问 Alice 的 ws_codex_demo\n  CP->>Agent: 后续调度/启动 agent job\n  CP-->>Client: 200 {id:'job_123'}\n  Client-->>CG: job created\n  CG->>DB: 记录 channel event\njob_id='job_123', status='success'"
          },
          {
            "title": "源码锚点：CP 在 Browser/Sandbox tool 内读取 token，再转发 Bearer",
            "code": "control-plane/src/mcp/tools/browser.ts\n  registerBrowserTools(server, workspaceId)\n  getBrowserToken():\n    ws = await getWorkspace(workspaceId)\n    token = await getPlatformToken(ws.user_id)\n  create_browser -> browser.createBrowser(token, ...)\n\ncontrol-plane/src/services/browser.ts\n  fetch(BROWSER_SERVICE_URL + path, {\n    headers: { Authorization: `Bearer ${token}` }\n  })\n\ncontrol-plane/src/mcp/tools/sandbox.ts\n  registerSandboxTools(server, workspaceId)\n  getToken():\n    ws = await getWorkspace(workspaceId)\n    token = await getPlatformToken(ws.user_id)\n  create_sandbox -> sandbox.createSandbox(token, ...)\n\ncontrol-plane/src/services/sandbox.ts\n  buildHeaders(token) -> { Authorization: `Bearer ${token}` }"
          },
          {
            "title": "源码锚点：Browser Service 和 CP 如何校验 Bearer",
            "code": "browser-service/src/lib/token.ts\n  verifyServiceToken(token):\n    fetch(`${NAP_URL}/api/auth/me`, {\n      headers: { Authorization: `Bearer ${token}` }\n    })\n\ncontrol-plane/src/index.ts\n  if path startsWith /api or /_proxy:\n    Authorization: Bearer tos_xxx\n    tokenRecord = getServiceTokenByHash(hashToken(tos_xxx))\n    req.user = {\n      sub: tokenRecord.created_by,\n      username: `svc:${tokenRecord.name}`,\n      role: 'user'\n    }"
          },
          {
            "title": "源码锚点：Channel Gateway 自己查 token，再用 NapClient 调 CP",
            "code": "channel-gateway/src/services/db.ts:195-200\n  SELECT token FROM service_tokens\n   WHERE created_by=$1 AND is_platform=true AND revoked_at IS NULL\n\nchannel-gateway/src/connectors/slack.ts\n  platformToken = await db.getPlatformToken(connector.user_id)\n  napClient = new NapClient({ baseUrl: NAP_API_URL, serviceToken: platformToken })\n  if (route.user_id !== connector.user_id):\n    routeToken = await db.getPlatformToken(route.user_id)\n    jobClient = new NapClient({ serviceToken: routeToken })\n  jobClient.jobs.create(route.workspace_id, { prompt, trigger })\n\ninternal/client/src/http.ts\n  if (serviceToken):\n    headers['Authorization'] = `Bearer ${serviceToken}`\n\ninternal/client/src/jobs.ts\n  POST /api/workspaces/:workspaceId/jobs"
          }
        ],
        "db": [
          {
            "table": "service_tokens",
            "op": "SELECT plaintext platform token",
            "values": {
              "created_by": "u_alice",
              "is_platform": true,
              "revoked_at": null,
              "token": "tos_platform_plaintext"
            }
          },
          {
            "table": "workspaces",
            "op": "SELECT owner before token lookup",
            "values": {
              "id": "ws_codex_demo",
              "user_id": "u_alice"
            }
          },
          {
            "table": "channel events / routes",
            "op": "Channel Gateway dispatch result",
            "values": {
              "route_user_id": "u_alice",
              "workspace_id": "ws_codex_demo",
              "job_id": "job_123",
              "status": "success"
            }
          }
        ],
        "note": "最短理解：platform token 不是“agent 的 token”，而是“内部服务代替某个用户办事的 token”。user_id=u_alice 这一步就是在确定代替谁办事。"
      },
      {
        "title": "CP 如何校验 service token：Bearer 先查 service_tokens，再尝试 JWT/cookie",
        "actor": "API client / internal service -> control-plane",
        "request": "GET /api/... with Authorization: Bearer tos_xxx",
        "url": "GET http://127.0.0.1:30080/api/workspaces",
        "params": {
          "Authorization": "Bearer tos_plaintext_or_oauth_jwt"
        },
        "paramNotes": [
          "Authorization：对 /api 或 /_proxy 请求，CP 会读取 Bearer。",
          "tos_xxx：如果 hash 后命中 service_tokens，当前用户被设置为 token.created_by。",
          "OAuth JWT：如果不是 service token，CP 再按 CP 自己签发的 OAuth/JWT 校验。",
          "cookie：没有 Bearer 或 Bearer 不成立时，浏览器登录态走 cookie JWT。"
        ],
        "response": {
          "success": {
            "user": {
              "sub": "u_alice",
              "username": "svc:ci-bot",
              "role": "user"
            }
          },
          "failure": {
            "status": 401,
            "body": {
              "error": "Unauthorized"
            }
          }
        },
        "implementation": [
          "index.ts 全局 middleware 对 /api 和 /_proxy 的 Bearer 做 service token 识别。",
          "命中 service_tokens 后，CP 构造 synthetic user：sub=created_by，username=svc:<token name>，role=user。",
          "这意味着 service token 的权限本质上继承 created_by 用户的资源权限，而不是独立 service account 权限模型。",
          "/_cp/* 和 /mcp 是显式跳过全局 auth middleware 的路径，不按 Authorization Bearer 做 service_token 认证。"
        ],
        "codeBlocks": [
          {
            "title": "源码：全局 auth middleware 的顺序",
            "code": "control-plane/src/index.ts:209-236\n\nif ((isApiRequest || isProxyRequest) && Authorization startsWith Bearer) {\n  tokenRecord = getServiceTokenByHash(hashToken(bearerToken))\n  if (tokenRecord) {\n    c.set(\"user\", { sub: tokenRecord.created_by, username: `svc:${tokenRecord.name}`, role: \"user\" })\n    return next()\n  }\n\n  jwtPayload = verifyToken(bearerToken)\n  if (jwtPayload) { c.set(\"user\", jwtPayload); return next() }\n}\n\nthen try cookie JWT"
          }
        ],
        "db": [
          {
            "table": "service_tokens",
            "op": "SELECT",
            "values": {
              "token_hash": "sha256(tos_xxx)",
              "revoked_at": null
            }
          }
        ],
        "note": "service_token 认证的是 CP /api 和 /_proxy 的调用者；不是所有内部路径都走它。"
      },
      {
        "title": "agent 启动拉配置：/_cp/* 不带 service token",
        "actor": "agent Pod -> control-plane internal route",
        "request": "GET /_cp/workspaces/:id/config",
        "url": "GET http://control-plane:30080/_cp/workspaces/ws_codex_demo/config",
        "params": {
          "workspace_id": "ws_codex_demo",
          "Authorization": null
        },
        "paramNotes": [
          "workspace_id：由平台在 agent Pod 环境变量里下发，agent 用它拼 /_cp 路径。",
          "Authorization：Codex/Claude/Goose 的 config.ts 里 fetch /_cp/config、/_cp/credentials 没有设置 Bearer。",
          "/_cp/*：index.ts 显式跳过全局 auth middleware，属于内部接口面。"
        ],
        "response": {
          "status": 200,
          "body": {
            "workspace_id": "ws_codex_demo",
            "model": "gpt-5",
            "mcp_config": {
              "tos-platform": {}
            }
          }
        },
        "implementation": [
          "agent 启动或 reload 时直接 fetch CP_URL + /_cp/workspaces/${WORKSPACE_ID}/config。",
          "这条链路不是 service_token 认证链路。保护边界主要是集群网络、服务暴露范围、Pod 环境变量由平台控制。",
          "不要把它和 chat 阶段的 session_token 混在一起：session_token 是 CP dispatch chat 时才下发给 agent /chat body 的。"
        ],
        "codeBlocks": [
          {
            "title": "源码：agent config fetch 没有 Authorization header",
            "code": "agents/codex/src/config.ts:80-84\nagents/claude-code/src/config.ts:44-47\nagents/goose/src/config.ts:81-85\n\nconst url = `${CP_URL}/_cp/workspaces/${WORKSPACE_ID}/config`\nconst resp = await fetch(url)"
          },
          {
            "title": "源码：/_cp/* 跳过全局 auth",
            "code": "control-plane/src/index.ts:170-181\n\nif (path.startsWith(\"/_cp/\") || path === \"/mcp\") {\n  return next()\n}"
          }
        ],
        "db": [],
        "relatedTables": [
          "service_tokens"
        ],
        "note": "结论：agent 启动拉配置不靠 service_token。"
      },
      {
        "title": "chat 时 CP 下发 session_token：给后续 MCP 调用做归属",
        "actor": "control-plane -> agent /chat",
        "request": "POST agent /chat body includes session_token",
        "url": "POST http://agent-ws-codex-demo:3000/chat",
        "params": {
          "message": "用户消息",
          "session_id": null,
          "session_token": "uuid-session-token"
        },
        "paramNotes": [
          "message：用户当前 turn 的输入。",
          "session_id：恢复旧会话时存在；新会话时为空。",
          "session_token：CP 在 dispatch agent /chat 前 mint 或复用，放在 JSON body 里，不是 HTTP Authorization。"
        ],
        "response": {
          "status": 200,
          "headers": {
            "Content-Type": "text/event-stream"
          },
          "firstEvent": {
            "type": "session.started",
            "session_id": "sess_123"
          }
        },
        "implementation": [
          "executeChat.ts 在发 agent /chat 前，如果是旧 session 调 ensureTokenForSession；新 session 调 mintToken({ workspaceId })。",
          "新 session 的 token 先以 session_id=null 写入 session_tokens；等 agent 发 session.started 后，SSE persist 逻辑再绑定真实 session id。",
          "agent 收到 session_token 后，不把它当登录凭据，而是注入到平台 MCP 或用户 HTTP MCP 的 headers。"
        ],
        "codeBlocks": [
          {
            "title": "源码：chat dispatch mint session_token",
            "code": "control-plane/src/services/chat/executeChat.ts:130-151\ncontrol-plane/src/lib/session-token.ts:29-57\n\nconst sessionToken = sessionId\n  ? await ensureTokenForSession(workspaceId, sessionId)\n  : await mintToken({ workspaceId })\n\nbody = JSON.stringify({ message, session_id, source, session_token: sessionToken })\nfetch(agentAddress + \"/chat\", { method: \"POST\", body })"
          }
        ],
        "db": [
          {
            "table": "session_tokens",
            "op": "INSERT/SELECT",
            "values": {
              "token": "uuid-session-token",
              "workspace_id": "ws_codex_demo",
              "session_id": null,
              "resolved_at": null
            }
          }
        ],
        "note": "session_token 的生命周期跟 chat session 绑定，用于后续 MCP 请求恢复 session/task 上下文。"
      },
      {
        "title": "平台 MCP 如何“启动”：agent 把 CP /mcp 注册成 tos-platform server",
        "actor": "agent adapter -> MCP client / SDK",
        "request": "loadAcpMcpServers(sessionToken) or Claude query mcpServers",
        "url": "MCP server URL: http://control-plane:30080/mcp",
        "params": {
          "name": "tos-platform",
          "type": "http",
          "url": "http://control-plane:30080/mcp",
          "headers": {
            "X-Workspace-ID": "ws_codex_demo",
            "X-Agent-ID": "ws_codex_demo",
            "X-Session-Token": "uuid-session-token"
          }
        },
        "paramNotes": [
          "name=tos-platform：平台内置 MCP server 名，agent 看到的是一个 MCP server。",
          "type=http：不是 stdio 子进程，而是 HTTP MCP。",
          "url=/mcp：control-plane 暴露的 Streamable HTTP MCP endpoint。",
          "X-Workspace-ID：CP /mcp 必需，用来确定工具操作哪个 workspace。",
          "X-Agent-ID：标识发起请求的 agent/workspace，主要给平台工具做上下文。",
          "X-Session-Token：可选但关键，用来把工具调用归属到 chat session/task。"
        ],
        "response": {
          "agent_mcp_config": {
            "servers": [
              {
                "name": "tos-platform",
                "url": "http://control-plane:30080/mcp",
                "headers": [
                  "X-Workspace-ID",
                  "X-Agent-ID",
                  "X-Session-Token"
                ]
              }
            ]
          }
        },
        "implementation": [
          "Codex/Goose：createAcpAgentApp 的 /chat 在 createSession/loadSession 时调用 config.loadMcpServers(sessionToken)，把返回的 MCP server 列表传给 ACP session。",
          "Codex 的 loadAcpMcpServers 会把 tos-platform 放进 servers，URL 指向 CP /mcp，并注入 X-Workspace-ID/X-Agent-ID/X-Session-Token。",
          "Claude Code：agent.ts 构造 mcpServers 参数传给 Claude SDK query；同样把 tos-platform 指向 CP /mcp，并注入这些 headers。",
          "所以“平台 MCP 启动”不是 CP 额外启动一个长驻 MCP 进程，而是 agent 的 MCP client 在本 turn/session 里注册一个 HTTP MCP server endpoint。"
        ],
        "codeBlocks": [
          {
            "title": "源码：Codex 注册 tos-platform MCP",
            "code": "agents/codex/src/config.ts:402-424\ninternal/acp-adapter/acp-server.ts:421-443\n\nconst platformHeaders = new Map()\nplatformHeaders.set(\"X-Workspace-ID\", WORKSPACE_ID)\nplatformHeaders.set(\"X-Agent-ID\", WORKSPACE_ID)\nif (sessionToken) platformHeaders.set(\"X-Session-Token\", sessionToken)\nservers.push({ type: \"http\", name: \"tos-platform\", url: `${CP_URL}/mcp`, headers })\n\ncurrentSessionId = await newBridge.createSession({ mcpServers })"
          },
          {
            "title": "源码：Claude 每 turn 注入 tos-platform MCP",
            "code": "agents/claude-code/src/agent.ts:200-241\n\nplatformHeaders[\"X-Workspace-ID\"] = WORKSPACE_ID\nplatformHeaders[\"X-Agent-ID\"] = WORKSPACE_ID\nif (sessionToken) platformHeaders[\"X-Session-Token\"] = sessionToken\nmcpServers[\"tos-platform\"] = { type: \"http\", url: `${CP_URL}/mcp`, headers: platformHeaders }\n\nquery({ ..., options: { mcpServers } })"
          }
        ],
        "db": [],
        "relatedTables": [
          "session_tokens"
        ],
        "note": "从 agent 视角：tos-platform 是一个 HTTP MCP server。从 CP 视角：/mcp 是一个普通 HTTP endpoint，每次请求时创建 MCP server/transport 并注册 tools。"
      },
      {
        "title": "agent 请求平台 tool：CP /mcp 每次请求创建 server，并按 session_token 恢复上下文",
        "actor": "agent MCP client -> control-plane /mcp -> platform tools",
        "request": "POST /mcp Streamable HTTP JSON-RPC",
        "url": "POST http://control-plane:30080/mcp",
        "params": {
          "headers": {
            "X-Workspace-ID": "ws_codex_demo",
            "X-Agent-ID": "ws_codex_demo",
            "X-Session-Token": "uuid-session-token"
          },
          "body": {
            "jsonrpc": "2.0",
            "method": "tools/call",
            "params": {
              "name": "browser.open",
              "arguments": {
                "url": "https://example.com"
              }
            }
          }
        },
        "paramNotes": [
          "X-Workspace-ID：没有这个 header，handleMcpRequest 直接 400。",
          "X-Session-Token：CP resolveToken(token)，如果 workspace 匹配，就得到 sessionId，再尝试找 teamwork task。缺失或未知不是 fatal，会降级成 workspace-level。",
          "body.method=tools/call：MCP 标准调用，具体 tool 名由模型选择。",
          "params.name：平台工具来自 registerTools 注册，包括 memory、agent、browser、sandbox、skills、afs、export-file-url、builder-mode。"
        ],
        "response": {
          "status": 200,
          "body": {
            "jsonrpc": "2.0",
            "result": {
              "content": [
                {
                  "type": "text",
                  "text": "tool result"
                }
              ]
            }
          }
        },
        "implementation": [
          "index.ts 把 /mcp 注册到 handleMcpRequest，并且 /mcp 跳过全局 Bearer/cookie auth。",
          "handleMcpRequest 读取 X-Workspace-ID；这是平台 MCP 的最小必需上下文。",
          "如果有 X-Session-Token，CP 查 session_tokens，并校验 token.workspaceId === X-Workspace-ID。匹配后得到 sessionId。",
          "如果 sessionId 能找到 teamwork task，还会确认 task.coordinator_workspace_id 等于当前 workspace，才把 taskId 放进工具上下文。",
          "然后 CP new McpServer({ name: \"tos-platform\" })，registerTools(server, ctx)，new WebStandardStreamableHTTPServerTransport，server.connect(transport)，最后 transport.handleRequest(request)。",
          "registerTools 挂上平台工具：memory、agent、browser、sandbox、skills、afs、export-file-url、builder-mode。工具内部再按 workspaceId/sessionId/taskId 做具体业务处理。"
        ],
        "codeBlocks": [
          {
            "title": "源码：CP /mcp endpoint",
            "code": "control-plane/src/index.ts:392-395\ncontrol-plane/src/mcp/server.ts:11-70\ncontrol-plane/src/mcp/tools.ts:26-36\n\napp.all(\"/mcp\", async (c) => handleMcpRequest(c.req.raw))\n\nhandleMcpRequest(request):\n  workspaceId = request.headers.get(\"x-workspace-id\")\n  record = resolveToken(request.headers.get(\"x-session-token\"))\n  server = new McpServer({ name: \"tos-platform\" })\n  registerTools(server, { workspaceId, sessionId, taskId, headers })\n  transport = new WebStandardStreamableHTTPServerTransport({ enableJsonResponse: true })\n  server.connect(transport)\n  return transport.handleRequest(request)"
          },
          {
            "title": "平台 tool 注册清单",
            "code": "registerMemoryTools(server, workspaceId)\nregisterAgentTools(server, workspaceId, taskId)\nregisterBrowserTools(server, workspaceId)\nregisterSandboxTools(server, workspaceId)\nregisterSkillsTools(server, workspaceId)\nregisterAfsTools(server, workspaceId)\nregisterExportFileUrlTool(server, workspaceId)\nregisterBuilderTools(server, workspaceId, headers)"
          }
        ],
        "db": [
          {
            "table": "session_tokens",
            "op": "SELECT/RESOLVE",
            "values": {
              "token": "uuid-session-token",
              "workspace_id": "ws_codex_demo",
              "session_id": "sess_123"
            }
          }
        ],
        "note": "平台 MCP 的认证边界主要不是 service_token，而是 agent 由平台注入的 workspace/session headers 加集群内访问边界。"
      },
      {
        "title": "用户配置的 HTTP MCP：agent 仍带 session_token，CP proxy 再转换给上游",
        "actor": "agent MCP client -> CP MCP proxy -> upstream MCP",
        "request": "POST user MCP URL configured by /_cp/config",
        "url": "POST http://control-plane:30080/_cp/mcp/u_alice/https%3A%2F%2Fdocs.example.com/mcp",
        "params": {
          "headers_from_agent": {
            "X-Session-Token": "uuid-session-token"
          },
          "upstream_headers_after_cp": {
            "X-Tos-Session-Id": "sess_123",
            "Authorization": "Bearer upstream_access_token_if_oauth"
          }
        },
        "paramNotes": [
          "headers_from_agent.X-Session-Token：agent 对用户 HTTP MCP 也注入它，让 CP proxy 能知道 session。",
          "upstream_headers_after_cp.X-Tos-Session-Id：CP 不把原始 X-Session-Token 泄露给第三方 MCP，而是转换成上游可见的 session id。",
          "Authorization：如果该 MCP 走 MCP OAuth，CP proxy 用 mcp_oauth_tokens 里的用户授权 token 注入上游 Authorization。"
        ],
        "response": {
          "upstreamVisible": {
            "X-Tos-Session-Id": "sess_123"
          },
          "upstreamNotVisible": [
            "X-Session-Token"
          ]
        },
        "implementation": [
          "workspace config 返回用户 HTTP MCP 时，URL 通常已经被改写成 CP proxy URL。",
          "agent 的 loadAcpMcpServers 对用户 HTTP MCP 也注入 X-Session-Token。",
          "CP proxy 接到请求后 resolveTokenForUser，确认这个 token 属于当前 user 的 workspace。",
          "随后删除内部 X-Session-Token，改成上游 header，例如 X-Tos-Session-Id；如果有 MCP OAuth token，再注入 Authorization Bearer。",
          "这条链路说明 session_token 不只给 tos-platform 用，也给用户 MCP proxy 做 session 归属转换。"
        ],
        "codeBlocks": [
          {
            "title": "源码锚点",
            "code": "agents/codex/src/config.ts:427-438\nagents/goose/src/config.ts:434-443\ncontrol-plane/src/routes/mcp-proxy.ts\ncontrol-plane/src/lib/session-token.ts:73-90\n\nagent -> CP proxy: X-Session-Token: uuid\nCP proxy:\n  record = resolveTokenForUser(uuid, userId)\n  delete X-Session-Token\n  set X-Tos-Session-Id / X-Session-Id for upstream\n  maybe set Authorization: Bearer <mcp_oauth_tokens.access_token>"
          }
        ],
        "db": [
          {
            "table": "session_tokens",
            "op": "JOIN workspaces for user ownership",
            "values": {
              "token": "uuid-session-token",
              "user_id": "u_alice"
            }
          }
        ],
        "note": "原始 session_token 是 CP 内部凭据，不应该交给第三方 MCP。"
      }
    ]
  }
}
