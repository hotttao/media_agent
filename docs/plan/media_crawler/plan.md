# media_crawler 实现规划

基于 [架构规范](../../../harness/contributing/architecture.md) + 已对齐的领域模型（`docs/media_crawler/`）与 IDL（`idl/media_crawler/`）。

## 目标与约束

- 单 Go 进程 hertz `:8004`，双门面：REST（`/api/...`，hz 生成）+ MCP（`/mcp`，mcp-go，Streamable HTTP）。
- 双库：PG（自有 `jobs`，读写）+ MySQL（爬虫内容，只读）。
- 不改 Python 爬虫代码，加垫片 `mcp_runner.py`。
- v1 只实现小红书（xhs）数据查询；编排类 tool 平台无关。
- v1 不做认证（auth 直通，留中间件位）。

## 架构归位（分层文件清单）

| 层 | 文件 | 来源 | 权限 |
|---|---|---|---|
| IDL model | `hertz_gen/model/media_crawler/{crawler,xhs}/**` | `make hz-model` | 🔴 |
| IDL client | `hertz_gen/media_crawler/{crawler,xhs}_service.go` + `hertz_client.go` | `make hz-client` | 🔴 |
| 路由 | `biz/router/{crawler,xhs}/*.go` + `register.go` | `make -C media_crawler hz-update` | 🔴 |
| handler 骨架 | `biz/handler/{crawler,xhs}/*_service.go` | `make hz-update` | 🟡 填函数体 |
| migration | `migrations/000002_jobs.{up,down}.sql` | 🟢 手写 | |
| dal 初始化 | `biz/dal/db.go`（扩展 mysql）+ `migrate.go`（只 PG）+ `transaction.go`（已有） | 🟢 手写 | |
| dal model | `biz/dal/model/{job,xhs_note,xhs_comment}.go` | 🟢 手写（GORM tag） | |
| dal query | `biz/dal/query/`（PG jobs）+ `biz/dal/query_read/`（MySQL 只读） | GORM Gen | 🔴 |
| dal repo | `biz/dal/repo/{crawl_job,xhs_data}/*.go` | 🟢 手写 | |
| domain | `biz/domain/{shared,crawl_job,xhs_data}/*.go` | 🟢 手写 | |
| service | `biz/service/{crawl_job,xhs_data}/{command,service}.go` | 🟢 手写 | |
| handler/mcp | `biz/handler/mcp/mcp_handler.go` | 🟢 手写（mcp-go 注册） | |
| middleware | `biz/middleware/auth.go` | 🟢 手写（v1 直通） | |
| policy | `biz/policy/crawl_job/*.go` | 🟢 手写（可选，v1 可简化） | |
| shared | `biz/shared/{pagination,checkpoint}.go` | 🟢 手写 | |
| gormgen | `tools/gormgen/main.go` + Makefile target | 🟢 新建 | |
| config | `conf/dev.yaml`（加 crawler/crawl_data 段） | 🟢 | |
| main | `main.go`（双库 init + mcp server）+ `router.go`（挂 /mcp） | 🟡 初始化/wiring | |

## GORM Gen 双 DB 配置

Gen 一次连一个 DB、`Execute()` 重写整个 query 包。jobs（PG）与 xhs_note（MySQL）必须分目录：

```text
tools/gormgen/main.go
  ├─ 读 PG_DSN，连 PG，ApplyBasic(model.Job{})           → biz/dal/query/      (jobs)
  └─ 读 CRAWL_DATA_DSN，连 MySQL，ApplyBasic(model.XhsNote{}, model.XhsComment{})
                                                        → biz/dal/query_read/ (xhs 只读)
```

- `model` 包共用（`biz/dal/model/`，含 Job + XhsNote + XhsComment）。
- 两个 query 包各自 `SetDefault(db)` 绑定 DB。
- Makefile 加 `gorm-gen` target（DSN 从环境变量读，参考 media_auth）。
- schema 变更流程：加 migration（仅 PG）→ 执行 → 更新 model → 重跑 gorm-gen。

## dal 双 DB

- `db.go`：
  - `NewDB(cfg *DatabaseConfig)`（已有，PG）保持。
  - 新增 `NewCrawlDataDB(cfg *CrawlDataConfig) (*gorm.DB, error)`（MySQL 只读，`gorm.io/driver/mysql`，go.mod 加依赖）。
- `migrate.go`：只 PG（MySQL 是爬虫的表，不 migration），保持不变。
- `transaction.go`：`TxRunner` 仅 PG（MySQL 只读无事务）。
- repo 取连接统一走 `dal.FromContext(ctx, db)`：crawl_job 用 PG db，xhs_data 用 MySQL db（无 tx 时用默认连接）。

## migrations

- `000001_init`（已有，建 schema `media-crawler`）。
- 新增 `000002_jobs.up.sql` / `.down.sql`：建 `jobs` 表（字段见 crawl_job domain 文档：job_id pk / platform / crawler_type / params jsonb / status / pid / checkpoint_id / progress jsonb / result_summary jsonb / error / stdout_tail / queued_at / started_at / finished_at）。

## 各层要点

### domain
- `domain/shared/`：JobID 类型、分页、错误码。
- `domain/crawl_job/`：`Job` 实体、`Status` 值对象、`JobRepo` 接口、`CheckpointInfo` 值对象。
- `domain/xhs_data/`：`XhsNote`/`XhsComment` 实体、`XhsRepo` 接口。
- domain 不带 JSON/ORM tag，不依赖框架（架构规范禁止）。

### service
- `service/crawl_job/`：
  - `start_crawler`（**含 subprocess 内联**，不抽 shared）：写 job（事务内）→ commit → `exec.Command(python, mcp_runner.py)`（事务外）→ 异步监控子进程（读 checkpoint 算进度 + 捕获 `---CRAWL_SUMMARY---`）→ 更新 job 终态。
  - 其他用例（纯本地）：`get_crawl_status` / `stop_crawler` / `list_checkpoints` / `delete_checkpoint` / `list_platforms` / `get_config` / `health_check` / `list_jobs` / `get_job`。
- `service/xhs_data/`：`QueryXhsNotes` / `GetXhsNote` / `QueryXhsComments`。
- 同层隔离：`service/crawl_job` 不 import `service/xhs_data`（两者无跨 domain 用例）。

### handler
- `biz/handler/crawler/crawler_service.go`（🟡 填体）：`ListJobs` / `GetJob` → 转 Command → 调 `service/crawl_job`。
- `biz/handler/xhs/xhs_service.go`（🟡 填体）：`ListXhsNotes` / `GetXhsNote` / `ListXhsComments` → 调 `service/xhs_data`。
- `biz/handler/mcp/mcp_handler.go`（🟢 手写）：mcp-go 注册 10 tool（编排 8 + xhs 查询 2），input schema 用 mcp-go fluent API（与 proto message 字段对齐），handler 内 unmarshal → proto struct → 调 service → marshal response struct → `mcp.NewToolResultText(json)`。挂 `/mcp`。

### middleware / policy / shared
- `middleware/auth.go`：v1 直通（`c.Next()`），留 `auth.Subject` 注入位待 media_auth。
- `policy/crawl_job/`：校验 platform/crawler_type 必填（search→keywords、detail/creator→urls）。v1 可简化为 service 内联校验。
- `shared/`：`pagination.go`（分页工具）、`checkpoint.go`（checkpoint 文件解析，供 service 读 `crawled_note_list`/`current_search_*`）。
  - **不抽 subprocess runner**（用户决定 start_crawler 归 service，subprocess 内联）。

## config + main.go

- `conf/dev.yaml`：
  - `database`（PG，已有）。
  - `crawler`（python_path / runner_path / work_dir / max_serial / log_tail_lines / timeout_seconds / default_on_busy）。
  - `crawl_data`（dsn 爬虫 MySQL 只读 / max_open_conns / max_idle_conns / checkpoint_dir）。
- `main.go`（🟡）：
  - `dal.NewDB(pgCfg)` + `dal.Migrate`（已有）。
  - `dal.NewCrawlDataDB(crawlDataCfg)`（新增）。
  - `query.SetDefault(pgDB)` + `query_read.SetDefault(mysqlDB)`（Gen 生成）。
  - 注册 mcp server（`biz/handler/mcp` 的 `Register(s)`）。
- `router.go`（🟡）：挂 `/mcp` endpoint（非 IDL 自定义路由）。

## Python 垫片

`/home/tao/code/github/media_crawler/MediaCrawlerPro-Python/mcp_runner.py`（~20 行，不改原代码）：
```python
import os, sys, asyncio
sys.argv = ["main.py"] + os.getenv("MCP_CLI_ARGS", "").split()
import config
for key, env in [("CRAWLER_MAX_NOTES_COUNT","MCP_MAX_NOTES_COUNT"),
                 ("SAVE_DATA_OPTION","MCP_SAVE_DATA_OPTION"),
                 ("START_PAGE","MCP_START_PAGE")]:
    if v := os.getenv(env):
        setattr(config, key, int(v) if v.isdigit() else v)
import main
asyncio.get_event_loop().run_until_complete(main.main())
```
service/crawl_job 的 start_crawler 拼 `MCP_CLI_ARGS` env + 传 `MCP_*` env 给子进程。

## 开发顺序（依赖序）

1. IDL 生成：`make hz-model && make hz-client && make -C media_crawler hz-update`
2. `migrations/000002_jobs` + `dal/model/{job,xhs_note,xhs_comment}.go`
3. `tools/gormgen/main.go` + Makefile `gorm-gen` + 生成 `query/` + `query_read/`
4. `dal/db.go` 扩展 `NewCrawlDataDB` + `repo/{crawl_job,xhs_data}`
5. `domain/{shared,crawl_job,xhs_data}`
6. `service/{crawl_job,xhs_data}`（crawl_job 含 start_crawler subprocess）
7. `handler/{crawler,xhs}` 填体 + `handler/mcp` 注册 10 tool
8. `conf/dev.yaml` + `main.go` + `router.go`
9. Python 垫片 `mcp_runner.py`
10. 检查：`gofmt -l .` + `go test ./...` + `make -C media_crawler arch-check`
11. 联调：跑 xhs search → 落 MySQL → REST/MCP 查

## 偏离架构规范的说明（按规范第 11 条）

1. **start_crawler 归 service（含 subprocess 外部副作用）**：规范说 service"副作用仅本地 DB 写"。偏离原因：start_crawler 是 crawl_job 单 domain 用例，subprocess 是该用例的执行手段，不跨 domain；用户决定归 service，subprocess 内联不抽 shared。事务遵守"外部副作用前先 commit"：job 入队写 PG（事务内）→ commit → subprocess（事务外）。

2. **双 DB query 分目录（`query/` + `query_read/`）**：规范假设单 DB `query/`。双 DB 因 Gen `Execute()` 重写整个包，必须分目录；`arch-check` 不限制目录名（repo 可 import 两者）。

3. **`workflow/` 代码层暂空**：start_crawler 归 service 后，无跨 domain 流程；`workflow/` 目录暂无代码文件，`crawl_job_execution.md` 文档保留（描述 start_crawler 端到端流程）。
