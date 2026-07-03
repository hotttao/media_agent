# 痛点、约束与待确认决策

## 一、改造中遇到的痛点

### 痛点 1：配置是模块级全局可变状态

`config/base_config.py` 里 `PLATFORM` `KEYWORDS` `CRAWLER_TYPE` 等都是模块级全局变量。`cmd_arg/arg.py` 的 `parse_cmd()` 通过 `typer` 解析 `sys.argv` 后**直接改写**这些全局变量：

```python
# cmd_arg/arg.py
config.PLATFORM = platform.value
config.CRAWLER_TYPE = crawler_type.value
config.KEYWORDS = keywords
...
```

**影响**：同一进程内并发跑两个爬虫会互相覆盖配置（先设 xhs+keywords=A，再设 dy+keywords=B，第一个爬虫实际跑成 dy+B）。

**规避**：不共用进程。上层用子进程跑爬虫，`config.*` 在各自子进程内独享，天然隔离。这是选择**子进程方案**而非"进程内 + 全局锁"的根本原因——即使加锁串行，原程序退出后的全局状态残留、`media_crawler_db_var` ContextVar 跨任务继承等仍是隐患，子进程最干净。

### 痛点 2：爬虫是长耗时阻塞任务

`CRAWLER_TIME_SLEEP=1s`，200 条笔记 + 评论可能跑几十分钟到数小时。MCP tool 调用若同步等待，agent 客户端会超时。

**规避**：异步 Job 模式。`start_crawler` 立即返回 `job_id`，agent 用 `get_crawl_status` 轮询。原 `main.py` 已埋 `---CRAWL_SUMMARY: {...}---` 输出，上层捕获 stdout 解析即得结果摘要。

### 痛点 3：数据落库表名 / 字段名按平台分散

各平台内容表名、主键字段名、时间字段名都不一样（见 [02-mcp-tools-interface.md](02-mcp-tools-interface.md) 映射表）。若让 agent 自己记，极易出错。

**规避**：上层 `query_crawl_data` 工具内置 `(platform, data_type) → 表名/字段` 映射，对外暴露统一语义（`ids` / `time_range` / `sort.field=time`），agent 不需要知道 `douyin_aweme` 用 `aweme_id` 还是 `video_id`。

### 痛点 4：部分参数既不在 CLI 也不读 env

`CRAWLER_MAX_NOTES_COUNT` `SAVE_DATA_OPTION` `START_PAGE` `SORT_TYPE` `MAX_CONCURRENCY_NUM` `CRAWLER_TIME_SLEEP` 等在 `base_config.py` 里硬编码，typer CLI 不接受、`os.getenv` 也不读。

**影响**：纯 `python main.py` 调用方式下，这几项无法按任务调整。尤其 `CRAWLER_MAX_NOTES_COUNT` 默认 200，agent 想小批量试跑都做不到。

**规避**：加薄垫片 `mcp_runner.py`（不修改原代码），从 `MCP_*` env 注入到 `config` 全局变量后再调 `main.main()`。是否采用见下方决策 1。

### 痛点 5：原 DB 初始化每次重建连接池

`db.init_db()` → `init_mediacrawler_db()` 每次都 `aiomysql.create_pool(...)`，无单例检查。上层若每个查询都 init 会泄漏连接池。

**规避**：上层启动时 init 一次，pool 常驻；只读查询工具复用该 pool。爬虫子进程内部自己 init 一次（子进程内，互不影响）。

### 痛点 6：ContextVar 跨任务不自动继承

`media_crawler_db_var` 是 `contextvars.ContextVar`，`asyncio.create_task` 会 copy 当前 context，但不同 tool 调用处于不同 task 链，需要显式 `set` 或 `contextvars.copy_context()` 传递。

**规避**：上层在每个 tool 的 task 入口 `set` 一次 pool 引用；或封装一个公共装饰器统一注入。

## 二、设计约束（不可逾越）

1. **不深入修改原作者代码**：原 `main.py` / `config/` / `media_platform/` / `db.py` 等只读 import，不改逻辑。新增文件仅 `mcp_server.py` + `mcp_runner.py` + `pyproject.toml` 追加依赖。
2. **存储后端 = MySQL**：用户明确配置用 MySQL（非 SQLite）。所有查询工具走 MySQL。
3. **爬虫执行全局串行**：同一时刻只有一个爬虫子进程（防账号/代理风控 + 平台压力）。查询工具不受限，可并发。
4. **只读工具用独立只读账号**：`execute_readonly_sql` 强制走 `mc_reader` 只读账号 + SQL 白名单双护栏。

## 三、待用户确认的决策点

以下 3 个点需用户拍板后才能进入实现，确认后协议层无需再改，仅影响上层内部实现。

### 决策 1：是否加垫片 `mcp_runner.py`

- **加**（推荐）：agent 可按任务控制 `max_notes_count` / `save_data_option` / `start_page`。代价是多一个 ~20 行新文件（不修改原代码）。
- **不加**：上层直接 `python main.py ...`，仅支持 CLI 暴露的参数；上述三项吃 `base_config` 默认值（`CRAWLER_MAX_NOTES_COUNT=200` 等）。

### 决策 2：是否保留 `execute_readonly_sql` 进 v1

- **保留**（推荐）：给 agent 极大灵活性（自定义聚合 / 连表 / 跨平台统计）。代价：需用户额外建只读 MySQL 账号 `mc_reader`。
- **不保留**：v1 只靠 `query_crawl_data` 的固定语义查询，安全但灵活度低。后续按需再加。

### 决策 3：Job 是否落盘持久化

- **落盘**（推荐）：job 元数据写 `data/jobs/{job_id}.json`，上层 server 重启后仍可查历史 job 状态与结果。
- **不落盘**：job 仅存内存，server 重启后历史 job 丢失（但爬取的数据仍在 MySQL 里，`query_crawl_data` 仍可查）。

## 四、其他实现细节（非决策，落地时注意）

- **传输方式**：默认 stdio（适配 Claude Code / Cursor 本地接入）；若 agent 是远程/云端调用，需切 SSE 或 Streamable HTTP。
- **进度通知**：若客户端传 `progress_token`，上层通过 MCP progress notification 周期推送 percent（基于 checkpoint 更新），agent 不必反复轮询。
- **MCP SDK**：用官方 `mcp` Python SDK（`FastMCP`），`requires-python>=3.11` 与项目一致。
- **checkpoint_id 定位**：新任务的 `checkpoint_id` 由爬虫运行时生成，上层在 job 跑起来后扫 `data/checkpoints/` 目录按 mtime 定位最新匹配文件补上。
- **`list_platforms` 的 `data_tables`**：可选启动时查 `information_schema.columns` 校验表是否存在，缺失则在 `health_check` 标红，避免 agent 查不存在的表。
