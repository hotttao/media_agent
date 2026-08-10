---
name: crawl_job
description: Crawl Job 生命周期管理、子进程调度、断点续爬与平台元数据查询
dependencies: []
related_workflow:
    - crawl_job_execution
---

## 一句话职责
crawl_job 负责单个抓取任务的持久化、状态流转、全局串行调度与子进程编排，是 media_crawler 写路径的核心；同时提供平台能力发现、配置查询与健康检查。

## 边界

### 做什么
- 维护 Crawl Job 生命周期（queued→running→completed/failed/cancelled/rejected）
- 全局串行调度子进程 `python mcp_runner.py`（一次仅一个 running，防账号/代理风控）
- 解析子进程 stdout 的 CRAWL_SUMMARY 作为 job 终态
- 读 checkpoint 文件推算进度 + 定位续爬 checkpoint_id
- 提供平台元数据（list_platforms）、配置查询（get_config）、健康检查（health_check）

### 不做什么
- 不实际执行抓取——执行由 Python 爬虫（外部模块），本 domain 仅编排
- 不查询落库内容数据——归 crawl_data domain
- 不存内容数据——内容在爬虫的 MySQL，本 domain 仅存 job 元数据（自治 PG）

## 能力清单

| 能力 | 说明 | 对外接口/函数 |
|:---|:---|:---|
| 提交抓取任务 | 入队 + subprocess 调爬虫 | `start_crawler`（MCP） |
| 查询任务状态 | 返回 status/progress/summary | `get_crawl_status`（MCP）/ `GetJob`（REST） |
| 取消任务 | kill 子进程 | `stop_crawler`（MCP） |
| 列断点 | 读 checkpoint 目录 | `list_checkpoints`（MCP） |
| 删除断点 | 删 checkpoint 文件 | `delete_checkpoint`（MCP） |
| 列平台能力 | 平台/crawler_type/必填参数 | `list_platforms`（MCP） |
| 查运行配置 | 回显 db/redis/sign/账号池配置 | `get_config`（MCP） |
| 健康检查 | 探 sign/db/redis/accounts | `health_check`（MCP） |
| 列任务（REST） | 分页返回历史 job | `ListJobs`（REST） |

## 数据模型

### Tables

#### jobs（自治 PG）

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| job_id | varchar(pk) | 主键 |
| workspace_id | text | 所属 Workspace |
| requested_by | text | 发起用户 |
| platform | varchar | 目标平台 |
| crawler_type | varchar | 抓取类型 |
| params | jsonb | 入参回显 |
| status | varchar | 状态 |
| pid | int | 子进程 pid |
| checkpoint_id | varchar | 断点 id |
| progress | jsonb | 进度快照 |
| result_summary | jsonb | CRAWL_SUMMARY |
| error | text | 失败原因 |
| stdout_tail | text | 日志尾部 |
| queued_at | timestamptz | 入队时间 |
| started_at | timestamptz | 开始时间 |
| finished_at | timestamptz | 结束时间 |

### UUID 生成规则
`job_id = job_{YYYYMMDDHHMMSS}_{6位随机}`，全局唯一，作为跨 tool 传递主键。

## 能力详细说明

### start_crawler
```
1. 入参: platform, crawler_type, keywords, urls, enable_comments, enable_sub_comments,
         max_notes_count, save_data_option, start_page, enable_checkpoint, checkpoint_id,
         hot_search_max_items, hot_search_per_item_count, on_busy, wait_for_completion
2. 校验
   - platform ∈ {xhs,dy,ks,bili,wb,tieba,zhihu}
   - crawler_type 必填项: search→keywords, detail/creator→urls
3. 生成 job_id = job_{ts}_{rand}
4. 写 PG jobs(status=queued, params=回显, queued_at=now)
5. wait_for_completion=false → 立即返回 job_id
6. 串行锁:
   - on_busy=reject 且锁占用 → status=rejected, 返回 conflict_with
   - on_busy=queue(默认) → 排队等锁
7. 抢到锁 → status=running, started_at=now, 启动子进程:
   subprocess: python mcp_runner.py
     CLI: --platform --type --keywords --urls --enable_checkpoint
          --checkpoint_id --enable_comments --enable_sub_comments
          --hot_search_max_items --hot_search_per_item_count
     env:  MCP_MAX_NOTES_COUNT / MCP_SAVE_DATA_OPTION / MCP_START_PAGE (仅传时)
     cwd:  爬虫项目根
8. 异步监控:
   - 扫 data/checkpoints/ 最新匹配文件 → 回填 checkpoint_id + progress
     crawled_count      = len(crawled_note_list)
     current_keyword    = checkpoint.current_search_keyword
     current_page       = checkpoint.current_search_page
   - 捕获 stdout → 正则匹配 ---CRAWL_SUMMARY: {...}---
9. 子进程退出:
   - 解析 CRAWL_SUMMARY.status → completed / failed
   - 更新 jobs(status, result_summary, error, finished_at, pid=null)
   - 释放串行锁 → 唤醒下一个 queued
10. wait_for_completion=true → 阻塞到第9步，返回完整状态（同 get_crawl_status）
```

### get_crawl_status / GetJob
```
1. 入参: job_id, include_logs_tail(默认 0)
2. 查 PG jobs by job_id
3. 若 status=running:
   - 按 checkpoint_id 定位 data/checkpoints/{checkpoint_id}.json
   - progress.crawled_count   = len(crawled_note_list)
   - progress.current_keyword/page = checkpoint.current_search_keyword/page
   - progress.total_target    = params.max_notes_count(若传，否则默认 200)
4. include_logs_tail>0 → 返回 stdout 尾部 N 行
5. 返回 status/progress/result_summary/error/logs_tail
```

### stop_crawler
```
1. 入参: job_id, signal(term=SIGTERM 默认 / kill=SIGKILL)
2. 查 job;仅 running 可取消
3. 发 signal 到 pid
4. 更新 status=cancelled, finished_at
5. 释放串行锁
6. 幂等: 已终态 job 直接返回当前状态，不重复 kill
```

### list_checkpoints
```
1. 入参: platform?, mode?, include_completed?(默认 true)
2. 扫 data/checkpoints/{platform}_{mode}_*.json(过滤参数可选)
3. 解析每文件: id/platform/mode/is_completed/created_at/last_modify_ts
   + progress(crawled_note_list 长度 / current_search_*)
4. 返回列表
```

### delete_checkpoint
```
1. 入参: checkpoint_id
2. 删 data/checkpoints/{checkpoint_id}.json
3. 返回 deleted=true/false
```

### list_platforms
```
1. 返回内置平台元数据(硬编码常量表):
   每 platform: {label, crawler_types[], data_tables{content,comment,creator},
                content_id_field, time_field, needs_login, params{crawler_type→required/optional}}
2. 各平台表/字段差异参考见 CONTEXT.md 的 Platform Data Mapping（crawl_data 按平台分接口，不做运行时映射）
```

### get_config
```
1. 入参: include_sensitive(默认 false)
2. 返回运行配置: db_type / mysql(脱敏) / redis / sign_srv / account_pool_save_type
   / job_queue(running/queued/max_serial)
3. include_sensitive=false → 密码等敏感字段脱敏
```

### health_check
```
1. 入参: probe[](默认全量 sign/db/redis/accounts)
2. sign:    HTTP 探 sign_srv:8989 可达性 + 延迟
3. db:      连爬虫 MySQL(只读)试通 + tables_count + pool_in_use
4. redis:   连 redis 试通 + 延迟
5. accounts:查账号池可用数(经 CookieBridge :8274)
6. overall: ok(全通) / degraded(部分不通) / down(关键不通)
```

### ListJobs / GetJob（REST）
```
ListJobs: platform?/status?/pagination → 分页返回 job 列表(轻量，不含 progress)
GetJob:   job_id → 完整状态(同 get_crawl_status 结构)
```

## 关联关系

### 依赖的 domain
- 无

### 被依赖的 domain
- 无（由 REST handler / MCP service 直接调用）

### 参与的 workflow
- [0001-crawl_job_execution](../../workflow/0001-crawl_job_execution/crawl_job_execution.md) — 在 workflow 中作为编排者参与

### 关联术语
- Crawler / Crawl Job / Checkpoint / Crawl Summary / Platform / Crawler Type（见 [CONTEXT.md](../../CONTEXT.md)）
