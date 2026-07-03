# 架构设计

## 一、背景与痛点

MediaCrawlerPro-Python 是一个多平台社交媒体爬虫（小红书 / 抖音 / 快手 / B站 / 微博 / 贴吧 / 知乎）。当前只能通过命令行启动：

```bash
python main.py --platform xhs --type search --keywords "deepseek,chatgpt"
```

改造为 MCP Server 让 agent 调用时，遇到以下痛点（详见 [03-open-issues.md](03-open-issues.md)）：

1. **配置是模块级全局可变状态**：`config.PLATFORM` / `config.KEYWORDS` / `config.CRAWLER_TYPE` 等。`cmd_arg.parse_cmd()` 通过 typer 解析 `sys.argv` 后改写这些全局变量。同一进程内并发跑两个爬虫会互相覆盖配置。
2. **爬虫是长耗时阻塞任务**：200 条笔记 + 评论按 `CRAWLER_TIME_SLEEP=1s` 可能跑几十分钟到数小时。MCP tool 调用不能阻塞这么久。
3. **数据落库表名按平台分散**：`xhs_note` / `douyin_aweme` / `kuaishou_video` / `bilibili_video` / `weibo_note` / `tieba_note` / `zhihu_content`，内容主键字段名（`note_id` / `aweme_id` / `video_id` / `content_id`）和时间字段名（`time` / `create_time` / `publish_time` / `created_time`）各不相同。

## 二、核心思路：上层 / 下层 + 读写分离

**不深入修改原作者代码**，在原程序之上加一个上层 MCP Server，由上层调用原程序。

```
┌─────────────────────────────────────────────────────────────┐
│  上层 = MCP Server（新增，仅 import 原代码做只读查询）         │
│                                                              │
│   ① 执行路径（写）：start_crawler                            │
│      组装 CLI 参数 + 环境变量                                 │
│      → subprocess: python mcp_runner.py --platform xhs ...  │──┐
│      → 跟踪 PID / 捕获 stdout / 解析 CRAWL_SUMMARY           │  │
│      → 全局串行队列（一次只跑一个爬虫）                       │  │
│                                                              │  │ 调用
│   ② 查询路径（读）：query_crawl_data / execute_readonly_sql  │  │ 原程序
│      / list_checkpoints / health_check / get_config          │  │
│      → 上层自有的 MySQL 只读连接池 + 文件系统/Redis 直读      │  │
│      → 与爬虫完全解耦，可并发                                │  │
└─────────────────────────────────────────────────────────────┘  │
                                                                  ▼
┌─────────────────────────────────────────────────────────────┐
│  下层 = 原程序（不改动）                                       │
│  main.py → CrawlerFactory → 各平台 Crawler                   │
│  config.* 全局变量在子进程内独享，进程间天然隔离               │
│  落库 → MySQL（media_crawler_pro）                            │
└─────────────────────────────────────────────────────────────┘
```

### 为什么这样分

- **子进程隔离**解决了痛点 1：`config.*` 全局变量只存在于子进程内，进程退出即销毁。上层不用改它，每个任务起独立子进程，配置通过 CLI + env 注入，零侵入。
- **异步 Job 模式**解决了痛点 2：`start_crawler` 立即返回 `job_id`，agent 用 `get_crawl_status` 轮询。`main.py` 已埋 `---CRAWL_SUMMARY: {...}---` 输出（注释写明 "for Agent consumption"），正好作为 job 结果。
- **映射表**解决了痛点 3：上层维护 `(platform, data_type) → 表名/字段` 映射，对外暴露统一语义，agent 无需记表名。

### MySQL 的加成

aiomysql 连接池本身并发安全（多协程可同时 acquire 连接），所以：

- **全局锁只锁"爬虫执行"**（因为 `config.*` 全局状态 + 账号/代理占用会互相覆盖），**不锁查询工具**。
- `query_crawl_data` / `execute_readonly_sql` / `list_checkpoints` / `health_check` 等**只读工具可并发**，直接走连接池。
- **爬虫运行期间也能并发查询**：InnoDB 的 MVCC 让读写不互斥，agent 一边爬一边查最新落库的数据没问题。

## 三、配置注入方式（不改原代码）

原代码已支持两条注入通道，上层直接用：

| 通道 | 注入项 | 来源 |
|---|---|---|
| **CLI 参数**（`cmd_arg/arg.py` 的 typer） | `--platform --type --keywords --urls --enable_checkpoint/--no-... --checkpoint_id --enable_comments/--no-... --enable_sub_comments/--no-... --hot_search_max_items --hot_search_per_item_count` | `main.py` |
| **环境变量**（`base_config.py` / `db_config.py` 读 `os.getenv`） | `DB_TYPE RELATION_DB_* REDIS_DB_* ACCOUNT_POOL_SAVE_TYPE COOKIE_BRIDGE_URL HOT_SEARCH_CHECKPOINT_MAX_AGE_SECONDS` 等 | 进程启动环境 |

### `mcp_runner.py`（新增薄垫片，~20 行，不修改原代码）

原代码有几个**既不在 CLI、也不读 env**的硬编码项（`CRAWLER_MAX_NOTES_COUNT` `SAVE_DATA_OPTION` `START_PAGE` `SORT_TYPE` `MAX_CONCURRENCY_NUM` `CRAWLER_TIME_SLEEP` 等）。要让 agent 按需控制爬取量，加一个只 import、不改原文件的垫片：

```python
# mcp_runner.py —— 新文件，不修改 main.py / config
import os, sys, asyncio
sys.argv = ["main.py"] + os.getenv("MCP_CLI_ARGS", "").split()  # 让原 typer 正常解析
import config
# 原代码没读 env 的几项，由垫片从 MCP_* env 注入到 config 全局变量
for key, env in [("CRAWLER_MAX_NOTES_COUNT","MCP_MAX_NOTES_COUNT"),
                 ("SAVE_DATA_OPTION","MCP_SAVE_DATA_OPTION"),
                 ("START_PAGE","MCP_START_PAGE")]:
    if v := os.getenv(env):
        setattr(config, key, int(v) if v.isdigit() else v)
import main
asyncio.get_event_loop().run_until_complete(main.main())
```

> 不想加垫片也行：上层直接 `python main.py ...`，但 `max_notes_count / save_data_option / start_page` 这几项就不可按任务调，只能吃 `base_config` 默认值。

## 四、可覆盖 vs 固定参数矩阵（诚实清单）

| 参数 | 可否按任务覆盖 | 通道 |
|---|---|---|
| platform / crawler_type / keywords / urls / checkpoint_id | ✅ | CLI |
| enable_comments / enable_sub_comments / enable_checkpoint | ✅ | CLI |
| hot_search_max_items / hot_search_per_item_count | ✅ | CLI |
| max_notes_count / save_data_option / start_page | ✅ | shim |
| DB 连接 / Redis / 账号池类型 | ❌ 部署级固定 | env（启动时设一次） |
| SORT_TYPE / PUBLISH_TIME_TYPE / MAX_CONCURRENCY_NUM / CRAWLER_TIME_SLEEP / ENABLE_WEIBO_FULL_TEXT | ❌ 默认 | 可选 shim 扩展 |

## 五、Job 状态模型与生命周期

```
queued → running → completed
              ↘ failed
              ↘ cancelled
```

- **全局串行**：同一时刻只有一个爬虫子进程在跑（防账号/代理风控），其余 job 排队。查询工具不受此限制，全程并发。
- **Job 记录**（内存 + 可选落盘 `data/jobs/{job_id}.json` 以便重启后可查）：
  `job_id, params(回显), platform, crawler_type, status, pid, queued_at, started_at, finished_at, checkpoint_id, exit_code, stdout_tail, stderr_tail, result_summary, error`
- **进度来源**：`get_crawl_status` 直接读 `data/checkpoints/{platform}_{mode}_*.json`（按 job 启动时间定位最新文件）算 `crawled_count/percent`，不依赖爬虫主动上报。
- **checkpoint_id 发现**：续爬任务上游传；新任务由爬虫运行时生成，上层在 job 跑起来后扫 checkpoint 目录补上，`get_crawl_status` 一旦拿到就回填。

## 六、DB 初始化复用（落地的关键改动）

上层 MCP Server 是长驻进程，启动时建一次只读 MySQL 连接池：

```python
# 上层启动时
await db.init_mediacrawler_db()                # 建 aiomysql pool
await db.init_table_schema()                  # 首次建表（幂等）
await db.ensure_mysql_schema_compatibility()  # 补 douyin_aweme.title / xhs_note_comment.pictures
await db.ensure_hot_search_schema_compatibility()
```

> 注意：原 `db.init_db()` / `init_mediacrawler_db()` 每次都会重建连接池。上层只调用一次即可，不会泄漏。爬虫子进程内部仍会自己 init 一次（在子进程内，互不影响）。

`media_crawler_db_var` 是 ContextVar——每个 tool 的 task 里 `set` 一次（或用 `contextvars.copy_context()` 传递），因为 aiomysql pool 共享、但 ContextVar 不跨 task 自动继承。

## 七、新增文件清单（原代码 0 修改）

| 文件 | 作用 | 大小 |
|---|---|---|
| `mcp_server.py` | 上层 MCP server：job 管理 + 只读查询池 + 工具注册 | 主文件 |
| `mcp_runner.py` | 薄垫片，注入 CLI/env/shim 参数后调用 `main.main()`（可选） | ~20 行 |
| `data/jobs/` | job 元数据落盘目录（可选，支持重启后可查历史） | — |
| `pyproject.toml` | 新增依赖 `mcp`（仅追加，不动原有） | 1 行 |

**对原项目的依赖**：仅 `import` 层面读取 `config`、`db`、`schema`、`constant`（只读用），以及 `subprocess` 调 `python main.py` / `mcp_runner.py`。
