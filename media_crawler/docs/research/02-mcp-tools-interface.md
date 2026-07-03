# MCP Tools 接口协议

按"agent 调用一个爬虫任务"的自然流程分组，共 **10 个 tool**。

> 协议用 JSON Schema 描述 input；output 为 MCP text content（JSON 文本）。所有时间戳为 Unix 秒。

## 工具清单

| # | 分组 | Tool | 性质 | 阻塞 |
|---|---|---|---|---|
| 1 | 发现 | `list_platforms` | 只读 | 否 |
| 2 | 发现 | `get_config` | 只读 | 否 |
| 3 | 发现 | `health_check` | 只读 | 否 |
| 4 | 执行 | `start_crawler` | 写（起子进程） | 可选 |
| 5 | 执行 | `get_crawl_status` | 只读 | 否 |
| 6 | 执行 | `stop_crawler` | 写（kill 子进程） | 否 |
| 7 | 取数 | `query_crawl_data` | 只读 | 否 |
| 8 | 取数 | `execute_readonly_sql` | 只读 | 否 |
| 9 | 续爬 | `list_checkpoints` | 只读 | 否 |
| 10 | 续爬 | `delete_checkpoint` | 写（删文件/redis key） | 否 |

> 续爬不需要单独 `resume_crawler`——`start_crawler` 传 `checkpoint_id` 即续爬。

---

## 1. `list_platforms` — 能力发现（agent 第一个该调用的）

**Input**：`{}`

**Output**：
```jsonc
{
  "platforms": [
    {
      "platform": "xhs",
      "label": "小红书",
      "crawler_types": ["search","detail","creator","hot_search"],
      "data_tables": {"content":"xhs_note","comment":"xhs_note_comment","creator":"xhs_creator"},
      "content_id_field": "note_id",
      "time_field": "time",
      "needs_login": true,
      "params": {
        "search":    {"required":["keywords"],"optional":["sort_type"]},
        "detail":    {"required":["urls"],"optional":[]},
        "creator":   {"required":["urls"],"optional":[]},
        "homefeed":  {"required":[],"optional":[]},
        "hot_search":{"required":[],"optional":["hot_search_max_items","hot_search_per_item_count"]}
      }
    }
    // dy / ks / bili / wb / tieba / zhihu 同构
  ],
  "global_params": [
    {"name":"enable_comments",     "source":"cli"},
    {"name":"enable_sub_comments", "source":"cli"},
    {"name":"enable_checkpoint",   "source":"cli"},
    {"name":"checkpoint_id",       "source":"cli"},
    {"name":"max_notes_count",     "source":"shim"},
    {"name":"save_data_option",    "source":"shim"},
    {"name":"start_page",          "source":"shim"}
  ]
}
```

## 2. `get_config`

**Input**：`{ "include_sensitive": false }`（是否回显 DB 密码等，默认 false 脱敏）

**Output**：
```jsonc
{
  "db_type": "mysql",
  "mysql": {"host":"localhost","port":13306,"database":"media_crawler_pro","user":"root","pool_size":10},
  "redis": {"host":"127.0.0.1","port":6379,"db_num":0},
  "save_data_option": "db",
  "account_pool_save_type": "cookie_bridge",
  "checkpoint_storage_type": "file",
  "enable_checkpoint": true,
  "max_concurrency_num": 1,
  "crawler_time_sleep": 1,
  "sign_service": {"base_url":"http://127.0.0.1:...","reachable": null},
  "job_queue": {"running": null, "queued": 0, "max_serial": 1}
}
```

## 3. `health_check`

**Input**：`{ "probe": ["sign","db","redis","accounts"] }`（可指定子集，默认全量）

**Output**：
```jsonc
{
  "overall": "ok",                      // ok | degraded | down
  "sign_service": {"reachable": true,  "latency_ms": 12},
  "db":   {"reachable": true, "db_type":"mysql", "database":"media_crawler_pro",
            "tables_count": 18, "pool_in_use": 1, "pool_size": 10},
  "redis":{"reachable": true, "latency_ms": 3},
  "accounts": [
    {"platform":"xhs","available":2,"total":3,"status":"ok"},
    {"platform":"dy","available":0,"total":0,"status":"no_account"}   // 缺账号会被高亮
  ]
}
```

## 4. `start_crawler` — 核心执行（异步 Job）

**Input**：
```jsonc
{
  "platform": "xhs",                    // 必填，enum: xhs|dy|ks|bili|wb|tieba|zhihu
  "crawler_type": "search",             // 必填，enum: search|detail|creator|homefeed|hot_search
  "keywords": "deepseek,chatgpt",       // search 必填，逗号分隔
  "urls": ["https://www.xiaohongshu.com/explore/xxx?xsec_token=..."],  // detail/creator 必填

  "enable_comments": true,             // 可选，默认 true
  "enable_sub_comments": false,        // 可选，默认 false
  "max_notes_count": 50,               // 可选 (shim)，覆盖 CRAWLER_MAX_NOTES_COUNT
  "start_page": 1,                     // 可选 (shim)
  "save_data_option": "db",            // 可选 (shim)，覆盖 SAVE_DATA_OPTION：csv|db|json

  "enable_checkpoint": true,           // 可选，默认 true
  "checkpoint_id": "",                  // 可选，传值则续爬

  "hot_search_max_items": 50,          // hot_search 专用
  "hot_search_per_item_count": 4,

  "on_busy": "queue",                  // queue(默认排队) | reject(占用中直接返回)
  "wait_for_completion": false,         // false=立即返回 job_id；true=阻塞到结束
  "wait_timeout_seconds": 600,
  "progress_token": "any-string"       // 可选，MCP progress notification 的 token
}
```

**Output（`wait_for_completion=false`，立即返回）**：
```jsonc
{
  "job_id": "job_20260703143022_a1b2",
  "status": "queued",                   // queued | running（若立即抢到锁）
  "platform": "xhs",
  "crawler_type": "search",
  "checkpoint_id": null,                // 新任务暂未知；续爬任务回显传入值
  "queued_at": 1751536800,
  "started_at": null,
  "note": "已入队，用 get_crawl_status 轮询。"
}
```

**Output（`wait_for_completion=true`，阻塞返回）**：直接返回 `get_crawl_status` 的完整结构（见下）。

**`on_busy=reject` 且锁占用时**：
```jsonc
{ "job_id": null, "status":"rejected", "conflict_with":"<running job_id>", "retry_after_hint": 60 }
```

## 5. `get_crawl_status`

**Input**：`{ "job_id": "job_...", "include_logs_tail": 20 }`（`include_logs_tail` 默认 0，即不返回日志）

**Output**：
```jsonc
{
  "job_id": "job_20260703143022_a1b2",
  "status": "running",                  // queued|running|completed|failed|cancelled
  "platform": "xhs",
  "crawler_type": "search",
  "pid": 88421,
  "checkpoint_id": "xhs_search_20260703143022",
  "queued_at": 1751536800,
  "started_at": 1751536801,
  "finished_at": null,
  "progress": {
    "current_keyword": "deepseek",
    "current_page": 3,
    "crawled_count": 47,                // 从 checkpoint.crawled_note_list 推断
    "total_target": 50,                 // 来自 max_notes_count
    "percent": 94.0
  },
  "result_summary": null,              // 来自 main.py 的 CRAWL_SUMMARY，结束后填充
  "error": null,                       // failed 时给字符串
  "logs_tail": ["...最近 N 行日志..."]  // 仅 include_logs_tail>0 时
}
```

## 6. `stop_crawler`

**Input**：`{ "job_id": "job_...", "signal": "term", "cleanup_checkpoint": false }`
（`signal`: `term`=SIGTERM 默认 | `kill`=SIGKILL）

**Output**：
```jsonc
{
  "job_id": "...",
  "previous_status": "running",
  "status": "cancelled",
  "checkpoint_id": "xhs_search_...",
  "signal": "term"
}
```

## 7. `query_crawl_data` — 统一结果查询（最重要的取数工具）

**Input**：
```jsonc
{
  "platform": "xhs",
  "data_type": "content",               // content|comment|creator|hot_search

  "filters": {                          // 全部可选
    "keywords": ["deepseek"],           // 命中 source_keyword 或正文（LIKE）
    "ids": ["69ba256f..."],             // 内容主键，内部映射到 note_id/aweme_id/video_id/content_id
    "creator_ids": ["5f58bd99..."],     // user_id / sec_uid 过滤
    "time_range": {"start": 1751500000, "end": 1751600000},  // 映射到平台时间字段
    "source_keyword": "deepseek"        // 精确匹配 source_keyword 列
  },

  "sort": {"field": "time", "order": "desc"},  // field 用语义名：time|liked_count|comment_count；内部映射

  "pagination": {"limit": 50, "offset": 0},     // limit 默认 50，上限 200
  "fields": ["note_id","title","desc","liked_count","time","source_keyword"]  // 可选，投影以减小体积
}
```

**Output**：
```jsonc
{
  "platform": "xhs",
  "data_type": "content",
  "mapped_table": "xhs_note",
  "total": 132,
  "limit": 50,
  "offset": 0,
  "truncated": false,
  "rows": [
    {"note_id":"69ba256f...","title":"...","desc":"...","liked_count":"1.2k","time":1751530000,"source_keyword":"deepseek"}
  ]
}
```

### (platform, data_type) → 表名/字段 映射表（上层内置，agent 无需感知）

| platform | content | comment | creator | content_id 字段 | time 字段 |
|---|---|---|---|---|---|
| xhs | xhs_note | xhs_note_comment | xhs_creator | note_id | time |
| dy | douyin_aweme | douyin_aweme_comment | dy_creator | aweme_id | create_time |
| ks | kuaishou_video | kuaishou_video_comment | kuaishou_creator | video_id | create_time |
| bili | bilibili_video | bilibili_video_comment | bilibili_up_info | video_id | create_time |
| wb | weibo_note | weibo_note_comment | weibo_creator | note_id | create_time |
| tieba | tieba_note | tieba_comment | tieba_creator | note_id | publish_time |
| zhihu | zhihu_content | zhihu_comment | zhihu_creator | content_id | created_time |

`hot_search`（任意平台）→ `platform_hot_search_item`（按 platform 列过滤），内容关联表为 `platform_hot_search_content_rel`。

## 8. `execute_readonly_sql` — 逃生通道

**Input**：
```jsonc
{
  "sql": "SELECT source_keyword, COUNT(*) c FROM xhs_note GROUP BY source_keyword ORDER BY c DESC LIMIT 10",
  "limit": 100,                         // 默认 100，硬上限 1000
  "db_name": null                       // 可选，跨库查询（仅 mysql，需 mc_reader 有对应库 SELECT 权限）
}
```

**护栏**（在 server 端强制）：

1. **账号层（推荐，最稳）**：建只读 MySQL 账号 `mc_reader`，MCP 给此工具开**独立连接池**：
   ```sql
   CREATE USER 'mc_reader'@'%' IDENTIFIED BY '...';
   GRANT SELECT ON media_crawler_pro.* TO 'mc_reader'@'%';
   ```
   即使白名单漏判，也写不进库。
2. **SQL 层（第二道防线）**：正则要求以 `SELECT` / `WITH` 开头、单条语句、禁止 `INSERT/UPDATE/DELETE/DROP/ALTER/TRUNCATE/CREATE/GRANT` 等关键字、强制套 `LIMIT`。

**Output**：`{ "columns": [...], "rows": [...], "rowcount": 12, "truncated": false }`

## 9. `list_checkpoints`

**Input**：`{ "platform": "xhs", "mode": "search", "include_completed": true }`（均可选，不传则全列）

**Output**：
```jsonc
{
  "storage_type": "file",               // file | redis
  "checkpoints": [
    {
      "checkpoint_id": "xhs_search_20260703143022",
      "platform": "xhs", "mode": "search",
      "is_completed": false,
      "created_at": 1751536800,
      "last_modify_ts": 1751537099,
      "progress": {
        "crawled_count": 47, "total_target": 50, "percent": 94.0,
        "current_keyword": "deepseek", "current_page": 3
      }
    }
  ]
}
```

> `storage_type=file` 读 `data/checkpoints/*.json`；`redis` 时按 `checkpoint:{platform}:{mode}:*` 扫 key。

## 10. `delete_checkpoint`

**Input**：`{ "checkpoint_id": "xhs_search_20260703143022" }`

**Output**：`{ "checkpoint_id": "...", "deleted": true }`
