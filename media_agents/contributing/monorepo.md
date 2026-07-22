# Monorepo 结构与新增 Agent

本文档说明 `media_agents/` 这个 uv workspace 的包结构、依赖方向、以及新增 agent
的步骤。adapter 内部的 DDD 分层规则见 [architecture.md](adapter/contributing/architecture.md)。

## 包结构

```
media_agents/                       # uv workspace 根（不产出包）
├── pyproject.toml                  # [tool.uv.workspace] members = [...]
├── scripts/
│   └── check_layers.py             # monorepo 跨包边界 AST 检查
├── contributing/
│   └── monorepo.md                  # 本文档
├── adapter/                        # 共享 ACP 适配代码（其他 agent 复用）
│   ├── pyproject.toml              # name="adapter"，import-linter forbidden contracts
│   ├── main.py                     # build_app(cfg, hooks, bridge_factory) 工厂
│   ├── biz/
│   │   ├── api/v1/routes/acp/router.py  # build_acp_router() APIRouter 工厂
│   │   ├── domain/acp/                  # AcpBridge Protocol / AgentHooks Protocol / 实体
│   │   ├── service/acp/service.py       # SessionManager（LRU + busy_turns + pending_destroy）
│   │   └── shared/                      # Clock / ID 生成器
│   ├── schemas/acp/                     # ChatRequest / AgentInfo / AgentCapabilities DTO
│   ├── config/config.py                # AdapterConfig + load_adapter_config（omegaconf + pydantic）
│   ├── infra/logging.py
│   └── contributing/architecture.md     # adapter 内部 DDD 分层规则（用户编写）
└── agents/                        # 三个 agent 实现
    ├── codex/                     # 依赖 adapter；spawn `codex-acp` 子进程
    │   ├── pyproject.toml         # name="codex"，dependencies: ["adapter", ...]
    │   ├── conf/config.yaml       # omegaconf 加载（含 ${oc.env:VAR,default}）
    │   └── src/agents_codex/{main,config,bridge}.py
    ├── goose/                     # 依赖 adapter；spawn `goose acp --with-builtin developer`
    │   └── ... 同上结构
    └── claude_code/               # 不用 ACP router；build_base_app + 自有路由
        └── ...
```

## 依赖方向

```
adapter  <──── agents/codex
           <── agents/goose
           <── agents/claude_code
```

- **adapter** 是被依赖方：`biz / schemas / config / infra / clients` 不得 import `agents_*`
- **agents 互不依赖**：`agents_codex` / `agents_goose` / `agents_claude_code` 不得相互 import；跨 agent
  的公共能力下沉到 `adapter`
- **agents 可 import adapter**：`from adapter.main import build_app` / `from biz.domain.acp.repository
  import AgentHooks` / `from config.config import load_adapter_config` 等

## 配置加载：omegaconf + Pydantic

`AdapterConfig`（在 `adapter/config/config.py`）是 Pydantic BaseModel，声明 agent
静态配置的 schema 与默认值。agent 的 `conf/config.yaml` 只写 override 字段，未列
字段走 Pydantic 默认值。

`load_adapter_config(yaml_path)` 流程：

1. `OmegaConf.structured(AdapterConfig)` 用 Pydantic schema 作为模板
2. `OmegaConf.load(yaml_path)` 读 yaml（支持 `${oc.env:VAR,default}` 环境变量插值）
3. `OmegaConf.merge(schema, yaml_cfg)` 合并
4. `OmegaConf.to_object(merged)` 返回 Pydantic 实例

agent 特有字段（如 goose 的 `goose_path_root`、claude_code 的 `claude_code_bin`）需要在
`AdapterConfig` 里加字段。或单独定义 agent 自己的 Pydantic model，加载流程复用 omegaconf。

## AgentHooks Protocol

agent 回调契约定义在 `adapter/biz/domain/acp/repository.py`。每个 agent 实现一个类，
注入到 `build_app(cfg, hooks=..., bridge_factory=...)`：

| 回调 | 用途 |
|------|------|
| `load_mcp_servers(session_token)` | ACP session 要挂载的 MCP 服务器（HTTP 走 cp 代理 + X-Session-Token） |
| `has_mcp_servers()` | 是否在 config.toml 注册了 MCP（决定是否等 wait_for_mcp_ready） |
| `async load_config()` | 从 CP 拉取 workspace config，写 runtime.json / config.toml / AGENTS.md |
| `async load_skills()` | 从 CP 同步 skills，返回 (ok, failed_names) |
| `async load_credentials()` | 从 CP 注入 credentials 到 env / 文件 |
| `async restart_bridge()` | config/credentials 重载后刷新 provider env，新 bridge 接管 |
| `record_usage(session_id, usage)` | 单 turn 完成后记录 token 用量（失败应吞掉异常） |

## 新增 Agent 步骤

1. 在 `agents/<name>/` 下创建目录：

   ```
   agents/<name>/
   ├── pyproject.toml              # name="<name>"，dependencies 含 "adapter"
   ├── conf/
   │   └── config.yaml             # omegaconf 配置（agent_type / capabilities / ...）
   └── src/agents_<name>/
       ├── __init__.py
       ├── main.py                 # create_app() + uvicorn.run
       ├── config.py               # <Name>Hooks(AgentHooks) 实现
       └── bridge.py               # <Name>AcpBridge 实现 AcpBridge Protocol
   ```

2. 把新成员加进 `media_agents/pyproject.toml` 的 `[tool.uv.workspace] members` 列表，
   以及 `[tool.uv.sources]` 段。

3. 在 `scripts/check_layers.py` 的 `AGENT_PKG` 集合加 `"agents_<name>"`，让跨包
   隔离检查覆盖新 agent。

4. 若新 agent 不走 ACP bridge（如 claude_code 自管子进程），不调 `build_app`，改用
   `build_base_app(cfg)` 拿骨架再 include 自有路由。

## 强制检查

```bash
# adapter 内部层级依赖（forbidden + independence contracts）
cd adapter && lint-imports

# monorepo 跨包边界
python scripts/check_layers.py

# 全量（根目录）
ruff check .
mypy adapter/biz adapter/schemas adapter/config adapter/infra adapter/clients \
      agents/codex/src agents/goose/src agents/claude_code/src
vulture adapter agents --min-confidence 60
pytest
```

## 为什么不用 internal/acp_adapter

原 TS 代码用 `internal/acp-adapter` 路径。Python monorepo 直接用 `adapter/`：

- uv workspace 的成员包名 = 项目名 = 顶层 import 包名。`adapter` 比
  `internal.acp_adapter` 短且 Pythonic
- `internal/` 是 Go 风格的"内部包"约定，Python 用 `_` 前缀或 namespace package 表达
- adapter 本身就是"供其他 agent 复用的代码"，不需要再加 `internal/` 强调可见性
